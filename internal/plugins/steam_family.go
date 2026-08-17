package plugins

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// errSteamTokenInvalid signals an absent/expired access_token so callers can degrade to
// API-key-only mode instead of treating it as a hard failure.
var errSteamTokenInvalid = fmt.Errorf("steam access_token missing or expired")

func steamIsTokenError(err error) bool {
	if err == nil {
		return false
	}
	if err == errSteamTokenInvalid {
		return true
	}
	var he *steamHTTPError
	if asSteamHTTPError(err, &he) {
		return he.Status == http.StatusUnauthorized || he.Status == http.StatusForbidden
	}
	return false
}

// familyGroupID resolves the family group the user belongs to.
func (a *steamAPI) familyGroupID(ctx context.Context, token, steamID string) (string, error) {
	if strings.TrimSpace(token) == "" {
		return "", errSteamTokenInvalid
	}
	var resp steamFamilyGroupResponse
	q := url.Values{"access_token": {token}, "steamid": {steamID}}
	if err := a.get(ctx, "/IFamilyGroupsService/GetFamilyGroupForUser/v1/", q, &resp); err != nil {
		return "", err
	}
	if resp.Response.IsNotMemberOfAnyGroup || resp.Response.FamilyGroupID == "" {
		return "", nil
	}
	return resp.Response.FamilyGroupID, nil
}

// sharedLibraryApps lists the apps shared with the user by their family group.
func (a *steamAPI) sharedLibraryApps(ctx context.Context, token, groupID, steamID string) ([]steamSharedLibraryApp, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errSteamTokenInvalid
	}
	var resp steamSharedLibraryResponse
	// include_own=true matters beyond family sharing: this is the only endpoint that reports
	// rt_time_acquired, so it is where real "date added" values for our own games come from.
	q := url.Values{
		"access_token":     {token},
		"family_groupid":   {groupID},
		"steamid":          {steamID},
		"include_own":      {"true"},
		"include_free":     {"true"},
		"include_excluded": {"false"},
	}
	if err := a.get(ctx, "/IFamilyGroupsService/GetSharedLibraryApps/v1/", q, &resp); err != nil {
		return nil, err
	}
	return resp.Response.Apps, nil
}

// fetchFamilyGames returns family-shared games as SteamGame values. A missing or expired token
// returns errSteamTokenInvalid so the caller can fall back to the API key alone.
func (a *steamAPI) fetchFamilyGames(ctx context.Context, token, steamID string) ([]SteamGame, string, error) {
	groupID, err := a.familyGroupID(ctx, token, steamID)
	if err != nil {
		if steamIsTokenError(err) {
			return nil, "", errSteamTokenInvalid
		}
		return nil, "", err
	}
	if groupID == "" {
		return nil, "", nil
	}

	apps, err := a.sharedLibraryApps(ctx, token, groupID, steamID)
	if err != nil {
		if steamIsTokenError(err) {
			return nil, groupID, errSteamTokenInvalid
		}
		return nil, groupID, err
	}

	games := make([]SteamGame, 0, len(apps))
	for _, app := range apps {
		if app.AppID == 0 || app.ExcludeReason != 0 {
			continue
		}
		// Apps we own ourselves come back too (include_own=true); they are not "family shared",
		// we only want their acquisition date.
		source := "family"
		for _, owner := range app.OwnerSteamIDs {
			if owner == steamID {
				source = ""
				break
			}
		}
		games = append(games, SteamGame{
			AppID:      app.AppID,
			Name:       app.Name,
			LastPlayed: app.RtLastPlayed,
			AcquiredAt: app.RtTimeAcquired,
			Source:     source,
		})
	}
	return games, groupID, nil
}
