package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type LastFMPlugin struct {
	storage             *storage.Storage
	hub                 *stream.Hub
	apiKey              string
	currentTrack        *LastFMTrack
	recentTracks        []LastFMTrack
	userInfo            *LastFMUser
	currentSong         *AkarpovrMusicTrack
	lastUpdateTime      time.Time
	lastWebsocketUpdate time.Time
	httpClient          *http.Client
}

type LastFMResponse struct {
	RecentTracks struct {
		Track []LastFMTrack `json:"track"`
		Attr  struct {
			Page       string `json:"page"`
			PerPage    string `json:"perPage"`
			User       string `json:"user"`
			Total      string `json:"total"`
			TotalPages string `json:"totalPages"`
		} `json:"@attr"`
	} `json:"recenttracks"`
}

type LastFMUserResponse struct {
	User LastFMUser `json:"user"`
}

type LastFMUser struct {
	Name       string `json:"name"`
	PlayCount  string `json:"playcount"`
	Registered struct {
		UnixTime string `json:"unixtime"`
	} `json:"registered"`
}

type LastFMTrack struct {
	Name   string `json:"name"`
	Artist struct {
		Text string `json:"#text"`
	} `json:"artist"`
	Album struct {
		Text string `json:"#text"`
	} `json:"album"`
	Image []struct {
		Text string `json:"#text"`
		Size string `json:"size"`
	} `json:"image"`
	Attr struct {
		NowPlaying string `json:"nowplaying"`
	} `json:"@attr"`
	Date struct {
		Uts string `json:"uts"`
	} `json:"date"`
	URL string `json:"url"`
}

type lastfmTrackInfoResp struct {
	Track struct {
		Album struct {
			Title string `json:"title"`
			Image []struct {
				Text string `json:"#text"`
				Size string `json:"size"`
			} `json:"image"`
		} `json:"album"`
	} `json:"track"`
}

type lastfmAlbumInfoResp struct {
	Album struct {
		Image []struct {
			Text string `json:"#text"`
			Size string `json:"size"`
		} `json:"image"`
	} `json:"album"`
}

type AkarpovrMusicResponse struct {
	Count   int                  `json:"count"`
	Results []AkarpovrMusicTrack `json:"results"`
}

type AkarpovrMusicTrack struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	File         string `json:"file"`
	ImageCropped string `json:"image_cropped"`
	Length       int    `json:"length"`
	Album        struct {
		Name         string `json:"name"`
		ImageCropped string `json:"image_cropped"`
	} `json:"album"`
	Authors []struct {
		Name string `json:"name"`
	} `json:"authors"`
}

func NewLastFMPlugin(storage *storage.Storage, hub *stream.Hub, apiKey string) *LastFMPlugin {
	plugin := &LastFMPlugin{
		storage: storage,
		hub:     hub,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	go plugin.startWebSocketUpdates()

	return plugin
}

func (p *LastFMPlugin) Name() string { return "lastfm" }

func (p *LastFMPlugin) startWebSocketUpdates() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if p.hub.GetClientCount() > 0 && p.apiKey != "" {
			if time.Since(p.lastWebsocketUpdate) >= 15*time.Second {
				config := p.storage.GetPluginConfig(p.Name())
				username, ok := config.Settings["username"].(string)
				if ok && strings.TrimSpace(username) != "" {
					if err := p.updateRecentTracksWebSocket(username); err != nil {
						log.Printf("WebSocket LastFM update failed: %v", err)
					} else {
						p.lastWebsocketUpdate = time.Now()
					}
				}
			}
		}
	}
}

func (p *LastFMPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "🎵 Music")
	showScrobbles := p.getConfigBool(settings, "ui.showScrobbles", true)
	showPlayButton := p.getConfigBool(settings, "ui.showPlayButton", true)
	showRecentTracks := p.getConfigBool(settings, "ui.showRecentTracks", true)

	if p.currentTrack == nil {
		return p.renderNoTrack(sectionTitle), nil
	}

	image := p.pickBestImage(p.currentTrack)
	if image == "" && p.currentSong != nil && p.currentSong.ImageCropped != "" {
		image = p.currentSong.ImageCropped
	}

	nowPlaying := p.currentTrack.Attr.NowPlaying == "true"
	statusText := "Last played " + p.relativePlayedAt()
	statusClass := ""
	if nowPlaying {
		statusText = "Now Playing"
		statusClass = "status-online"
	}

	scrobblesText := ""
	if showScrobbles && p.userInfo != nil && p.userInfo.PlayCount != "" {
		scrobblesText = fmt.Sprintf("Total scrobbles: %s", formatScrobbles(p.userInfo.PlayCount))
	}

	searchQuery := fmt.Sprintf("%s %s", p.currentTrack.Artist.Text, p.currentTrack.Name)

	var recentTracksToShow []LastFMTrack
	if showRecentTracks && len(p.recentTracks) > 0 {
		startIdx := 0
		if nowPlaying && len(p.recentTracks) > 1 {
			startIdx = 1
		}
		endIdx := startIdx + 2
		if endIdx > len(p.recentTracks) {
			endIdx = len(p.recentTracks)
		}
		if startIdx < endIdx {
			recentTracksToShow = p.recentTracks[startIdx:endIdx]
		}
	}

	tmpl := `
	<div class="lastfm-section section">
		<div class="plugin-header">
			<h3 class="plugin-title">{{.SectionTitle}}</h3>
		</div>
		<div class="plugin__inner">
			{{if .ShowScrobbles}}
			<div class="lastfm-stats">
				<span class="scrobbles-text">{{.ScrobblesText}}</span>
			</div>
			{{end}}

			<div class="current-track">
				<div class="track-main">
					{{if .Image}}
					<div class="track-cover-large">
						<img src="{{.Image}}" alt="Album art" loading="lazy">
						<div class="track-overlay">
							{{if and .ShowPlayButton .CanPlay}}
							<button class="play-btn" onclick="playTrack('{{.SearchQuery}}')" title="Play track">
								<svg viewBox="0 0 24 24" width="24" height="24">
									<path fill="currentColor" d="M8 5v14l11-7z"/>
								</svg>
							</button>
							{{end}}
						</div>
					</div>
					{{end}}
					
					<div class="track-info">
						<div class="track-status">
							<span class="status-indicator {{.StatusClass}}"></span>
							<span class="status-text">{{.StatusText}}</span>
						</div>
						<div class="track-title">{{.Name}}</div>
						<div class="track-artist">by {{.Artist}}</div>
						{{if .Album}}
						<div class="track-album">from {{.Album}}</div>
						{{end}}
						
						<div class="track-actions">
							<a class="btn btn-sm" href="{{.TrackURL}}" target="_blank" rel="noopener">
								<img src="https://www.last.fm/static/images/favicon.ico" width="16" height="16" alt="Last.fm" style="margin-right: 4px;">
								Last.fm
							</a>
						</div>
					</div>
				</div>
			</div>

			{{if .ShowRecentTracks}}
			{{if .RecentTracks}}
			<div class="recent-tracks">
				<h4>Recently played</h4>
				<div class="recent-tracks-list">
					{{range .RecentTracks}}
					<div class="recent-track-item">
						{{if .Image}}
						<div class="recent-track-cover">
							<img src="{{.Image}}" alt="{{.Name}}" loading="lazy">
						</div>
						{{end}}
						<div class="recent-track-info">
							<div class="recent-track-name">{{.Name}}</div>
							<div class="recent-track-artist">{{.Artist}}</div>
							<div class="recent-track-time">{{.RelativeTime}}</div>
						</div>
					</div>
					{{end}}
				</div>
			</div>
			{{end}}
			{{end}}
		</div>
	</div>`

	var processedRecentTracks []map[string]interface{}
	for _, track := range recentTracksToShow {
		image := p.pickBestImage(&track)
		relativeTime := p.getRelativeTimeForTrack(&track)

		processedRecentTracks = append(processedRecentTracks, map[string]interface{}{
			"Name":         track.Name,
			"Artist":       track.Artist.Text,
			"Image":        image,
			"RelativeTime": relativeTime,
		})
	}

	data := struct {
		SectionTitle     string
		Name             string
		Artist           string
		Album            string
		Image            string
		ShowScrobbles    bool
		ShowPlayButton   bool
		ShowRecentTracks bool
		ScrobblesText    string
		StatusText       string
		StatusClass      string
		CanPlay          bool
		SearchQuery      string
		TrackURL         string
		RecentTracks     []map[string]interface{}
	}{
		SectionTitle:     sectionTitle,
		Name:             p.currentTrack.Name,
		Artist:           p.currentTrack.Artist.Text,
		Album:            p.currentTrack.Album.Text,
		Image:            image,
		ShowScrobbles:    showScrobbles,
		ShowPlayButton:   showPlayButton,
		ShowRecentTracks: showRecentTracks,
		ScrobblesText:    scrobblesText,
		StatusText:       statusText,
		StatusClass:      statusClass,
		CanPlay:          true,
		SearchQuery:      searchQuery,
		TrackURL:         p.currentTrack.URL,
		RecentTracks:     processedRecentTracks,
	}

	t, err := template.New("lastfm").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = t.Execute(&buf, data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (p *LastFMPlugin) renderNoTrack(sectionTitle string) string {
	if p.apiKey == "" {
		return fmt.Sprintf(`<div class="lastfm-section section">
			<div class="plugin-header">
				<h3 class="plugin-title">%s</h3>
			</div>
			<div class="plugin__inner">
				<p class="text-muted">Last.fm API key not configured</p>
			</div>
		</div>`, sectionTitle)
	}

	return fmt.Sprintf(`<div class="lastfm-section section">
		<div class="plugin-header">
			<h3 class="plugin-title">%s</h3>
		</div>
		<div class="plugin__inner">
			<p class="text-muted">No recent tracks found</p>
		</div>
	</div>`, sectionTitle)
}

func (p *LastFMPlugin) UpdateData(ctx context.Context) error {
	if p.apiKey == "" {
		return nil
	}

	if time.Since(p.lastUpdateTime) < 90*time.Second {
		return nil
	}

	config := p.storage.GetPluginConfig(p.Name())
	username, ok := config.Settings["username"].(string)
	if !ok || strings.TrimSpace(username) == "" {
		return fmt.Errorf("username not configured")
	}

	changed, err := p.updateRecentTracks(username)
	if err != nil {
		return err
	}

	if p.userInfo == nil || time.Since(p.lastUpdateTime) > 30*time.Minute {
		if err := p.updateUserInfo(username); err != nil {
			log.Printf("Warning: Failed to update Last.fm user info: %v", err)
		}
	}

	p.lastUpdateTime = time.Now()

	if changed && p.currentTrack != nil {
		if p.pickBestImage(p.currentTrack) == "" {
			_ = p.tryArtworkFallbacks(p.currentTrack)
		}
	}

	return nil
}

func (p *LastFMPlugin) updateRecentTracksWebSocket(username string) error {
	urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getrecenttracks&user=%s&api_key=%s&format=json&limit=10",
		url.QueryEscape(username), url.QueryEscape(p.apiKey))

	var response LastFMResponse
	if err := p.getJSONWithRetry(urlStr, &response); err != nil {
		return fmt.Errorf("failed to fetch Last.fm data: %w", err)
	}

	if len(response.RecentTracks.Track) > 0 {
		newCurrentTrack := &response.RecentTracks.Track[0]

		trackChanged := p.currentTrack == nil ||
			p.currentTrack.Name != newCurrentTrack.Name ||
			p.currentTrack.Artist.Text != newCurrentTrack.Artist.Text ||
			p.currentTrack.Attr.NowPlaying != newCurrentTrack.Attr.NowPlaying

		if trackChanged {
			p.currentTrack = newCurrentTrack
			p.recentTracks = response.RecentTracks.Track

			for i := range p.recentTracks {
				if p.pickBestImage(&p.recentTracks[i]) == "" {
					_ = p.tryArtworkFallbacks(&p.recentTracks[i])
				}
			}

			var recentTracksData []map[string]interface{}
			for i, track := range p.recentTracks {
				if track.Attr.NowPlaying == "true" {
					if i == 0 {
						continue // Skip the current track if it's now playing
					}
				} else if i == 0 {
					// If the first track is not playing, we only want to show it once
					continue
				}
				if i >= 3 {
					break // Limit to 5 recent tracks
				}
				recentTracksData = append(recentTracksData, map[string]interface{}{
					"name":         track.Name,
					"artist":       track.Artist.Text,
					"album":        track.Album.Text,
					"image":        p.pickBestImage(&track),
					"url":          track.URL,
					"isPlaying":    track.Attr.NowPlaying == "true",
					"relativeTime": p.getRelativeTimeForTrack(&track),
				})
			}

			p.hub.Broadcast("lastfm_update", map[string]interface{}{
				"name":         newCurrentTrack.Name,
				"artist":       newCurrentTrack.Artist.Text,
				"album":        newCurrentTrack.Album.Text,
				"isPlaying":    newCurrentTrack.Attr.NowPlaying == "true",
				"url":          newCurrentTrack.URL,
				"image":        p.pickBestImage(newCurrentTrack),
				"recentTracks": recentTracksData,
				"timestamp":    time.Now().Unix(),
			})

			return nil
		}
	}

	return nil
}

func (p *LastFMPlugin) updateRecentTracks(username string) (bool, error) {
	urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getrecenttracks&user=%s&api_key=%s&format=json&limit=10",
		url.QueryEscape(username), url.QueryEscape(p.apiKey))

	var response LastFMResponse
	if err := p.getJSONWithRetry(urlStr, &response); err != nil {
		return false, fmt.Errorf("failed to fetch Last.fm data: %w", err)
	}

	if len(response.RecentTracks.Track) > 0 {
		newCurrentTrack := &response.RecentTracks.Track[0]

		trackChanged := p.currentTrack == nil ||
			p.currentTrack.Name != newCurrentTrack.Name ||
			p.currentTrack.Artist.Text != newCurrentTrack.Artist.Text ||
			p.currentTrack.Attr.NowPlaying != newCurrentTrack.Attr.NowPlaying

		p.currentTrack = newCurrentTrack
		p.recentTracks = response.RecentTracks.Track

		for i := range p.recentTracks {
			if p.pickBestImage(&p.recentTracks[i]) == "" {
				_ = p.tryArtworkFallbacks(&p.recentTracks[i])
			}
		}

		if trackChanged {
			p.hub.Broadcast("lastfm_update", map[string]interface{}{
				"name":      newCurrentTrack.Name,
				"artist":    newCurrentTrack.Artist.Text,
				"album":     newCurrentTrack.Album.Text,
				"isPlaying": newCurrentTrack.Attr.NowPlaying == "true",
				"url":       newCurrentTrack.URL,
				"image":     p.pickBestImage(newCurrentTrack),
			})
			return true, nil
		}
	}

	return false, nil
}

func (p *LastFMPlugin) updateUserInfo(username string) error {
	urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getinfo&user=%s&api_key=%s&format=json",
		url.QueryEscape(username), url.QueryEscape(p.apiKey))

	var response LastFMUserResponse
	if err := p.getJSONWithRetry(urlStr, &response); err != nil {
		return fmt.Errorf("failed to fetch Last.fm user info: %w", err)
	}

	p.userInfo = &response.User
	return nil
}

func (p *LastFMPlugin) tryArtworkFallbacks(t *LastFMTrack) error {
	if art := p.fetchTrackInfoImage(t.Artist.Text, t.Name); art != "" {
		t.Image = []struct {
			Text string `json:"#text"`
			Size string `json:"size"`
		}{
			{Text: art, Size: "extralarge"},
		}
		return nil
	}

	if t.Album.Text != "" {
		if art := p.fetchAlbumInfoImage(t.Artist.Text, t.Album.Text); art != "" {
			t.Image = []struct {
				Text string `json:"#text"`
				Size string `json:"size"`
			}{
				{Text: art, Size: "extralarge"},
			}
			return nil
		}
	}

	return errors.New("no artwork found via fallbacks")
}

func (p *LastFMPlugin) fetchTrackInfoImage(artist, track string) string {
	endpoint := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=track.getInfo&artist=%s&track=%s&api_key=%s&format=json",
		url.QueryEscape(artist), url.QueryEscape(track), url.QueryEscape(p.apiKey))
	var resp lastfmTrackInfoResp
	if err := p.getJSONWithRetry(endpoint, &resp); err != nil {
		return ""
	}
	for i := len(resp.Track.Album.Image) - 1; i >= 0; i-- {
		url := ensureHTTPS(resp.Track.Album.Image[i].Text)
		if url != "" {
			return url
		}
	}
	return ""
}

func (p *LastFMPlugin) fetchAlbumInfoImage(artist, album string) string {
	endpoint := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=album.getInfo&artist=%s&album=%s&api_key=%s&format=json",
		url.QueryEscape(artist), url.QueryEscape(album), url.QueryEscape(p.apiKey))
	var resp lastfmAlbumInfoResp
	if err := p.getJSONWithRetry(endpoint, &resp); err != nil {
		return ""
	}
	for i := len(resp.Album.Image) - 1; i >= 0; i-- {
		url := ensureHTTPS(resp.Album.Image[i].Text)
		if url != "" {
			return url
		}
	}
	return ""
}

func (p *LastFMPlugin) pickBestImage(t *LastFMTrack) string {
	if t == nil || len(t.Image) == 0 {
		return ""
	}
	for i := len(t.Image) - 1; i >= 0; i-- {
		u := ensureHTTPS(t.Image[i].Text)
		if u != "" {
			return u
		}
	}
	return ""
}

func (p *LastFMPlugin) getRelativeTimeForTrack(t *LastFMTrack) string {
	if t.Attr.NowPlaying == "true" {
		return "now playing"
	}
	uts := t.Date.Uts
	sec, err := strconv.ParseInt(uts, 10, 64)
	if err != nil {
		return ""
	}
	then := time.Unix(sec, 0)
	d := time.Since(then)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func ensureHTTPS(u string) string {
	if u == "" {
		return ""
	}
	u = strings.TrimSpace(u)
	u = strings.Replace(u, "http://", "https://", 1)
	return u
}

func (p *LastFMPlugin) getJSONWithRetry(urlStr string, target interface{}) error {
	backoff := 500 * time.Millisecond
	var lastErr error

	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequest("GET", urlStr, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")
		req.Header.Set("Accept", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			func() {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
					lastErr = fmt.Errorf("status %d", resp.StatusCode)
					return
				}
				dec := json.NewDecoder(resp.Body)
				if err := dec.Decode(target); err != nil {
					lastErr = err
					return
				}
				lastErr = nil
			}()
		}

		if lastErr == nil {
			return nil
		}
		time.Sleep(backoff)
		backoff *= 2
	}

	return lastErr
}

func (p *LastFMPlugin) SearchAndPlayTrack(query string) (*AkarpovrMusicTrack, error) {
	encodedQuery := url.QueryEscape(query)
	searchURL := fmt.Sprintf("https://new.akarpov.ru/api/v1/music/song/?search=%s", encodedQuery)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(searchURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response AkarpovrMusicResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	if len(response.Results) == 0 {
		return nil, fmt.Errorf("no tracks found")
	}

	bestMatch := &response.Results[0]
	p.currentSong = bestMatch

	p.hub.Broadcast("music_play", map[string]interface{}{
		"name":   bestMatch.Name,
		"file":   bestMatch.File,
		"image":  bestMatch.ImageCropped,
		"length": bestMatch.Length,
		"album":  bestMatch.Album.Name,
		"query":  query,
	})

	return bestMatch, nil
}

func (p *LastFMPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *LastFMPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings

	err := p.storage.SetPluginConfig(p.Name(), config)
	if err != nil {
		return err
	}

	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(),
		"action": "settings_changed",
	})

	return nil
}

func (p *LastFMPlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(string); ok {
				return value
			}
			return defaultValue
		} else {
			if next, ok := current[k].(map[string]interface{}); ok {
				current = next
			} else {
				return defaultValue
			}
		}
	}
	return defaultValue
}

func (p *LastFMPlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(bool); ok {
				return value
			}
			return defaultValue
		} else {
			if next, ok := current[k].(map[string]interface{}); ok {
				current = next
			} else {
				return defaultValue
			}
		}
	}
	return defaultValue
}

func formatScrobbles(count string) string {
	runes := []rune(count)
	if len(runes) <= 3 {
		return count
	}
	var result []rune
	for i, r := range runes {
		if i > 0 && (len(runes)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, r)
	}
	return string(result)
}

func (p *LastFMPlugin) relativePlayedAt() string {
	if p.currentTrack == nil || p.currentTrack.Attr.NowPlaying == "true" {
		return "just now"
	}
	uts := p.currentTrack.Date.Uts
	sec, err := strconv.ParseInt(uts, 10, 64)
	if err != nil {
		return ""
	}
	then := time.Unix(sec, 0)
	d := time.Since(then)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}
