package plugins

import (
	"context"
	"fmt"
	"html/template"
	"net/url"
	"strings"
)

const steamSectionTemplate = `
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
							View on Steam
						</a>
						{{else}}
						<a href="https://store.steampowered.com/search/?term={{.CurrentGameNameEncoded}}" target="_blank" rel="noopener" class="btn btn-sm">
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

		<div class="steam-actions">
			{{if .HasLibrary}}
			<a href="/steam" class="steam-all-games-btn">View all games <span class="steam-count">{{.LibraryCount}}</span></a>
			{{end}}
			{{if .PlayerSummary}}
			<a href="{{.PlayerSummary.ProfileURL}}" target="_blank" rel="noopener" class="view-profile-btn">
				View Steam Profile
			</a>
			{{end}}
		</div>
	</div>
</section>`

type steamGameView struct {
	Name        string
	Icon        string
	RecentHours string
	TotalHours  string
	AppID       int
}

func steamGameViews(games []SteamGame, limit int) []steamGameView {
	out := make([]steamGameView, 0, limit)
	for i, game := range games {
		if i >= limit {
			break
		}
		out = append(out, steamGameView{
			Name:        game.Name,
			Icon:        steamIconImage(game.AppID, game.ImgIconURL),
			RecentHours: fmt.Sprintf("%.1f", float64(game.Playtime2w)/60.0),
			TotalHours:  fmt.Sprintf("%.1f", float64(game.PlaytimeAll)/60.0),
			AppID:       game.AppID,
		})
	}
	return out
}

func (p *SteamPlugin) Render(_ context.Context) (string, error) {
	settings := p.storage.GetPluginConfig(p.Name()).Settings

	if !p.getConfigBool(settings, "ui.showSteam", true) {
		return "", nil
	}
	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Gaming Activity")

	if p.apiKey == "" {
		return p.renderNoAPI(sectionTitle), nil
	}

	p.mu.RLock()
	summary := p.playerSummary
	recent := make([]SteamGame, len(p.recentGames))
	copy(recent, p.recentGames)
	top := make([]SteamGame, len(p.topGames))
	copy(top, p.topGames)
	libraryCount := len(p.games)
	p.mu.RUnlock()

	if summary == nil && len(recent) == 0 && len(top) == 0 {
		return p.renderLoading(sectionTitle), nil
	}

	isPlayingNow := summary != nil && summary.GameExtraInfo != ""
	currentGameName := ""
	currentGameNameEncoded := ""
	currentGameImage := ""
	currentGameStoreURL := ""
	playerStatusClass := "status-offline"
	playerStatusText := "Offline"

	if summary != nil {
		if isPlayingNow {
			currentGameName = summary.GameExtraInfo
			currentGameNameEncoded = url.QueryEscape(currentGameName)

			if summary.GameID != "" {
				currentGameStoreURL = fmt.Sprintf("https://store.steampowered.com/app/%s/", summary.GameID)
				currentGameImage = fmt.Sprintf(
					"https://cdn.cloudflare.steamstatic.com/steam/apps/%s/header.jpg", summary.GameID)
			}
		}

		switch summary.PersonaState {
		case 0:
			playerStatusClass, playerStatusText = "status-offline", "Offline"
		case 1:
			playerStatusClass, playerStatusText = "status-online", "Online"
		case 2:
			playerStatusClass, playerStatusText = "status-loading", "Busy"
		case 3:
			playerStatusClass, playerStatusText = "status-loading", "Away"
		case 4:
			playerStatusClass, playerStatusText = "status-loading", "Snooze"
		case 5:
			playerStatusClass, playerStatusText = "status-loading", "Looking to trade"
		case 6:
			playerStatusClass, playerStatusText = "status-loading", "Looking to play"
		}
	}

	data := struct {
		SectionTitle           string
		RecentGames            []steamGameView
		TopGames               []steamGameView
		PlayerSummary          *SteamPlayerSummary
		IsPlayingNow           bool
		CurrentGameName        string
		CurrentGameNameEncoded string
		CurrentGameImage       string
		PlayerStatusClass      string
		PlayerStatusText       string
		CurrentGameStoreURL    string
		HasLibrary             bool
		LibraryCount           int
	}{
		SectionTitle:           sectionTitle,
		RecentGames:            steamGameViews(recent, 3),
		TopGames:               steamGameViews(top, 9),
		PlayerSummary:          summary,
		IsPlayingNow:           isPlayingNow,
		CurrentGameName:        currentGameName,
		CurrentGameNameEncoded: currentGameNameEncoded,
		CurrentGameImage:       currentGameImage,
		PlayerStatusClass:      playerStatusClass,
		PlayerStatusText:       playerStatusText,
		CurrentGameStoreURL:    currentGameStoreURL,
		HasLibrary:             libraryCount > 0,
		LibraryCount:           libraryCount,
	}

	tmplParsed, err := template.New("steam").Parse(steamSectionTemplate)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := tmplParsed.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (p *SteamPlugin) renderNoAPI(sectionTitle string) string {
	return `<section class="steam-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">` + template.HTMLEscapeString(sectionTitle) + `</h3>
	</header>
	<div class="plugin__inner">
		<p class="text-muted">Steam API key not configured</p>
	</div>
</section>`
}

func (p *SteamPlugin) renderLoading(sectionTitle string) string {
	return `<section class="steam-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">` + template.HTMLEscapeString(sectionTitle) + `</h3>
	</header>
	<div class="plugin__inner">
		<div class="loading-indicator">
			<div class="loading"></div>
			<p class="text-muted">Loading Steam data...</p>
		</div>
	</div>
</section>`
}
