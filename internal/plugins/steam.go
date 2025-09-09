package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
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
	currentGame   *SteamCurrentGame
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
	if p.apiKey == "" {
		return p.renderNoAPI(), nil
	}

	if p.playerSummary == nil && len(p.recentGames) == 0 {
		return p.renderLoading(), nil
	}

	tmpl := `
	<div class="steam-section" id="steam-section">
		<h3>Gaming Activity</h3>
		
		{{if and .PlayerSummary .IsPlayingNow}}
		<div class="current-game">
			<div class="current-game-header">
				<span class="status-indicator status-online"></span>
				<span class="current-game-status">Currently Playing</span>
			</div>
			<div class="current-game-info">
				<div class="current-game-name">{{.CurrentGameName}}</div>
				<div class="current-game-actions">
					<a href="https://store.steampowered.com/search/?term={{.CurrentGameNameEncoded}}" target="_blank" rel="noopener" class="btn btn-sm">
						<svg viewBox="0 0 24 24" width="14" height="14">
							<path fill="currentColor" d="M14 3v2h3.59l-9.83 9.83 1.41 1.41L19 6.41V10h2V3m-2 16H5V5h7V3H5c-1.11 0-2 .89-2 2v14c0 1.11.89 2 2 2h14c1.11 0 2-.89 2-2v-7h-2v7Z"/>
						</svg>
						View on Steam
					</a>
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
			<h4>Recent Games</h4>
			<div class="games-list">
				{{range .RecentGames}}
				<div class="game-item" data-app-id="{{.AppID}}">
					{{if .Icon}}
					<img src="{{.Icon}}" alt="{{.Name}}" class="game-icon" loading="lazy">
					{{end}}
					<div class="game-info">
						<div class="game-name">{{.Name}}</div>
						<div class="game-stats">
							<span class="game-playtime">{{.RecentHours}}h last 2 weeks</span>
							<span class="game-total">{{.TotalHours}}h total</span>
						</div>
					</div>
					<div class="game-actions">
						<button class="btn btn-sm" onclick="window.open('https://store.steampowered.com/app/{{.AppID}}', '_blank')">
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
	</div>`

	type gameData struct {
		Name        string
		Icon        string
		RecentHours string
		TotalHours  string
		AppID       int
	}

	var games []gameData
	for i, game := range p.recentGames {
		if i >= 3 {
			break
		}

		var icon string
		if game.ImgIconURL != "" {
			icon = fmt.Sprintf("https://media.steampowered.com/steamcommunity/public/images/apps/%d/%s.jpg",
				game.AppID, game.ImgIconURL)
		}

		recentHours := fmt.Sprintf("%.1f", float64(game.Playtime2w)/60.0)
		totalHours := fmt.Sprintf("%.1f", float64(game.PlaytimeAll)/60.0)

		games = append(games, gameData{
			Name:        game.Name,
			Icon:        icon,
			RecentHours: recentHours,
			TotalHours:  totalHours,
			AppID:       game.AppID,
		})
	}

	isPlayingNow := p.playerSummary != nil && p.playerSummary.GameExtraInfo != ""
	currentGameName := ""
	currentGameNameEncoded := ""
	playerStatusClass := "status-offline"
	playerStatusText := "Offline"

	if p.playerSummary != nil {
		if isPlayingNow {
			currentGameName = p.playerSummary.GameExtraInfo
			currentGameNameEncoded = strings.ReplaceAll(currentGameName, " ", "%20")
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
		RecentGames            []gameData
		PlayerSummary          *SteamPlayerSummary
		IsPlayingNow           bool
		CurrentGameName        string
		CurrentGameNameEncoded string
		PlayerStatusClass      string
		PlayerStatusText       string
	}{
		RecentGames:            games,
		PlayerSummary:          p.playerSummary,
		IsPlayingNow:           isPlayingNow,
		CurrentGameName:        currentGameName,
		CurrentGameNameEncoded: currentGameNameEncoded,
		PlayerStatusClass:      playerStatusClass,
		PlayerStatusText:       playerStatusText,
	}

	tmplParsed, err := template.New("steam").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = tmplParsed.Execute(&buf, data)
	return buf.String(), err
}

func (p *SteamPlugin) renderNoAPI() string {
	return `<div class="steam-section">
		<h3>Recent Gaming Activity</h3>
		<p class="text-muted">Steam API key not configured</p>
	</div>`
}

func (p *SteamPlugin) renderLoading() string {
	return `<div class="steam-section">
		<h3>Recent Gaming Activity</h3>
		<div class="loading-indicator">
			<div class="loading"></div>
			<p class="text-muted">Loading Steam data...</p>
		</div>
	</div>`
}

func (p *SteamPlugin) UpdateData(ctx context.Context) error {
	if p.apiKey == "" || time.Since(p.lastUpdate) < 5*time.Minute {
		return nil
	}

	config := p.storage.GetPluginConfig(p.Name())
	steamID, ok := config.Settings["steamid"].(string)
	if !ok {
		return fmt.Errorf("steamid not configured")
	}

	if err := p.updatePlayerSummary(steamID); err != nil {
		fmt.Printf("Warning: Failed to update Steam player summary: %v\n", err)
	}

	if err := p.updateRecentGames(steamID); err != nil {
		fmt.Printf("Warning: Failed to update Steam recent games: %v\n", err)
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
		if p.playerSummary != nil {
			oldGameStatus = p.playerSummary.GameExtraInfo
		}

		p.playerSummary = &response.Response.Players[0]

		newGameStatus := p.playerSummary.GameExtraInfo
		if oldGameStatus != newGameStatus {
			p.hub.Broadcast("steam_status_update", map[string]interface{}{
				"isPlaying":    newGameStatus != "",
				"currentGame":  newGameStatus,
				"personaState": p.playerSummary.PersonaState,
				"personaName":  p.playerSummary.PersonaName,
				"timestamp":    time.Now().Unix(),
			})
		}
	}

	return nil
}

func (p *SteamPlugin) updateRecentGames(steamID string) error {
	url := fmt.Sprintf("http://api.steampowered.com/IPlayerService/GetRecentlyPlayedGames/v0001/?key=%s&steamid=%s&format=json&count=3",
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
		return fmt.Errorf("failed to fetch Steam data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Steam API returned status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return fmt.Errorf("Steam API returned non-JSON content: %s", contentType)
	}

	var response SteamResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("failed to decode Steam data: %v", err)
	}

	if len(response.Response.Games) > 0 {
		oldCount := len(p.recentGames)
		p.recentGames = response.Response.Games

		if oldCount != len(p.recentGames) {
			p.hub.Broadcast("steam_games_update", map[string]interface{}{
				"games": len(p.recentGames),
			})
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
