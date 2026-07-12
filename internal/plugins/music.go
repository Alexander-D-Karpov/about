package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

const musicHeartIcon = template.HTML(`<svg class="music-ico" viewBox="0 0 24 24" width="13" height="13" aria-hidden="true"><path fill="currentColor" d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"/></svg>`)
const musicSpotifyIcon = template.HTML(`<svg class="music-ico" viewBox="0 0 24 24" width="13" height="13" aria-hidden="true"><path fill="currentColor" d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm4.59 14.44c-.18.29-.56.38-.85.2-2.33-1.42-5.27-1.75-8.72-.96-.33.08-.66-.13-.74-.46-.08-.33.13-.66.46-.74 3.78-.86 7.02-.48 9.64 1.11.29.18.38.56.21.85zm1.22-2.72c-.22.36-.7.48-1.06.26-2.67-1.64-6.73-2.11-9.89-1.16-.41.13-.85-.1-.98-.51-.12-.41.11-.84.51-.97 3.61-1.1 8.09-.57 11.15 1.31.36.22.48.7.27 1.07zm.11-2.84C14.5 8.61 9.4 8.43 6.24 9.39c-.49.15-1.01-.13-1.16-.62-.15-.49.13-1.01.62-1.16 3.63-1.1 9.26-.89 12.98 1.32.44.26.59.83.33 1.27-.26.44-.83.59-1.27.33z"/></svg>`)

var musicTagColors = []string{
	"#6ab0ff", "#b07aff", "#ff6ea6", "#4fd08a", "#f0a850",
	"#5ccdd6", "#ff7a7a", "#c8a04a", "#7a9cff", "#54c98a", "#e07ad0", "#f2c14e",
}

func tagColorIndex(name string) int {
	var sum int
	for _, r := range name {
		sum = sum*31 + int(r)
	}
	if sum < 0 {
		sum = -sum
	}
	return sum % len(musicTagColors)
}

func formatTagCount(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

func formatMMSS(sec int) string {
	if sec < 0 {
		sec = 0
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

const musicPlaceholderImage = "https://lastfm.freetls.fastly.net/i/u/300x300/2a96cbd8b46e442fc41c2b86b821562f.png"

const musicDefaultPeriod = "3month"

var musicPeriods = []struct{ Value, Label string }{
	{"7day", "Last 7 days"},
	{"1month", "Last month"},
	{"3month", "Last 3 months"},
	{"6month", "Last 6 months"},
	{"12month", "Last year"},
	{"overall", "All time"},
}

const musicTemplate = `<section class="music-section section plugin" data-w="2" id="music-section">
  <div class="plugin-header">
    <h3 class="plugin-title">{{.SectionTitle}}</h3>
    <span class="music-likes text-muted">
      <span class="music-likes__item loved" title="Loved on Last.fm">{{heartIcon}}<span data-music-loved-count>{{.LovedCount}}</span></span>
      <span class="music-likes__sep">·</span>
      <span class="music-likes__item liked" title="Saved on Spotify">{{spotifyIcon}}<span data-music-liked-count>{{.SpotifyLikedCount}}</span></span>
    </span>
  </div>
  <div class="plugin__inner">
    <div class="music-now {{if .Now.Playing}}is-playing{{end}}" id="music-now" data-music-now>
      <div class="music-now__cover" data-music-cover>
        {{if .Now.Image}}<img src="{{.Now.Image}}" alt="" loading="lazy">{{else}}<div class="music-now__ph"></div>{{end}}
        <button class="music-now__play" type="button" onclick="playCurrentLastFMTrack()" aria-label="Play"></button>
      </div>
      <div class="music-now__meta">
        <div class="music-now__status" data-music-status>{{.Now.StatusText}}</div>
        <a class="music-now__name" href="{{.Now.URL}}" target="_blank" rel="noopener" data-music-name>{{.Now.Name}}</a>
        <div class="music-now__artist" data-music-artist>{{.Now.Artist}}</div>
        <div class="music-badges">
          <span class="badge loved {{if not .Now.LovedLastfm}}is-off{{end}}" data-music-loved title="Loved on Last.fm">{{heartIcon}}</span>
          <span class="badge liked {{if not .Now.LikedSpot}}is-off{{end}}" data-music-liked title="Saved on Spotify">{{spotifyIcon}}</span>
        </div>
        <div class="music-progress" data-music-progress data-started="{{.Now.Started}}" data-duration="{{.Now.Duration}}">
          <div class="music-progress__fill" data-music-fill style="width:{{.Now.ProgressPct}}%"></div>
        </div>
        <div class="music-progress__time">
          <span data-music-elapsed>{{.Now.ElapsedText}}</span>
          <span data-music-total>{{.Now.DurationText}}</span>
        </div>
      </div>
    </div>
    <span id="lastfm-track-name" hidden>{{.Now.Name}}</span>
    <span id="lastfm-track-artist" hidden>{{.Now.Artist}}</span>
    <span id="lastfm-track-image" hidden>{{.Now.Image}}</span>
    {{if .Recent}}
    <ul class="music-recent" id="music-recent" data-music-recent>
      {{range .Recent}}
      <li class="music-recent__item">
        <span class="music-recent__cover">
          {{if .Image}}<img src="{{.Image}}" alt="" loading="lazy">{{else}}<span class="music-recent__ph" style="background:{{.Color}}">{{.Initial}}</span>{{end}}
        </span>
        <span class="music-recent__text">
          <span class="music-recent__name">{{.Name}}</span>
          <span class="music-recent__artist">{{.Artist}}</span>
        </span>
        <span class="music-recent__badges">
          {{if .LovedLastfm}}<span class="badge loved">{{heartIcon}}</span>{{end}}
          {{if .LikedSpot}}<span class="badge liked">{{spotifyIcon}}</span>{{end}}
        </span>
        <span class="music-recent__time">{{.Time}}</span>
      </li>
      {{end}}
    </ul>
    {{end}}
    {{if .HasStats}}
    <details class="music-stats" data-music-stats {{if .StatsOpen}}open{{end}}>
      <summary class="music-stats__summary">
        <span>Statistics</span>
        <select class="music-period" data-music-period aria-label="Stats period" onclick="event.stopPropagation()" onchange="musicChangePeriod(this.value)">
          {{range .Periods}}<option value="{{.Value}}" {{if .Selected}}selected{{end}}>{{.Label}}</option>{{end}}
        </select>
      </summary>
      <div class="music-stats__body">
        <div class="music-totals">
          {{range .Totals}}
          <div class="music-total" style="--grad:{{.Grad}}">
            <div class="music-total__label">{{.Label}}</div>
            <div class="music-total__value">{{.Value}}</div>
            {{if .Delta}}<div class="music-total__delta {{if .DeltaPos}}up{{else}}down{{end}}">{{.Delta}}</div>{{end}}
            <svg class="music-total__spark" viewBox="0 0 100 28" preserveAspectRatio="none">
              <polyline points="{{barsSpark .Spark}}" fill="none" stroke="currentColor" stroke-width="2"/>
            </svg>
          </div>
          {{end}}
        </div>
        {{if .Tags}}
        <details class="music-sub" open>
          <summary>Genres</summary>
          <div class="music-tags">
            {{range .Tags}}<span class="music-tag" style="font-size:{{.Size}}px;color:{{.Color}}">{{.Name}}<sup class="music-tag__count">{{.Count}}</sup></span>{{end}}
          </div>
        </details>
        {{end}}
        {{if .TopArtists}}
        <details class="music-sub" data-role="artists" open>
          <summary>Top artists</summary>
          <div class="music-covers">
            {{range .TopArtists}}
            <div class="music-cover" style="--grad:{{.Grad}}">
              {{if .Image}}<img src="{{.Image}}" alt="" loading="lazy">{{else}}<div class="music-cover__ph">{{.Initial}}</div>{{end}}
              <div class="music-cover__name">{{.Name}}</div>
              <div class="music-cover__plays">{{.Plays}} plays</div>
            </div>
            {{end}}
          </div>
        </details>
        {{end}}
        {{if .TopAlbums}}
        <details class="music-sub" data-role="albums" open>
          <summary>Top albums</summary>
          <div class="music-covers">
            {{range .TopAlbums}}
            <div class="music-cover" style="--grad:{{.Grad}}">
              {{if .Image}}<img src="{{.Image}}" alt="" loading="lazy">{{else}}<div class="music-cover__ph">{{.Initial}}</div>{{end}}
              <div class="music-cover__name">{{.Name}}</div>
              <div class="music-cover__plays">{{.Plays}} plays</div>
            </div>
            {{end}}
          </div>
        </details>
        {{end}}
        {{if .WeeklyBars}}
        <div class="music-weekly">
          <div class="music-weekly__head">
            <span class="music-weekly__title">Weekly Scrobbles</span>
            {{if .WeeklyPeak}}<span class="music-weekly__peak">busiest · {{.WeeklyPeak}}</span>{{end}}
          </div>
          <div class="music-weekly__bars" data-weekly='{{.WeeklyJSON}}'>
            {{range $i, $b := .WeeklyBars}}
            <div class="music-weekly__col {{if eq $i $.TodayIndex}}is-today{{end}}">
              <div class="music-weekly__track"><div class="music-weekly__bar" style="height:{{$b}}%"></div></div>
              <span class="music-weekly__day">{{index $.WeeklyDays $i}}</span>
            </div>
            {{end}}
          </div>
        </div>
        {{end}}
      </div>
    </details>
    {{end}}
  </div>
</section>`

type MusicPlugin struct {
	storage   *storage.Storage
	hub       *stream.Hub
	cfg       *config.Config
	apiKey    string
	mediaPath string

	httpClient *http.Client

	currentTrack *MusicTrack
	recentTracks []MusicTrack
	userInfo     *MusicUser
	trackMutex   sync.RWMutex

	currentTrackKey    string
	trackStartTime     time.Time
	currentTrackLength int
	currentIsPlaying   bool

	imageCache      map[string]imageCacheEntry
	imageCacheMutex sync.RWMutex

	imgLocal   map[string]string
	imgLocalMu sync.RWMutex

	stats        *musicStats
	statsMutex   sync.RWMutex
	statsRunning int32
	statsAt      time.Time

	lovedSet   map[string]struct{}
	lovedCount int
	lovedMu    sync.RWMutex

	spotify  *SpotifyClient
	lastfm   *LastFMClient
	tagCache *ArtistTagCache

	spSavedCache map[string]struct{}
	spLikedCount int
	spSavedAt    time.Time
	spSavedMu    sync.RWMutex

	spNow   *SpotifyNowPlaying
	spNowAt time.Time
	spNowMu sync.Mutex

	pluginManager interface{ GetClientCount() int }

	pollMutex sync.Mutex
	stopPoll  chan struct{}

	scheduledUpdate *time.Timer
	scheduleMutex   sync.Mutex

	lastUpdateTime      time.Time
	lastWebsocketUpdate time.Time

	lastTrackLength  int
	statsByPeriod    map[string]*musicStats
	statsAtByPeriod  map[string]time.Time
	lastScrobbledKey string
	lastNowPollAt    time.Time
}

type MusicTrack struct {
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

type MusicUser struct {
	Name       string `json:"name"`
	PlayCount  string `json:"playcount"`
	Registered struct {
		UnixTime string `json:"unixtime"`
	} `json:"registered"`
}

type imageCacheEntry struct {
	URL      string `json:"url"`
	CachedAt int64  `json:"cached_at"`
}

type AkarpovMusicSearchResponse struct {
	Songs []AkarpovMusicTrack `json:"songs"`
}

type AkarpovMusicTrack struct {
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

type statPair struct {
	Cur  int `json:"cur"`
	Prev int `json:"prev"`
}

type topItem struct {
	Name   string `json:"name"`
	Artist string `json:"artist,omitempty"`
	Image  string `json:"image"`
	Plays  int    `json:"plays"`
}

type tagItem struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

type musicStats struct {
	Scrobbles  statPair  `json:"scrobbles"`
	Tracks     statPair  `json:"tracks"`
	Artists    statPair  `json:"artists"`
	Albums     statPair  `json:"albums"`
	Weekly     [7]int    `json:"weekly"`
	WeeklyDays [7]string `json:"weeklyDays"`
	PeakDay    string    `json:"peakDay"`
	TopArtists []topItem `json:"topArtists"`
	TopAlbums  []topItem `json:"topAlbums"`
	Tags       []tagItem `json:"tags"`
	Period     string    `json:"period"`
	FetchedAt  int64     `json:"fetchedAt"`
}

func (p *MusicPlugin) enrichArtistImages(ctx context.Context, items []topItem) {
	for i := range items {
		if items[i].Image != "" {
			continue
		}
		if p.spotify.Enabled() {
			if img, _ := p.spotify.ArtistImage(ctx, items[i].Name); img != "" {
				items[i].Image = img
				continue
			}
		}
		if img := p.akarpovArtistImage(ctx, items[i].Name); img != "" {
			items[i].Image = img
		}
	}
}

func peakIndex(bars []int) int {
	idx, best := 0, -1
	for i, v := range bars {
		if v > best {
			best, idx = v, i
		}
	}
	return idx
}

func peakLabel(bars []int, labels []string) string {
	i := peakIndex(bars)
	if i >= 0 && i < len(labels) {
		return labels[i]
	}
	return ""
}

var weekdayOrder = [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

func (p *MusicPlugin) computeWeeklyAndDeltas(ctx context.Context, username, period string, now time.Time, st *musicStats) {
	if period == "7day" {
		w, err := p.lastfm.RecentTracksFrom(ctx, username, now.AddDate(0, 0, -14).Unix(), 8)
		if err != nil {
			log.Printf("[Music] weekly 7day fetch: %v", err)
			return
		}
		bars := last7DayBars(w, now)
		days := last7DayLabels(now)
		st.Weekly = bars
		st.WeeklyDays = days
		st.PeakDay = peakLabel(bars[:], days[:])

		cur, prev := splitWeek(w, now)
		applySampledDeltas(st, cur, prev)
		if st.Scrobbles.Cur == 0 {
			cs, ctr, car, cal := countWindow(cur)
			st.Scrobbles.Cur, st.Tracks.Cur, st.Artists.Cur, st.Albums.Cur = cs, ctr, car, cal
		}
		return
	}

	if period == "overall" {
		w, err := p.lastfm.RecentTracksWindow(ctx, username, now.AddDate(-1, 0, 0).Unix(), now.Unix(), 30)
		if err != nil {
			log.Printf("[Music] weekly overall fetch: %v", err)
			return
		}
		st.Weekly = weekdayBars(w)
		st.WeeklyDays = weekdayOrder
		st.PeakDay = peakLabel(st.Weekly[:], weekdayOrder[:])
		return
	}

	cf, ct, pf, pt, ok := periodBounds(period, now)
	if !ok {
		return
	}
	maxPages := 40
	if period == "1month" {
		maxPages = 60
	}

	curTracks, err := p.lastfm.RecentTracksWindow(ctx, username, cf, ct, maxPages)
	if err != nil {
		log.Printf("[Music] weekly window (%s): %v", period, err)
	}
	st.Weekly = weekdayBars(curTracks)
	st.WeeklyDays = weekdayOrder
	st.PeakDay = peakLabel(st.Weekly[:], weekdayOrder[:])
	log.Printf("[Music] weekday buckets (%s): %v peak=%s", period, st.Weekly, st.PeakDay)

	p.applyWindowDeltas(ctx, username, cf, ct, pf, pt, st)
}

func (p *MusicPlugin) applyWindowDeltas(ctx context.Context, username string, curFrom, curTo, prevFrom, prevTo int64, st *musicStats) {
	prevScrobbles, err := p.lastfm.ScrobbleCountWindow(ctx, username, prevFrom, prevTo)
	if err != nil {
		log.Printf("[Music] prev window scrobbles: %v", err)
		return
	}
	curScrobbles := st.Scrobbles.Cur
	if curScrobbles <= 0 {
		if cs, cerr := p.lastfm.ScrobbleCountWindow(ctx, username, curFrom, curTo); cerr == nil {
			curScrobbles = cs
			st.Scrobbles.Cur = cs
		}
	}
	st.Scrobbles.Prev = prevScrobbles
	if curScrobbles <= 0 {
		return
	}
	ratio := float64(prevScrobbles) / float64(curScrobbles)
	st.Tracks.Prev = int(math.Round(float64(st.Tracks.Cur) * ratio))
	st.Artists.Prev = int(math.Round(float64(st.Artists.Cur) * ratio))
	st.Albums.Prev = int(math.Round(float64(st.Albums.Cur) * ratio))
	log.Printf("[Music] deltas %d→%d ratio %.3f: tracksPrev=%d artistsPrev=%d albumsPrev=%d",
		prevScrobbles, curScrobbles, ratio, st.Tracks.Prev, st.Artists.Prev, st.Albums.Prev)
}

func last7DayLabels(now time.Time) [7]string {
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var days [7]string
	for i := 0; i < 7; i++ {
		days[i] = todayStart.AddDate(0, 0, -(6 - i)).Format("Mon")
	}
	return days
}

func last7DayBars(tracks []windowedTrack, now time.Time) [7]int {
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var weekly [7]int
	for _, t := range tracks {
		played := time.Unix(t.uts, 0)
		ps := time.Date(played.Year(), played.Month(), played.Day(), 0, 0, 0, 0, played.Location())
		daysAgo := int(todayStart.Sub(ps).Hours() / 24)
		if daysAgo >= 0 && daysAgo < 7 {
			weekly[6-daysAgo]++
		}
	}
	return weekly
}

func weekdayBars(tracks []windowedTrack) [7]int {
	var weekly [7]int
	for _, t := range tracks {
		wd := int(time.Unix(t.uts, 0).Weekday())
		weekly[(wd+6)%7]++
	}
	return weekly
}

func splitWeek(tracks []windowedTrack, now time.Time) (cur, prev []windowedTrack) {
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for _, t := range tracks {
		played := time.Unix(t.uts, 0)
		ps := time.Date(played.Year(), played.Month(), played.Day(), 0, 0, 0, 0, played.Location())
		daysAgo := int(todayStart.Sub(ps).Hours() / 24)
		switch {
		case daysAgo >= 0 && daysAgo < 7:
			cur = append(cur, t)
		case daysAgo >= 7 && daysAgo < 14:
			prev = append(prev, t)
		}
	}
	return
}

func applySampledDeltas(st *musicStats, cur, prev []windowedTrack) {
	cs, ctr, car, cal := countWindow(cur)
	ps, ptr, par, pal := countWindow(prev)
	if cs > 0 {
		st.Scrobbles.Prev = scaleDelta(st.Scrobbles.Cur, cs, ps)
	}
	if ctr > 0 {
		st.Tracks.Prev = scaleDelta(st.Tracks.Cur, ctr, ptr)
	}
	if car > 0 {
		st.Artists.Prev = scaleDelta(st.Artists.Cur, car, par)
	}
	if cal > 0 {
		st.Albums.Prev = scaleDelta(st.Albums.Cur, cal, pal)
	}
}

func (p *MusicPlugin) enrichAlbumImages(ctx context.Context, items []topItem) {
	for i := range items {
		if items[i].Image != "" {
			continue
		}
		if p.spotify.Enabled() {
			if img, _ := p.spotify.AlbumImage(ctx, items[i].Artist, items[i].Name); img != "" {
				items[i].Image = img
				continue
			}
		}
		if img := p.akarpovAlbumImage(ctx, items[i].Artist, items[i].Name); img != "" {
			items[i].Image = img
		}
	}
}

func (p *MusicPlugin) computeStats(ctx context.Context, username, period string) (*musicStats, error) {
	now := time.Now()
	st := &musicStats{Period: period, FetchedAt: now.Unix()}

	scrobbles, tracks, albums, artists := p.lastfm.TopCounts(ctx, username, period)
	log.Printf("[Music] period totals (%s): scrobbles=%d tracks=%d albums=%d artists=%d", period, scrobbles, tracks, albums, artists)
	if scrobbles > 0 {
		st.Scrobbles.Cur = scrobbles
	}
	if tracks > 0 {
		st.Tracks.Cur = tracks
	}
	if artists > 0 {
		st.Artists.Cur = artists
	}
	if albums > 0 {
		st.Albums.Cur = albums
	}

	if period == "overall" {
		if u, err := p.lastfm.UserInfo(ctx, username); err == nil && u.PlayCount != "" {
			if n, e := strconv.Atoi(u.PlayCount); e == nil && n > 0 {
				st.Scrobbles.Cur = n
			}
			p.trackMutex.Lock()
			p.userInfo = u
			p.trackMutex.Unlock()
		}
	}

	p.computeWeeklyAndDeltas(ctx, username, period, now, st)

	if allArtists, err := p.lastfm.TopArtistsAll(ctx, username, period, 3, 1000); err == nil && len(allArtists) > 0 {
		display := allArtists
		if len(display) > 12 {
			display = display[:12]
		}
		p.enrichArtistImages(ctx, display)
		st.TopArtists = display

		if topAlbums, err := p.lastfm.TopAlbums(ctx, username, period, 12); err == nil {
			p.enrichAlbumImages(ctx, topAlbums)
			st.TopAlbums = topAlbums
		} else {
			log.Printf("[Music] top albums (%s): %v", period, err)
		}

		st.Tags = p.lastfm.TopTags(ctx, allArtists, len(allArtists), p.tagCache)
		log.Printf("[Music] top artists (%s): fetched=%d display=%d genres=%d", period, len(allArtists), len(display), len(st.Tags))
	} else if err != nil {
		log.Printf("[Music] top artists (%s): %v", period, err)
	}

	return st, nil
}

func NewMusicPlugin(store *storage.Storage, hub *stream.Hub, cfg *config.Config) *MusicPlugin {
	p := &MusicPlugin{
		storage:         store,
		hub:             hub,
		cfg:             cfg,
		apiKey:          cfg.LastFMKey,
		mediaPath:       cfg.MediaPath,
		httpClient:      NewHTTPClientWithTimeout(15 * time.Second),
		stopPoll:        make(chan struct{}),
		imageCache:      make(map[string]imageCacheEntry),
		imgLocal:        make(map[string]string),
		lovedSet:        make(map[string]struct{}),
		spSavedCache:    make(map[string]struct{}),
		spotify:         NewSpotifyClient(cfg.SpotifyClientID, cfg.SpotifyClientSecret, cfg.SpotifyRefreshToken),
		lastfm:          NewLastFMClient(cfg.LastFMKey),
		statsByPeriod:   map[string]*musicStats{},
		statsAtByPeriod: map[string]time.Time{},
		tagCache:        NewArtistTagCache(cfg.DataPath),
	}
	p.loadImageCache()
	p.loadImgLocalCache()
	p.loadRuntimeCache()
	p.loadStats()
	p.loadLoved()
	p.loadSpotifySavedCache()
	go p.startConstantPolling()
	go p.refreshSpotifySavedIfStale()
	return p
}

func (p *MusicPlugin) Name() string { return "music" }

func (p *MusicPlugin) SetPluginManager(pm interface{ GetClientCount() int }) {
	p.pluginManager = pm
}

func (p *MusicPlugin) startConstantPolling() {
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

func (p *MusicPlugin) shouldPoll() bool {
	if p.apiKey == "" || p.pluginManager == nil {
		return false
	}
	return p.pluginManager.GetClientCount() > 0
}

func (p *MusicPlugin) pollAndBroadcast() {
	config := p.storage.GetPluginConfig(p.Name())
	username, ok := config.Settings["username"].(string)
	if !ok || strings.TrimSpace(username) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	changed, err := p.updateRecentTracksInternal(ctx, username)
	if err != nil {
		log.Printf("[Music] Poll error: %v", err)
		return
	}
	if !changed {
		return
	}

	p.trackMutex.RLock()
	current := p.currentTrack
	recent := p.recentTracks
	isPlaying := p.currentIsPlaying
	p.trackMutex.RUnlock()

	if current != nil {
		p.broadcastTrackUpdate(current, recent, isPlaying)
		p.lastWebsocketUpdate = time.Now()
	}
}

func (p *MusicPlugin) refreshScrobbleCount(username, period string) {
	st := p.currentStats(period)
	if st == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var scrobbles int
	if period == "overall" {
		if u, err := p.lastfm.UserInfo(ctx, username); err == nil && u.PlayCount != "" {
			if n, e := strconv.Atoi(u.PlayCount); e == nil {
				st.Scrobbles.Cur = n
			}
			p.trackMutex.Lock()
			p.userInfo = u
			p.trackMutex.Unlock()
		}
		_, tracks, albums, artists := p.lastfm.TopCounts(ctx, username, period)
		if tracks > 0 {
			st.Tracks.Cur = tracks
		}
		if albums > 0 {
			st.Albums.Cur = albums
		}
		if artists > 0 {
			st.Artists.Cur = artists
		}
		log.Printf("[Music] period totals (overall): scrobbles=%d tracks=%d albums=%d artists=%d", st.Scrobbles.Cur, tracks, albums, artists)
	} else {
		var tracks, albums, artists int
		scrobbles, tracks, albums, artists = p.lastfm.TopCounts(ctx, username, period)
		log.Printf("[Music] period totals (%s): scrobbles=%d tracks=%d albums=%d artists=%d", period, scrobbles, tracks, albums, artists)
		if scrobbles > 0 {
			st.Scrobbles.Cur = scrobbles
		}
		if tracks > 0 {
			st.Tracks.Cur = tracks
		}
		if artists > 0 {
			st.Artists.Cur = artists
		}
		if albums > 0 {
			st.Albums.Cur = albums
		}
	}
	if scrobbles <= 0 {
		p.hub.Broadcast("music_stats", p.statsPayload(st))
		return
	}

	p.statsMutex.Lock()
	if cur := p.statsByPeriod[period]; cur != nil {
		cur.Scrobbles.Cur = scrobbles
		st = cur
	}
	p.statsMutex.Unlock()

	log.Printf("[Music] live scrobble count (%s): %d", period, scrobbles)
	p.hub.Broadcast("music_stats", p.statsPayload(st))
}

func (p *MusicPlugin) UpdateData(ctx context.Context) error {
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
	if _, err := p.updateRecentTracksInternal(updateCtx, username); err != nil {
		log.Printf("[Music] recent tracks update failed: %v", err)
		return err
	}
	if p.userInfo == nil || time.Since(p.lastUpdateTime) > 30*time.Minute {
		if err := p.updateUserInfo(updateCtx, username); err != nil {
			log.Printf("[Music] user info update failed: %v", err)
		}
	}
	go p.scheduleStatsRefresh(username)
	go p.refreshSpotifySavedIfStale()
	p.lastUpdateTime = time.Now()
	return nil
}

func (p *MusicPlugin) warmStatsBlocking(ctx context.Context, username, period string) {
	if p.statsFresh(period) {
		return
	}
	wctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	log.Printf("[Music] warming %s stats before serving", period)
	st, err := p.computeStats(wctx, username, period)
	if err != nil {
		log.Printf("[Music] warm %s failed: %v", period, err)
		return
	}
	p.storeStats(period, st)
	log.Printf("[Music] warmed %s: scrobbles=%d tracks=%d artists=%d albums=%d topArtists=%d topAlbums=%d genres=%d",
		period, st.Scrobbles.Cur, st.Tracks.Cur, st.Artists.Cur, st.Albums.Cur,
		len(st.TopArtists), len(st.TopAlbums), len(st.Tags))
}

func (p *MusicPlugin) updateRecentTracksInternal(ctx context.Context, username string) (bool, error) {
	tracks, err := p.lastfm.RecentTracks(ctx, username, 10)
	if err != nil {
		return false, fmt.Errorf("failed to fetch music data: %w", err)
	}
	if len(tracks) == 0 {
		return false, nil
	}
	newCurrent := &tracks[0]
	key := p.trackKey(newCurrent)
	explicitNowPlaying := newCurrent.Attr.NowPlaying == "true" || newCurrent.Date.Uts == ""

	p.trackMutex.RLock()
	oldTrack := p.currentTrack
	oldIsPlaying := p.currentIsPlaying
	oldKey := p.currentTrackKey
	oldName, oldArtist := "", ""
	if oldTrack != nil {
		oldName = oldTrack.Name
		oldArtist = oldTrack.Artist.Text
	}
	p.trackMutex.RUnlock()

	identityChanged := oldTrack == nil || oldName != newCurrent.Name || oldArtist != newCurrent.Artist.Text

	newIsPlaying := false
	shouldSetWindow := false
	windowStart := time.Time{}
	windowLength := 0
	shouldRefineLengthAsync := false

	if explicitNowPlaying {
		newIsPlaying = true
		if identityChanged || !oldIsPlaying || oldKey != key {
			shouldSetWindow = true
			windowStart = time.Now()
			windowLength = 240
			shouldRefineLengthAsync = true
		}
	} else {
		if oldIsPlaying && oldKey == key {
			p.trackMutex.RLock()
			start := p.trackStartTime
			length := p.currentTrackLength
			p.trackMutex.RUnlock()
			if length <= 0 {
				length = 240
			}
			if !start.IsZero() && time.Since(start) < time.Duration(length+5)*time.Second {
				newIsPlaying = true
			}
		}
		if !newIsPlaying && strings.TrimSpace(newCurrent.Date.Uts) != "" {
			if sec, err := strconv.ParseInt(newCurrent.Date.Uts, 10, 64); err == nil {
				playedAt := time.Unix(sec, 0)
				playedAgo := time.Since(playedAt)
				if playedAgo >= 0 && playedAgo < 10*time.Minute {
					fetchCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
					length := p.fetchTrackLength(fetchCtx, newCurrent.Artist.Text, newCurrent.Name)
					cancel()
					if length <= 0 {
						length = 240
					}
					if playedAgo < time.Duration(length+10)*time.Second {
						newIsPlaying = true
						shouldSetWindow = true
						windowStart = playedAt
						windowLength = length
					}
				}
			}
		}
	}

	if oldIsPlaying && !newIsPlaying {
		p.stopScheduledUpdate()
	}

	p.trackMutex.Lock()
	p.currentTrack = newCurrent
	p.recentTracks = tracks
	p.currentIsPlaying = newIsPlaying
	if newIsPlaying {
		p.currentTrackKey = key
	} else {
		p.currentTrackKey = ""
		p.currentTrackLength = 0
		p.trackStartTime = time.Time{}
	}
	p.trackMutex.Unlock()

	if shouldSetWindow && newIsPlaying {
		p.setNowPlayingWindow(key, windowStart, windowLength)
		if shouldRefineLengthAsync {
			go p.refineLengthAsync(newCurrent.Artist.Text, newCurrent.Name, key, windowStart)
		}
	}

	trackChanged := identityChanged || (oldIsPlaying != newIsPlaying)
	if newIsPlaying && identityChanged && oldTrack != nil && key != "" {
		p.trackMutex.Lock()
		count := p.lastScrobbledKey != key
		if count {
			p.lastScrobbledKey = key
		}
		p.trackMutex.Unlock()
		if count {
			go p.incrementScrobble()
		}
	}
	go p.prefetchArtwork(tracks)
	if trackChanged {
		go p.saveRuntimeCache()
	}
	return trackChanged, nil
}

func (p *MusicPlugin) visiblePeriod() string {
	cfg := p.storage.GetPluginConfig(p.Name())
	return normalizePeriod(p.getConfigValue(cfg.Settings, "ui.statsPeriod", musicDefaultPeriod))
}

func (p *MusicPlugin) incrementScrobble() {
	vp := p.visiblePeriod()
	now := time.Now()
	weekdayIdx := (int(now.Weekday()) + 6) % 7

	p.statsMutex.Lock()
	for _, st := range p.statsByPeriod {
		if st == nil {
			continue
		}
		st.Scrobbles.Cur++
		if st.Period == "7day" {
			st.Weekly[6]++
		} else {
			st.Weekly[weekdayIdx]++
		}
	}
	vis := p.statsByPeriod[vp]
	p.statsMutex.Unlock()

	p.saveStats()
	if vis != nil {
		p.hub.Broadcast("music_stats", p.statsPayload(vis))
		log.Printf("[Music] scrobble +1 (period=%s, total=%d)", vp, vis.Scrobbles.Cur)
	}
}

func (p *MusicPlugin) refineLengthAsync(artist, name, key string, start time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	length := p.fetchTrackLength(ctx, artist, name)
	if length <= 0 {
		length = 240
	}
	p.trackMutex.RLock()
	curKey := p.currentTrackKey
	curPlaying := p.currentIsPlaying
	p.trackMutex.RUnlock()
	if !curPlaying || curKey != key {
		return
	}
	p.trackMutex.Lock()
	p.currentTrackLength = length
	p.lastTrackLength = length
	if !start.IsZero() {
		p.trackStartTime = start
	}
	p.trackMutex.Unlock()
	remaining := length - int(time.Since(start).Seconds())
	if remaining < 1 {
		remaining = 1
	}
	p.scheduleEndOfTrackUpdate(remaining)
}

func (p *MusicPlugin) isNowPlaying(track *MusicTrack) bool {
	if track == nil {
		return false
	}
	if track.Attr.NowPlaying == "true" || track.Date.Uts == "" {
		return true
	}
	key := p.trackKey(track)
	p.trackMutex.RLock()
	curKey := p.currentTrackKey
	start := p.trackStartTime
	length := p.currentTrackLength
	curIsPlaying := p.currentIsPlaying
	p.trackMutex.RUnlock()
	if !curIsPlaying || key == "" || key != curKey {
		return false
	}
	if length <= 0 {
		length = 240
	}
	if start.IsZero() {
		return true
	}
	return time.Since(start) < time.Duration(length+5)*time.Second
}

func (p *MusicPlugin) setNowPlayingWindow(key string, start time.Time, lengthSec int) {
	if lengthSec <= 0 {
		lengthSec = 240
	}
	if start.IsZero() {
		start = time.Now()
	}
	p.trackMutex.Lock()
	p.currentTrackKey = key
	p.currentTrackLength = lengthSec
	p.trackStartTime = start
	p.currentIsPlaying = true
	p.trackMutex.Unlock()
	remaining := lengthSec - int(time.Since(start).Seconds())
	if remaining < 1 {
		remaining = 1
	}
	p.scheduleEndOfTrackUpdate(remaining)
}

func (p *MusicPlugin) scheduleEndOfTrackUpdate(remainingSeconds int) {
	p.scheduleMutex.Lock()
	defer p.scheduleMutex.Unlock()
	if p.scheduledUpdate != nil {
		p.scheduledUpdate.Stop()
		p.scheduledUpdate = nil
	}
	if remainingSeconds < 0 {
		remainingSeconds = 0
	}
	delay := time.Duration(remainingSeconds)*time.Second + 500*time.Millisecond
	p.scheduledUpdate = time.AfterFunc(delay, func() {
		if !p.shouldPoll() {
			return
		}
		p.pollAndBroadcast()
		time.AfterFunc(3*time.Second, func() {
			if p.shouldPoll() {
				p.pollAndBroadcast()
			}
		})
	})
}

func (p *MusicPlugin) stopScheduledUpdate() {
	p.scheduleMutex.Lock()
	defer p.scheduleMutex.Unlock()
	if p.scheduledUpdate != nil {
		p.scheduledUpdate.Stop()
		p.scheduledUpdate = nil
	}
}

func (p *MusicPlugin) trackKey(t *MusicTrack) string {
	if t == nil {
		return ""
	}
	artist := strings.ToLower(strings.TrimSpace(t.Artist.Text))
	name := strings.ToLower(strings.TrimSpace(t.Name))
	if artist == "" && name == "" {
		return ""
	}
	return artist + "|" + name
}

func (p *MusicPlugin) fetchTrackLength(ctx context.Context, artist, trackName string) int {
	searchQuery := strings.TrimSpace(artist + " " + trackName)
	searchURL := "https://new.akarpov.ru/api/v1/music/search/?query=" + url.QueryEscape(searchQuery)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return 0
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0
	}
	var searchResp AkarpovMusicSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return 0
	}
	if len(searchResp.Songs) > 0 {
		return searchResp.Songs[0].Length
	}
	return 0
}

func (p *MusicPlugin) updateUserInfo(ctx context.Context, username string) error {
	u, err := p.lastfm.UserInfo(ctx, username)
	if err != nil {
		return err
	}
	p.trackMutex.Lock()
	p.userInfo = u
	p.trackMutex.Unlock()
	return nil
}

type nowVM struct {
	HasTrack     bool
	Name         string
	Artist       string
	Album        string
	Image        string
	URL          string
	Playing      bool
	StatusText   string
	Started      int64
	Duration     int
	ProgressPct  int
	ElapsedText  string
	DurationText string
	LovedLastfm  bool
	LikedSpot    bool
}

type recentVM struct {
	Name        string
	Artist      string
	Image       string
	Initial     string
	Color       string
	Time        string
	LovedLastfm bool
	LikedSpot   bool
}

type coverVM struct {
	Name    string
	Image   string
	Plays   int
	Grad    string
	Initial string
}

type totalCard struct {
	Key      string
	Label    string
	Value    string
	Delta    string
	DeltaPos bool
	Spark    []int
	Grad     string
}

type periodOpt struct {
	Value    string
	Label    string
	Selected bool
}

type tagVM struct {
	Name  string
	Size  int
	Color string
	Count string
}

type musicVM struct {
	SectionTitle      string
	HasStats          bool
	StatsOpen         bool
	Likes             string
	LovedCount        int
	SpotifyLikedCount int
	SpotifyConnected  bool
	Totals            []totalCard
	Now               nowVM
	Recent            []recentVM
	TopArtists        []coverVM
	TopAlbums         []coverVM
	WeeklyJSON        template.JS
	WeeklyPeak        string
	Tags              []tagVM
	Period            string
	WeeklyBars        []int
	WeeklyDays        []string
	Periods           []periodOpt
	TodayIndex        int
}

func (p *MusicPlugin) Render(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings
	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Music")
	showRecent := p.getConfigBool(settings, "ui.showRecentTracks", true)
	showStats := p.getConfigBool(settings, "ui.showStats", true)

	p.trackMutex.RLock()
	currentTrack := p.currentTrack
	recentTracks := p.recentTracks
	userInfo := p.userInfo
	p.trackMutex.RUnlock()

	if currentTrack == nil {
		return p.renderNoTrack(sectionTitle), nil
	}

	period := normalizePeriod(p.getConfigValue(settings, "ui.statsPeriod", musicDefaultPeriod))

	vm := musicVM{
		SectionTitle:     sectionTitle,
		HasStats:         showStats,
		SpotifyConnected: p.spotify.UserEnabled(),
		Period:           period,
		Periods:          buildPeriodOpts(period),
		StatsOpen:        false,
	}
	vm.Now = p.buildNow(currentTrack)
	if showRecent {
		vm.Recent = p.buildRecent(currentTrack, recentTracks)
	}
	if showStats {
		st := p.currentStats(period)
		if st != nil {
			vm.Totals = p.buildTotals(st, userInfo, period)
			vm.TopArtists = buildCovers(st.TopArtists)
			vm.TopAlbums = buildCovers(st.TopAlbums)
			vm.Tags = buildTags(st.Tags, periodScrobbleTotal(st, userInfo, period))
			vm.WeeklyPeak = st.PeakDay
			vm.WeeklyBars, vm.WeeklyDays = buildWeekly(st)
			vm.TodayIndex = todayWeeklyIndex(period)
			if b, err := json.Marshal(st.Weekly); err == nil {
				vm.WeeklyJSON = template.JS(b)
			} else {
				vm.WeeklyJSON = template.JS("[0,0,0,0,0,0,0]")
			}
		}
		if uname, ok := config.Settings["username"].(string); ok && strings.TrimSpace(uname) != "" {
			go p.ensureAllPeriods(uname, period)
		}
	}
	p.lovedMu.RLock()
	vm.LovedCount = p.lovedCount
	p.lovedMu.RUnlock()
	p.spSavedMu.RLock()
	vm.SpotifyLikedCount = p.spLikedCount
	p.spSavedMu.RUnlock()
	vm.Likes = fmt.Sprintf("%d loved · %d liked", vm.LovedCount, vm.SpotifyLikedCount)

	return p.execRender(vm)
}

func (p *MusicPlugin) buildNow(t *MusicTrack) nowVM {
	image := p.getTrackImage(t)
	playing := p.isNowPlaying(t)
	n := nowVM{
		HasTrack: true,
		Name:     t.Name,
		Artist:   t.Artist.Text,
		Album:    t.Album.Text,
		Image:    p.imageForVM(t.Artist.Text, t.Name, image),
		URL:      t.URL,
		Playing:  playing,
	}
	p.trackMutex.RLock()
	length := p.currentTrackLength
	start := p.trackStartTime
	p.trackMutex.RUnlock()

	if now := p.spotifyNowCached(); now != nil && now.IsPlaying && sameTrack(now, t) {
		length = now.DurationMs / 1000
		start = time.Now().Add(-time.Duration(now.ProgressMs) * time.Millisecond)
		playing = true
		n.Playing = true
	}

	if playing {
		n.StatusText = "Now Playing"
		n.Duration = length
		n.DurationText = formatMMSS(length)
		if !start.IsZero() {
			n.Started = start.Unix()
			if length > 0 {
				elapsed := int(time.Since(start).Seconds())
				if elapsed < 0 {
					elapsed = 0
				}
				if elapsed > length {
					elapsed = length
				}
				n.ProgressPct = int(float64(elapsed) / float64(length) * 100)
				n.ElapsedText = formatMMSS(elapsed)
			}
		}
	} else {
		n.StatusText = "Last played " + p.relativePlayedAt(t)
		n.ElapsedText = "0:00"
		if length > 0 {
			n.Duration = length
			n.DurationText = formatMMSS(length)
		} else if l := p.lastTrackLength; l > 0 {
			n.Duration = l
			n.DurationText = formatMMSS(l)
		} else {
			n.DurationText = "0:00"
		}
	}
	n.LovedLastfm = p.isLovedTrack(t.Artist.Text, t.Name)
	n.LikedSpot = p.isTrackLikedCached(t.Artist.Text, t.Name)
	return n
}

func (p *MusicPlugin) buildRecent(current *MusicTrack, recent []MusicTrack) []recentVM {
	var out []recentVM
	seen := make(map[string]bool)
	currentKey := strings.ToLower(current.Artist.Text + "|" + current.Name)
	for i := range recent {
		t := &recent[i]
		key := strings.ToLower(t.Artist.Text + "|" + t.Name)
		if key == currentKey || seen[key] {
			continue
		}
		seen[key] = true
		image := p.getTrackImage(t)
		vm := recentVM{
			Name:        t.Name,
			Artist:      t.Artist.Text,
			Image:       p.imageForVM(t.Artist.Text, t.Name, image),
			Time:        p.getRelativeTimeForTrack(t),
			LovedLastfm: p.isLovedTrack(t.Artist.Text, t.Name),
			LikedSpot:   p.isTrackLikedCached(t.Artist.Text, t.Name),
		}
		if vm.Image == "" {
			color, initial := paletteFor(key)
			vm.Color = color
			vm.Initial = initial
		}
		out = append(out, vm)
		if len(out) >= 5 {
			break
		}
	}
	return out
}

func (p *MusicPlugin) buildTotals(st *musicStats, userInfo *MusicUser, period string) []totalCard {
	grads := []string{
		"linear-gradient(135deg,#4d9fff,#7aa2ff)",
		"linear-gradient(135deg,#b055ff,#7a5cff)",
		"linear-gradient(135deg,#10d060,#3ad38b)",
		"linear-gradient(135deg,#f0a010,#ffd166)",
	}
	showDelta := period != "overall"

	scrobbleVal := st.Scrobbles.Cur
	if period == "overall" && userInfo != nil && userInfo.PlayCount != "" {
		if n, err := strconv.Atoi(userInfo.PlayCount); err == nil {
			scrobbleVal = n
		}
	}

	cards := []struct {
		key, label string
		value      int
		cur, prev  int
	}{
		{"scrobbles", "Scrobbles", scrobbleVal, st.Scrobbles.Cur, st.Scrobbles.Prev},
		{"tracks", "Tracks", st.Tracks.Cur, st.Tracks.Cur, st.Tracks.Prev},
		{"artists", "Artists", st.Artists.Cur, st.Artists.Cur, st.Artists.Prev},
		{"albums", "Albums", st.Albums.Cur, st.Albums.Cur, st.Albums.Prev},
	}

	out := make([]totalCard, 0, len(cards))
	for i, c := range cards {
		card := totalCard{
			Key:   c.key,
			Label: c.label,
			Value: formatCount(c.value),
			Spark: sparkFor(i, st.Weekly),
			Grad:  grads[i%len(grads)],
		}
		if showDelta {
			card.Delta, card.DeltaPos = formatDelta(c.cur, c.prev)
		}
		out = append(out, card)
	}
	return out
}

func buildCovers(items []topItem) []coverVM {
	grads := []string{
		"linear-gradient(135deg,#4d9fff33,#7aa2ff22)",
		"linear-gradient(135deg,#b055ff33,#7a5cff22)",
		"linear-gradient(135deg,#10d06033,#3ad38b22)",
		"linear-gradient(135deg,#f0a01033,#ffd16622)",
	}
	out := make([]coverVM, 0, len(items))
	for i, it := range items {
		name := it.Name
		if it.Artist != "" {
			name = it.Artist + " – " + it.Name
		}
		vm := coverVM{
			Name:  name,
			Image: it.Image,
			Plays: it.Plays,
			Grad:  grads[i%len(grads)],
		}
		if strings.TrimSpace(it.Image) == "" {
			_, initial := paletteFor(strings.ToLower(name))
			vm.Initial = initial
		}
		out = append(out, vm)
	}
	return out
}

func buildWeekly(st *musicStats) ([]int, []string) {
	maxV := 1
	for _, v := range st.Weekly {
		if v > maxV {
			maxV = v
		}
	}
	bars := make([]int, 7)
	days := make([]string, 7)
	for i, v := range st.Weekly {
		bars[i] = int(math.Round(float64(v) / float64(maxV) * 100))
		if st.WeeklyDays[i] != "" {
			days[i] = st.WeeklyDays[i]
		} else {
			days[i] = weekdayOrder[i]
		}
	}
	return bars, days
}

func buildTags(tags []tagItem, totalScrobbles int) []tagVM {
	if len(tags) == 0 {
		return nil
	}
	maxW := 1
	sumW := 0
	for _, t := range tags {
		if t.Weight > maxW {
			maxW = t.Weight
		}
		sumW += t.Weight
	}
	if sumW < 1 {
		sumW = 1
	}
	out := make([]tagVM, 0, len(tags))
	for _, t := range tags {
		ratio := float64(t.Weight) / float64(maxW)
		size := 13 + int(ratio*21)
		est := int(math.Round(float64(t.Weight) / float64(sumW) * float64(totalScrobbles)))
		out = append(out, tagVM{
			Name:  t.Name,
			Size:  size,
			Color: musicTagColors[tagColorIndex(t.Name)],
			Count: formatTagCount(est),
		})
	}
	return centerOutTags(out)
}

func centerOutTags(in []tagVM) []tagVM {
	if len(in) < 3 {
		return in
	}
	out := make([]tagVM, len(in))
	mid := len(in) / 2
	lo, hi := mid, mid
	out[mid] = in[0]
	for i := 1; i < len(in); i++ {
		if i%2 == 1 {
			lo--
			out[lo] = in[i]
		} else {
			hi++
			out[hi] = in[i]
		}
	}
	return out
}

func periodScrobbleTotal(st *musicStats, userInfo *MusicUser, period string) int {
	total := st.Scrobbles.Cur
	if period == "overall" && userInfo != nil && userInfo.PlayCount != "" {
		if n, err := strconv.Atoi(userInfo.PlayCount); err == nil && n > 0 {
			total = n
		}
	}
	return total
}

func (p *MusicPlugin) execRender(vm musicVM) (string, error) {
	funcMap := template.FuncMap{
		"barsSpark":   barsSpark,
		"heartIcon":   func() template.HTML { return musicHeartIcon },
		"spotifyIcon": func() template.HTML { return musicSpotifyIcon },
	}
	t, err := template.New("music").Funcs(funcMap).Parse(musicTemplate)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, vm); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (p *MusicPlugin) renderNoTrack(sectionTitle string) string {
	msg := "No recent tracks found"
	if p.apiKey == "" {
		msg = "Last.fm API key not configured"
	}
	return fmt.Sprintf(`<section class="music-section section plugin" data-w="2" id="music-section">
	<div class="plugin-header"><h3 class="plugin-title">%s</h3></div>
	<div class="plugin__inner"><p class="text-muted">%s</p></div>
</section>`, sectionTitle, msg)
}

func (p *MusicPlugin) broadcastTrackUpdate(track *MusicTrack, recent []MusicTrack, isPlaying bool) {
	p.hub.Broadcast("music_update", p.buildTrackPayload(track, recent, isPlaying))
}

func (p *MusicPlugin) buildTrackPayload(track *MusicTrack, recent []MusicTrack, isPlaying bool) map[string]interface{} {
	var recentData []map[string]interface{}
	seen := make(map[string]bool)
	currentKey := strings.ToLower(track.Artist.Text + "|" + track.Name)
	for i := range recent {
		t := &recent[i]
		key := strings.ToLower(t.Artist.Text + "|" + t.Name)
		if key == currentKey || p.isNowPlaying(t) || seen[key] {
			continue
		}
		seen[key] = true
		if len(recentData) >= 5 {
			break
		}
		img := p.imageForVM(t.Artist.Text, t.Name, p.getTrackImage(t))
		item := map[string]interface{}{
			"name":         t.Name,
			"artist":       t.Artist.Text,
			"album":        t.Album.Text,
			"url":          t.URL,
			"loved":        p.isLovedTrack(t.Artist.Text, t.Name),
			"liked":        p.isTrackLikedCached(t.Artist.Text, t.Name),
			"relativeTime": p.getRelativeTimeForTrack(t),
		}
		if img != "" {
			item["image"] = img
		} else {
			color, initial := paletteFor(key)
			item["color"] = color
			item["initial"] = initial
		}
		recentData = append(recentData, item)
	}

	statusText := "Last played " + p.relativePlayedAt(track)
	if isPlaying {
		statusText = "Now Playing"
	}

	p.trackMutex.RLock()
	start := p.trackStartTime
	length := p.currentTrackLength
	p.trackMutex.RUnlock()
	started := int64(0)
	if isPlaying && !start.IsZero() {
		started = start.Unix()
	}

	p.lovedMu.RLock()
	lovedCount := p.lovedCount
	p.lovedMu.RUnlock()
	p.spSavedMu.RLock()
	likedCount := p.spLikedCount
	p.spSavedMu.RUnlock()

	return map[string]interface{}{
		"hasTrack":     true,
		"name":         track.Name,
		"artist":       track.Artist.Text,
		"album":        track.Album.Text,
		"isPlaying":    isPlaying,
		"statusText":   statusText,
		"url":          track.URL,
		"image":        imageOrPlaceholder(p.imageForVM(track.Artist.Text, track.Name, p.getTrackImage(track))),
		"loved":        p.isLovedTrack(track.Artist.Text, track.Name),
		"liked":        p.isTrackLikedCached(track.Artist.Text, track.Name),
		"likes":        fmt.Sprintf("%d loved · %d liked", lovedCount, likedCount),
		"lovedCount":   lovedCount,
		"likedCount":   likedCount,
		"started":      started,
		"duration":     length,
		"recentTracks": recentData,
		"timestamp":    time.Now().Unix(),
	}
}

func (p *MusicPlugin) HandleNowAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	if p.apiKey == "" {
		http.Error(w, `{"error":"not configured"}`, http.StatusServiceUnavailable)
		return
	}

	cfg := p.storage.GetPluginConfig(p.Name())
	username, _ := cfg.Settings["username"].(string)
	if strings.TrimSpace(username) != "" {
		p.pollMutex.Lock()
		stale := time.Since(p.lastNowPollAt) > 2*time.Second
		if stale {
			p.lastNowPollAt = time.Now()
		}
		p.pollMutex.Unlock()

		if stale {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			changed, err := p.updateRecentTracksInternal(ctx, username)
			cancel()
			if err != nil {
				log.Printf("[Music] now api poll: %v", err)
			} else if changed {
				p.trackMutex.RLock()
				t, rec, playing := p.currentTrack, p.recentTracks, p.currentIsPlaying
				p.trackMutex.RUnlock()
				if t != nil {
					p.hub.Broadcast("music_update", p.buildTrackPayload(t, rec, playing))
				}
			}
		}
	}

	p.trackMutex.RLock()
	track := p.currentTrack
	recent := p.recentTracks
	isPlaying := p.currentIsPlaying
	p.trackMutex.RUnlock()

	if track == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"hasTrack": false})
		return
	}
	_ = json.NewEncoder(w).Encode(p.buildTrackPayload(track, recent, isPlaying))
}

func (p *MusicPlugin) SearchAndPlayTrack(query string) (*AkarpovMusicTrack, error) {
	searchURL := "https://new.akarpov.ru/api/v1/music/search/?query=" + url.QueryEscape(query)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(searchURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var response AkarpovMusicSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}
	if len(response.Songs) == 0 {
		return nil, fmt.Errorf("no tracks found")
	}
	best := &response.Songs[0]
	image := best.ImageCropped
	if image != "" && !strings.HasPrefix(image, "http") {
		image = "https://new.akarpov.ru" + image
	}
	artists := make([]string, 0, len(best.Authors))
	for _, a := range best.Authors {
		artists = append(artists, a.Name)
	}
	p.hub.Broadcast("music_play", map[string]interface{}{
		"name":    best.Name,
		"file":    best.File,
		"image":   image,
		"length":  best.Length,
		"album":   best.Album.Name,
		"artists": artists,
		"query":   query,
	})
	return best, nil
}

func (p *MusicPlugin) RenderText(ctx context.Context) (string, error) {
	if p.apiKey == "" {
		return "Music: API key not configured", nil
	}
	p.trackMutex.RLock()
	current := p.currentTrack
	userInfo := p.userInfo
	p.trackMutex.RUnlock()
	if current == nil {
		return "Music: No recent tracks", nil
	}
	status := "Last played"
	if p.isNowPlaying(current) {
		status = "Now playing"
	}
	artist := clip(current.Artist.Text, 20)
	track := clip(current.Name, 25)
	scrobbles := ""
	if userInfo != nil && userInfo.PlayCount != "" {
		scrobbles = fmt.Sprintf(" (%s scrobbles)", formatScrobbles(userInfo.PlayCount))
	}
	return fmt.Sprintf("Music: %s - %s by %s%s", status, track, artist, scrobbles), nil
}

func (p *MusicPlugin) GetMetrics() map[string]interface{} {
	p.trackMutex.RLock()
	current := p.currentTrack
	userInfo := p.userInfo
	p.trackMutex.RUnlock()

	playing := current != nil && p.isNowPlaying(current)

	p.lovedMu.RLock()
	loved := p.lovedCount
	p.lovedMu.RUnlock()
	p.spSavedMu.RLock()
	liked := p.spLikedCount
	p.spSavedMu.RUnlock()

	scrobbles := 0
	trackName, artist := "", ""
	if current != nil {
		trackName = current.Name
		artist = current.Artist.Text
	}
	if userInfo != nil && userInfo.PlayCount != "" {
		if n, err := strconv.Atoi(userInfo.PlayCount); err == nil {
			scrobbles = n
		}
	}
	return map[string]interface{}{
		"nowPlaying":       playing,
		"track":            trackName,
		"artist":           artist,
		"scrobbles":        scrobbles,
		"loved":            loved,
		"spotifyLiked":     liked,
		"spotifyConnected": p.spotify.UserEnabled(),
	}
}

func (p *MusicPlugin) Stop() {
	select {
	case <-p.stopPoll:
	default:
		close(p.stopPoll)
	}
	p.stopScheduledUpdate()
}

func (p *MusicPlugin) getConfigValue(settings map[string]interface{}, dotted, def string) string {
	if v, ok := lookupDotted(settings, dotted); ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return def
}

func (p *MusicPlugin) getConfigBool(settings map[string]interface{}, dotted string, def bool) bool {
	if v, ok := lookupDotted(settings, dotted); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

func lookupDotted(settings map[string]interface{}, dotted string) (interface{}, bool) {
	parts := strings.Split(dotted, ".")
	var cur interface{} = settings
	for _, part := range parts {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func decodeSetting(settings map[string]interface{}, key string, target interface{}) bool {
	v, ok := settings[key]
	if !ok {
		return false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	return json.Unmarshal(b, target) == nil
}

func (p *MusicPlugin) persistSettings(mutate func(map[string]interface{})) {
	cfg := p.storage.GetPluginConfig(p.Name())
	if cfg.Settings == nil {
		cfg.Settings = map[string]interface{}{}
	}
	mutate(cfg.Settings)
	if err := p.storage.SetPluginConfig(p.Name(), cfg); err != nil {
		log.Printf("[Music] persist failed: %v", err)
	}
}

func (p *MusicPlugin) getTrackImage(t *MusicTrack) string {
	if img := pickLastfmImage(t); img != "" {
		return img
	}
	return p.getTrackImageFromCache(t)
}

func (p *MusicPlugin) getTrackImageFromCache(t *MusicTrack) string {
	key := cacheKey(t.Artist.Text, t.Album.Text)
	p.imageCacheMutex.RLock()
	entry, ok := p.imageCache[key]
	p.imageCacheMutex.RUnlock()
	if ok {
		return entry.URL
	}
	return ""
}

func (p *MusicPlugin) imageForVM(artist, name, fallback string) string {
	key := strings.ToLower(strings.TrimSpace(artist) + "|" + strings.TrimSpace(name))
	p.imgLocalMu.RLock()
	local := p.imgLocal[key]
	p.imgLocalMu.RUnlock()
	if local != "" {
		return local
	}
	return fallback
}

func imageOrPlaceholder(img string) string {
	if strings.TrimSpace(img) == "" {
		return musicPlaceholderImage
	}
	return img
}

func pickLastfmImage(t *MusicTrack) string {
	order := map[string]int{"small": 1, "medium": 2, "large": 3, "extralarge": 4}
	best, bestRank := "", 0
	for _, img := range t.Image {
		u := strings.TrimSpace(img.Text)
		if u == "" || strings.Contains(u, "2a96cbd8b46e442fc41c2b86b821562f") {
			continue
		}
		if r := order[img.Size]; r >= bestRank {
			best, bestRank = u, r
		}
	}
	return best
}

func cacheKey(artist, album string) string {
	return strings.ToLower(strings.TrimSpace(artist) + "|" + strings.TrimSpace(album))
}

func (p *MusicPlugin) prefetchArtwork(tracks []MusicTrack) {
	if !p.spotify.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	updated := false
	for i := range tracks {
		t := &tracks[i]
		if pickLastfmImage(t) != "" {
			continue
		}
		key := cacheKey(t.Artist.Text, t.Album.Text)
		p.imageCacheMutex.RLock()
		_, cached := p.imageCache[key]
		p.imageCacheMutex.RUnlock()
		if cached {
			continue
		}
		var img string
		if strings.TrimSpace(t.Album.Text) != "" {
			img, _ = p.spotify.AlbumImage(ctx, t.Artist.Text, t.Album.Text)
		}
		if img == "" {
			img, _ = p.spotify.ArtistImage(ctx, t.Artist.Text)
		}
		if img == "" {
			continue
		}
		p.imageCacheMutex.Lock()
		p.imageCache[key] = imageCacheEntry{URL: img, CachedAt: time.Now().Unix()}
		p.imageCacheMutex.Unlock()
		updated = true
	}
	if updated {
		p.saveImageCache()
	}
}

func (p *MusicPlugin) relativePlayedAt(t *MusicTrack) string {
	if t == nil || strings.TrimSpace(t.Date.Uts) == "" {
		return "recently"
	}
	return p.getRelativeTimeForTrack(t)
}

func (p *MusicPlugin) getRelativeTimeForTrack(t *MusicTrack) string {
	if strings.TrimSpace(t.Date.Uts) == "" {
		return "just now"
	}
	sec, err := strconv.ParseInt(t.Date.Uts, 10, 64)
	if err != nil {
		return "recently"
	}
	d := time.Since(time.Unix(sec, 0))
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return time.Unix(sec, 0).Format("Jan 2")
	}
}

func (p *MusicPlugin) refreshLoved(ctx context.Context, username string) {
	if p.apiKey == "" || strings.TrimSpace(username) == "" {
		return
	}
	tracks, err := p.lastfm.LovedTracks(ctx, username, 20)
	if err != nil {
		log.Printf("[Music] loved fetch: %v", err)
		return
	}
	set := make(map[string]struct{}, len(tracks))
	for _, t := range tracks {
		set[lovedKey(t.Artist, t.Name)] = struct{}{}
	}
	p.lovedMu.Lock()
	p.lovedSet = set
	p.lovedCount = len(set)
	count := p.lovedCount
	p.lovedMu.Unlock()

	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	p.persistSettings(func(s map[string]interface{}) {
		s["lovedCache"] = map[string]interface{}{"keys": keys, "count": count, "at": time.Now().Unix()}
	})
}

func (p *MusicPlugin) isLovedTrack(artist, name string) bool {
	k := lovedKey(artist, name)
	p.lovedMu.RLock()
	_, ok := p.lovedSet[k]
	p.lovedMu.RUnlock()
	return ok
}

func (p *MusicPlugin) loadLoved() {
	cfg := p.storage.GetPluginConfig(p.Name())
	var cache struct {
		Keys  []string `json:"keys"`
		Count int      `json:"count"`
	}
	if !decodeSetting(cfg.Settings, "lovedCache", &cache) {
		return
	}
	set := make(map[string]struct{}, len(cache.Keys))
	for _, k := range cache.Keys {
		set[k] = struct{}{}
	}
	p.lovedMu.Lock()
	p.lovedSet = set
	p.lovedCount = cache.Count
	if p.lovedCount == 0 {
		p.lovedCount = len(set)
	}
	p.lovedMu.Unlock()
}

func lovedKey(artist, name string) string {
	return strings.ToLower(strings.TrimSpace(artist)) + "|" + strings.ToLower(strings.TrimSpace(name))
}

func (p *MusicPlugin) refreshSpotifySavedIfStale() {
	if !p.spotify.UserEnabled() {
		return
	}
	p.spSavedMu.RLock()
	fresh := len(p.spSavedCache) > 0 && time.Since(p.spSavedAt) < 6*time.Hour
	p.spSavedMu.RUnlock()
	if fresh {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tracks, total, err := p.spotify.SavedTracks(ctx)
	if err != nil {
		log.Printf("[Music] spotify saved refresh: %v", err)
		return
	}
	set := make(map[string]struct{}, len(tracks)*2)
	for _, t := range tracks {
		for _, a := range t.Artists {
			set[likedKey(a, t.Name)] = struct{}{}
		}
	}
	if total == 0 {
		total = len(tracks)
	}
	p.spSavedMu.Lock()
	p.spSavedCache = set
	p.spLikedCount = total
	p.spSavedAt = time.Now()
	p.spSavedMu.Unlock()

	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	p.persistSettings(func(s map[string]interface{}) {
		s["spotifySavedCache"] = map[string]interface{}{"keys": keys, "count": total, "at": time.Now().Unix()}
	})
}

func (p *MusicPlugin) isTrackLikedCached(artist, name string) bool {
	p.spSavedMu.RLock()
	defer p.spSavedMu.RUnlock()
	if len(p.spSavedCache) == 0 {
		return false
	}
	if _, ok := p.spSavedCache[likedKey(artist, name)]; ok {
		return true
	}
	for _, a := range splitArtists(artist) {
		if _, ok := p.spSavedCache[likedKey(a, name)]; ok {
			return true
		}
	}
	return false
}

func (p *MusicPlugin) loadSpotifySavedCache() {
	cfg := p.storage.GetPluginConfig(p.Name())
	var cache struct {
		Keys  []string `json:"keys"`
		Count int      `json:"count"`
		At    int64    `json:"at"`
	}
	if !decodeSetting(cfg.Settings, "spotifySavedCache", &cache) {
		return
	}
	set := make(map[string]struct{}, len(cache.Keys))
	for _, k := range cache.Keys {
		set[k] = struct{}{}
	}
	p.spSavedMu.Lock()
	p.spSavedCache = set
	p.spLikedCount = cache.Count
	if cache.At > 0 {
		p.spSavedAt = time.Unix(cache.At, 0)
	}
	p.spSavedMu.Unlock()
}

func (p *MusicPlugin) spotifyNowCached() *SpotifyNowPlaying {
	if !p.spotify.UserEnabled() {
		return nil
	}
	p.spNowMu.Lock()
	defer p.spNowMu.Unlock()
	if p.spNow != nil && time.Since(p.spNowAt) < 5*time.Second {
		return p.spNow
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	now, err := p.spotify.CurrentlyPlaying(ctx)
	if err != nil {
		return p.spNow
	}
	p.spNow = now
	p.spNowAt = time.Now()
	return now
}

func likedKey(artist, name string) string {
	return strings.ToLower(strings.TrimSpace(artist)) + "|" + strings.ToLower(strings.TrimSpace(name))
}

func splitArtists(artist string) []string {
	seps := []string{",", " feat.", " ft.", " & ", " x "}
	parts := []string{artist}
	for _, sep := range seps {
		var next []string
		for _, part := range parts {
			for _, sub := range strings.Split(part, sep) {
				if s := strings.TrimSpace(sub); s != "" {
					next = append(next, s)
				}
			}
		}
		parts = next
	}
	return parts
}

func (p *MusicPlugin) loadImageCache() {
	cfg := p.storage.GetPluginConfig(p.Name())
	cache := make(map[string]imageCacheEntry)
	if decodeSetting(cfg.Settings, "imageCache", &cache) {
		p.imageCacheMutex.Lock()
		p.imageCache = cache
		p.imageCacheMutex.Unlock()
	}
}

func (p *MusicPlugin) saveImageCache() {
	p.imageCacheMutex.RLock()
	snapshot := make(map[string]imageCacheEntry, len(p.imageCache))
	for k, v := range p.imageCache {
		snapshot[k] = v
	}
	p.imageCacheMutex.RUnlock()
	p.persistSettings(func(s map[string]interface{}) {
		s["imageCache"] = snapshot
	})
}

func (p *MusicPlugin) loadImgLocalCache() {
	cfg := p.storage.GetPluginConfig(p.Name())
	cache := make(map[string]string)
	if decodeSetting(cfg.Settings, "imgLocalCache", &cache) {
		p.imgLocalMu.Lock()
		p.imgLocal = cache
		p.imgLocalMu.Unlock()
	}
}

func sameTrack(now *SpotifyNowPlaying, t *MusicTrack) bool {
	if now == nil || t == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(now.Name), strings.TrimSpace(t.Name)) {
		return false
	}
	for _, a := range now.Artists {
		if strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(t.Artist.Text)) {
			return true
		}
	}
	return len(now.Artists) == 0
}

func paletteFor(key string) (string, string) {
	colors := []string{"#4d9fff", "#b055ff", "#10d060", "#f0a010", "#ff5c7a", "#22c7c7"}
	var sum int
	for _, r := range key {
		sum += int(r)
	}
	color := colors[sum%len(colors)]
	initial := "?"
	for _, r := range key {
		if r != '|' && strings.TrimSpace(string(r)) != "" {
			initial = strings.ToUpper(string(r))
			break
		}
	}
	return color, initial
}

func formatDelta(cur, prev int) (string, bool) {
	if prev <= 0 {
		if cur > 0 {
			return "new", true
		}
		return "", true
	}
	diff := cur - prev
	pct := int(math.Round(float64(diff) / float64(prev) * 100))
	if pct >= 0 {
		return fmt.Sprintf("+%d%%", pct), true
	}
	return fmt.Sprintf("%d%%", pct), false
}

func formatCount(n int) string {
	s := strconv.Itoa(n)
	if n < 1000 {
		return s
	}
	var out []byte
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func formatScrobbles(playcount string) string {
	n, err := strconv.Atoi(playcount)
	if err != nil {
		return playcount
	}
	return formatCount(n)
}

func sparkFor(idx int, weekly [7]int) []int {
	out := make([]int, 7)
	for i := 0; i < 7; i++ {
		out[i] = weekly[(i+idx)%7]
	}
	return out
}

func barsSpark(vals []int) string {
	if len(vals) == 0 {
		return ""
	}
	maxV := 1
	for _, v := range vals {
		if v > maxV {
			maxV = v
		}
	}
	const w, h = 100.0, 28.0
	step := w
	if len(vals) > 1 {
		step = w / float64(len(vals)-1)
	} else {
		step = 0
	}
	var b strings.Builder
	for i, v := range vals {
		x := float64(i) * step
		y := h - (float64(v)/float64(maxV))*h
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%.1f,%.1f", x, y)
	}
	return b.String()
}

func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

func (p *MusicPlugin) HandleStatsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if p.apiKey == "" {
		http.Error(w, `{"error":"not configured"}`, http.StatusServiceUnavailable)
		return
	}
	cfg := p.storage.GetPluginConfig(p.Name())
	username, _ := cfg.Settings["username"].(string)
	if strings.TrimSpace(username) == "" {
		http.Error(w, `{"error":"no username"}`, http.StatusServiceUnavailable)
		return
	}
	period := normalizePeriod(r.URL.Query().Get("period"))

	if err := p.setStatsPeriodSetting(period); err != nil {
		log.Printf("[Music] persist period failed: %v", err)
	}

	if !p.statsFresh(period) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Minute)
		defer cancel()
		if st, err := p.computeStats(ctx, username, period); err == nil {
			p.storeStats(period, st)
		} else {
			log.Printf("[Music] stats api compute (%s): %v", period, err)
		}
	}

	st := p.currentStats(period)
	if st == nil || statsEmpty(st) {
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
		if fresh, err := p.computeStats(ctx, username, period); err == nil && !statsEmpty(fresh) {
			p.storeStats(period, fresh)
			st = fresh
		}
		cancel()
	}
	if st == nil {
		http.Error(w, `{"error":"no stats"}`, http.StatusServiceUnavailable)
		return
	}
	if st.Scrobbles.Cur == 0 {
		p.refreshScrobbleCount(username, period)
		st = p.currentStats(period)
	}

	p.trackMutex.RLock()
	userInfo := p.userInfo
	p.trackMutex.RUnlock()

	weeklyBars, weeklyDays := buildWeekly(st)
	payload := map[string]interface{}{
		"period":     period,
		"totals":     p.buildTotals(st, userInfo, period),
		"topArtists": buildCovers(st.TopArtists),
		"topAlbums":  buildCovers(st.TopAlbums),
		"tags":       buildTags(st.Tags, periodScrobbleTotal(st, userInfo, period)),
		"weeklyBars": weeklyBars,
		"weeklyDays": weeklyDays,
		"weeklyPeak": st.PeakDay,
		"todayIndex": todayWeeklyIndex(period),
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func (p *MusicPlugin) setStatsPeriodSetting(period string) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	if cfg.Settings == nil {
		cfg.Settings = map[string]interface{}{}
	}
	ui, _ := cfg.Settings["ui"].(map[string]interface{})
	if ui == nil {
		ui = map[string]interface{}{}
		cfg.Settings["ui"] = ui
	}
	ui["statsPeriod"] = period
	return p.storage.SetPluginConfig(p.Name(), cfg)
}

func (p *MusicPlugin) GetSettings() map[string]interface{} {
	return p.storage.GetPluginConfig(p.Name()).Settings
}

func (p *MusicPlugin) SetSettings(settings map[string]interface{}) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	cfg.Settings = settings
	if err := p.storage.SetPluginConfig(p.Name(), cfg); err != nil {
		return err
	}
	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(),
		"action": "settings_changed",
	})
	return nil
}

func normalizePeriod(v string) string {
	switch v {
	case "7day", "1month", "3month", "6month", "12month", "overall":
		return v
	default:
		return musicDefaultPeriod
	}
}

func buildPeriodOpts(sel string) []periodOpt {
	out := make([]periodOpt, 0, len(musicPeriods))
	for _, pr := range musicPeriods {
		out = append(out, periodOpt{Value: pr.Value, Label: pr.Label, Selected: pr.Value == sel})
	}
	return out
}

func (p *MusicPlugin) statsFresh(period string) bool {
	p.statsMutex.RLock()
	defer p.statsMutex.RUnlock()
	st := p.statsByPeriod[period]
	if st == nil || statsIncomplete(st) {
		return false
	}
	return time.Since(p.statsAtByPeriod[period]) < 24*time.Hour
}

func (p *MusicPlugin) currentStats(period string) *musicStats {
	p.statsMutex.RLock()
	defer p.statsMutex.RUnlock()
	return p.statsByPeriod[period]
}

func periodBounds(period string, now time.Time) (curFrom, curTo, prevFrom, prevTo int64, ok bool) {
	curTo = now.Unix()
	var d time.Duration
	switch period {
	case "7day":
		d = 7 * 24 * time.Hour
	case "1month":
		d = 30 * 24 * time.Hour
	case "3month":
		d = 90 * 24 * time.Hour
	case "6month":
		d = 180 * 24 * time.Hour
	case "12month":
		d = 365 * 24 * time.Hour
	default:
		return 0, 0, 0, 0, false
	}
	curFrom = now.Add(-d).Unix()
	prevTo = curFrom
	prevFrom = now.Add(-2 * d).Unix()
	return curFrom, curTo, prevFrom, prevTo, true
}

func countWindow(tracks []windowedTrack) (scrobbles, uniqTracks, uniqArtists, uniqAlbums int) {
	tr := map[string]struct{}{}
	ar := map[string]struct{}{}
	al := map[string]struct{}{}
	for _, t := range tracks {
		scrobbles++
		tr[lovedKey(t.artist, t.name)] = struct{}{}
		ar[strings.ToLower(strings.TrimSpace(t.artist))] = struct{}{}
		if strings.TrimSpace(t.album) != "" {
			al[cacheKey(t.artist, t.album)] = struct{}{}
		}
	}
	return scrobbles, len(tr), len(ar), len(al)
}

func (p *MusicPlugin) storeStats(period string, st *musicStats) {
	p.statsMutex.Lock()
	if prev := p.statsByPeriod[period]; prev != nil && statsIncomplete(st) && !statsIncomplete(prev) {
		p.statsMutex.Unlock()
		log.Printf("[Music] ignoring incomplete %s update (topArtists=%d), keeping cached", period, len(st.TopArtists))
		return
	}
	p.statsByPeriod[period] = st
	p.statsAtByPeriod[period] = time.Now()
	p.statsMutex.Unlock()
	p.saveStats()
	log.Printf("[Music] stats persisted (%s): scrobbles=%d tracks=%d artists=%d albums=%d topArtists=%d topAlbums=%d genres=%d",
		period, st.Scrobbles.Cur, st.Tracks.Cur, st.Artists.Cur, st.Albums.Cur,
		len(st.TopArtists), len(st.TopAlbums), len(st.Tags))
}

func (p *MusicPlugin) scheduleStatsRefresh(username string) {
	if p.apiKey == "" || strings.TrimSpace(username) == "" {
		return
	}
	p.ensureAllPeriods(username, p.visiblePeriod())
}

func (p *MusicPlugin) ensureAllPeriods(username, priority string) {
	if p.apiKey == "" || strings.TrimSpace(username) == "" {
		return
	}
	if !atomic.CompareAndSwapInt32(&p.statsRunning, 0, 1) {
		return
	}
	go func() {
		defer atomic.StoreInt32(&p.statsRunning, 0)

		for _, period := range periodsPrioritized(priority) {
			if p.statsFresh(period) {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
			log.Printf("[Music] stats refresh starting (period=%s, user=%s)", period, username)
			start := time.Now()
			st, err := p.computeStats(ctx, username, period)
			cancel()
			if err != nil {
				log.Printf("[Music] stats refresh failed (%s): %v", period, err)
				continue
			}
			p.storeStats(period, st)
			log.Printf("[Music] stats refresh done (%s) in %v", period, time.Since(start))
			p.hub.Broadcast("music_stats", p.statsPayload(st))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		p.refreshLoved(ctx, username)
		cancel()
	}()
}

func periodsPrioritized(priority string) []string {
	out := make([]string, 0, len(musicPeriods))
	if priority != "" {
		out = append(out, priority)
	}
	for _, pr := range musicPeriods {
		if pr.Value == priority {
			continue
		}
		out = append(out, pr.Value)
	}
	return out
}

func scaleDelta(accurateCur, sampledCur, sampledPrev int) int {
	if sampledCur <= 0 {
		return sampledPrev
	}
	ratio := float64(sampledPrev) / float64(sampledCur)
	return int(math.Round(float64(accurateCur) * ratio))
}

func (p *MusicPlugin) loadStats() {
	cfg := p.storage.GetPluginConfig(p.Name())
	var cache map[string]*musicStats
	if !decodeSetting(cfg.Settings, "statsCacheV4", &cache) {
		return
	}
	p.statsMutex.Lock()
	loaded := 0
	for period, st := range cache {
		if st == nil || statsEmpty(st) {
			continue
		}
		p.statsByPeriod[period] = st
		if st.FetchedAt > 0 {
			p.statsAtByPeriod[period] = time.Unix(st.FetchedAt, 0)
		}
		loaded++
		suffix := ""
		if statsIncomplete(st) {
			suffix = " (incomplete, will refresh)"
		}
		log.Printf("[Music] loaded %-7s scrobbles=%d(prev %d) tracks=%d(prev %d) artists=%d(prev %d) albums=%d(prev %d) topArtists=%d topAlbums=%d genres=%d peak=%s%s",
			period, st.Scrobbles.Cur, st.Scrobbles.Prev, st.Tracks.Cur, st.Tracks.Prev,
			st.Artists.Cur, st.Artists.Prev, st.Albums.Cur, st.Albums.Prev,
			len(st.TopArtists), len(st.TopAlbums), len(st.Tags), st.PeakDay, suffix)
	}
	p.statsMutex.Unlock()
	log.Printf("[Music] loaded %d cached stat periods", loaded)
}

func todayWeeklyIndex(period string) int {
	if period == "7day" {
		return 6
	}
	return (int(time.Now().Weekday()) + 6) % 7
}

func statsIncomplete(st *musicStats) bool {
	if st == nil {
		return true
	}
	if st.Scrobbles.Cur == 0 {
		return false
	}
	return st.Tracks.Cur == 0 || st.Artists.Cur == 0 || st.Albums.Cur == 0 ||
		len(st.TopArtists) == 0 || len(st.TopAlbums) == 0
}

func statsEmpty(st *musicStats) bool {
	return st.Scrobbles.Cur == 0 &&
		st.Tracks.Cur == 0 &&
		st.Artists.Cur == 0 &&
		st.Albums.Cur == 0 &&
		len(st.TopArtists) == 0 &&
		len(st.TopAlbums) == 0 &&
		len(st.Tags) == 0
}

func (p *MusicPlugin) saveStats() {
	p.statsMutex.RLock()
	snapshot := make(map[string]*musicStats, len(p.statsByPeriod))
	for period, st := range p.statsByPeriod {
		snapshot[period] = st
	}
	p.statsMutex.RUnlock()
	p.persistSettings(func(s map[string]interface{}) {
		delete(s, "statsCache")
		delete(s, "statsCacheV2")
		delete(s, "statsCacheV3")
		s["statsCacheV4"] = snapshot
	})
}

func (p *MusicPlugin) statsPayload(st *musicStats) map[string]interface{} {
	return map[string]interface{}{
		"scrobbles":  st.Scrobbles,
		"tracks":     st.Tracks,
		"artists":    st.Artists,
		"albums":     st.Albums,
		"weekly":     st.Weekly,
		"peakDay":    st.PeakDay,
		"topArtists": st.TopArtists,
		"topAlbums":  st.TopAlbums,
		"tags":       st.Tags,
		"period":     st.Period,
	}
}

type musicRuntimeCache struct {
	CurrentTrack *MusicTrack  `json:"currentTrack"`
	RecentTracks []MusicTrack `json:"recentTracks"`
	UserInfo     *MusicUser   `json:"userInfo"`
	TrackLength  int          `json:"trackLength"`
	SavedAt      int64        `json:"savedAt"`
}

func (p *MusicPlugin) loadRuntimeCache() {
	cfg := p.storage.GetPluginConfig(p.Name())
	var c musicRuntimeCache
	if !decodeSetting(cfg.Settings, "runtimeCache", &c) {
		return
	}
	p.trackMutex.Lock()
	p.currentTrack = c.CurrentTrack
	p.recentTracks = c.RecentTracks
	p.userInfo = c.UserInfo
	p.lastTrackLength = c.TrackLength
	p.currentIsPlaying = false
	p.trackMutex.Unlock()
	log.Printf("[Music] loaded runtime cache (track=%q, recent=%d, saved %s ago)",
		trackTitle(c.CurrentTrack), len(c.RecentTracks), sinceUnix(c.SavedAt))
}

func (p *MusicPlugin) saveRuntimeCache() {
	p.trackMutex.RLock()
	c := musicRuntimeCache{
		CurrentTrack: p.currentTrack,
		RecentTracks: p.recentTracks,
		UserInfo:     p.userInfo,
		TrackLength:  p.lastTrackLength,
		SavedAt:      time.Now().Unix(),
	}
	p.trackMutex.RUnlock()
	if c.CurrentTrack == nil {
		return
	}
	p.persistSettings(func(s map[string]interface{}) {
		s["runtimeCache"] = c
	})
}

func trackTitle(t *MusicTrack) string {
	if t == nil {
		return ""
	}
	return t.Artist.Text + " – " + t.Name
}

func sinceUnix(u int64) string {
	if u <= 0 {
		return "never"
	}
	return time.Since(time.Unix(u, 0)).Round(time.Second).String()
}

type akarpovAuthorSearch struct {
	Results []struct {
		Name         string `json:"name"`
		Slug         string `json:"slug"`
		ImageCropped string `json:"image_cropped"`
	} `json:"results"`
}

type akarpovAlbumSearch struct {
	Results []struct {
		Name         string `json:"name"`
		Slug         string `json:"slug"`
		ImageCropped string `json:"image_cropped"`
	} `json:"results"`
}

func akarpovImageURL(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http") {
		return raw
	}
	return "https://new.akarpov.ru" + raw
}

func (p *MusicPlugin) akarpovArtistImage(ctx context.Context, artist string) string {
	artist = strings.TrimSpace(artist)
	if artist == "" {
		return ""
	}
	u := "https://new.akarpov.ru/api/v1/music/authors/?search=" + url.QueryEscape(artist)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var res akarpovAuthorSearch
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return ""
	}
	want := normalizeMatch(artist)
	for _, r := range res.Results {
		if normalizeMatch(r.Name) == want && r.ImageCropped != "" {
			return akarpovImageURL(r.ImageCropped)
		}
	}
	if len(res.Results) > 0 && res.Results[0].ImageCropped != "" {
		return akarpovImageURL(res.Results[0].ImageCropped)
	}
	return ""
}

func (p *MusicPlugin) akarpovAlbumImage(ctx context.Context, artist, album string) string {
	album = strings.TrimSpace(album)
	if album == "" {
		return ""
	}
	u := "https://new.akarpov.ru/api/v1/music/albums/?search=" + url.QueryEscape(album)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var res akarpovAlbumSearch
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return ""
	}
	want := normalizeMatch(album)
	for _, r := range res.Results {
		if normalizeMatch(r.Name) == want && r.ImageCropped != "" {
			return akarpovImageURL(r.ImageCropped)
		}
	}
	if len(res.Results) > 0 && res.Results[0].ImageCropped != "" {
		return akarpovImageURL(res.Results[0].ImageCropped)
	}
	return ""
}

func normalizeMatch(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
