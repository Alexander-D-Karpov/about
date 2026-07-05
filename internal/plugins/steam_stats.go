package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"
)

func (p *SteamPlugin) computePlatformTotals(games []SteamGame) {
	totals := map[string]int{}
	for _, g := range games {
		linux := g.PlaytimeLinux - g.PlaytimeDeck
		if linux < 0 {
			linux = 0
		}
		totals["Windows"] += g.PlaytimeWindows
		totals["macOS"] += g.PlaytimeMac
		totals["Linux"] += linux
		totals["Steam Deck"] += g.PlaytimeDeck
	}
	p.statMu.Lock()
	p.platformTotals = totals
	p.statMu.Unlock()
}

func (p *SteamPlugin) computeGenreTotals(games []SteamGame) {
	p.genreMu.RLock()
	cache := p.genreCache
	totals := map[string]int{}
	for _, g := range games {
		if g.PlaytimeAll == 0 {
			continue
		}
		for _, gen := range cache[g.AppID] {
			totals[gen] += g.PlaytimeAll
		}
	}
	p.genreMu.RUnlock()

	p.statMu.Lock()
	p.genreTotals = totals
	p.statMu.Unlock()
}

func (p *SteamPlugin) fetchMissingGenres(games []SteamGame) {
	sorted := make([]SteamGame, len(games))
	copy(sorted, games)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PlaytimeAll > sorted[j].PlaytimeAll
	})

	const maxFetch = 60
	fetched := 0
	changed := false

	for _, g := range sorted {
		if g.PlaytimeAll == 0 {
			continue
		}
		p.genreMu.RLock()
		_, ok := p.genreCache[g.AppID]
		p.genreMu.RUnlock()
		if ok {
			continue
		}

		genres := p.fetchAppGenres(g.AppID)

		p.genreMu.Lock()
		if p.genreCache == nil {
			p.genreCache = map[int][]string{}
		}
		p.genreCache[g.AppID] = genres
		p.genreMu.Unlock()

		changed = true
		fetched++
		if fetched >= maxFetch {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	if changed {
		p.saveGenreCache()
		p.computeGenreTotals(games)
	}
}

func (p *SteamPlugin) fetchAppGenres(appID int) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("https://store.steampowered.com/api/appdetails?appids=%d&filters=genres&l=english", appID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var out map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Genres []struct {
				Description string `json:"description"`
			} `json:"genres"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil
	}

	entry, ok := out[strconv.Itoa(appID)]
	if !ok || !entry.Success {
		return []string{}
	}

	genres := make([]string, 0, len(entry.Data.Genres))
	for _, ge := range entry.Data.Genres {
		if ge.Description != "" {
			genres = append(genres, ge.Description)
		}
	}
	return genres
}

func (p *SteamPlugin) saveGenreCache() {
	p.genreMu.RLock()
	cp := make(map[string]interface{}, len(p.genreCache))
	for appID, genres := range p.genreCache {
		arr := make([]interface{}, 0, len(genres))
		for _, g := range genres {
			arr = append(arr, g)
		}
		cp[strconv.Itoa(appID)] = arr
	}
	p.genreMu.RUnlock()

	cfg := p.storage.GetPluginConfig(p.Name())
	if cfg.Settings == nil {
		cfg.Settings = map[string]interface{}{}
	}
	cfg.Settings["genreCache"] = cp
	_ = p.storage.SetPluginConfig(p.Name(), cfg)
}

func (p *SteamPlugin) loadGenreCache() {
	cfg := p.storage.GetPluginConfig(p.Name())
	raw, ok := cfg.Settings["genreCache"].(map[string]interface{})
	if !ok {
		return
	}
	cache := map[int][]string{}
	for k, v := range raw {
		appID, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		arr, ok := v.([]interface{})
		if !ok {
			continue
		}
		var genres []string
		for _, g := range arr {
			if s, ok := g.(string); ok {
				genres = append(genres, s)
			}
		}
		cache[appID] = genres
	}
	p.genreMu.Lock()
	p.genreCache = cache
	p.genreMu.Unlock()
}
