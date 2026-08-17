package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const steamAPIHost = "https://api.steampowered.com"

// steamAPI is a thin Steam Web API client with retry/backoff. Unlike the old implementation it
// reuses the shared HTTP client (proxy aware) and threads the caller's context into every request.
type steamAPI struct {
	key    string
	client *http.Client
}

func newSteamAPI(key string) *steamAPI {
	return &steamAPI{key: key, client: NewHTTPClientWithTimeout(60 * time.Second)}
}

type steamHTTPError struct {
	Status int
	URL    string
}

func (e *steamHTTPError) Error() string {
	return fmt.Sprintf("steam api %s returned status %d", e.URL, e.Status)
}

func steamRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// get issues a GET with up to 3 attempts, backing off 500ms → 1s → 2s (capped at 5s) on 429/5xx and
// transport errors.
func (a *steamAPI) get(ctx context.Context, path string, q url.Values, out interface{}) error {
	endpoint := steamAPIHost + path
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	// Keep the key out of any error string we might log.
	safeURL := path

	// One attempt, scoped so the request context stays alive until the body is fully decoded.
	// (The shared DoRequestWithContext helper cancels on return, which truncates large responses
	// such as the full owned-games library.)
	attempt := func() (bool, error) {
		reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, "GET", endpoint, nil)
		if err != nil {
			return false, err
		}
		req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")
		req.Header.Set("Accept", "application/json")

		resp, err := a.client.Do(req)
		if err != nil {
			return true, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			herr := &steamHTTPError{Status: resp.StatusCode, URL: safeURL}
			return steamRetryable(resp.StatusCode), herr
		}

		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			return true, fmt.Errorf("steam api %s returned non-JSON content: %s", safeURL, ct)
		}

		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return true, fmt.Errorf("decode %s: %w", safeURL, err)
		}
		return false, nil
	}

	backoff := 500 * time.Millisecond
	var lastErr error

	for i := 0; i < 3; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff *= 2; backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		retry, err := attempt()
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry {
			return err
		}
	}

	return lastErr
}

func (a *steamAPI) PlayerSummary(ctx context.Context, steamID string) (*SteamPlayerSummary, error) {
	var resp SteamPlayerSummaryResponse
	q := url.Values{"key": {a.key}, "steamids": {steamID}}
	if err := a.get(ctx, "/ISteamUser/GetPlayerSummaries/v0002/", q, &resp); err != nil {
		return nil, err
	}
	if len(resp.Response.Players) == 0 {
		return nil, fmt.Errorf("steam returned no player summary for %s", steamID)
	}
	p := resp.Response.Players[0]
	return &p, nil
}

// OwnedGames returns the entire library in one call.
func (a *steamAPI) OwnedGames(ctx context.Context, steamID string) ([]SteamGame, error) {
	var resp SteamOwnedGamesResponse
	q := url.Values{
		"key":                       {a.key},
		"steamid":                   {steamID},
		"format":                    {"json"},
		"include_appinfo":           {"1"},
		"include_played_free_games": {"1"},
	}
	if err := a.get(ctx, "/IPlayerService/GetOwnedGames/v0001/", q, &resp); err != nil {
		return nil, err
	}
	return resp.Response.Games, nil
}

func (a *steamAPI) RecentGames(ctx context.Context, steamID string) ([]SteamGame, error) {
	var resp SteamResponse
	q := url.Values{
		"key":     {a.key},
		"steamid": {steamID},
		"format":  {"json"},
		"count":   {"100"},
	}
	if err := a.get(ctx, "/IPlayerService/GetRecentlyPlayedGames/v0001/", q, &resp); err != nil {
		return nil, err
	}
	return resp.Response.Games, nil
}

// PlayerAchievements returns the caller's achievement list for one app. A private profile or a game
// without stats yields ok=false rather than an error.
func (a *steamAPI) PlayerAchievements(ctx context.Context, steamID string, appID int) ([]steamPlayerAchievement, string, bool, error) {
	var resp steamPlayerAchievementsResponse
	q := url.Values{
		"key":     {a.key},
		"steamid": {steamID},
		"appid":   {fmt.Sprintf("%d", appID)},
		"l":       {"english"},
	}
	err := a.get(ctx, "/ISteamUserStats/GetPlayerAchievements/v0001/", q, &resp)
	if err != nil {
		// 400/403 here means "no stats for this game" or "profile is private" — not a failure.
		var he *steamHTTPError
		if ok := asSteamHTTPError(err, &he); ok && (he.Status == http.StatusBadRequest || he.Status == http.StatusForbidden) {
			return nil, "", false, nil
		}
		return nil, "", false, err
	}
	if !resp.PlayerStats.Success {
		return nil, "", false, nil
	}
	return resp.PlayerStats.Achievements, resp.PlayerStats.GameName, true, nil
}

func (a *steamAPI) GlobalAchievementPercentages(ctx context.Context, appID int) (map[string]float64, error) {
	var resp steamGlobalPercentResponse
	q := url.Values{"gameid": {fmt.Sprintf("%d", appID)}}
	if err := a.get(ctx, "/ISteamUserStats/GetGlobalAchievementPercentagesForApp/v0002/", q, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(resp.AchievementPercentages.Achievements))
	for _, ach := range resp.AchievementPercentages.Achievements {
		out[ach.Name] = float64(ach.Percent)
	}
	return out, nil
}

// SchemaForGame returns achievement display names and icon URLs keyed by API name.
func (a *steamAPI) SchemaForGame(ctx context.Context, appID int) (map[string]steamSchemaAchievement, error) {
	var resp steamSchemaResponse
	q := url.Values{
		"key":    {a.key},
		"appid":  {fmt.Sprintf("%d", appID)},
		"l":      {"english"},
		"format": {"json"},
	}
	if err := a.get(ctx, "/ISteamUserStats/GetSchemaForGame/v2/", q, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]steamSchemaAchievement, len(resp.Game.AvailableGameStats.Achievements))
	for _, ach := range resp.Game.AvailableGameStats.Achievements {
		out[ach.Name] = ach
	}
	return out, nil
}

// errSteamRateLimited signals the store API is throttling us, so the caller should pause.
var errSteamRateLimited = fmt.Errorf("steam store API rate limited")

// steamAppInfo is what one appdetails lookup gives us: what the app is, and where its real art
// lives. Newer titles keep their art under a content-hashed path that cannot be derived from the
// appid, so this is the only way to get a correct banner for them.
type steamAppInfo struct {
	Type   string
	Header string
}

// AppInfo resolves an app's type ("game", "dlc", "software", "tool", "demo", "music") and its
// header image. GetOwnedGames returns software and tools alongside games without distinguishing
// them, and does not expose art paths at all.
func (a *steamAPI) AppInfo(ctx context.Context, appID int) (steamAppInfo, error) {
	endpoint := fmt.Sprintf(
		"https://store.steampowered.com/api/appdetails?appids=%d&filters=basic&l=english", appID)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", endpoint, nil)
	if err != nil {
		return steamAppInfo{}, err
	}
	req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return steamAppInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return steamAppInfo{}, errSteamRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return steamAppInfo{}, &steamHTTPError{Status: resp.StatusCode, URL: "appdetails"}
	}

	// Shape: {"<appid>":{"success":true,"data":{"type":"game","header_image":"..."}}}
	var payload map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Type        string `json:"type"`
			HeaderImage string `json:"header_image"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return steamAppInfo{}, err
	}

	entry, ok := payload[strconv.Itoa(appID)]
	if !ok || !entry.Success {
		// Delisted or region-locked apps report success=false; treat them as unknown rather than
		// guessing, so they are not silently dropped from the library.
		return steamAppInfo{}, nil
	}

	header := strings.TrimSpace(entry.Data.HeaderImage)
	if i := strings.IndexByte(header, '?'); i >= 0 {
		header = header[:i] // drop the cache-busting timestamp
	}

	return steamAppInfo{
		Type:   strings.ToLower(strings.TrimSpace(entry.Data.Type)),
		Header: header,
	}, nil
}

// asSteamHTTPError is a tiny stand-in for errors.As that avoids importing errors just for one call.
func asSteamHTTPError(err error, target **steamHTTPError) bool {
	if he, ok := err.(*steamHTTPError); ok {
		*target = he
		return true
	}
	return false
}
