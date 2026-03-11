package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type SteamPlugin struct {
	storage       *storage.Storage
	hub           *stream.Hub
	apiKey        string
	recentGames   []SteamGame
	topGames      []SteamGame
	playerSummary *SteamPlayerSummary
	lastUpdate    time.Time
}

type SteamGame struct {
	Name        string `json:"name"`
	Playtime2w  int    `json:"playtime_2weeks"`
	PlaytimeAll int    `json:"playtime_forever"`
	AppID       int    `json:"appid"`
	ImgIconURL  string `json:"img_icon_url"`
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

func NewSteamPlugin(storage *storage.Storage, hub *stream.Hub, apiKey string) *SteamPlugin {
	return &SteamPlugin{
		storage: storage,
		hub:     hub,
		apiKey:  apiKey,
	}
}

func (p *SteamPlugin) Name() string {
	return "steam"
}

func (p *SteamPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	// Windows-style toggles
	showSteam := p.getConfigBool(settings, "ui.showSteam", true)
	if !showSteam {
		return "", nil
	}
	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Gaming Activity")

	if p.apiKey == "" {
		return p.renderNoAPI(sectionTitle), nil
	}

	if p.playerSummary == nil && len(p.recentGames) == 0 && len(p.topGames) == 0 {
		return p.renderLoading(sectionTitle), nil
	}

	tmpl := `
<section class="steam-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
	</header>

	<div class="plugin__inner">
		{{if and .PlayerSummary .IsPlayingNow}}
		<div class="current-game">
			<div class="current-game-header">
				<span class="status-indicator status-online"></span>
				<span class="current-game-status">Currently Playing</span>
			</div>
			{{if .CurrentGameImage}}
			<div class="current-game-cover">
				<img src="{{.CurrentGameImage}}" alt="{{.CurrentGameName}}" class="game-cover-image" loading="lazy">
			</div>
			{{end}}
			<div class="current-game-info">
				<div class="current-game-name">{{.CurrentGameName}}</div>
					<div class="current-game-actions">
						{{if .CurrentGameStoreURL}}
						<a href="{{.CurrentGameStoreURL}}" target="_blank" rel="noopener" class="btn btn-sm">
							<!-- icon -->
							View on Steam
						</a>
						{{else}}
						<a href="https://store.steampowered.com/search/?term={{.CurrentGameNameEncoded}}" target="_blank" rel="noopener" class="btn btn-sm">
							<!-- icon -->
							Search on Steam
						</a>
						{{end}}
					</div>
			</div>
		</div>
		{{else if .PlayerSummary}}
		<div class="player-status">
			<div class="status-info">
				<span class="status-indicator {{.PlayerStatusClass}}"></span>
				<span class="status-text">{{.PlayerStatusText}}</span>
			</div>
		</div>
		{{end}}

		{{if .RecentGames}}
		<div class="recent-games">
			<h4>Recently Played Games</h4>
			<div class="games-list">
				{{range .RecentGames}}
				<div class="game-item" data-app-id="{{.AppID}}">
					{{if .Icon}}
					<img src="{{.Icon}}" alt="{{.Name}}" class="game-icon" loading="lazy">
					{{end}}
					<div class="game-info">
						<div class="game-name">{{.Name}}</div>
						<div class="game-stats">
							<a class="steam-stat-link" href="https://store.steampowered.com/app/{{.AppID}}/" target="_blank" rel="noopener">
								<span class="game-playtime">{{.RecentHours}}h last 2 weeks</span>
							</a>
							<a class="steam-stat-link" href="https://store.steampowered.com/app/{{.AppID}}/" target="_blank" rel="noopener">
								<span class="game-total">{{.TotalHours}}h total</span>
							</a>
						</div>
					</div>
					<div class="game-actions">
						<button class="btn btn-sm" onclick="window.open('https://store.steampowered.com/app/{{.AppID}}/', '_blank', 'noopener')">
							View
						</button>
					</div>
				</div>
				{{end}}
			</div>
		</div>
		{{end}}

		{{if .TopGames}}
		<div class="recent-games">
			<h4>Top Games by Playtime</h4>
			<div class="games-list">
				{{range .TopGames}}
				<div class="game-item" data-app-id="{{.AppID}}">
					{{if .Icon}}
					<img src="{{.Icon}}" alt="{{.Name}}" class="game-icon" loading="lazy">
					{{end}}
					<div class="game-info">
						<div class="game-name">{{.Name}}</div>
						<div class="game-stats">
							<a class="steam-stat-link" href="https://store.steampowered.com/app/{{.AppID}}/" target="_blank" rel="noopener">
								<span class="game-total">{{.TotalHours}}h total</span>
							</a>
						</div>
					</div>
					<div class="game-actions">
						<button class="btn btn-sm" onclick="window.open('https://store.steampowered.com/app/{{.AppID}}/', '_blank', 'noopener')">
							View
						</button>
					</div>
				</div>
				{{end}}
			</div>
		</div>
		{{end}}

		{{if .PlayerSummary}}
		<div class="steam-profile-link">
			<a href="{{.PlayerSummary.ProfileURL}}" target="_blank" rel="noopener" class="view-profile-btn">
				View Steam Profile
			</a>
		</div>
		{{end}}
	</div>
</section>`

	type gameData struct {
		Name           string
		Icon           string
		RecentHours    string
		RecentHoursNum float64
		TotalHours     string
		AppID          int
	}

	// Top games
	var topGames []gameData
	for i, game := range p.topGames {
		if i >= 9 {
			break
		}

		var icon string
		if game.ImgIconURL != "" {
			icon = fmt.Sprintf(
				"https://media.steampowered.com/steamcommunity/public/images/apps/%d/%s.jpg",
				game.AppID, game.ImgIconURL,
			)
		}

		recentHoursNum := float64(game.Playtime2w) / 60.0
		recentHours := fmt.Sprintf("%.1f", recentHoursNum)
		totalHours := fmt.Sprintf("%.1f", float64(game.PlaytimeAll)/60.0)

		topGames = append(topGames, gameData{
			Name:           game.Name,
			Icon:           icon,
			RecentHours:    recentHours,
			RecentHoursNum: recentHoursNum,
			TotalHours:     totalHours,
			AppID:          game.AppID,
		})
	}

	// Recent games
	var recentGames []gameData
	for i, game := range p.recentGames {
		if i >= 3 {
			break
		}

		var icon string
		if game.ImgIconURL != "" {
			icon = fmt.Sprintf(
				"https://media.steampowered.com/steamcommunity/public/images/apps/%d/%s.jpg",
				game.AppID, game.ImgIconURL,
			)
		}

		recentHoursNum := float64(game.Playtime2w) / 60.0
		recentHours := fmt.Sprintf("%.1f", recentHoursNum)
		totalHours := fmt.Sprintf("%.1f", float64(game.PlaytimeAll)/60.0)

		recentGames = append(recentGames, gameData{
			Name:           game.Name,
			Icon:           icon,
			RecentHours:    recentHours,
			RecentHoursNum: recentHoursNum,
			TotalHours:     totalHours,
			AppID:          game.AppID,
		})
	}

	isPlayingNow := p.playerSummary != nil && p.playerSummary.GameExtraInfo != ""
	currentGameName := ""
	currentGameNameEncoded := ""
	currentGameImage := ""
	currentGameID := ""
	currentGameStoreURL := ""
	currentGameRunURL := "" // optional
	playerStatusClass := "status-offline"
	playerStatusText := "Offline"

	if p.playerSummary != nil {
		if isPlayingNow {
			currentGameName = p.playerSummary.GameExtraInfo
			currentGameNameEncoded = url.QueryEscape(currentGameName)

			currentGameID = p.playerSummary.GameID
			if currentGameID != "" {
				currentGameStoreURL = fmt.Sprintf("https://store.steampowered.com/app/%s/", currentGameID)
				currentGameRunURL = fmt.Sprintf("steam://run/%s", currentGameID) // optional

				currentGameImage = fmt.Sprintf(
					"https://cdn.cloudflare.steamstatic.com/steam/apps/%s/header.jpg",
					currentGameID,
				)
			}
		}

		switch p.playerSummary.PersonaState {
		case 0:
			playerStatusClass = "status-offline"
			playerStatusText = "Offline"
		case 1:
			playerStatusClass = "status-online"
			playerStatusText = "Online"
		case 2:
			playerStatusClass = "status-loading"
			playerStatusText = "Busy"
		case 3:
			playerStatusClass = "status-loading"
			playerStatusText = "Away"
		case 4:
			playerStatusClass = "status-loading"
			playerStatusText = "Snooze"
		case 5:
			playerStatusClass = "status-loading"
			playerStatusText = "Looking to trade"
		case 6:
			playerStatusClass = "status-loading"
			playerStatusText = "Looking to play"
		}
	}

	data := struct {
		SectionTitle           string
		RecentGames            []gameData
		TopGames               []gameData
		PlayerSummary          *SteamPlayerSummary
		IsPlayingNow           bool
		CurrentGameName        string
		CurrentGameNameEncoded string
		CurrentGameImage       string
		CurrentGameID          string
		PlayerStatusClass      string
		PlayerStatusText       string
		CurrentGameStoreURL    string
		CurrentGameRunURL      string // optional
	}{
		SectionTitle:           sectionTitle,
		RecentGames:            recentGames,
		TopGames:               topGames,
		PlayerSummary:          p.playerSummary,
		IsPlayingNow:           isPlayingNow,
		CurrentGameName:        currentGameName,
		CurrentGameNameEncoded: currentGameNameEncoded,
		CurrentGameImage:       currentGameImage,
		CurrentGameID:          currentGameID,
		PlayerStatusClass:      playerStatusClass,
		PlayerStatusText:       playerStatusText,
		CurrentGameStoreURL:    currentGameStoreURL,
		CurrentGameRunURL:      currentGameRunURL, // optional

	}

	tmplParsed, err := template.New("steam").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = tmplParsed.Execute(&buf, data)
	return buf.String(), err
}

func (p *SteamPlugin) renderNoAPI(sectionTitle string) string {
	return `<section class="steam-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">` + sectionTitle + `</h3>
	</header>
	<div class="plugin__inner">
		<p class="text-muted">Steam API key not configured</p>
	</div>
</section>`
}

func (p *SteamPlugin) renderLoading(sectionTitle string) string {
	return `<section class="steam-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">` + sectionTitle + `</h3>
	</header>
	<div class="plugin__inner">
		<div class="loading-indicator">
			<div class="loading"></div>
			<p class="text-muted">Loading Steam data...</p>
		</div>
	</div>
</section>`
}

func (p *SteamPlugin) UpdateData(ctx context.Context) error {
	if p.apiKey == "" || time.Since(p.lastUpdate) < 4*time.Minute {
		return nil
	}

	config := p.storage.GetPluginConfig(p.Name())
	steamID, ok := config.Settings["steamid"].(string)
	if !ok || steamID == "" {
		return fmt.Errorf("steamid not configured")
	}

	if err := p.updatePlayerSummary(steamID); err != nil {
		fmt.Printf("Warning: Failed to update Steam player summary: %v\n", err)
	}

	if err := p.updateRecentGames(steamID); err != nil {
		fmt.Printf("Warning: Failed to update Steam recent games: %v\n", err)
	}

	if err := p.updateTopGames(steamID); err != nil {
		fmt.Printf("Warning: Failed to update Steam top games: %v\n", err)
	}

	p.lastUpdate = time.Now()
	return nil
}

func (p *SteamPlugin) updatePlayerSummary(steamID string) error {
	url := fmt.Sprintf("http://api.steampowered.com/ISteamUser/GetPlayerSummaries/v0002/?key=%s&steamids=%s",
		p.apiKey, steamID)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", "AboutPage/1.0 (aboutpage.akarpov.ru)")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch Steam player summary: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Steam API returned status %d", resp.StatusCode)
	}

	var response SteamPlayerSummaryResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("failed to decode Steam player summary: %v", err)
	}

	if len(response.Response.Players) > 0 {
		oldGameStatus := ""
		oldPersonaState := 0
		if p.playerSummary != nil {
			oldGameStatus = p.playerSummary.GameExtraInfo
			oldPersonaState = p.playerSummary.PersonaState
		}

		p.playerSummary = &response.Response.Players[0]

		newGameStatus := p.playerSummary.GameExtraInfo
		newPersonaState := p.playerSummary.PersonaState

		if oldGameStatus != newGameStatus || oldPersonaState != newPersonaState {
			gameImage := ""
			if p.playerSummary.GameID != "" {
				gameImage = fmt.Sprintf(
					"https://cdn.cloudflare.steamstatic.com/steam/apps/%s/header.jpg",
					p.playerSummary.GameID,
				)
			}

			p.hub.Broadcast("steam_status_update", map[string]interface{}{
				"isPlaying":    newGameStatus != "",
				"currentGame":  newGameStatus,
				"gameImage":    gameImage,
				"gameId":       p.playerSummary.GameID,
				"personaState": newPersonaState,
				"personaName":  p.playerSummary.PersonaName,
				"timestamp":    time.Now().Unix(),
			})
		}
	}

	return nil
}

func (p *SteamPlugin) updateRecentGames(steamID string) error {
	url := fmt.Sprintf(
		"http://api.steampowered.com/IPlayerService/GetRecentlyPlayedGames/v0001/?key=%s&steamid=%s&format=json&count=100",
		p.apiKey, steamID,
	)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", "AboutPage/1.0 (aboutpage.akarpov.ru)")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch Steam data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("steam API returned status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return fmt.Errorf("steam API returned non-JSON content: %s", contentType)
	}

	var response SteamResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("failed to decode Steam data: %v", err)
	}

	games := response.Response.Games

	sort.Slice(games, func(i, j int) bool {
		return games[i].Playtime2w > games[j].Playtime2w
	})

	if len(games) > 10 {
		games = games[:10]
	}

	oldCount := len(p.recentGames)
	p.recentGames = games

	if oldCount != len(p.recentGames) {
		p.hub.Broadcast("steam_games_update", map[string]interface{}{
			"games": len(p.recentGames),
		})
	}

	return nil
}

func (p *SteamPlugin) updateTopGames(steamID string) error {
	url := fmt.Sprintf(
		"http://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/?key=%s&steamid=%s&format=json&include_appinfo=1&include_played_free_games=1",
		p.apiKey, steamID,
	)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("User-Agent", "AboutPage/1.0 (aboutpage.akarpov.ru)")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch Steam data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("steam API returned status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return fmt.Errorf("steam API returned non-JSON content: %s", contentType)
	}

	var response SteamOwnedGamesResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("failed to decode Steam data: %v", err)
	}

	games := response.Response.Games

	sort.Slice(games, func(i, j int) bool {
		return games[i].PlaytimeAll > games[j].PlaytimeAll
	})

	if len(games) > 10 {
		games = games[:10]
	}

	if err := p.updateRecentPlaytime(steamID, games); err != nil {
		fmt.Printf("Warning: Failed to update recent playtime: %v\n", err)
	}

	p.topGames = games
	return nil
}

func (p *SteamPlugin) updateRecentPlaytime(steamID string, games []SteamGame) error {
	url := fmt.Sprintf(
		"http://api.steampowered.com/IPlayerService/GetRecentlyPlayedGames/v0001/?key=%s&steamid=%s&format=json&count=100",
		p.apiKey, steamID,
	)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "AboutPage/1.0 (aboutpage.akarpov.ru)")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("steam API returned status %d", resp.StatusCode)
	}

	var response SteamResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}

	recentMap := make(map[int]int)
	for _, game := range response.Response.Games {
		recentMap[game.AppID] = game.Playtime2w
	}

	for i := range games {
		if recent, ok := recentMap[games[i].AppID]; ok {
			games[i].Playtime2w = recent
		}
	}

	return nil
}

func (p *SteamPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *SteamPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings
	return p.storage.SetPluginConfig(p.Name(), config)
}

func (p *SteamPlugin) RenderText(ctx context.Context) (string, error) {
	if p.apiKey == "" {
		return "Gaming: Steam API key not configured", nil
	}

	if p.playerSummary == nil {
		return "Gaming: No Steam data available", nil
	}

	status := "Offline"
	currentGame := ""

	if p.playerSummary.GameExtraInfo != "" {
		status = "Playing"
		currentGame = fmt.Sprintf(" - %s", p.playerSummary.GameExtraInfo)
	} else {
		switch p.playerSummary.PersonaState {
		case 1:
			status = "Online"
		case 2:
			status = "Busy"
		case 3:
			status = "Away"
		}
	}

	recentGamesCount := len(p.recentGames)
	gamesInfo := ""
	if recentGamesCount > 0 {
		gamesInfo = fmt.Sprintf(", %d recent games", recentGamesCount)
	}

	return fmt.Sprintf("Gaming: %s%s%s", status, currentGame, gamesInfo), nil
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

func (p *SteamPlugin) GetMetrics() map[string]interface{} {
	metrics := map[string]interface{}{
		"is_online":            0,
		"is_playing":           0,
		"recent_games_count":   len(p.recentGames),
		"total_playtime_hours": 0.0,
	}

	if p.playerSummary != nil {
		if p.playerSummary.PersonaState == 1 {
			metrics["is_online"] = 1
		}
		if p.playerSummary.GameExtraInfo != "" {
			metrics["is_playing"] = 1
		}
	}

	var totalPlaytime int
	for _, g := range p.topGames {
		totalPlaytime += g.PlaytimeAll
	}
	metrics["total_playtime_hours"] = float64(totalPlaytime) / 60.0

	return metrics
}
