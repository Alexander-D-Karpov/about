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
	storage     *storage.Storage
	hub         *stream.Hub
	apiKey      string
	recentGames []SteamGame
	lastUpdate  time.Time
}

type SteamGame struct {
	Name        string `json:"name"`
	Playtime2w  int    `json:"playtime_2weeks"`
	PlaytimeAll int    `json:"playtime_forever"`
	AppID       int    `json:"appid"`
	ImgIconURL  string `json:"img_icon_url"`
}

type SteamResponse struct {
	Response struct {
		TotalCount int         `json:"total_count"`
		Games      []SteamGame `json:"games"`
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
	if len(p.recentGames) == 0 {
		return `<div class="steam-section">
			<h3>Recent Gaming Activity</h3>
			<p class="text-muted">No recent gaming activity</p>
		</div>`, nil
	}

	tmpl := `
	<div class="steam-section" id="steam-section">
		<h3>Recent Gaming Activity</h3>
		<div class="games-list">
			{{range .Games}}
			<div class="game-item">
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
			</div>
			{{end}}
		</div>
	</div>`

	type gameData struct {
		Name        string
		Icon        string
		RecentHours string
		TotalHours  string
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
		})
	}

	tmplParsed, err := template.New("steam").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = tmplParsed.Execute(&buf, struct{ Games []gameData }{Games: games})
	return buf.String(), err
}

func (p *SteamPlugin) UpdateData(ctx context.Context) error {
	if p.apiKey == "" || time.Since(p.lastUpdate) < 6*time.Hour {
		return nil
	}

	config := p.storage.GetPluginConfig(p.Name())
	steamID, ok := config.Settings["steamid"].(string)
	if !ok {
		return fmt.Errorf("steamid not configured")
	}

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
		p.lastUpdate = time.Now()

		if oldCount != len(p.recentGames) {
			p.hub.Broadcast("steam_update", map[string]interface{}{
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
