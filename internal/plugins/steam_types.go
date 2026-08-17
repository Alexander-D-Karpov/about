package plugins

import (
	"strconv"
	"strings"
)

// SteamGame is one owned or family-shared game. The json tags do double duty: they decode the
// Steam Web API responses and they serialise into the side-car library store.
type SteamGame struct {
	Name        string `json:"name"`
	Playtime2w  int    `json:"playtime_2weeks"`
	PlaytimeAll int    `json:"playtime_forever"`
	AppID       int    `json:"appid"`
	ImgIconURL  string `json:"img_icon_url"`
	HasStats    bool   `json:"has_community_visible_stats"`
	LastPlayed  int64  `json:"rtime_last_played"`

	// AcquiredAt is only ever populated by the family shared-library API; owned games fall back to
	// the store's first-seen map.
	AcquiredAt int64 `json:"rt_time_acquired,omitempty"`
	// Source is "" for owned games and "family" for shared-library games.
	Source string `json:"source,omitempty"`
	// Type is the resolved store type ("game", "software", ...); empty until looked up.
	Type string `json:"app_type,omitempty"`
}

type SteamCurrentGame struct {
	GameID            string `json:"gameid"`
	GameExtraInfo     string `json:"gameextrainfo"`
	GameServerIP      string `json:"gameserverip"`
	GameServerSteamID string `json:"gameserversteamid"`
}

type SteamPlayerSummary struct {
	SteamID                  string `json:"steamid"`
	CommunityVisibilityState int    `json:"communityvisibilitystate"`
	ProfileState             int    `json:"profilestate"`
	PersonaName              string `json:"personaname"`
	ProfileURL               string `json:"profileurl"`
	Avatar                   string `json:"avatar"`
	AvatarMedium             string `json:"avatarmedium"`
	AvatarFull               string `json:"avatarfull"`
	AvatarHash               string `json:"avatarhash"`
	LastLogoff               int64  `json:"lastlogoff"`
	PersonaState             int    `json:"personastate"`
	RealName                 string `json:"realname"`
	PrimaryClanID            string `json:"primaryclanid"`
	TimeCreated              int64  `json:"timecreated"`
	PersonaStateFlags        int    `json:"personastateflags"`
	GameID                   string `json:"gameid,omitempty"`
	GameExtraInfo            string `json:"gameextrainfo,omitempty"`
	GameServerIP             string `json:"gameserverip,omitempty"`
	GameServerSteamID        string `json:"gameserversteamid,omitempty"`
}

type SteamResponse struct {
	Response struct {
		TotalCount int         `json:"total_count"`
		Games      []SteamGame `json:"games"`
	} `json:"response"`
}

type SteamOwnedGamesResponse struct {
	Response struct {
		GameCount int         `json:"game_count"`
		Games     []SteamGame `json:"games"`
	} `json:"response"`
}

type SteamPlayerSummaryResponse struct {
	Response struct {
		Players []SteamPlayerSummary `json:"players"`
	} `json:"response"`
}

// --- Family shared library ---

type steamFamilyGroupResponse struct {
	Response struct {
		FamilyGroupID         string `json:"family_groupid"`
		IsNotMemberOfAnyGroup bool   `json:"is_not_member_of_any_group"`
	} `json:"response"`
}

type steamSharedLibraryApp struct {
	AppID          int      `json:"appid"`
	Name           string   `json:"name"`
	OwnerSteamIDs  []string `json:"owner_steamids"`
	RtTimeAcquired int64    `json:"rt_time_acquired"`
	RtLastPlayed   int64    `json:"rt_last_played"`
	ExcludeReason  int      `json:"exclude_reason"`
}

type steamSharedLibraryResponse struct {
	Response struct {
		Apps []steamSharedLibraryApp `json:"apps"`
	} `json:"response"`
}

// --- Achievements ---

type steamPlayerAchievement struct {
	APIName    string `json:"apiname"`
	Achieved   int    `json:"achieved"`
	UnlockTime int64  `json:"unlocktime"`
	Name       string `json:"name"`
	Desc       string `json:"description"`
}

type steamPlayerAchievementsResponse struct {
	PlayerStats struct {
		SteamID      string                   `json:"steamID"`
		GameName     string                   `json:"gameName"`
		Achievements []steamPlayerAchievement `json:"achievements"`
		Success      bool                     `json:"success"`
		Error        string                   `json:"error"`
	} `json:"playerstats"`
}

// steamFlexFloat accepts both 64.4 and "64.4". Steam's global-percentage endpoint returns the
// value as a JSON string, while other endpoints use plain numbers.
type steamFlexFloat float64

func (f *steamFlexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = steamFlexFloat(v)
	return nil
}

type steamGlobalPercentResponse struct {
	AchievementPercentages struct {
		Achievements []struct {
			Name    string         `json:"name"`
			Percent steamFlexFloat `json:"percent"`
		} `json:"achievements"`
	} `json:"achievementpercentages"`
}

type steamSchemaAchievement struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	IconGray    string `json:"icongray"`
	Hidden      int    `json:"hidden"`
}

type steamSchemaResponse struct {
	Game struct {
		GameName           string `json:"gameName"`
		AvailableGameStats struct {
			Achievements []steamSchemaAchievement `json:"achievements"`
		} `json:"availableGameStats"`
	} `json:"game"`
}

// SteamRarestAchievement is a single unlocked achievement with its global rarity, ready to render.
type SteamRarestAchievement struct {
	AppID         int     `json:"appid"`
	GameName      string  `json:"gameName"`
	APIName       string  `json:"apiname"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Icon          string  `json:"icon"`
	IconGray      string  `json:"iconGray,omitempty"`
	GlobalPercent float64 `json:"globalPercent"`
	UnlockedAt    int64   `json:"unlockedAt"`
}

// steamAchEntry is the cached achievement state for one game.
type steamAchEntry struct {
	Available bool                     `json:"available"`
	Achieved  int                      `json:"achieved"`
	Total     int                      `json:"total"`
	Percent   float64                  `json:"percent"`
	Rarest    []SteamRarestAchievement `json:"rarest,omitempty"`
	FetchedAt int64                    `json:"fetched_at"`
}
