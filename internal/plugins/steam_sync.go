package plugins

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"sync/atomic"
	"time"
)

const (
	// How often the whole library is re-pulled from Steam.
	steamFullSyncInterval = 24 * time.Hour
	// Minimum gap between the cheap player-summary/recent-games refreshes.
	steamCheapSyncInterval = 90 * time.Second
	// Games enriched with achievements per background cycle, and the pause between each, so we
	// never hammer the API.
	steamEnrichPerCycle = 40
	steamEnrichDelay    = 750 * time.Millisecond
)

// UpdateData returns immediately. PreloadData runs before the HTTP server starts and passes a
// context with no timeout, so any real work here has to be detached or it delays boot.
func (p *SteamPlugin) UpdateData(_ context.Context) error {
	if p.apiKey == "" {
		return nil
	}
	if !atomic.CompareAndSwapInt32(&p.updating, 0, 1) {
		return nil // a sync is already in flight
	}

	go func() {
		defer atomic.StoreInt32(&p.updating, 0)
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Steam] update panic recovered: %v", r)
			}
		}()

		// Detached from the caller so the manager's per-plugin timeout cannot kill a sync midway.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		p.syncCycle(ctx, false)
	}()

	return nil
}

func (p *SteamPlugin) startBackground() {
	p.bgStarted.Do(func() {
		go p.backgroundLoop()
	})
}

// backgroundLoop owns the daily full sync and the achievement trickle, independent of visitor
// traffic. It restarts itself if it ever panics.
func (p *SteamPlugin) backgroundLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Steam] background loop panic recovered: %v", r)
			time.Sleep(time.Minute)
			go p.backgroundLoop()
		}
	}()

	if p.apiKey == "" {
		log.Printf("[Steam] no API key configured, background loop disabled")
		return
	}

	time.Sleep(30 * time.Second) // let the server finish booting first
	log.Printf("[Steam] background loop started (full library sync every %s, achievement enrichment trickle)",
		steamFullSyncInterval)

	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		p.syncCycle(ctx, true)
	}

	run()
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		run()
	}
}

// syncCycle does the cheap refresh always, the full library sync at most daily, and (when allowed)
// a bounded slice of the resumable achievement queue.
func (p *SteamPlugin) syncCycle(ctx context.Context, allowEnrich bool) {
	steamID := p.steamID()
	if steamID == "" {
		log.Printf("[Steam] steamid not configured, skipping sync")
		return
	}

	p.refreshLive(ctx, steamID)

	if time.Since(p.store.LastFullSync()) >= steamFullSyncInterval {
		p.fullSync(ctx, steamID)
	}

	if allowEnrich {
		p.enrichAchievements(ctx, steamID, steamEnrichPerCycle)
	}
}

// refreshLive pulls the player summary and recently-played games and patches those playtimes into
// the in-memory library. This is the frequent, cheap path.
func (p *SteamPlugin) refreshLive(ctx context.Context, steamID string) {
	p.mu.RLock()
	last := p.lastCheapSync
	p.mu.RUnlock()
	if time.Since(last) < steamCheapSyncInterval {
		return
	}

	if summary, err := p.api.PlayerSummary(ctx, steamID); err != nil {
		log.Printf("[Steam] player summary failed: %v", err)
	} else {
		p.updatePlayerSummary(summary)
	}

	recent, err := p.api.RecentGames(ctx, steamID)
	if err != nil {
		log.Printf("[Steam] recent games failed: %v", err)
	} else {
		sort.Slice(recent, func(i, j int) bool { return recent[i].Playtime2w > recent[j].Playtime2w })
		if len(recent) > 10 {
			recent = recent[:10]
		}
		p.mu.Lock()
		p.recentGames = recent
		p.mu.Unlock()
		p.patchFromRecent(recent)
	}

	p.mu.Lock()
	p.lastCheapSync = time.Now()
	p.mu.Unlock()
}

func (p *SteamPlugin) updatePlayerSummary(summary *SteamPlayerSummary) {
	p.mu.Lock()
	oldGame, oldState := "", 0
	if p.playerSummary != nil {
		oldGame = p.playerSummary.GameExtraInfo
		oldState = p.playerSummary.PersonaState
	}
	p.playerSummary = summary
	p.mu.Unlock()

	if oldGame == summary.GameExtraInfo && oldState == summary.PersonaState {
		return
	}

	gameImage := ""
	if summary.GameID != "" {
		gameImage = fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%s/header.jpg", summary.GameID)
	}

	p.hub.Broadcast("steam_status_update", map[string]interface{}{
		"isPlaying":    summary.GameExtraInfo != "",
		"currentGame":  summary.GameExtraInfo,
		"gameImage":    gameImage,
		"gameId":       summary.GameID,
		"personaState": summary.PersonaState,
		"personaName":  summary.PersonaName,
		"timestamp":    time.Now().Unix(),
	})
}

// patchFromRecent updates playtimes for actively played games without pulling the whole library.
func (p *SteamPlugin) patchFromRecent(recent []SteamGame) {
	if len(recent) == 0 {
		return
	}

	stored, _ := p.store.Games()
	if len(stored) == 0 {
		return
	}

	byID := make(map[int]SteamGame, len(recent))
	for _, g := range recent {
		byID[g.AppID] = g
	}

	changed := false
	for i := range stored {
		r, ok := byID[stored[i].AppID]
		if !ok {
			continue
		}
		if stored[i].Playtime2w != r.Playtime2w || stored[i].PlaytimeAll != r.PlaytimeAll {
			stored[i].Playtime2w = r.Playtime2w
			if r.PlaytimeAll > stored[i].PlaytimeAll {
				stored[i].PlaytimeAll = r.PlaytimeAll
			}
			if r.LastPlayed > stored[i].LastPlayed {
				stored[i].LastPlayed = r.LastPlayed
			}
			changed = true
		}
	}

	if !changed {
		return
	}
	p.store.SetGames(stored)
	p.applyLibrary(stored)
}

// fullSync re-pulls the entire owned library, merges the family shared library, stamps first-seen
// dates and captures the yearly snapshot.
func (p *SteamPlugin) fullSync(ctx context.Context, steamID string) {
	start := time.Now()
	token := p.accessToken()
	log.Printf("[Steam] full library sync starting (steamid=%s, family=%t)", steamID, token != "")

	owned, err := p.api.OwnedGames(ctx, steamID)
	if err != nil {
		log.Printf("[Steam] owned games failed: %v", err)
		return
	}

	// Anti-clobber: never replace a healthy library with an empty response.
	if len(owned) == 0 {
		if existing := p.store.GameCount(); existing > 0 {
			log.Printf("[Steam] ignoring empty owned-games response, keeping %d cached games", existing)
			return
		}
	}

	merged := make([]SteamGame, len(owned))
	copy(merged, owned)

	familyCount := 0
	if token != "" {
		family, groupID, ferr := p.api.fetchFamilyGames(ctx, token, steamID)
		switch {
		case ferr == errSteamTokenInvalid:
			p.store.SetFamilyValid(false)
			log.Printf("[Steam] family token invalid/expired, falling back to API key only")
		case ferr != nil:
			p.store.SetFamilyValid(false)
			log.Printf("[Steam] family library failed: %v", ferr)
		default:
			p.store.SetFamilyValid(true)
			ownedIDs := make(map[int]bool, len(merged))
			for _, g := range merged {
				ownedIDs[g.AppID] = true
			}
			for _, g := range family {
				if ownedIDs[g.AppID] {
					continue
				}
				merged = append(merged, g)
				familyCount++
			}
			log.Printf("[Steam] family library: %d shared apps merged (group=%s)", familyCount, groupID)
		}
	} else {
		p.store.SetFamilyValid(false)
	}

	// Preserve the 2-week playtimes we already know, which GetOwnedGames does not return.
	p.mu.RLock()
	recent := make([]SteamGame, len(p.recentGames))
	copy(recent, p.recentGames)
	p.mu.RUnlock()
	if len(recent) > 0 {
		recentByID := make(map[int]int, len(recent))
		for _, g := range recent {
			recentByID[g.AppID] = g.Playtime2w
		}
		for i := range merged {
			if mins, ok := recentByID[merged[i].AppID]; ok {
				merged[i].Playtime2w = mins
			}
		}
	}

	appIDs := make([]int, 0, len(merged))
	for _, g := range merged {
		appIDs = append(appIDs, g.AppID)
	}
	firstRun := p.store.IsFirstRun()
	newlyAdded := p.store.StampFirstSeen(appIDs)

	p.store.SetGames(merged)
	p.applyLibrary(merged)
	p.captureYearSnapshot(merged)
	p.queueEnrichment(merged)
	p.store.MarkFullSync()

	hidden := len(merged) - len(p.snapshotGames())
	var totalMinutes int
	for _, g := range merged {
		totalMinutes += g.PlaytimeAll
	}

	if firstRun {
		log.Printf("[Steam] owned games: %d fetched, %d after blacklist, first run (library dates recorded as today)",
			len(owned), len(merged)-hidden)
	} else {
		log.Printf("[Steam] owned games: %d fetched, %d after blacklist, %d newly added",
			len(owned), len(merged)-hidden, newlyAdded)
	}
	log.Printf("[Steam] full library sync done in %v (games=%d, playtime=%.1fh)",
		time.Since(start).Round(time.Millisecond), len(merged), float64(totalMinutes)/60.0)

	if p.invalidateCache != nil {
		p.invalidateCache()
	}
	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": "steam",
		"action": "library_updated",
		"games":  len(merged),
	})
}

// captureYearSnapshot records all-time playtime at the first sync of each calendar year so
// "played this year" can be derived as current minus snapshot.
func (p *SteamPlugin) captureYearSnapshot(games []SteamGame) {
	year := time.Now().Year()
	snap := p.store.YearSnapshot()
	if snap.Year == year && len(snap.Playtime) > 0 {
		return
	}

	playtime := make(map[int]int, len(games))
	for _, g := range games {
		playtime[g.AppID] = g.PlaytimeAll
	}
	p.store.SetYearSnapshot(steamYearSnapshot{
		Year:     year,
		Playtime: playtime,
		TakenAt:  time.Now().Unix(),
	})
	log.Printf("[Steam] year snapshot captured for %d (games=%d)", year, len(playtime))
}

// playtimeThisYear returns minutes played this year per appid, plus the snapshot date.
func (p *SteamPlugin) playtimeThisYear() (map[int]int, time.Time) {
	snap := p.store.YearSnapshot()
	out := make(map[int]int, len(snap.Playtime))
	if snap.Year != time.Now().Year() {
		return out, time.Unix(snap.TakenAt, 0)
	}
	for _, g := range p.snapshotGames() {
		base, ok := snap.Playtime[g.AppID]
		if !ok {
			// Acquired after the snapshot: everything played counts.
			base = 0
		}
		if delta := g.PlaytimeAll - base; delta > 0 {
			out[g.AppID] = delta
		}
	}
	return out, time.Unix(snap.TakenAt, 0)
}

// queueEnrichment refills the resumable achievement queue with games that have stats and no fresh
// cached entry, most-played first.
func (p *SteamPlugin) queueEnrichment(games []SteamGame) {
	candidates := make([]SteamGame, 0, len(games))
	for _, g := range games {
		if !g.HasStats || g.PlaytimeAll <= 0 {
			continue
		}
		if _, fresh := p.store.Achievement(g.AppID); fresh {
			continue
		}
		candidates = append(candidates, g)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].PlaytimeAll > candidates[j].PlaytimeAll
	})

	ids := make([]int, 0, len(candidates))
	for _, g := range candidates {
		ids = append(ids, g.AppID)
	}
	p.store.SetPending(ids)
	if len(ids) > 0 {
		log.Printf("[Steam] queued %d games for achievement enrichment", len(ids))
	}
}

// enrichAchievements drains up to max entries from the persisted queue. Progress is flushed in
// batches, so an interrupt resumes here rather than restarting.
func (p *SteamPlugin) enrichAchievements(ctx context.Context, steamID string, max int) {
	remaining := p.store.PendingCount()
	if remaining == 0 {
		return
	}

	batch := p.store.PopPending(max)
	if len(batch) == 0 {
		return
	}

	total := len(batch)
	done := 0
	for _, appID := range batch {
		if ctx.Err() != nil {
			break
		}

		game, ok := p.gameByID(appID)
		if !ok {
			continue
		}

		entry, err := p.buildAchievementEntry(ctx, steamID, game)
		if err != nil {
			log.Printf("[Steam] achievement enrichment failed for %d (%s): %v", appID, game.Name, err)
			continue
		}
		p.store.SetAchievement(appID, entry)
		done++

		fmt.Fprintf(os.Stderr, "\r[Steam] Enriching achievements: %d/%d games processed", done, total)

		select {
		case <-ctx.Done():
		case <-time.After(steamEnrichDelay):
		}
	}

	if done > 0 {
		fmt.Fprintf(os.Stderr, "\r[Steam] Enriching achievements: %d/%d games processed\n", done, total)
	}
	p.store.Flush()

	log.Printf("[Steam] achievements: %d/%d games enriched, %d pending", done, total, p.store.PendingCount())
}

// syncStatus reports what the admin page shows.
func (p *SteamPlugin) syncStatus() map[string]interface{} {
	last := p.store.LastFullSync()
	lastUnix := int64(0)
	if !last.IsZero() {
		lastUnix = last.Unix()
	}

	p.mu.RLock()
	family := p.familyApps
	games := len(p.games)
	p.mu.RUnlock()

	tokenState := "not set"
	if p.accessToken() != "" {
		if p.store.FamilyValid() {
			tokenState = "valid"
		} else {
			tokenState = "expired"
		}
	}

	return map[string]interface{}{
		"lastFullSync":   lastUnix,
		"games":          games,
		"familyApps":     family,
		"pending":        p.store.PendingCount(),
		"tokenState":     tokenState,
		"hasKey":         p.apiKey != "",
		"totalLibrary":   p.store.GameCount(),
		"achievementsOf": len(p.store.AllAchievements()),
	}
}
