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
		<h2 class="plugin-title">{{.SectionTitle}}</h2>
		{{if and .PlayerSummary (not .IsPlayingNow)}}
		<div class="player-status" data-state="{{.PlayerStatusState}}">
			<span class="status-indicator {{.PlayerStatusClass}}"></span>
			<span class="status-text">{{.PlayerStatusText}}</span>
		</div>
		{{end}}
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
				<img src="{{.CurrentGameImage}}" data-fallback="{{.CurrentGameImageFallback}}" onerror="if(this.dataset.fallback){this.src=this.dataset.fallback;this.dataset.fallback=''}" alt="{{.CurrentGameName}}" class="game-cover-image" width="460" height="215" loading="lazy">
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
		{{end}}

		{{if .RecentGames}}
		<div class="recent-games">
			<h3 class="steam-sub">Recently played</h3>
			<div class="games-list">
				{{range .RecentGames}}
				<div class="game-item" data-app-id="{{.AppID}}">
					<a class="game-link" href="https://store.steampowered.com/app/{{.AppID}}/" target="_blank" rel="noopener" aria-label="{{.Name}} on Steam"></a>
					{{if .Icon}}
					<img src="{{.Icon}}" data-fallback="{{.FallbackIcon}}" onerror="if(this.dataset.fallback){this.src=this.dataset.fallback;this.dataset.fallback=''}" alt="{{.Name}}" class="game-icon" width="40" height="40" loading="lazy">
					{{end}}
					<div class="game-info">
						<div class="game-name">{{.Name}}</div>
						<div class="game-meta">
							<span><b>{{.RecentHours}}h</b> past 2 weeks</span><i aria-hidden="true">·</i><span><b>{{.TotalHours}}h</b> total</span>
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
		<div class="recent-games top-games">
			<h3 class="steam-sub">Most played</h3>
			<div class="games-list">
				{{range .TopGames}}
				<div class="game-item" data-app-id="{{.AppID}}">
					<a class="game-link" href="https://store.steampowered.com/app/{{.AppID}}/" target="_blank" rel="noopener" aria-label="{{.Name}} on Steam"></a>
					{{if .Icon}}
					<img src="{{.Icon}}" data-fallback="{{.FallbackIcon}}" onerror="if(this.dataset.fallback){this.src=this.dataset.fallback;this.dataset.fallback=''}" alt="{{.Name}}" class="game-icon" width="40" height="40" loading="lazy">
					{{end}}
					<div class="game-info">
						<div class="game-name">{{.Name}}</div>
						<div class="game-meta">
							<span><b>{{.TotalHours}}h</b> played</span>
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

		{{if or (gt (len .RecentGames) 2) (gt (len .TopGames) 4)}}
		<button type="button" class="steam-more-btn" aria-expanded="false"
		        onclick="var o=this.closest('.steam-section').classList.toggle('is-open');this.setAttribute('aria-expanded',o)">
			<span class="steam-more-open">Show all games</span><span class="steam-more-close">Show less</span>
			<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>
		</button>
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
	Name         string
	Icon         string
	FallbackIcon string
	RecentHours  string
	TotalHours   string
	AppID        int
}

func steamGameViews(games []SteamGame, limit int) []steamGameView {
	out := make([]steamGameView, 0, limit)
	for i, game := range games {
		if i >= limit {
			break
		}
		icon := steamIconImage(game.AppID, game.ImgIconURL)
		cover := icon
		if game.AppID > 0 {
			cover = fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%d/library_600x900.jpg", game.AppID)
		}
		out = append(out, steamGameView{
			Name:         game.Name,
			Icon:         cover,
			FallbackIcon: icon,
			RecentHours:  fmt.Sprintf("%.1f", float64(game.Playtime2w)/60.0),
			TotalHours:   fmt.Sprintf("%.1f", float64(game.PlaytimeAll)/60.0),
			AppID:        game.AppID,
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
	currentGameImageFallback := ""
	currentGameStoreURL := ""
	playerStatusClass := "status-offline"
	playerStatusText := "Offline"

	if summary != nil {
		if isPlayingNow {
			currentGameName = summary.GameExtraInfo
			currentGameNameEncoded = url.QueryEscape(currentGameName)

			if summary.GameID != "" {
				currentGameStoreURL = fmt.Sprintf("https://store.steampowered.com/app/%s/", summary.GameID)
				currentGameImage, currentGameImageFallback = steamCurrentGameImages(summary.GameID)
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
		SectionTitle             string
		RecentGames              []steamGameView
		TopGames                 []steamGameView
		PlayerSummary            *SteamPlayerSummary
		IsPlayingNow             bool
		CurrentGameName          string
		CurrentGameNameEncoded   string
		CurrentGameImage         string
		CurrentGameImageFallback string
		PlayerStatusClass        string
		PlayerStatusText         string
		PlayerStatusState        string
		CurrentGameStoreURL      string
		HasLibrary               bool
		LibraryCount             int
	}{
		SectionTitle:             sectionTitle,
		RecentGames:              steamGameViews(recent, 6),
		TopGames:                 steamGameViews(top, 9),
		PlayerSummary:            summary,
		IsPlayingNow:             isPlayingNow,
		CurrentGameName:          currentGameName,
		CurrentGameNameEncoded:   currentGameNameEncoded,
		CurrentGameImage:         currentGameImage,
		CurrentGameImageFallback: currentGameImageFallback,
		PlayerStatusClass:        playerStatusClass,
		PlayerStatusText:         playerStatusText,
		PlayerStatusState:        steamStatusState(playerStatusText),
		CurrentGameStoreURL:      currentGameStoreURL,
		HasLibrary:               libraryCount > 0,
		LibraryCount:             libraryCount,
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

// steamStatusState buckets a persona state into the colour the status pill
// uses: online, idle (away, snooze), busy, or offline.
func steamStatusState(text string) string {
	switch text {
	case "Online", "Looking to play", "Looking to trade":
		return "online"
	case "Away", "Snooze":
		return "idle"
	case "Busy":
		return "busy"
	default:
		return "offline"
	}
}

func (p *SteamPlugin) renderNoAPI(sectionTitle string) string {
	return `<section class="steam-section section plugin" data-w="2">
	<header class="plugin-header">
		<h2 class="plugin-title">` + template.HTMLEscapeString(sectionTitle) + `</h2>
	</header>
	<div class="plugin__inner">
		<p class="text-muted">Steam API key not configured</p>
	</div>
</section>`
}

func (p *SteamPlugin) renderLoading(sectionTitle string) string {
	return `<section class="steam-section section plugin" data-w="2">
	<header class="plugin-header">
		<h2 class="plugin-title">` + template.HTMLEscapeString(sectionTitle) + `</h2>
	</header>
	<div class="plugin__inner">
		<div class="loading-indicator">
			<div class="loading"></div>
			<p class="text-muted">Loading Steam data...</p>
		</div>
	</div>
</section>`
}
