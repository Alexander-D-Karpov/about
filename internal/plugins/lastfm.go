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

const lastfmPlaceholderImage = "https://lastfm.freetls.fastly.net/i/u/300x300/2a96cbd8b46e442fc41c2b86b821562f.png"

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
	imageCache          map[string]imageCacheEntry
	imageCacheMutex     sync.RWMutex
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

type imageCacheEntry struct {
	URL      string `json:"url"`
	CachedAt int64  `json:"cached_at"`
}

type lastfmAlbumInfoResp struct {
	Album struct {
		Image []struct {
			Text string `json:"#text"`
			Size string `json:"size"`
		} `json:"image"`
	} `json:"album"`
}

type AkarpovrMusicSearchResponse struct {
	Songs []AkarpovrMusicTrack `json:"songs"`
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
		Slug         string `json:"slug"`
		ImageCropped string `json:"image_cropped"`
	} `json:"album"`
	Authors []struct {
		Name         string `json:"name"`
		Slug         string `json:"slug"`
		ImageCropped string `json:"image_cropped"`
	} `json:"authors"`
}

func NewLastFMPlugin(storage *storage.Storage, hub *stream.Hub, apiKey string) *LastFMPlugin {
	plugin := &LastFMPlugin{
		storage:    storage,
		hub:        hub,
		apiKey:     apiKey,
		httpClient: NewHTTPClientWithTimeout(15 * time.Second),
		stopPoll:   make(chan struct{}),
		imageCache: make(map[string]imageCacheEntry),
	}

	plugin.loadImageCache()
	go plugin.startConstantPolling()

	return plugin
}

func (p *LastFMPlugin) loadImageCache() {
	config := p.storage.GetPluginConfig(p.Name())
	if cachedImages, ok := config.Settings["imageCache"].(map[string]interface{}); ok {
		p.imageCacheMutex.Lock()
		for k, v := range cachedImages {
			if entry, ok := v.(map[string]interface{}); ok {
				url, _ := entry["url"].(string)
				cachedAt, _ := entry["cached_at"].(float64)
				p.imageCache[k] = imageCacheEntry{URL: url, CachedAt: int64(cachedAt)}
			} else if str, ok := v.(string); ok {
				p.imageCache[k] = imageCacheEntry{URL: str, CachedAt: time.Now().Unix()}
			}
		}
		p.imageCacheMutex.Unlock()
	}
}

func (p *LastFMPlugin) isCacheValid(entry imageCacheEntry) bool {
	maxAge := int64(7 * 24 * 60 * 60) // 7 days for valid URLs
	if entry.URL == "" {
		maxAge = int64(24 * 60 * 60) // 1 day for empty results
	}
	return time.Now().Unix()-entry.CachedAt < maxAge
}

func (p *LastFMPlugin) saveImageCache() {
	p.imageCacheMutex.RLock()
	cacheCopy := make(map[string]interface{})
	for k, v := range p.imageCache {
		cacheCopy[k] = map[string]interface{}{
			"url":       v.URL,
			"cached_at": v.CachedAt,
		}
	}
	p.imageCacheMutex.RUnlock()

	config := p.storage.GetPluginConfig(p.Name())
	if config.Settings == nil {
		config.Settings = make(map[string]interface{})
	}
	config.Settings["imageCache"] = cacheCopy

	if err := p.storage.SetPluginConfig(p.Name(), config); err != nil {
		log.Printf("[LastFM] Failed to save image cache: %v", err)
	}
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

	image := p.getTrackImage(currentTrack)

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

	var recentTracksToShow []LastFMTrack
	if showRecentTracks && len(recentTracks) > 0 {
		seen := make(map[string]bool)
		currentKey := fmt.Sprintf("%s|%s", currentTrack.Artist.Text, currentTrack.Name)

		for _, track := range recentTracks {
			trackKey := fmt.Sprintf("%s|%s", track.Artist.Text, track.Name)

			if trackKey == currentKey {
				continue
			}

			if seen[trackKey] {
				continue
			}
			seen[trackKey] = true

			recentTracksToShow = append(recentTracksToShow, track)

			if len(recentTracksToShow) >= 5 {
				break
			}
		}
	}

	tmpl := `
<section class="lastfm-section section plugin" data-w="2">
	<div class="plugin-header">
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
	</div>
	<div class="plugin__inner">
		{{if .ShowScrobbles}}
		<div class="lastfm-stats">
			<span class="scrobbles-text">{{.ScrobblesText}}</span>
		</div>
		{{end}}

		<div class="current-track" data-artist="{{.Artist}}" data-track="{{.Name}}">
			<div class="track-main">
				{{if .Image}}
				<div class="track-cover-large">
					<img src="{{.Image}}" alt="Album art" loading="lazy" id="lastfm-artwork">
					<div class="track-overlay">
						{{if and .ShowPlayButton .CanPlay}}
						<button class="play-btn play-btn-large" onclick="playCurrentLastFMTrack()" title="Play track">
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
					<div class="track-title" id="lastfm-track-title">{{.Name}}</div>
					<div class="track-artist" id="lastfm-track-artist">by {{.Artist}}</div>
					{{if .Album}}
					<div class="track-album" id="lastfm-track-album">from {{.Album}}</div>
					{{end}}
					
					<div class="track-actions">
						<a class="btn btn-sm" href="{{.TrackURL}}" target="_blank" rel="noopener" id="lastfm-link">
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
</section>`

	var processedRecentTracks []map[string]interface{}
	for _, track := range recentTracksToShow {
		trackImage := p.getTrackImage(&track)
		relativeTime := p.getRelativeTimeForTrack(&track)

		processedRecentTracks = append(processedRecentTracks, map[string]interface{}{
			"Name":         track.Name,
			"Artist":       track.Artist.Text,
			"Image":        trackImage,
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

func (p *LastFMPlugin) getTrackImage(track *LastFMTrack) string {
	if track == nil {
		return ""
	}

	cacheKey := fmt.Sprintf("%s-%s", track.Artist.Text, track.Name)

	p.imageCacheMutex.RLock()
	if entry, ok := p.imageCache[cacheKey]; ok && p.isCacheValid(entry) {
		p.imageCacheMutex.RUnlock()
		if entry.URL != "" {
			return entry.URL
		}
	} else {
		p.imageCacheMutex.RUnlock()
	}

	image := p.pickBestImage(track)

	if image == "" || p.isPlaceholderImage(image) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.tryAkarpovImageFallback(ctx, track)

		p.imageCacheMutex.RLock()
		if entry, ok := p.imageCache[cacheKey]; ok && entry.URL != "" {
			p.imageCacheMutex.RUnlock()
			return entry.URL
		}
		p.imageCacheMutex.RUnlock()
	}

	if image == "" || p.isPlaceholderImage(image) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.tryArtworkFallbacks(ctx, track)
		image = p.pickBestImage(track)
	}

	p.imageCacheMutex.Lock()
	if image != "" && !p.isPlaceholderImage(image) {
		p.imageCache[cacheKey] = imageCacheEntry{URL: image, CachedAt: time.Now().Unix()}
	} else {
		p.imageCache[cacheKey] = imageCacheEntry{URL: "", CachedAt: time.Now().Unix()}
	}
	p.imageCacheMutex.Unlock()

	go p.saveImageCache()

	return image
}

func (p *LastFMPlugin) renderNoTrack(sectionTitle string) string {
	if p.apiKey == "" {
		return fmt.Sprintf(`<section class="lastfm-section section plugin" data-w="2">
			<div class="plugin-header">
				<h3 class="plugin-title">%s</h3>
			</div>
			<div class="plugin__inner">
				<p class="text-muted">Last.fm API key not configured</p>
			</div>
		</section>`, sectionTitle)
	}

	return fmt.Sprintf(`<section class="lastfm-section section plugin" data-w="2">
		<div class="plugin-header">
			<h3 class="plugin-title">%s</h3>
		</div>
		<div class="plugin__inner">
			<p class="text-muted">No recent tracks found</p>
		</div>
	</section>`, sectionTitle)
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

	p.trackMutex.Lock()
	p.currentTrack = newCurrentTrack
	p.recentTracks = response.RecentTracks.Track
	p.trackMutex.Unlock()

	go func() {
		needsSave := false
		for i := range response.RecentTracks.Track {
			track := &response.RecentTracks.Track[i]
			cacheKey := fmt.Sprintf("%s-%s", track.Artist.Text, track.Name)

			p.imageCacheMutex.RLock()
			_, hasCached := p.imageCache[cacheKey]
			p.imageCacheMutex.RUnlock()

			if hasCached {
				continue
			}

			currentImage := p.pickBestImage(track)
			if currentImage == "" || p.isPlaceholderImage(currentImage) {
				artCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				p.tryAkarpovImageFallback(artCtx, track)
				cancel()

				p.imageCacheMutex.RLock()
				_, nowCached := p.imageCache[cacheKey]
				p.imageCacheMutex.RUnlock()

				if nowCached {
					needsSave = true
				}
			}
		}
		if needsSave {
			p.saveImageCache()
		}
	}()

	if trackChanged {
		p.broadcastTrackUpdate(newCurrentTrack, response.RecentTracks.Track)
	}

	return trackChanged, nil
}

func (p *LastFMPlugin) isPlaceholderImage(imageURL string) bool {
	return imageURL == lastfmPlaceholderImage ||
		strings.Contains(imageURL, "2a96cbd8b46e442fc41c2b86b821562f")
}

func (p *LastFMPlugin) tryAkarpovImageFallback(ctx context.Context, track *LastFMTrack) {
	if ctx.Err() != nil {
		return
	}

	cacheKey := fmt.Sprintf("%s-%s", track.Artist.Text, track.Name)

	p.imageCacheMutex.RLock()
	if entry, ok := p.imageCache[cacheKey]; ok && p.isCacheValid(entry) {
		p.imageCacheMutex.RUnlock()
		if entry.URL != "" {
			track.Image = []struct {
				Text string `json:"#text"`
				Size string `json:"size"`
			}{
				{Text: entry.URL, Size: "extralarge"},
			}
		}
		return
	}
	p.imageCacheMutex.RUnlock()

	searchQuery := fmt.Sprintf("%s %s", track.Artist.Text, track.Name)
	searchURL := fmt.Sprintf("https://new.akarpov.ru/api/v1/music/search/?query=%s", url.QueryEscape(searchQuery))

	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var searchResp AkarpovrMusicSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return
	}

	if len(searchResp.Songs) == 0 {
		p.imageCacheMutex.Lock()
		p.imageCache[cacheKey] = imageCacheEntry{URL: "", CachedAt: time.Now().Unix()}
		p.imageCacheMutex.Unlock()
		return
	}

	imageURL := ""
	for _, song := range searchResp.Songs {
		if song.ImageCropped != "" {
			imageURL = song.ImageCropped
			if !strings.HasPrefix(imageURL, "http") {
				imageURL = "https://new.akarpov.ru" + imageURL
			}
			break
		}
		if song.Album.ImageCropped != "" {
			imageURL = song.Album.ImageCropped
			if !strings.HasPrefix(imageURL, "http") {
				imageURL = "https://new.akarpov.ru" + imageURL
			}
			break
		}
	}

	p.imageCacheMutex.Lock()
	p.imageCache[cacheKey] = imageCacheEntry{URL: imageURL, CachedAt: time.Now().Unix()}
	p.imageCacheMutex.Unlock()

	if imageURL != "" {
		track.Image = []struct {
			Text string `json:"#text"`
			Size string `json:"size"`
		}{
			{Text: imageURL, Size: "extralarge"},
		}
		log.Printf("[LastFM] Found image from akarpov.ru for %s - %s", track.Artist.Text, track.Name)
	}
}

func (p *LastFMPlugin) broadcastTrackUpdate(track *LastFMTrack, recentTracks []LastFMTrack) {
	var recentTracksData []map[string]interface{}
	nowPlaying := track.Attr.NowPlaying == "true"

	seen := make(map[string]bool)
	currentKey := fmt.Sprintf("%s|%s", track.Name, track.Artist.Text)

	for _, t := range recentTracks {
		trackKey := fmt.Sprintf("%s|%s", t.Name, t.Artist.Text)

		if trackKey == currentKey {
			continue
		}

		if t.Attr.NowPlaying == "true" {
			continue
		}
		if seen[trackKey] {
			continue
		}
		seen[trackKey] = true

		if len(recentTracksData) >= 5 {
			break
		}

		trackImage := p.getTrackImageFromCache(&t)

		recentTracksData = append(recentTracksData, map[string]interface{}{
			"name":         t.Name,
			"artist":       t.Artist.Text,
			"album":        t.Album.Text,
			"image":        trackImage,
			"url":          t.URL,
			"isPlaying":    false,
			"relativeTime": p.getRelativeTimeForTrack(&t),
		})
	}

	currentImage := p.getTrackImage(track)

	p.hub.Broadcast("lastfm_update", map[string]interface{}{
		"name":         track.Name,
		"artist":       track.Artist.Text,
		"album":        track.Album.Text,
		"isPlaying":    nowPlaying,
		"url":          track.URL,
		"image":        currentImage,
		"recentTracks": recentTracksData,
		"timestamp":    time.Now().Unix(),
	})
}

func (p *LastFMPlugin) getTrackImageFromCache(track *LastFMTrack) string {
	if track == nil {
		return ""
	}

	cacheKey := fmt.Sprintf("%s-%s", track.Artist.Text, track.Name)

	p.imageCacheMutex.RLock()
	if entry, ok := p.imageCache[cacheKey]; ok && entry.URL != "" && p.isCacheValid(entry) {
		p.imageCacheMutex.RUnlock()
		return entry.URL
	}
	p.imageCacheMutex.RUnlock()

	return p.getTrackImage(track)
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

	if art := p.fetchTrackInfoImage(ctx, t.Artist.Text, t.Name); art != "" && !p.isPlaceholderImage(art) {
		t.Image = []struct {
			Text string `json:"#text"`
			Size string `json:"size"`
		}{
			{Text: art, Size: "extralarge"},
		}
		return nil
	}

	if t.Album.Text != "" {
		if art := p.fetchAlbumInfoImage(ctx, t.Artist.Text, t.Album.Text); art != "" && !p.isPlaceholderImage(art) {
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
		if url != "" && !p.isPlaceholderImage(url) {
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
		if url != "" && !p.isPlaceholderImage(url) {
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
	searchURL := fmt.Sprintf("https://new.akarpov.ru/api/v1/music/search/?query=%s", encodedQuery)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(searchURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response AkarpovrMusicSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	if len(response.Songs) == 0 {
		return nil, fmt.Errorf("no tracks found")
	}

	bestMatch := &response.Songs[0]

	imageURL := bestMatch.ImageCropped
	if imageURL != "" && !strings.HasPrefix(imageURL, "http") {
		imageURL = "https://new.akarpov.ru" + imageURL
	}

	p.currentSong = bestMatch

	artists := make([]string, 0, len(bestMatch.Authors))
	for _, author := range bestMatch.Authors {
		artists = append(artists, author.Name)
	}

	p.hub.Broadcast("music_play", map[string]interface{}{
		"name":    bestMatch.Name,
		"file":    bestMatch.File,
		"image":   imageURL,
		"length":  bestMatch.Length,
		"album":   bestMatch.Album.Name,
		"artists": artists,
		"query":   query,
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
