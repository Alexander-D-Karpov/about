package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type lfmTopItem struct {
	Name  string
	Plays int64
	Image string
}

type lfmTag struct {
	Name  string
	Count int64
}

type lfmStats struct {
	Scrobbles      int64
	Tracks         int64
	Artists        int64
	Albums         int64
	ScrobblesDelta float64
	TracksDelta    float64
	ArtistsDelta   float64
	AlbumsDelta    float64
	Weekly         [7]int
	WeeklyPeak     string
	TopArtists     []lfmTopItem
	TopAlbums      []lfmTopItem
	Tags           []lfmTag
	UpdatedAt      time.Time
}

var weekdayNames = [7]string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

func weekdayIndex(t time.Time) int {
	return (int(t.Weekday()) + 6) % 7
}

func (p *LastFMPlugin) isLovedTrack(artist, name string) bool {
	p.lovedMu.RLock()
	defer p.lovedMu.RUnlock()
	if _, ok := p.lovedSet[p.lovedKey(artist, name)]; ok {
		return true
	}
	_, ok := p.lovedSet[p.lovedKeyNorm(artist, name)]
	return ok
}

const keySep = "\x1f"

func lfmTrackKey(artist, name string) string {
	return strings.ToLower(strings.TrimSpace(artist)) + keySep + strings.ToLower(strings.TrimSpace(name))
}

func (p *LastFMPlugin) lovedKey(artist, name string) string {
	return lfmTrackKey(artist, name)
}

func (p *LastFMPlugin) lovedKeyNorm(artist, name string) string {
	return strings.ToLower(strings.TrimSpace(artist)) + keySep + normalizeTrackName(name)
}

func (p *LastFMPlugin) spotifySavedCached(artist, track string) bool {
	if p.spotify == nil || !p.spotify.UserEnabled() {
		return false
	}
	key := spotifySavedKey(artist, track)
	if key == "" {
		return false
	}
	p.spSavedMu.RLock()
	v, ok := p.spSavedCache[key]
	p.spSavedMu.RUnlock()
	if ok {
		return v
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		saved := p.spotify.IsTrackSaved(ctx, artist, track)
		p.spSavedMu.Lock()
		p.spSavedCache[key] = saved
		p.spSavedMu.Unlock()
	}()
	return false
}

func (p *LastFMPlugin) resolveArtistImage(ctx context.Context, name string) string {
	if p.spotify != nil && p.spotify.Enabled() {
		if u := p.spotify.ArtistImage(ctx, name); u != "" {
			return p.localizeImage(u)
		}
	}
	if u := p.akarpovAuthorImage(ctx, name); u != "" {
		return p.localizeImage(u)
	}
	want := akarpovNormalize(name)
	for _, s := range p.akarpovSearch(ctx, name) {
		if !akarpovAuthorMatch(s, name) {
			continue
		}
		for _, a := range s.Authors {
			if akarpovNormalize(a.Name) == want && a.ImageCropped != "" {
				return p.localizeImage(absAkarpov(a.ImageCropped))
			}
		}
		if s.ImageCropped != "" {
			return p.localizeImage(absAkarpov(s.ImageCropped))
		}
		if s.Album.ImageCropped != "" {
			return p.localizeImage(absAkarpov(s.Album.ImageCropped))
		}
	}
	return ""
}

func (p *LastFMPlugin) resolveAlbumImage(ctx context.Context, artist, album, lfmImg string) string {
	if lfmImg != "" && !p.isPlaceholderImage(lfmImg) {
		return p.localizeImage(lfmImg)
	}
	if p.spotify != nil && p.spotify.Enabled() {
		if u := p.spotify.AlbumImage(ctx, artist, album); u != "" {
			return p.localizeImage(u)
		}
	}
	if u := p.akarpovAlbumImage(ctx, artist, album); u != "" {
		return p.localizeImage(u)
	}
	wantAlbum := akarpovNormalize(album)
	for _, s := range p.akarpovSearch(ctx, artist+" "+album) {
		if akarpovNormalize(s.Album.Name) != wantAlbum || !akarpovAuthorMatch(s, artist) {
			continue
		}
		if s.Album.ImageCropped != "" {
			return p.localizeImage(absAkarpov(s.Album.ImageCropped))
		}
		if s.ImageCropped != "" {
			return p.localizeImage(absAkarpov(s.ImageCropped))
		}
	}
	return ""
}

type TopItem struct {
	Name  string
	Plays int64
	Image string
}

func normalizePeriod(p string) string {
	switch p {
	case "7day", "1month", "3month", "6month", "12month", "overall":
		return p
	default:
		return "overall"
	}
}

func (p *LastFMPlugin) TopItems(ctx context.Context, kind, period string) ([]TopItem, error) {
	cfg := p.storage.GetPluginConfig(p.Name())
	username, _ := cfg.Settings["username"].(string)
	if strings.TrimSpace(username) == "" {
		return nil, fmt.Errorf("username not configured")
	}
	period = normalizePeriod(period)
	if kind == "albums" {
		return p.fetchTopAlbums(ctx, username, period)
	}
	return p.fetchTopArtists(ctx, username, period)
}

func (p *LastFMPlugin) fetchTopArtists(ctx context.Context, username, period string) ([]TopItem, error) {
	var resp struct {
		TopArtists struct {
			Artist []struct {
				Name      string `json:"name"`
				PlayCount string `json:"playcount"`
			} `json:"artist"`
		} `json:"topartists"`
	}
	extra := "&period=" + period + "&limit=8"
	if err := p.getJSONWithRetry(ctx, p.statsURL("user.gettopartists", username, extra), &resp); err != nil {
		return nil, err
	}
	out := make([]TopItem, 0, 8)
	for i, a := range resp.TopArtists.Artist {
		if i >= 8 {
			break
		}
		plays, _ := strconv.ParseInt(a.PlayCount, 10, 64)
		out = append(out, TopItem{Name: a.Name, Plays: plays, Image: p.resolveArtistImage(ctx, a.Name)})
	}
	return out, nil
}

func (p *LastFMPlugin) fetchTopAlbums(ctx context.Context, username, period string) ([]TopItem, error) {
	var resp struct {
		TopAlbums struct {
			Album []struct {
				Name      string `json:"name"`
				PlayCount string `json:"playcount"`
				Artist    struct {
					Name string `json:"name"`
				} `json:"artist"`
				Image []struct {
					Text string `json:"#text"`
					Size string `json:"size"`
				} `json:"image"`
			} `json:"album"`
		} `json:"topalbums"`
	}
	extra := "&period=" + period + "&limit=8"
	if err := p.getJSONWithRetry(ctx, p.statsURL("user.gettopalbums", username, extra), &resp); err != nil {
		return nil, err
	}
	out := make([]TopItem, 0, 8)
	for i, a := range resp.TopAlbums.Album {
		if i >= 8 {
			break
		}
		plays, _ := strconv.ParseInt(a.PlayCount, 10, 64)
		lfmImg := ""
		for j := len(a.Image) - 1; j >= 0; j-- {
			if a.Image[j].Text != "" {
				lfmImg = ensureHTTPS(a.Image[j].Text)
				break
			}
		}
		out = append(out, TopItem{Name: a.Name, Plays: plays, Image: p.resolveAlbumImage(ctx, a.Artist.Name, a.Name, lfmImg)})
	}
	return out, nil
}

func (p *LastFMPlugin) spotifyNowCached() *SpotifyNowPlaying {
	if p.spotify == nil || !p.spotify.UserEnabled() {
		return nil
	}
	p.spNowMu.RLock()
	cur := p.spNow
	at := p.spNowAt
	p.spNowMu.RUnlock()
	if time.Since(at) > 3*time.Second {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			np, _ := p.spotify.CurrentlyPlaying(ctx)
			p.spNowMu.Lock()
			p.spNow = np
			p.spNowAt = time.Now()
			p.spNowMu.Unlock()
		}()
	}
	return cur
}

func (p *LastFMPlugin) akarpovSearch(ctx context.Context, query string) []AkarpovrMusicTrack {
	searchURL := "https://new.akarpov.ru/api/v1/music/search/?query=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil
	}

	var a AkarpovrMusicSearchResponse
	if json.Unmarshal(body, &a) == nil && len(a.Songs) > 0 {
		return a.Songs
	}
	var b AkarpovrMusicResponse
	if json.Unmarshal(body, &b) == nil && len(b.Results) > 0 {
		return b.Results
	}
	var c []AkarpovrMusicTrack
	if json.Unmarshal(body, &c) == nil && len(c) > 0 {
		return c
	}
	return nil
}

func absAkarpov(u string) string {
	if u == "" || strings.HasPrefix(u, "http") {
		return u
	}
	return "https://new.akarpov.ru" + u
}

func akarpovAuthorMatch(s AkarpovrMusicTrack, artist string) bool {
	want := strings.ToLower(strings.TrimSpace(artist))
	if want == "" {
		return false
	}
	for _, a := range s.Authors {
		if strings.ToLower(strings.TrimSpace(a.Name)) == want {
			return true
		}
	}
	return false
}

func (p *LastFMPlugin) statsURL(method, username string, extra string) string {
	return fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=%s&user=%s&api_key=%s&format=json%s",
		method, url.QueryEscape(username), url.QueryEscape(p.apiKey), extra)
}

func (p *LastFMPlugin) loadStats() {
	cfg := p.storage.GetPluginConfig(p.Name())
	raw, ok := cfg.Settings["stats_cache"].(string)
	if !ok || raw == "" {
		return
	}
	var s lfmStats
	if json.Unmarshal([]byte(raw), &s) != nil {
		return
	}
	p.statsMutex.Lock()
	p.stats = &s
	p.statsMutex.Unlock()

	p.lovedMu.Lock()
	p.lovedSet = make(map[string]struct{})
	if keys, ok := cfg.Settings["loved_keys"].([]interface{}); ok {
		for _, k := range keys {
			if ks, ok := k.(string); ok {
				p.lovedSet[ks] = struct{}{}
			}
		}
	}
	p.lovedCount = int(getFloat(cfg.Settings, "loved_count"))
	p.lovedMu.Unlock()
}

func normalizeTrackName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, sep := range []string{" - ", " (", " ["} {
		if i := strings.Index(n, sep); i > 0 {
			tail := n[i:]
			if strings.Contains(tail, "remaster") || strings.Contains(tail, "remix") ||
				strings.Contains(tail, "version") || strings.Contains(tail, "edit") ||
				strings.Contains(tail, "deluxe") || strings.Contains(tail, "mono") ||
				strings.Contains(tail, "stereo") || strings.Contains(tail, "live") {
				n = strings.TrimSpace(n[:i])
			}
		}
	}
	return n
}

func (p *LastFMPlugin) persistStats(s *lfmStats) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	cfg := p.storage.GetPluginConfig(p.Name())
	if cfg.Settings == nil {
		cfg.Settings = map[string]interface{}{}
	}
	cfg.Settings["stats_cache"] = string(data)
	cfg.Settings["stats_prev"] = map[string]interface{}{
		"scrobbles": s.Scrobbles, "tracks": s.Tracks, "artists": s.Artists, "albums": s.Albums,
	}

	p.lovedMu.RLock()
	lovedKeys := make([]interface{}, 0, len(p.lovedSet))
	for k := range p.lovedSet {
		lovedKeys = append(lovedKeys, k)
	}
	lovedCount := p.lovedCount
	p.lovedMu.RUnlock()
	cfg.Settings["loved_keys"] = lovedKeys
	cfg.Settings["loved_count"] = lovedCount

	_ = p.storage.SetPluginConfig(p.Name(), cfg)
}

func (p *LastFMPlugin) prevTotals() (sc, tr, ar, al int64) {
	cfg := p.storage.GetPluginConfig(p.Name())
	m, ok := cfg.Settings["stats_prev"].(map[string]interface{})
	if !ok {
		return
	}
	return int64(getFloat(m, "scrobbles")), int64(getFloat(m, "tracks")), int64(getFloat(m, "artists")), int64(getFloat(m, "albums"))
}

func pctDelta(old, now int64) float64 {
	if old <= 0 {
		return 0
	}
	return (float64(now) - float64(old)) / float64(old) * 100
}

func (p *LastFMPlugin) scheduleStatsRefresh(username string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
		defer cancel()
		if err := p.UpdateStats(ctx, username); err != nil {
			log.Printf("[LastFM] stats refresh error: %v", err)
		}
	}()
}

func (p *LastFMPlugin) UpdateStats(ctx context.Context, username string) error {
	p.statsMutex.Lock()
	if p.statsRunning {
		p.statsMutex.Unlock()
		return nil
	}
	p.statsRunning = true
	p.statsMutex.Unlock()
	defer func() {
		p.statsMutex.Lock()
		p.statsRunning = false
		p.statsMutex.Unlock()
	}()

	s := &lfmStats{UpdatedAt: time.Now()}

	var userResp struct {
		User struct {
			PlayCount string `json:"playcount"`
		} `json:"user"`
	}
	if err := p.getJSONWithRetry(ctx, p.statsURL("user.getinfo", username, ""), &userResp); err == nil {
		s.Scrobbles, _ = strconv.ParseInt(userResp.User.PlayCount, 10, 64)
	}

	var artistsResp struct {
		TopArtists struct {
			Artist []struct {
				Name      string `json:"name"`
				PlayCount string `json:"playcount"`
			} `json:"artist"`
			Attr struct {
				Total string `json:"total"`
			} `json:"@attr"`
		} `json:"topartists"`
	}
	if err := p.getJSONWithRetry(ctx, p.statsURL("user.gettopartists", username, "&period=overall&limit=50"), &artistsResp); err == nil {
		s.Artists, _ = strconv.ParseInt(artistsResp.TopArtists.Attr.Total, 10, 64)
		for i, a := range artistsResp.TopArtists.Artist {
			if i >= 8 {
				break
			}
			plays, _ := strconv.ParseInt(a.PlayCount, 10, 64)
			img := p.resolveArtistImage(ctx, a.Name)
			s.TopArtists = append(s.TopArtists, lfmTopItem{Name: a.Name, Plays: plays, Image: img})
		}
	}

	var albumsResp struct {
		TopAlbums struct {
			Album []struct {
				Name      string `json:"name"`
				PlayCount string `json:"playcount"`
				Artist    struct {
					Name string `json:"name"`
				} `json:"artist"`
				Image []struct {
					Text string `json:"#text"`
					Size string `json:"size"`
				} `json:"image"`
			} `json:"album"`
			Attr struct {
				Total string `json:"total"`
			} `json:"@attr"`
		} `json:"topalbums"`
	}
	if err := p.getJSONWithRetry(ctx, p.statsURL("user.gettopalbums", username, "&period=overall&limit=8"), &albumsResp); err == nil {
		s.Albums, _ = strconv.ParseInt(albumsResp.TopAlbums.Attr.Total, 10, 64)
		for _, a := range albumsResp.TopAlbums.Album {
			plays, _ := strconv.ParseInt(a.PlayCount, 10, 64)
			lfmImg := ""
			for i := len(a.Image) - 1; i >= 0; i-- {
				if a.Image[i].Text != "" {
					lfmImg = ensureHTTPS(a.Image[i].Text)
					break
				}
			}
			img := p.resolveAlbumImage(ctx, a.Artist.Name, a.Name, lfmImg)
			s.TopAlbums = append(s.TopAlbums, lfmTopItem{Name: a.Name, Plays: plays, Image: img})
		}
	}

	var tracksResp struct {
		TopTracks struct {
			Attr struct {
				Total string `json:"total"`
			} `json:"@attr"`
		} `json:"toptracks"`
	}
	if err := p.getJSONWithRetry(ctx, p.statsURL("user.gettoptracks", username, "&period=overall&limit=1"), &tracksResp); err == nil {
		s.Tracks, _ = strconv.ParseInt(tracksResp.TopTracks.Attr.Total, 10, 64)
	}

	s.Tags = p.collectGenres(ctx, artistsResp.TopArtists.Artist)
	s.Weekly, s.WeeklyPeak = p.collectWeekly(ctx, username)

	osc, otr, oar, oal := p.prevTotals()
	s.ScrobblesDelta = pctDelta(osc, s.Scrobbles)
	s.TracksDelta = pctDelta(otr, s.Tracks)
	s.ArtistsDelta = pctDelta(oar, s.Artists)
	s.AlbumsDelta = pctDelta(oal, s.Albums)

	p.refreshLoved(ctx, username)

	if p.spotify != nil && p.spotify.UserEnabled() {
		if cnt := p.spotify.SavedTracksCount(ctx); cnt > 0 {
			p.spSavedMu.Lock()
			p.spLikedCount = cnt
			p.spSavedMu.Unlock()
			p.saveSpotifySavedCache()
		}
	}

	p.statsMutex.Lock()
	p.stats = s
	p.statsMutex.Unlock()
	p.persistStats(s)
	p.requestSectionRender()
	return nil
}

func (p *LastFMPlugin) collectGenres(ctx context.Context, artists []struct {
	Name      string `json:"name"`
	PlayCount string `json:"playcount"`
}) []lfmTag {
	weights := map[string]int64{}
	limit := len(artists)
	if limit > 20 {
		limit = 20
	}
	for i := 0; i < limit; i++ {
		if ctx.Err() != nil {
			break
		}
		plays, _ := strconv.ParseInt(artists[i].PlayCount, 10, 64)
		if plays <= 0 {
			plays = 1
		}
		endpoint := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=artist.gettoptags&artist=%s&api_key=%s&format=json",
			url.QueryEscape(artists[i].Name), url.QueryEscape(p.apiKey))
		var tagResp struct {
			TopTags struct {
				Tag []struct {
					Name string `json:"name"`
				} `json:"tag"`
			} `json:"toptags"`
		}
		if err := p.getJSONWithRetry(ctx, endpoint, &tagResp); err == nil {
			for j, t := range tagResp.TopTags.Tag {
				if j >= 3 {
					break
				}
				weights[strings.ToLower(t.Name)] += plays
			}
		}
		time.Sleep(40 * time.Millisecond)
	}

	tags := make([]lfmTag, 0, len(weights))
	for name, c := range weights {
		tags = append(tags, lfmTag{Name: name, Count: c})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Count > tags[j].Count })
	if len(tags) > 14 {
		tags = tags[:14]
	}
	return tags
}

func (p *LastFMPlugin) collectWeekly(ctx context.Context, username string) ([7]int, string) {
	var week [7]int
	to := time.Now().Unix()
	from := time.Now().AddDate(0, 0, -7).Unix()

	for page := 1; page <= 15; page++ {
		if ctx.Err() != nil {
			break
		}
		extra := fmt.Sprintf("&from=%d&to=%d&limit=200&page=%d", from, to, page)
		var resp LastFMResponse
		if err := p.getJSONWithRetry(ctx, p.statsURL("user.getrecenttracks", username, extra), &resp); err != nil {
			break
		}
		if len(resp.RecentTracks.Track) == 0 {
			break
		}
		for _, t := range resp.RecentTracks.Track {
			if t.Date.Uts == "" {
				continue
			}
			sec, err := strconv.ParseInt(t.Date.Uts, 10, 64)
			if err != nil {
				continue
			}
			week[weekdayIndex(time.Unix(sec, 0))]++
		}
		total, _ := strconv.Atoi(resp.RecentTracks.Attr.TotalPages)
		if page >= total {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}

	peakIdx, peakVal := 0, -1
	for i, v := range week {
		if v > peakVal {
			peakVal, peakIdx = v, i
		}
	}
	return week, weekdayNames[peakIdx]
}

func (p *LastFMPlugin) refreshLoved(ctx context.Context, username string) {
	loved := make(map[string]struct{})
	count := 0
	for page := 1; page <= 6; page++ {
		if ctx.Err() != nil {
			break
		}
		endpoint := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getlovedtracks&user=%s&api_key=%s&format=json&limit=200&page=%d",
			url.QueryEscape(username), url.QueryEscape(p.apiKey), page)
		var resp struct {
			LovedTracks struct {
				Track []struct {
					Name   string `json:"name"`
					Artist struct {
						Name string `json:"name"`
					} `json:"artist"`
				} `json:"track"`
				Attr struct {
					Total      string `json:"total"`
					TotalPages string `json:"totalPages"`
				} `json:"@attr"`
			} `json:"lovedtracks"`
		}
		if err := p.getJSONWithRetry(ctx, endpoint, &resp); err != nil {
			break
		}
		if page == 1 {
			count, _ = strconv.Atoi(resp.LovedTracks.Attr.Total)
		}
		for _, t := range resp.LovedTracks.Track {
			loved[p.lovedKey(t.Artist.Name, t.Name)] = struct{}{}
			loved[p.lovedKeyNorm(t.Artist.Name, t.Name)] = struct{}{}
		}
		total, _ := strconv.Atoi(resp.LovedTracks.Attr.TotalPages)
		if page >= total || len(resp.LovedTracks.Track) == 0 {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}

	p.lovedMu.Lock()
	p.lovedSet = loved
	p.lovedCount = count
	p.lovedMu.Unlock()
}

func formatNumberWithCommas(n int64) string {
	str := strconv.FormatInt(n, 10)
	neg := false
	if strings.HasPrefix(str, "-") {
		neg = true
		str = str[1:]
	}
	if len(str) <= 3 {
		if neg {
			return "-" + str
		}
		return str
	}
	var out []byte
	for i, c := range []byte(str) {
		if i > 0 && (len(str)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
