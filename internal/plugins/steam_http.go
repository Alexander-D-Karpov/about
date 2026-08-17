package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const steamDefaultPageSize = 40

// steamGameDTO is the wire shape consumed by the /steam page.
type steamGameDTO struct {
	AppID            int      `json:"appid"`
	Name             string   `json:"name"`
	Img              string   `json:"img"`
	ImgFallbacks     []string `json:"imgFallbacks"`
	PlaytimeForever  int      `json:"playtimeForever"`
	Playtime2w       int      `json:"playtime2w"`
	PlaytimeYear     int      `json:"playtimeYear"`
	LastPlayed       int64    `json:"lastPlayed"`
	AddedAt          int64    `json:"addedAt"`
	AddedIsFirstSeen bool     `json:"addedIsFirstSeen"`
	HasStats         bool     `json:"hasStats"`
	Source           string   `json:"source,omitempty"`
	StoreURL         string   `json:"storeUrl"`

	// Achievements are served inline from cache so rows render complete on first paint. When
	// AchLoaded is false the page fetches them for that row on demand.
	AchLoaded   bool                     `json:"achLoaded"`
	AchAchieved int                      `json:"achAchieved"`
	AchTotal    int                      `json:"achTotal"`
	AchPercent  float64                  `json:"achPercent"`
	Rarest      []SteamRarestAchievement `json:"rarest,omitempty"`
	// IconURL is the small library icon, used only as a last-resort banner.
	IconURL string `json:"iconUrl,omitempty"`
}

func (p *SteamPlugin) toDTO(g SteamGame, firstSeen map[int]int64, yearly map[int]int, ach map[int]steamAchEntry, headers map[int]string) steamGameDTO {
	dto := steamGameDTO{
		AppID:           g.AppID,
		Name:            g.Name,
		Img:             steamHeaderImage(g.AppID),
		PlaytimeForever: g.PlaytimeAll,
		Playtime2w:      g.Playtime2w,
		PlaytimeYear:    yearly[g.AppID],
		LastPlayed:      g.LastPlayed,
		HasStats:        g.HasStats,
		Source:          g.Source,
		StoreURL:        "https://store.steampowered.com/app/" + strconv.Itoa(g.AppID) + "/",
	}

	// A cached header from the store wins: newer titles keep their art under a content-hashed
	// path that the appid-derived URLs cannot reach.
	fallbacks := steamBannerFallbacks(g.AppID)
	if header := headers[g.AppID]; header != "" && header != dto.Img {
		fallbacks = append([]string{dto.Img}, fallbacks...)
		dto.Img = header
	}
	dto.ImgFallbacks = fallbacks
	dto.IconURL = steamIconImage(g.AppID, g.ImgIconURL)

	// Real acquisition date when Steam gives us one (family library), else our first-seen stamp.
	if g.AcquiredAt > 0 {
		dto.AddedAt = g.AcquiredAt
	} else if seen, ok := firstSeen[g.AppID]; ok {
		dto.AddedAt = seen
		dto.AddedIsFirstSeen = true
	}

	if entry, ok := ach[g.AppID]; ok {
		dto.AchLoaded = true
		if entry.Available {
			dto.AchAchieved = entry.Achieved
			dto.AchTotal = entry.Total
			dto.AchPercent = entry.Percent
			dto.Rarest = entry.Rarest
		}
	}

	return dto
}

// HandleGamesAPI serves the paginated, searchable, sortable library. It is served entirely from
// memory — no Steam call happens on the request path.
func (p *SteamPlugin) HandleGamesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = steamDefaultPageSize
	}
	search := strings.ToLower(strings.TrimSpace(q.Get("q")))
	sortBy := q.Get("sort")
	filter := q.Get("filter")

	games := p.snapshotGames()
	firstSeen := p.store.FirstSeenAll()
	yearly, snapshotAt := p.playtimeThisYear()
	ach := p.store.AllAchievements()
	headers := p.store.AppHeaders()

	filtered := make([]SteamGame, 0, len(games))
	for _, g := range games {
		if search != "" && !strings.Contains(strings.ToLower(g.Name), search) {
			continue
		}
		switch filter {
		case "played":
			if g.PlaytimeAll <= 0 {
				continue
			}
		case "unplayed":
			if g.PlaytimeAll > 0 {
				continue
			}
		case "stats":
			if !g.HasStats {
				continue
			}
		case "family":
			if g.Source != "family" {
				continue
			}
		}
		filtered = append(filtered, g)
	}

	switch sortBy {
	case "recent":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].LastPlayed > filtered[j].LastPlayed })
	case "added":
		// Without a family token every game shares the same first-seen stamp, and a stable sort
		// would just leave them in playtime order. Fall back to appid, which tracks roughly when
		// a title appeared on Steam, so the ordering is at least about recency rather than hours.
		sort.SliceStable(filtered, func(i, j int) bool {
			ai, aj := steamAddedAt(filtered[i], firstSeen), steamAddedAt(filtered[j], firstSeen)
			if ai != aj {
				return ai > aj
			}
			return filtered[i].AppID > filtered[j].AppID
		})
	case "ach":
		sort.SliceStable(filtered, func(i, j int) bool {
			return steamAchPercent(filtered[i], ach) > steamAchPercent(filtered[j], ach)
		})
	case "year":
		sort.SliceStable(filtered, func(i, j int) bool { return yearly[filtered[i].AppID] > yearly[filtered[j].AppID] })
	default: // playtime
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].PlaytimeAll > filtered[j].PlaytimeAll })
	}

	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := filtered[offset:end]

	out := make([]steamGameDTO, 0, len(page))
	for _, g := range page {
		out = append(out, p.toDTO(g, firstSeen, yearly, ach, headers))
	}

	resp := map[string]interface{}{
		"total":   total,
		"offset":  offset,
		"limit":   limit,
		"hasMore": end < total,
		"games":   out,
	}
	if offset == 0 {
		resp["stats"] = p.libraryStats(games, yearly, snapshotAt, firstSeen, ach)
	}

	json.NewEncoder(w).Encode(resp)
}

func steamAddedAt(g SteamGame, firstSeen map[int]int64) int64 {
	if g.AcquiredAt > 0 {
		return g.AcquiredAt
	}
	return firstSeen[g.AppID]
}

func steamAchPercent(g SteamGame, ach map[int]steamAchEntry) float64 {
	if e, ok := ach[g.AppID]; ok && e.Available {
		return e.Percent
	}
	return -1
}

// libraryStats builds the header summary for the page.
func (p *SteamPlugin) libraryStats(games []SteamGame, yearly map[int]int, snapshotAt time.Time, firstSeen map[int]int64, ach map[int]steamAchEntry) map[string]interface{} {
	var totalMinutes, played int
	for _, g := range games {
		totalMinutes += g.PlaytimeAll
		if g.PlaytimeAll > 0 {
			played++
		}
	}

	unlocked, totalAch, perfect, enriched := p.achievementSummary()

	// Top played this year.
	type yearEntry struct {
		AppID        int    `json:"appid"`
		Name         string `json:"name"`
		Img          string `json:"img"`
		Minutes      int    `json:"minutes"`
		TotalMinutes int    `json:"totalMinutes"`
	}
	var topYear []yearEntry
	for _, g := range games {
		if m := yearly[g.AppID]; m > 0 {
			topYear = append(topYear, yearEntry{
				AppID:        g.AppID,
				Name:         g.Name,
				Img:          steamHeaderImage(g.AppID),
				Minutes:      m,
				TotalMinutes: g.PlaytimeAll,
			})
		}
	}
	sort.Slice(topYear, func(i, j int) bool { return topYear[i].Minutes > topYear[j].Minutes })
	if len(topYear) > 5 {
		topYear = topYear[:5]
	}

	summary := p.snapshotSummary()
	current := map[string]interface{}{"playing": false}
	if summary != nil {
		current["personaName"] = summary.PersonaName
		current["personaState"] = summary.PersonaState
		current["avatar"] = summary.AvatarFull
		current["profileUrl"] = summary.ProfileURL
		if summary.GameExtraInfo != "" {
			current["playing"] = true
			current["gameName"] = summary.GameExtraInfo
			current["gameId"] = summary.GameID
			if summary.GameID != "" {
				current["gameImage"] = "https://cdn.cloudflare.steamstatic.com/steam/apps/" + summary.GameID + "/header.jpg"
			}
		}
	}

	snapshotUnix := int64(0)
	if !snapshotAt.IsZero() {
		snapshotUnix = snapshotAt.Unix()
	}

	return map[string]interface{}{
		"totalGames":           len(games),
		"playedGames":          played,
		"unplayedGames":        len(games) - played,
		"totalPlaytimeHours":   float64(totalMinutes) / 60.0,
		"achievementsUnlocked": unlocked,
		"achievementsTotal":    totalAch,
		"perfectGames":         perfect,
		"gamesEnriched":        enriched,
		"pendingEnrichment":    p.store.PendingCount(),
		"topThisYear":          topYear,
		"yearSnapshotAt":       snapshotUnix,
		"year":                 time.Now().Year(),
		"current":              current,
	}
}

// HandleAchievementsAPI lazily resolves one game's achievements when a card is expanded.
func (p *SteamPlugin) HandleAchievementsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	appID, err := strconv.Atoi(r.URL.Query().Get("appid"))
	if err != nil || appID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"available": false, "error": "invalid appid"})
		return
	}

	if _, ok := p.gameByID(appID); !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"available": false, "error": "unknown game"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	entry, ok := p.achievementFor(ctx, appID)
	if !ok || !entry.Available {
		json.NewEncoder(w).Encode(map[string]interface{}{"available": false})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"available": true,
		"achieved":  entry.Achieved,
		"total":     entry.Total,
		"percent":   entry.Percent,
		"rarest":    entry.Rarest,
	})
}

// HandleRarestAPI serves the library-wide rarest unlocked achievements.
func (p *SteamPlugin) HandleRarestAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 12
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"achievements": p.rarestAcrossLibrary(limit),
	})
}
