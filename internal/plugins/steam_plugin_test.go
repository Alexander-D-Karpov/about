package plugins

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

func newTestSteamPlugin(t *testing.T, settings map[string]interface{}) *SteamPlugin {
	t.Helper()

	// Not t.TempDir(): storage.SetPluginConfig spawns an async backup goroutine that can still be
	// writing when the test ends, and t.TempDir fails the test if cleanup finds a non-empty dir.
	dir, err := os.MkdirTemp("", "steamplugin")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	st := storage.New(dir)
	if err := st.Load(); err != nil {
		t.Fatal(err)
	}
	if settings != nil {
		if err := st.SetPluginConfig("steam", &storage.PluginConfig{Enabled: true, Settings: settings}); err != nil {
			t.Fatal(err)
		}
	}

	// Empty API key keeps the background loop and all network calls inert.
	return NewSteamPlugin(st, stream.New(), "", dir)
}

func TestSteamApplyLibraryFiltersAndSorts(t *testing.T) {
	p := newTestSteamPlugin(t, map[string]interface{}{
		"steamid":     "123",
		"hiddenGames": []interface{}{float64(2)},
	})

	p.applyLibrary([]SteamGame{
		{AppID: 1, Name: "Middle", PlaytimeAll: 100},
		{AppID: 2, Name: "Hidden", PlaytimeAll: 999},
		{AppID: 3, Name: "Most", PlaytimeAll: 500},
		{AppID: 4, Name: "Least", PlaytimeAll: 10},
	})

	games := p.snapshotGames()
	if len(games) != 3 {
		t.Fatalf("blacklisted game not filtered: got %d games", len(games))
	}
	if games[0].Name != "Most" || games[1].Name != "Middle" || games[2].Name != "Least" {
		t.Fatalf("games not sorted by playtime desc: %v", []string{games[0].Name, games[1].Name, games[2].Name})
	}
	for _, g := range games {
		if g.AppID == 2 {
			t.Fatalf("hidden game leaked into the visible library")
		}
	}
}

func TestSteamHiddenGamesExcludedFromMetrics(t *testing.T) {
	p := newTestSteamPlugin(t, map[string]interface{}{
		"steamid":     "123",
		"hiddenGames": []interface{}{float64(2)},
	})
	p.applyLibrary([]SteamGame{
		{AppID: 1, PlaytimeAll: 60},
		{AppID: 2, PlaytimeAll: 6000},
	})

	m := p.GetMetrics()
	if hours := m["total_playtime_hours"].(float64); hours != 1 {
		t.Fatalf("hidden game counted in playtime: %v", hours)
	}

	// The stats page pre-declares these four keys; they must keep existing.
	for _, k := range []string{"is_online", "is_playing", "recent_games_count", "total_playtime_hours"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("required metric %q missing", k)
		}
	}
}

func TestSteamUpdateDataReturnsImmediately(t *testing.T) {
	p := newTestSteamPlugin(t, map[string]interface{}{"steamid": "123"})

	done := make(chan struct{})
	go func() {
		_ = p.UpdateData(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("UpdateData blocked; it must return immediately so it cannot delay server start")
	}
}

func TestSteamPlaytimeThisYearDelta(t *testing.T) {
	p := newTestSteamPlugin(t, map[string]interface{}{"steamid": "123"})

	p.applyLibrary([]SteamGame{
		{AppID: 1, Name: "Played more", PlaytimeAll: 300},
		{AppID: 2, Name: "Untouched", PlaytimeAll: 100},
		{AppID: 3, Name: "New this year", PlaytimeAll: 45},
	})
	p.store.SetYearSnapshot(steamYearSnapshot{
		Year:     time.Now().Year(),
		Playtime: map[int]int{1: 200, 2: 100}, // appid 3 acquired after the snapshot
		TakenAt:  time.Now().Add(-30 * 24 * time.Hour).Unix(),
	})

	yearly, _ := p.playtimeThisYear()
	if yearly[1] != 100 {
		t.Fatalf("expected 100 minutes this year for appid 1, got %d", yearly[1])
	}
	if _, ok := yearly[2]; ok {
		t.Fatalf("game with no new playtime should be absent, got %d", yearly[2])
	}
	if yearly[3] != 45 {
		t.Fatalf("game acquired after the snapshot should count fully, got %d", yearly[3])
	}
}

func TestSteamPlaytimeThisYearIgnoresStaleSnapshot(t *testing.T) {
	p := newTestSteamPlugin(t, map[string]interface{}{"steamid": "123"})
	p.applyLibrary([]SteamGame{{AppID: 1, PlaytimeAll: 300}})
	p.store.SetYearSnapshot(steamYearSnapshot{
		Year:     time.Now().Year() - 1,
		Playtime: map[int]int{1: 10},
	})

	if yearly, _ := p.playtimeThisYear(); len(yearly) != 0 {
		t.Fatalf("last year's snapshot must not produce this-year figures: %v", yearly)
	}
}

func TestSteamPatchFromRecentUpdatesPlaytime(t *testing.T) {
	p := newTestSteamPlugin(t, map[string]interface{}{"steamid": "123"})

	initial := []SteamGame{
		{AppID: 1, Name: "Active", PlaytimeAll: 100, Playtime2w: 0, LastPlayed: 10},
		{AppID: 2, Name: "Idle", PlaytimeAll: 50},
	}
	p.store.SetGames(initial)
	p.applyLibrary(initial)

	p.patchFromRecent([]SteamGame{
		{AppID: 1, PlaytimeAll: 160, Playtime2w: 60, LastPlayed: 99},
	})

	games := p.snapshotGames()
	var active SteamGame
	for _, g := range games {
		if g.AppID == 1 {
			active = g
		}
	}
	if active.PlaytimeAll != 160 || active.Playtime2w != 60 || active.LastPlayed != 99 {
		t.Fatalf("recent playtime not patched in: %+v", active)
	}

	// It must also be persisted, so a restart keeps the fresher numbers.
	stored, _ := p.store.Games()
	for _, g := range stored {
		if g.AppID == 1 && g.PlaytimeAll != 160 {
			t.Fatalf("patched playtime not persisted: %+v", g)
		}
	}
}

func TestSteamPatchFromRecentNeverLowersTotal(t *testing.T) {
	p := newTestSteamPlugin(t, map[string]interface{}{"steamid": "123"})
	initial := []SteamGame{{AppID: 1, PlaytimeAll: 500, Playtime2w: 10}}
	p.store.SetGames(initial)
	p.applyLibrary(initial)

	// A stale/partial response must not shrink a known total.
	p.patchFromRecent([]SteamGame{{AppID: 1, PlaytimeAll: 5, Playtime2w: 2}})

	if got := p.snapshotGames()[0].PlaytimeAll; got != 500 {
		t.Fatalf("total playtime regressed to %d", got)
	}
}

func TestSteamSetHiddenGamesRefiltersLibrary(t *testing.T) {
	p := newTestSteamPlugin(t, map[string]interface{}{"steamid": "123"})
	games := []SteamGame{{AppID: 1, PlaytimeAll: 10}, {AppID: 2, PlaytimeAll: 20}}
	p.store.SetGames(games)
	p.applyLibrary(games)

	if len(p.snapshotGames()) != 2 {
		t.Fatalf("setup failed")
	}

	p.SetHiddenGames([]int{1})
	if got := len(p.snapshotGames()); got != 1 {
		t.Fatalf("expected 1 visible game after hiding, got %d", got)
	}

	p.SetHiddenGames(nil)
	if got := len(p.snapshotGames()); got != 2 {
		t.Fatalf("expected 2 visible games after unhiding, got %d", got)
	}
}

// Regression: Steam returns global achievement rarity as a JSON string ("64.4"), not a number.
// Decoding it into float64 used to fail silently and made every achievement look 100% common.
func TestSteamFlexFloatAcceptsStringAndNumber(t *testing.T) {
	var resp steamGlobalPercentResponse
	body := `{"achievementpercentages":{"achievements":[
		{"name":"a","percent":"64.4"},
		{"name":"b","percent":3.1},
		{"name":"c","percent":"0"}
	]}}`
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("failed to decode mixed string/number percentages: %v", err)
	}

	got := map[string]float64{}
	for _, a := range resp.AchievementPercentages.Achievements {
		got[a.Name] = float64(a.Percent)
	}
	if got["a"] != 64.4 || got["b"] != 3.1 || got["c"] != 0 {
		t.Fatalf("unexpected parsed percentages: %v", got)
	}
}

func TestSteamTokenStateLifecycle(t *testing.T) {
	p := newTestSteamPlugin(t, map[string]interface{}{"steamid": "123"})

	if got := p.AdminStatus()["tokenState"]; got != "not set" {
		t.Fatalf("expected 'not set' with no token, got %q", got)
	}

	// A freshly saved token has not been exercised yet; it must not be called expired.
	p.SetAccessToken("some-token")
	if got := p.AdminStatus()["tokenState"]; got != "not checked yet" {
		t.Fatalf("expected 'not checked yet' for a fresh token, got %q", got)
	}

	// A sync tried it and it failed.
	p.store.MarkTokenChecked()
	p.store.SetFamilyValid(false)
	if got := p.AdminStatus()["tokenState"]; got != "expired" {
		t.Fatalf("expected 'expired' after a failed check, got %q", got)
	}

	// A sync tried it and it worked.
	p.store.SetFamilyValid(true)
	if got := p.AdminStatus()["tokenState"]; got != "valid" {
		t.Fatalf("expected 'valid' after a successful check, got %q", got)
	}

	// Replacing the token resets verification.
	p.SetAccessToken("another-token")
	if got := p.AdminStatus()["tokenState"]; got != "not checked yet" {
		t.Fatalf("replacing the token must reset verification, got %q", got)
	}

	p.SetAccessToken("")
	if got := p.AdminStatus()["tokenState"]; got != "not set" {
		t.Fatalf("expected 'not set' after clearing, got %q", got)
	}
	if p.HasAccessToken() {
		t.Fatalf("HasAccessToken should be false after clearing")
	}
}

func TestSteamRarestOrdersByGlobalRarity(t *testing.T) {
	p := newTestSteamPlugin(t, map[string]interface{}{"steamid": "123"})
	games := []SteamGame{{AppID: 1, Name: "A"}, {AppID: 2, Name: "B"}}
	p.store.SetGames(games)
	p.applyLibrary(games)

	p.store.SetAchievement(1, steamAchEntry{Available: true, Achieved: 1, Total: 1, Rarest: []SteamRarestAchievement{
		{AppID: 1, Name: "common", GlobalPercent: 40},
		{AppID: 1, Name: "rare", GlobalPercent: 2.5},
	}})
	p.store.SetAchievement(2, steamAchEntry{Available: true, Achieved: 1, Total: 1, Rarest: []SteamRarestAchievement{
		{AppID: 2, Name: "rarest", GlobalPercent: 0.4},
	}})

	out := p.rarestAcrossLibrary(10)
	if len(out) != 3 {
		t.Fatalf("expected 3 achievements, got %d", len(out))
	}
	if out[0].Name != "rarest" || out[1].Name != "rare" || out[2].Name != "common" {
		t.Fatalf("wrong rarity order: %v", []string{out[0].Name, out[1].Name, out[2].Name})
	}
}

func TestSteamRarestExcludesHiddenGames(t *testing.T) {
	p := newTestSteamPlugin(t, map[string]interface{}{
		"steamid":     "123",
		"hiddenGames": []interface{}{float64(2)},
	})
	games := []SteamGame{{AppID: 1, Name: "A"}, {AppID: 2, Name: "Hidden"}}
	p.store.SetGames(games)
	p.applyLibrary(games)

	p.store.SetAchievement(1, steamAchEntry{Available: true, Rarest: []SteamRarestAchievement{{AppID: 1, Name: "keep", GlobalPercent: 5}}})
	p.store.SetAchievement(2, steamAchEntry{Available: true, Rarest: []SteamRarestAchievement{{AppID: 2, Name: "drop", GlobalPercent: 0.1}}})

	out := p.rarestAcrossLibrary(10)
	if len(out) != 1 || out[0].Name != "keep" {
		t.Fatalf("hidden game leaked into the rarest showcase: %+v", out)
	}

	unlocked, _, _, enriched := p.achievementSummary()
	if enriched != 1 {
		t.Fatalf("hidden game counted in the achievement summary (enriched=%d, unlocked=%d)", enriched, unlocked)
	}
}

// Steam reports Blender and Wallpaper Engine as type "game", so only the genres reveal that they
// are software. Getting this wrong is what left them in the library.
func TestSteamIsGameUsesGenresNotType(t *testing.T) {
	cases := []struct {
		name string
		info steamAppInfo
		want bool
	}{
		{"plain game", steamAppInfo{Type: "game", Genres: []string{"Action", "Indie"}}, true},
		{"blender", steamAppInfo{Type: "game", Genres: []string{"Animation & Modeling", "Design & Illustration", "Video Production"}}, false},
		{"wallpaper engine", steamAppInfo{Type: "game", Genres: []string{"Casual", "Indie", "Utilities", "Photo Editing"}}, false},
		{"dlc", steamAppInfo{Type: "dlc", Genres: []string{"Action"}}, false},
		{"demo", steamAppInfo{Type: "demo"}, false},
		{"soundtrack", steamAppInfo{Type: "music"}, false},
		{"unknown genres", steamAppInfo{Type: "game"}, true},
		{"case insensitive", steamAppInfo{Type: "game", Genres: []string{"UTILITIES"}}, false},
	}
	for _, c := range cases {
		if got := steamIsGame(c.info); got != c.want {
			t.Errorf("%s: steamIsGame = %v, want %v", c.name, got, c.want)
		}
	}
}

// Steam moved achievement art; the schema still returns the old path, which 404s for some games.
func TestSteamModernIconURLRewritesLegacyPaths(t *testing.T) {
	const hash = "73495fcb0e9edc5b4a752f774289d5fcbdf4c693.jpg"
	legacy := "https://steamcdn-a.akamaihd.net/steamcommunity/public/images/apps/427520/" + hash
	want := "https://shared.akamai.steamstatic.com/community_assets/images/apps/427520/" + hash

	if got := steamModernIconURL(legacy); got != want {
		t.Fatalf("legacy rewrite = %q, want %q", got, want)
	}
	if got := steamModernIconURL(want); got != "" {
		t.Fatalf("already-modern URL should not be rewritten, got %q", got)
	}
	if got := steamModernIconURL("https://example.com/x.jpg"); got != "" {
		t.Fatalf("unrelated URL should not be rewritten, got %q", got)
	}
}

// Cached entries written before the path change must still be corrected when served.
func TestSteamNormalizeIconsUpgradesCachedEntries(t *testing.T) {
	legacy := "https://steamcdn-a.akamaihd.net/steamcommunity/public/images/apps/427520/a.jpg"
	in := []SteamRarestAchievement{{Name: "x", Icon: legacy}}

	out := steamNormalizeIcons(in)
	if !strings.HasPrefix(out[0].Icon, steamAssetIconPrefix) {
		t.Fatalf("icon not upgraded: %s", out[0].Icon)
	}
	if out[0].IconGray != legacy {
		t.Fatalf("original URL should be kept as a fallback, got %q", out[0].IconGray)
	}
	if in[0].Icon != legacy {
		t.Fatalf("input slice must not be mutated")
	}
}
