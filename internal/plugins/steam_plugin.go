package plugins

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type SteamPlugin struct {
	storage *storage.Storage
	hub     *stream.Hub
	apiKey  string
	store   *SteamStore
	api     *steamAPI

	// mu guards the in-memory view rendered by Render/RenderText/GetMetrics while the background
	// sync writes to it.
	mu            sync.RWMutex
	games         []SteamGame
	recentGames   []SteamGame
	topGames      []SteamGame
	playerSummary *SteamPlayerSummary
	familyApps    int
	lastCheapSync time.Time

	// settingsMu serialises the read-modify-write of the plugin's settings map. storage hands back
	// its live map, so concurrent mutations would otherwise be a concurrent map write.
	settingsMu sync.Mutex

	updating        int32
	bgStarted       sync.Once
	invalidateCache func()
}

func NewSteamPlugin(st *storage.Storage, hub *stream.Hub, apiKey, dataPath string) *SteamPlugin {
	store := NewSteamStore(dataPath)
	p := &SteamPlugin{
		storage: st,
		hub:     hub,
		apiKey:  apiKey,
		store:   store,
		api:     newSteamAPI(apiKey),
	}

	// Restore from disk so a restart continues instead of re-pulling the library.
	games, updated := store.Games()
	if len(games) > 0 {
		p.applyLibrary(games)
		log.Printf("[Steam] restored %d games from file (age %s), %d pending achievement enrichment",
			len(games), time.Since(updated).Round(time.Minute), store.PendingCount())
	}

	p.startBackground()
	return p
}

func (p *SteamPlugin) Name() string { return "steam" }

func (p *SteamPlugin) SetCacheInvalidator(fn func()) { p.invalidateCache = fn }

// --- settings helpers ---

func (p *SteamPlugin) GetSettings() map[string]interface{} {
	return p.storage.GetPluginConfig(p.Name()).Settings
}

func (p *SteamPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings
	if err := p.storage.SetPluginConfig(p.Name(), config); err != nil {
		return err
	}
	// The blacklist may have changed; re-apply it to the in-memory library.
	games, _ := p.store.Games()
	if len(games) > 0 {
		p.applyLibrary(games)
	}
	return nil
}

// persistSettings serialises a read-modify-write against the plugin's settings map.
func (p *SteamPlugin) persistSettings(mutate func(map[string]interface{})) {
	p.settingsMu.Lock()
	defer p.settingsMu.Unlock()

	cfg := p.storage.GetPluginConfig(p.Name())
	if cfg.Settings == nil {
		cfg.Settings = map[string]interface{}{}
	}
	mutate(cfg.Settings)
	if err := p.storage.SetPluginConfig(p.Name(), cfg); err != nil {
		log.Printf("[Steam] persist failed: %v", err)
	}
}

func (p *SteamPlugin) steamID() string {
	id, _ := p.GetSettings()["steamid"].(string)
	return strings.TrimSpace(id)
}

func (p *SteamPlugin) accessToken() string {
	tok, _ := p.GetSettings()["access_token"].(string)
	return strings.TrimSpace(tok)
}

// hiddenSet returns the blacklisted appids as a lookup set.
func (p *SteamPlugin) hiddenSet() map[int]bool {
	out := map[int]bool{}
	raw, ok := p.GetSettings()["hiddenGames"].([]interface{})
	if !ok {
		return out
	}
	for _, v := range raw {
		switch n := v.(type) {
		case float64:
			out[int(n)] = true
		case int:
			out[n] = true
		case string:
			var id int
			if _, err := fmt.Sscanf(n, "%d", &id); err == nil {
				out[id] = true
			}
		}
	}
	return out
}

func (p *SteamPlugin) setHiddenGames(appIDs []int) {
	vals := make([]interface{}, 0, len(appIDs))
	for _, id := range appIDs {
		vals = append(vals, id)
	}
	p.persistSettings(func(s map[string]interface{}) {
		s["hiddenGames"] = vals
	})
	games, _ := p.store.Games()
	p.applyLibrary(games)
}

func (p *SteamPlugin) setAccessToken(token string) {
	p.persistSettings(func(s map[string]interface{}) {
		s["access_token"] = token
	})
}

// --- in-memory library view ---

// applyLibrary filters the stored library through the blacklist, sorts by total playtime and
// refreshes the derived top-games slice.
func (p *SteamPlugin) applyLibrary(games []SteamGame) {
	hidden := p.hiddenSet()
	types := p.store.AppTypes()

	visible := make([]SteamGame, 0, len(games))
	for _, g := range games {
		if hidden[g.AppID] {
			continue
		}
		// Only actual games. Apps whose type we have not resolved yet stay visible so the list
		// is never mysteriously short while the lookup is still running.
		if t, ok := types[g.AppID]; ok && t != "game" {
			continue
		}
		g.Type = types[g.AppID]
		visible = append(visible, g)
	}

	sort.Slice(visible, func(i, j int) bool {
		if visible[i].PlaytimeAll != visible[j].PlaytimeAll {
			return visible[i].PlaytimeAll > visible[j].PlaytimeAll
		}
		return strings.ToLower(visible[i].Name) < strings.ToLower(visible[j].Name)
	})

	top := visible
	if len(top) > 10 {
		top = top[:10]
	}
	topCopy := make([]SteamGame, len(top))
	copy(topCopy, top)

	familyCount := 0
	for _, g := range visible {
		if g.Source == "family" {
			familyCount++
		}
	}

	p.mu.Lock()
	p.games = visible
	p.topGames = topCopy
	p.familyApps = familyCount
	p.mu.Unlock()
}

// snapshotGames returns a copy of the visible library.
func (p *SteamPlugin) snapshotGames() []SteamGame {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]SteamGame, len(p.games))
	copy(out, p.games)
	return out
}

func (p *SteamPlugin) gameByID(appID int) (SteamGame, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, g := range p.games {
		if g.AppID == appID {
			return g, true
		}
	}
	return SteamGame{}, false
}

func (p *SteamPlugin) snapshotSummary() *SteamPlayerSummary {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.playerSummary == nil {
		return nil
	}
	s := *p.playerSummary
	return &s
}

// --- image helpers ---

func steamHeaderImage(appID int) string {
	return fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%d/header.jpg", appID)
}

// steamBannerFallbacks lists progressively less ideal banner art for one app. Everything here has
// the same wide aspect as header.jpg, so a fallback never renders stretched or blurry; the tiny
// library icon is deliberately excluded and handled separately by the page.
func steamBannerFallbacks(appID int) []string {
	return []string{
		fmt.Sprintf("https://shared.cloudflare.steamstatic.com/store_item_assets/steam/apps/%d/header.jpg", appID),
		fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%d/capsule_616x353.jpg", appID),
		fmt.Sprintf("https://shared.cloudflare.steamstatic.com/store_item_assets/steam/apps/%d/capsule_616x353.jpg", appID),
		fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%d/capsule_231x87.jpg", appID),
		// Wide key art. Not a header crop, but far better than falling through to a 32px icon.
		fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%d/library_hero.jpg", appID),
		fmt.Sprintf("https://shared.cloudflare.steamstatic.com/store_item_assets/steam/apps/%d/library_hero.jpg", appID),
	}
}

func steamIconImage(appID int, iconHash string) string {
	if iconHash == "" {
		return ""
	}
	return fmt.Sprintf("https://media.steampowered.com/steamcommunity/public/images/apps/%d/%s.jpg", appID, iconHash)
}

// --- text + metrics ---

func (p *SteamPlugin) RenderText(_ context.Context) (string, error) {
	if p.apiKey == "" {
		return "Gaming: Steam API key not configured", nil
	}

	summary := p.snapshotSummary()
	if summary == nil {
		return "Gaming: No Steam data available", nil
	}

	status := "Offline"
	currentGame := ""

	if summary.GameExtraInfo != "" {
		status = "Playing"
		currentGame = fmt.Sprintf(" - %s", summary.GameExtraInfo)
	} else {
		switch summary.PersonaState {
		case 1:
			status = "Online"
		case 2:
			status = "Busy"
		case 3:
			status = "Away"
		}
	}

	p.mu.RLock()
	recentCount := len(p.recentGames)
	p.mu.RUnlock()

	gamesInfo := ""
	if recentCount > 0 {
		gamesInfo = fmt.Sprintf(", %d recent games", recentCount)
	}

	return fmt.Sprintf("Gaming: %s%s%s", status, currentGame, gamesInfo), nil
}

// GetMetrics keeps the four keys the stats page pre-declares as about_steam_*.
func (p *SteamPlugin) GetMetrics() map[string]interface{} {
	p.mu.RLock()
	recentCount := len(p.recentGames)
	games := p.games
	summary := p.playerSummary

	var totalPlaytime int
	for _, g := range games {
		totalPlaytime += g.PlaytimeAll
	}
	gameCount := len(games)
	p.mu.RUnlock()

	metrics := map[string]interface{}{
		"is_online":            0,
		"is_playing":           0,
		"recent_games_count":   recentCount,
		"total_playtime_hours": float64(totalPlaytime) / 60.0,
		"games_count":          gameCount,
	}

	if summary != nil {
		if summary.PersonaState == 1 {
			metrics["is_online"] = 1
		}
		if summary.GameExtraInfo != "" {
			metrics["is_playing"] = 1
		}
	}

	return metrics
}

func (p *SteamPlugin) getConfigValue(settings map[string]interface{}, key, defaultValue string) string {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(string); ok {
				return value
			}
			return defaultValue
		}
		if next, ok := current[k].(map[string]interface{}); ok {
			current = next
		} else {
			return defaultValue
		}
	}

	return defaultValue
}

func (p *SteamPlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(bool); ok {
				return value
			}
			return defaultValue
		}
		if next, ok := current[k].(map[string]interface{}); ok {
			current = next
		} else {
			return defaultValue
		}
	}

	return defaultValue
}
