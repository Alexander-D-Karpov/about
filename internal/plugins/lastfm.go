package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
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
	pollTicker          *time.Ticker
	stopPoll            chan struct{}
	pollMutex           sync.Mutex
	trackMutex          sync.RWMutex
	pluginManager       interface{ GetClientCount() int }
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
		storage:    storage,
		hub:        hub,
		apiKey:     apiKey,
		httpClient: NewHTTPClientWithTimeout(15 * time.Second),
		stopPoll:   make(chan struct{}),
	}

	go plugin.startConstantPolling()

	return plugin
}

func (p *LastFMPlugin) SetPluginManager(pm interface{ GetClientCount() int }) {
	p.pluginManager = pm
}

func (p *LastFMPlugin) startConstantPolling() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if p.shouldPoll() {
				p.pollAndBroadcast()
			}
		case <-p.stopPoll:
			return
		}
	}
}

func (p *LastFMPlugin) shouldPoll() bool {
	if p.apiKey == "" {
		return false
	}

	if p.pluginManager == nil {
		return false
	}

	clientCount := p.pluginManager.GetClientCount()
	return clientCount > 0
}

func (p *LastFMPlugin) pollAndBroadcast() {
	config := p.storage.GetPluginConfig(p.Name())
	username, ok := config.Settings["username"].(string)
	if !ok || strings.TrimSpace(username) == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	changed, err := p.updateRecentTracksInternal(ctx, username)
	if err != nil {
		log.Printf("[LastFM] Poll error: %v", err)
		return
	}

	if changed {
		p.lastWebsocketUpdate = time.Now()
		log.Printf("[LastFM] Track changed, broadcasted update")
	}
}

func (p *LastFMPlugin) Name() string { return "lastfm" }

func (p *LastFMPlugin) Render(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Music")
	showScrobbles := p.getConfigBool(settings, "ui.showScrobbles", true)
	showPlayButton := p.getConfigBool(settings, "ui.showPlayButton", true)
	showRecentTracks := p.getConfigBool(settings, "ui.showRecentTracks", true)

	p.trackMutex.RLock()
	currentTrack := p.currentTrack
	recentTracks := p.recentTracks
	userInfo := p.userInfo
	p.trackMutex.RUnlock()

	if currentTrack == nil {
		return p.renderNoTrack(sectionTitle), nil
	}

	image := p.pickBestImage(currentTrack)
	if image == "" && p.currentSong != nil && p.currentSong.ImageCropped != "" {
		image = p.currentSong.ImageCropped
	}

	nowPlaying := currentTrack.Attr.NowPlaying == "true"
	statusText := "Last played " + p.relativePlayedAt(currentTrack)
	statusClass := ""
	if nowPlaying {
		statusText = "Now Playing"
		statusClass = "status-online"
	}

	scrobblesText := ""
	if showScrobbles && userInfo != nil && userInfo.PlayCount != "" {
		scrobblesText = fmt.Sprintf("Total scrobbles: %s", formatScrobbles(userInfo.PlayCount))
	}

	searchQuery := fmt.Sprintf("%s %s", currentTrack.Artist.Text, currentTrack.Name)

	var recentTracksToShow []LastFMTrack
	if showRecentTracks && len(recentTracks) > 0 {
		for i, track := range recentTracks {
			if i == 0 && nowPlaying {
				continue
			}

			if track.Name == currentTrack.Name &&
				track.Artist.Text == currentTrack.Artist.Text &&
				track.Album.Text == currentTrack.Album.Text {
				continue
			}

			recentTracksToShow = append(recentTracksToShow, track)

			if len(recentTracksToShow) >= 5 {
				break
			}
		}
	}

	tmpl := `
	<div class="lastfm-section section" data-w="2">
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
						<img src="{{.Image}}" alt="Album art" loading="lazy" id="lastfm-artwork">
						<div class="track-overlay">
							{{if and .ShowPlayButton .CanPlay}}
							<button class="play-btn play-btn-large" onclick="playTrack('{{.SearchQuery}}')" title="Play track">
								<svg viewBox="0 0 24 24" width="32" height="32">
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

			<div class="custom-music-player" id="custom-music-player" style="display:none">
				<div class="player-artwork-mini">
					<img src="" alt="" id="player-artwork-mini">
				</div>
				<div class="player-info">
					<div class="player-track-name" id="player-track-name"></div>
					<div class="player-artist-name" id="player-artist-name"></div>
				</div>
				<div class="player-progress-container">
					<div class="player-progress-bar" id="player-progress-bar">
						<div class="player-progress-fill" id="player-progress-fill"></div>
					</div>
					<div class="player-time">
						<span id="player-current-time">0:00</span>
						<span id="player-duration">0:00</span>
					</div>
				</div>
				<div class="player-controls">
					<button class="player-btn" id="player-play-pause" onclick="toggleMusicPlayPause()" title="Play/Pause">
						<svg viewBox="0 0 24 24" width="20" height="20" id="play-pause-icon">
							<path fill="currentColor" d="M8 5v14l11-7z"/>
						</svg>
					</button>
					<button class="player-btn" onclick="stopMusicPlayback()" title="Stop">
						<svg viewBox="0 0 24 24" width="20" height="20">
							<rect fill="currentColor" x="6" y="6" width="12" height="12"/>
						</svg>
					</button>
					<input type="range" class="player-volume" id="player-volume" min="0" max="100" value="80" title="Volume">
				</div>
			</div>

			<audio id="lastfm-audio-element" preload="metadata"></audio>

			{{if .ShowRecentTracks}}
			{{if .RecentTracks}}
			<div class="recent-tracks">
				<h4>Recently played</h4>
				<div class="recent-tracks-list">
					{{range .RecentTracks}}
					<div class="recent-track-item" data-track="{{.Name}}" data-artist="{{.Artist}}">
						{{if .Image}}
						<div class="recent-track-cover">
							<img src="{{.Image}}" alt="{{.Name}}" loading="lazy">
						</div>
						{{end}}
						<div class="recent-track-info">
							<div class="recent-track-name">{{.Name}}</div>
							<div class="recent-track-artist">{{.Artist}}</div>
						</div>
						<div class="recent-track-time">{{.RelativeTime}}</div>
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
		Name:             currentTrack.Name,
		Artist:           currentTrack.Artist.Text,
		Album:            currentTrack.Album.Text,
		Image:            image,
		ShowScrobbles:    showScrobbles,
		ShowPlayButton:   showPlayButton,
		ShowRecentTracks: showRecentTracks,
		ScrobblesText:    scrobblesText,
		StatusText:       statusText,
		StatusClass:      statusClass,
		CanPlay:          true,
		SearchQuery:      searchQuery,
		TrackURL:         currentTrack.URL,
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

	if time.Since(p.lastUpdateTime) < 15*time.Second {
		return nil
	}

	config := p.storage.GetPluginConfig(p.Name())
	username, ok := config.Settings["username"].(string)
	if !ok || strings.TrimSpace(username) == "" {
		return fmt.Errorf("username not configured")
	}

	updateCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	_, err := p.updateRecentTracksInternal(updateCtx, username)
	if err != nil {
		log.Printf("LastFM recent tracks update failed: %v", err)
		return err
	}

	if p.userInfo == nil || time.Since(p.lastUpdateTime) > 30*time.Minute {
		if err := p.updateUserInfo(updateCtx, username); err != nil {
			log.Printf("Warning: Failed to update Last.fm user info: %v", err)
		}
	}

	p.lastUpdateTime = time.Now()

	return nil
}

func (p *LastFMPlugin) updateRecentTracksInternal(ctx context.Context, username string) (bool, error) {
	urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getrecenttracks&user=%s&api_key=%s&format=json&limit=10",
		url.QueryEscape(username), url.QueryEscape(p.apiKey))

	var response LastFMResponse
	if err := p.getJSONWithRetry(ctx, urlStr, &response); err != nil {
		return false, fmt.Errorf("failed to fetch Last.fm data: %w", err)
	}

	if len(response.RecentTracks.Track) == 0 {
		return false, nil
	}

	newCurrentTrack := &response.RecentTracks.Track[0]

	p.trackMutex.RLock()
	oldTrack := p.currentTrack
	p.trackMutex.RUnlock()

	trackChanged := oldTrack == nil ||
		oldTrack.Name != newCurrentTrack.Name ||
		oldTrack.Artist.Text != newCurrentTrack.Artist.Text ||
		oldTrack.Attr.NowPlaying != newCurrentTrack.Attr.NowPlaying

	if !trackChanged {
		return false, nil
	}

	p.trackMutex.Lock()
	p.currentTrack = newCurrentTrack
	p.recentTracks = response.RecentTracks.Track
	p.trackMutex.Unlock()

	go func() {
		for i := range response.RecentTracks.Track {
			if p.pickBestImage(&response.RecentTracks.Track[i]) == "" {
				artCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				_ = p.tryArtworkFallbacks(artCtx, &response.RecentTracks.Track[i])
				cancel()
			}
		}
	}()

	p.broadcastTrackUpdate(newCurrentTrack, response.RecentTracks.Track)

	return true, nil
}

func (p *LastFMPlugin) broadcastTrackUpdate(track *LastFMTrack, recentTracks []LastFMTrack) {
	var recentTracksData []map[string]interface{}
	nowPlaying := track.Attr.NowPlaying == "true"

	seen := make(map[string]bool)
	currentKey := fmt.Sprintf("%s|%s", track.Name, track.Artist.Text)
	seen[currentKey] = true

	for _, t := range recentTracks {
		trackKey := fmt.Sprintf("%s|%s", t.Name, t.Artist.Text)

		if trackKey == currentKey {
			continue
		}

		if seen[trackKey] {
			continue
		}
		seen[trackKey] = true

		if len(recentTracksData) >= 5 {
			break
		}

		recentTracksData = append(recentTracksData, map[string]interface{}{
			"name":         t.Name,
			"artist":       t.Artist.Text,
			"album":        t.Album.Text,
			"image":        p.pickBestImage(&t),
			"url":          t.URL,
			"isPlaying":    t.Attr.NowPlaying == "true",
			"relativeTime": p.getRelativeTimeForTrack(&t),
		})
	}

	p.hub.Broadcast("lastfm_update", map[string]interface{}{
		"name":         track.Name,
		"artist":       track.Artist.Text,
		"album":        track.Album.Text,
		"isPlaying":    nowPlaying,
		"url":          track.URL,
		"image":        p.pickBestImage(track),
		"recentTracks": recentTracksData,
		"timestamp":    time.Now().Unix(),
	})
}

func (p *LastFMPlugin) updateUserInfo(ctx context.Context, username string) error {
	urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getinfo&user=%s&api_key=%s&format=json",
		url.QueryEscape(username), url.QueryEscape(p.apiKey))

	var response LastFMUserResponse
	if err := p.getJSONWithRetry(ctx, urlStr, &response); err != nil {
		return fmt.Errorf("failed to fetch Last.fm user info: %w", err)
	}

	p.trackMutex.Lock()
	p.userInfo = &response.User
	p.trackMutex.Unlock()

	return nil
}

func (p *LastFMPlugin) tryArtworkFallbacks(ctx context.Context, t *LastFMTrack) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if art := p.fetchTrackInfoImage(ctx, t.Artist.Text, t.Name); art != "" {
		t.Image = []struct {
			Text string `json:"#text"`
			Size string `json:"size"`
		}{
			{Text: art, Size: "extralarge"},
		}
		return nil
	}

	if t.Album.Text != "" {
		if art := p.fetchAlbumInfoImage(ctx, t.Artist.Text, t.Album.Text); art != "" {
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

func (p *LastFMPlugin) fetchTrackInfoImage(ctx context.Context, artist, track string) string {
	if ctx.Err() != nil {
		return ""
	}

	endpoint := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=track.getInfo&artist=%s&track=%s&api_key=%s&format=json",
		url.QueryEscape(artist), url.QueryEscape(track), url.QueryEscape(p.apiKey))

	var resp lastfmTrackInfoResp
	if err := p.getJSONWithRetry(ctx, endpoint, &resp); err != nil {
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

func (p *LastFMPlugin) fetchAlbumInfoImage(ctx context.Context, artist, album string) string {
	if ctx.Err() != nil {
		return ""
	}

	endpoint := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=album.getInfo&artist=%s&album=%s&api_key=%s&format=json",
		url.QueryEscape(artist), url.QueryEscape(album), url.QueryEscape(p.apiKey))

	var resp lastfmAlbumInfoResp
	if err := p.getJSONWithRetry(ctx, endpoint, &resp); err != nil {
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

func (p *LastFMPlugin) getJSONWithRetry(ctx context.Context, urlStr string, target interface{}) error {
	backoff := 500 * time.Millisecond
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)

		req, err := http.NewRequest("GET", urlStr, nil)
		if err != nil {
			cancel()
			return err
		}

		req = req.WithContext(reqCtx)
		req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")
		req.Header.Set("Accept", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			cancel()
			lastErr = err
		} else {
			func() {
				defer resp.Body.Close()
				defer cancel()

				if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
					lastErr = fmt.Errorf("status %d", resp.StatusCode)
					return
				}

				limitedReader := io.LimitReader(resp.Body, 1024*1024)
				dec := json.NewDecoder(limitedReader)
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

		if attempt < 2 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
				if backoff > 5*time.Second {
					backoff = 5 * time.Second
				}
			}
		}
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

func (p *LastFMPlugin) relativePlayedAt(track *LastFMTrack) string {
	if track == nil || track.Attr.NowPlaying == "true" {
		return "just now"
	}
	uts := track.Date.Uts
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

func (p *LastFMPlugin) RenderText(ctx context.Context) (string, error) {
	if p.apiKey == "" {
		return "Music: API key not configured", nil
	}

	p.trackMutex.RLock()
	currentTrack := p.currentTrack
	userInfo := p.userInfo
	p.trackMutex.RUnlock()

	if currentTrack == nil {
		return "Music: No recent tracks", nil
	}

	status := "Last played"
	if currentTrack.Attr.NowPlaying == "true" {
		status = "Now playing"
	}

	artist := currentTrack.Artist.Text
	if artist == "" {
		artist = "Unknown Artist"
	}

	track := currentTrack.Name
	if track == "" {
		track = "Unknown Track"
	}

	maxArtistLen := 20
	maxTrackLen := 25

	if len(artist) > maxArtistLen {
		artist = artist[:maxArtistLen-3] + "..."
	}

	if len(track) > maxTrackLen {
		track = track[:maxTrackLen-3] + "..."
	}

	scrobbles := ""
	if userInfo != nil && userInfo.PlayCount != "" {
		count := formatScrobbles(userInfo.PlayCount)
		scrobbles = fmt.Sprintf(" (%s scrobbles)", count)

		totalLen := len(status) + len(track) + len(artist) + len(scrobbles) + 10
		if totalLen > 55 {
			scrobbles = ""
		}
	}

	return fmt.Sprintf("Music: %s - %s by %s%s",
		status, track, artist, scrobbles), nil
}
