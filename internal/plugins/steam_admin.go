package plugins

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
)

// SteamAdminGame is one row in the admin blacklist manager.
type SteamAdminGame struct {
	AppID    int    `json:"appid"`
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	Hours    string `json:"hours"`
	Hidden   bool   `json:"hidden"`
	Source   string `json:"source,omitempty"`
	StoreURL string `json:"storeUrl"`
}

// AdminGames lists every game in the stored library, including blacklisted ones, so the admin can
// toggle them. Unlike the public API this deliberately does not filter by the blacklist.
func (p *SteamPlugin) AdminGames(search string) []SteamAdminGame {
	all, _ := p.store.Games()
	hidden := p.hiddenSet()
	needle := strings.ToLower(strings.TrimSpace(search))

	out := make([]SteamAdminGame, 0, len(all))
	for _, g := range all {
		if needle != "" && !strings.Contains(strings.ToLower(g.Name), needle) {
			continue
		}
		out = append(out, SteamAdminGame{
			AppID:    g.AppID,
			Name:     g.Name,
			Icon:     steamIconImage(g.AppID, g.ImgIconURL),
			Hours:    fmt.Sprintf("%.1fh", float64(g.PlaytimeAll)/60.0),
			Hidden:   hidden[g.AppID],
			Source:   g.Source,
			StoreURL: "https://store.steampowered.com/app/" + strconv.Itoa(g.AppID) + "/",
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// AdminStatus is the sync/token panel payload. It never includes the token itself.
func (p *SteamPlugin) AdminStatus() map[string]interface{} {
	return p.syncStatus()
}

// SetHiddenGames replaces the blacklist and re-filters the in-memory library.
func (p *SteamPlugin) SetHiddenGames(appIDs []int) {
	p.setHiddenGames(appIDs)
}

// SetAccessToken stores a new Steam family access token. An empty string clears it.
func (p *SteamPlugin) SetAccessToken(token string) {
	token = strings.TrimSpace(token)
	p.setAccessToken(token)
	// The new token has not been exercised yet, so drop both the validity flag and the
	// "we checked it" marker; the next full sync decides.
	p.store.SetFamilyValid(false)
	p.store.ResetTokenCheck()
	p.store.Flush()
}

// HasAccessToken reports whether a token is stored, without revealing it.
func (p *SteamPlugin) HasAccessToken() bool {
	return p.accessToken() != ""
}

// ForceFullSync clears the daily-sync marker so the next cycle re-pulls the whole library.
func (p *SteamPlugin) ForceFullSync() {
	p.store.ClearFullSync()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Steam] forced sync panic recovered: %v", r)
			}
		}()
		_ = p.UpdateData(context.Background())
	}()
}
