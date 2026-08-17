package plugins

import (
	"context"
	"log"
	"sort"
)

// steamRarestPerGame caps how many rarest achievements we keep per game.
const steamRarestPerGame = 3

// buildAchievementEntry joins the player's unlocked achievements with global rarity percentages and
// the game schema (for display names and icon URLs), producing the cached entry for one game.
//
// It costs up to three API calls, which is why it only ever runs from the background trickle or an
// explicit on-demand expand — never in a loop over the whole library.
func (p *SteamPlugin) buildAchievementEntry(ctx context.Context, steamID string, game SteamGame) (steamAchEntry, error) {
	achievements, gameName, ok, err := p.api.PlayerAchievements(ctx, steamID, game.AppID)
	if err != nil {
		return steamAchEntry{}, err
	}
	if !ok || len(achievements) == 0 {
		return steamAchEntry{Available: false}, nil
	}

	if gameName == "" {
		gameName = game.Name
	}

	entry := steamAchEntry{Available: true, Total: len(achievements)}
	for _, a := range achievements {
		if a.Achieved == 1 {
			entry.Achieved++
		}
	}
	if entry.Total > 0 {
		entry.Percent = float64(entry.Achieved) / float64(entry.Total) * 100
	}

	// Rarity and icons are best-effort: without them we still report completion.
	global, gErr := p.api.GlobalAchievementPercentages(ctx, game.AppID)
	if gErr != nil {
		log.Printf("[Steam] global achievement rarity unavailable for %d (%s): %v", game.AppID, gameName, gErr)
		global = nil
	}
	schema, sErr := p.api.SchemaForGame(ctx, game.AppID)
	if sErr != nil {
		log.Printf("[Steam] achievement schema unavailable for %d (%s): %v", game.AppID, gameName, sErr)
		schema = nil
	}

	unlocked := make([]SteamRarestAchievement, 0, entry.Achieved)
	for _, a := range achievements {
		if a.Achieved != 1 {
			continue
		}
		r := SteamRarestAchievement{
			AppID:         game.AppID,
			GameName:      gameName,
			APIName:       a.APIName,
			Name:          a.Name,
			Description:   a.Desc,
			UnlockedAt:    a.UnlockTime,
			GlobalPercent: 100,
		}
		if pct, found := global[a.APIName]; found {
			r.GlobalPercent = pct
		}
		if s, found := schema[a.APIName]; found {
			if s.DisplayName != "" {
				r.Name = s.DisplayName
			}
			if s.Description != "" {
				r.Description = s.Description
			}
			// Some games ship an achievement with only the grey (locked) art, and a few have art
			// Steam itself no longer serves, so keep both and let the page fall through.
			r.Icon = s.Icon
			r.IconGray = s.IconGray
			if r.Icon == "" {
				r.Icon = s.IconGray
			}
		}
		if r.Name == "" {
			r.Name = a.APIName
		}
		unlocked = append(unlocked, r)
	}

	sort.Slice(unlocked, func(i, j int) bool {
		if unlocked[i].GlobalPercent != unlocked[j].GlobalPercent {
			return unlocked[i].GlobalPercent < unlocked[j].GlobalPercent
		}
		return unlocked[i].APIName < unlocked[j].APIName
	})
	if len(unlocked) > steamRarestPerGame {
		unlocked = unlocked[:steamRarestPerGame]
	}
	entry.Rarest = unlocked

	return entry, nil
}

// achievementFor returns the cached entry for a game, fetching it on demand when missing or stale.
func (p *SteamPlugin) achievementFor(ctx context.Context, appID int) (steamAchEntry, bool) {
	if entry, fresh := p.store.Achievement(appID); fresh {
		return entry, true
	}

	game, ok := p.gameByID(appID)
	if !ok || !game.HasStats {
		return steamAchEntry{Available: false}, false
	}

	steamID := p.steamID()
	if steamID == "" || p.apiKey == "" {
		return steamAchEntry{Available: false}, false
	}

	entry, err := p.buildAchievementEntry(ctx, steamID, game)
	if err != nil {
		// Fall back to a stale cached value rather than showing nothing.
		if stale, exists := p.store.Achievement(appID); exists {
			return stale, true
		}
		return steamAchEntry{Available: false}, false
	}

	p.store.SetAchievement(appID, entry)
	return entry, true
}

// rarestAcrossLibrary returns the rarest unlocked achievements across every enriched game.
func (p *SteamPlugin) rarestAcrossLibrary(limit int) []SteamRarestAchievement {
	if limit <= 0 {
		limit = 12
	}

	hidden := p.hiddenSet()
	cached := p.store.AllAchievements()

	var all []SteamRarestAchievement
	for appID, entry := range cached {
		if hidden[appID] || !entry.Available {
			continue
		}
		all = append(all, entry.Rarest...)
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].GlobalPercent != all[j].GlobalPercent {
			return all[i].GlobalPercent < all[j].GlobalPercent
		}
		return all[i].UnlockedAt > all[j].UnlockedAt
	})

	if len(all) > limit {
		all = all[:limit]
	}
	return all
}

// achievementSummary aggregates unlocked totals and 100%-completed games from the cache.
func (p *SteamPlugin) achievementSummary() (unlocked, total, perfect, enriched int) {
	hidden := p.hiddenSet()
	for appID, entry := range p.store.AllAchievements() {
		if hidden[appID] || !entry.Available {
			continue
		}
		enriched++
		unlocked += entry.Achieved
		total += entry.Total
		if entry.Total > 0 && entry.Achieved == entry.Total {
			perfect++
		}
	}
	return
}
