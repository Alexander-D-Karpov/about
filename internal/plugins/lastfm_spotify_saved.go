package plugins

import (
	"context"
	"log"
	"strings"
	"time"
)

func spotifySavedKey(artist, name string) string {
	a := strings.ToLower(strings.TrimSpace(artist))
	n := strings.ToLower(strings.TrimSpace(name))
	if a == "" && n == "" {
		return ""
	}
	return a + keySep + n
}

func (p *LastFMPlugin) loadSpotifySavedCache() {
	cfg := p.storage.GetPluginConfig(p.Name())
	set := make(map[string]bool)
	if raw, ok := cfg.Settings["spotify_saved_keys"].([]interface{}); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok && s != "" {
				set[s] = true
			}
		}
	}
	count := len(set)
	if c, ok := cfg.Settings["spotify_saved_count"].(float64); ok && int(c) > 0 {
		count = int(c)
	}
	p.spSavedMu.Lock()
	p.spSavedCache = set
	p.spLikedCount = count
	p.spSavedMu.Unlock()
}

func (p *LastFMPlugin) saveSpotifySavedCache() {
	p.spSavedMu.RLock()
	keys := make([]interface{}, 0, len(p.spSavedCache))
	for k := range p.spSavedCache {
		keys = append(keys, k)
	}
	count := p.spLikedCount
	p.spSavedMu.RUnlock()

	cfg := p.storage.GetPluginConfig(p.Name())
	if cfg.Settings == nil {
		cfg.Settings = map[string]interface{}{}
	}
	cfg.Settings["spotify_saved_keys"] = keys
	cfg.Settings["spotify_saved_count"] = float64(count)
	_ = p.storage.SetPluginConfig(p.Name(), cfg)
}

func (p *LastFMPlugin) UpdateSpotifySaved(ctx context.Context) error {
	if p.spotify == nil || !p.spotify.UserEnabled() {
		return nil
	}
	tracks, total, err := p.spotify.SavedTracks(ctx)
	if err != nil {
		return err
	}
	if len(tracks) == 0 {
		return nil
	}
	set := make(map[string]bool, len(tracks))
	for _, t := range tracks {
		if key := spotifySavedKey(t.Artist, t.Track); key != "" {
			set[key] = true
		}
	}
	if total <= 0 {
		total = len(set)
	}
	p.spSavedMu.Lock()
	p.spSavedCache = set
	p.spLikedCount = total
	p.spSavedMu.Unlock()
	p.saveSpotifySavedCache()
	return nil
}

func (p *LastFMPlugin) refreshSpotifySavedIfStale(ctx context.Context) {
	if p.spotify == nil || !p.spotify.UserEnabled() {
		return
	}
	cfg := p.storage.GetPluginConfig(p.Name())
	if ts, ok := cfg.Settings["spotify_saved_at"].(float64); ok {
		if time.Since(time.Unix(int64(ts), 0)) < 30*time.Minute {
			return
		}
	}
	if err := p.UpdateSpotifySaved(ctx); err != nil {
		log.Printf("[LastFM] spotify saved update failed: %v", err)
	}
	cfg = p.storage.GetPluginConfig(p.Name())
	if cfg.Settings == nil {
		cfg.Settings = map[string]interface{}{}
	}
	cfg.Settings["spotify_saved_at"] = float64(time.Now().Unix())
	_ = p.storage.SetPluginConfig(p.Name(), cfg)
}
