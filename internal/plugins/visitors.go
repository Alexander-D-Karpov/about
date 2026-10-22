package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

const (
	visitorsHistoryDays  = 365
	visitorsGeoQueueSize = 256
	visitorsGeoCacheCap  = 10000
	visitorsTrendDays    = 30
)

type VisitorsPlugin struct {
	storage      *storage.Storage
	hub          *stream.Hub
	visitCount   int64
	todayCount   int64
	botCount     int64
	currentDay   string
	dataPath     string
	mutex        sync.RWMutex
	saveMu       sync.Mutex
	lastSaveTime time.Time
	dailyStats   map[string]int64
	dailyUnique  map[string]int64
	uniqueToday  map[string]struct{}
	countryStats map[string]int64

	geoQueue   chan string
	geoCache   map[string]string
	geoCacheMu sync.Mutex
	geoClient  *http.Client
}

type VisitorsData struct {
	TotalVisits  int64            `json:"total_visits"`
	TodayVisits  int64            `json:"today_visits"`
	TodayBots    int64            `json:"today_bots"`
	CurrentDay   string           `json:"current_day"`
	LastUpdate   time.Time        `json:"last_update"`
	DailyStats   map[string]int64 `json:"daily_stats"`
	DailyUnique  map[string]int64 `json:"daily_unique"`
	UniqueHashes []string         `json:"unique_hashes"`
	CountryStats map[string]int64 `json:"country_stats"`
	MonthlyStats map[string]int64 `json:"monthly_stats"`
}

var botUAFragments = []string{
	"bot", "crawler", "spider", "slurp", "bingpreview", "facebookexternalhit",
	"headless", "lighthouse", "pingdom", "uptimerobot", "statuscake", "monitor",
	"python-requests", "python-urllib", "go-http-client", "wget",
	"scrapy", "httpclient", "okhttp", "java/", "libwww", "phantomjs",
}

func isBotUA(ua string) bool {
	ua = strings.ToLower(strings.TrimSpace(ua))
	if ua == "" {
		return true
	}
	for _, f := range botUAFragments {
		if strings.Contains(ua, f) {
			return true
		}
	}
	return false
}

func normalizeVisitorIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if host, _, err := net.SplitHostPort(ip); err == nil {
		return host
	}
	return ip
}

func isLoopbackIP(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.IsLoopback()
}

func visitorHash(ip, ua string) string {
	sum := sha256.Sum256([]byte(normalizeVisitorIP(ip) + "|" + ua))
	return hex.EncodeToString(sum[:6])
}

func isPublicIP(ipStr string) bool {
	ip := net.ParseIP(normalizeVisitorIP(ipStr))
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

func NewVisitorsPlugin(storage *storage.Storage, hub *stream.Hub, dataPath string) *VisitorsPlugin {
	plugin := &VisitorsPlugin{
		storage:      storage,
		hub:          hub,
		dataPath:     dataPath,
		dailyStats:   make(map[string]int64),
		dailyUnique:  make(map[string]int64),
		uniqueToday:  make(map[string]struct{}),
		countryStats: make(map[string]int64),
		currentDay:   time.Now().Format("2006-01-02"),
		geoQueue:     make(chan string, visitorsGeoQueueSize),
		geoCache:     make(map[string]string),
		geoClient:    NewHTTPClientWithTimeout(8 * time.Second),
	}

	log.Printf("[Visitors] Initializing plugin, data path: %s", dataPath)

	plugin.loadVisitorsData()

	log.Printf("[Visitors] Loaded data: total=%d, today=%d, unique_today=%d, countries=%d, current_day=%s",
		plugin.visitCount, plugin.todayCount, len(plugin.uniqueToday), len(plugin.countryStats), plugin.currentDay)

	plugin.checkDayTransition()

	go plugin.startDayChecker()
	go plugin.geoWorker()

	return plugin
}

func (p *VisitorsPlugin) Name() string {
	return "visitors"
}

func (p *VisitorsPlugin) geoWorker() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Visitors] geo worker panic recovered: %v", r)
			time.Sleep(30 * time.Second)
			go p.geoWorker()
		}
	}()

	for ip := range p.geoQueue {
		cc := p.resolveCountry(ip)
		if cc == "" {
			continue
		}

		p.mutex.Lock()
		p.countryStats[cc]++
		p.mutex.Unlock()

		p.broadcastRegionsUpdate()

		time.Sleep(1500 * time.Millisecond)
	}
}

func (p *VisitorsPlugin) broadcastRegionsUpdate() {
	p.mutex.RLock()
	countries := make(map[string]int64, len(p.countryStats))
	for k, v := range p.countryStats {
		if v > 0 {
			countries[k] = v
		}
	}
	p.mutex.RUnlock()

	p.hub.Broadcast("visitors_regions_update", map[string]interface{}{
		"countries": countries,
		"timestamp": time.Now().Unix(),
	})
}

func (p *VisitorsPlugin) HandleRegionsAPI(w http.ResponseWriter, r *http.Request) {
	p.mutex.RLock()
	countries := make(map[string]int64, len(p.countryStats))
	for k, v := range p.countryStats {
		if v > 0 {
			countries[k] = v
		}
	}
	p.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(countries)
}

func (p *VisitorsPlugin) resolveCountry(ip string) string {
	p.geoCacheMu.Lock()
	if cc, ok := p.geoCache[ip]; ok {
		p.geoCacheMu.Unlock()
		return cc
	}
	p.geoCacheMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://ip-api.com/json/"+ip+"?fields=status,countryCode,message", nil)
	if err != nil {
		log.Printf("[Visitors] geo request build failed for %s: %v", ip, err)
		return ""
	}

	resp, err := p.geoClient.Do(req)
	if err != nil {
		log.Printf("[Visitors] geo lookup failed for %s: %v", ip, err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Visitors] geo lookup for %s returned status %d", ip, resp.StatusCode)
		return ""
	}

	var body struct {
		Status      string `json:"status"`
		CountryCode string `json:"countryCode"`
		Message     string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		log.Printf("[Visitors] geo decode failed for %s: %v", ip, err)
		return ""
	}
	if body.Status != "success" || len(body.CountryCode) != 2 {
		log.Printf("[Visitors] geo lookup for %s unsuccessful: status=%s msg=%s", ip, body.Status, body.Message)
		return ""
	}

	cc := strings.ToUpper(body.CountryCode)

	p.geoCacheMu.Lock()
	if len(p.geoCache) >= visitorsGeoCacheCap {
		p.geoCache = make(map[string]string)
	}
	p.geoCache[ip] = cc
	p.geoCacheMu.Unlock()

	return cc
}

func (p *VisitorsPlugin) enqueueGeo(ip string) {
	ip = normalizeVisitorIP(ip)
	if !isPublicIP(ip) {
		log.Printf("[Visitors] skipping geo for non-public IP: %s", ip)
		return
	}
	select {
	case p.geoQueue <- ip:
	default:
		log.Printf("[Visitors] geo queue full, dropping IP: %s", ip)
	}
}

func (p *VisitorsPlugin) startDayChecker() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		p.checkDayTransition()
	}
}

func (p *VisitorsPlugin) rolloverLocked(today string) {
	if p.currentDay != "" && p.todayCount > 0 {
		p.dailyStats[p.currentDay] = p.todayCount
	}
	if p.currentDay != "" && len(p.uniqueToday) > 0 {
		p.dailyUnique[p.currentDay] = int64(len(p.uniqueToday))
	}
	p.currentDay = today
	p.todayCount = 0
	p.botCount = 0
	p.uniqueToday = make(map[string]struct{})
	p.pruneHistoryLocked(visitorsHistoryDays)
}

func (p *VisitorsPlugin) pruneHistoryLocked(keepDays int) {
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
	for date := range p.dailyStats {
		if date < cutoff {
			delete(p.dailyStats, date)
		}
	}
	for date := range p.dailyUnique {
		if date < cutoff {
			delete(p.dailyUnique, date)
		}
	}
}

func (p *VisitorsPlugin) checkDayTransition() {
	p.mutex.Lock()
	today := time.Now().Format("2006-01-02")
	needsSave := false

	if today != p.currentDay {
		log.Printf("[Visitors] Day transition detected: %s -> %s (today count: %d)", p.currentDay, today, p.todayCount)
		p.rolloverLocked(today)
		needsSave = true
	}
	p.mutex.Unlock()

	if needsSave {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := p.saveVisitorsDataAsync(ctx); err != nil {
				log.Printf("[Visitors] ERROR: Failed to save day transition data: %v", err)
			}

			p.broadcastUpdate()
		}()
	}
}

func (p *VisitorsPlugin) saveVisitorsDataAsync(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	p.mutex.RLock()
	visitorsData := VisitorsData{
		TotalVisits:  p.visitCount,
		TodayVisits:  p.todayCount,
		TodayBots:    p.botCount,
		CurrentDay:   p.currentDay,
		LastUpdate:   time.Now(),
		DailyStats:   make(map[string]int64, len(p.dailyStats)),
		DailyUnique:  make(map[string]int64, len(p.dailyUnique)),
		UniqueHashes: make([]string, 0, len(p.uniqueToday)),
		CountryStats: make(map[string]int64, len(p.countryStats)),
		MonthlyStats: make(map[string]int64),
	}

	for k, v := range p.dailyStats {
		visitorsData.DailyStats[k] = v
	}
	for k, v := range p.dailyUnique {
		visitorsData.DailyUnique[k] = v
	}
	for k, v := range p.countryStats {
		visitorsData.CountryStats[k] = v
	}
	for h := range p.uniqueToday {
		visitorsData.UniqueHashes = append(visitorsData.UniqueHashes, h)
	}

	month := time.Now().Format("2006-01")
	visitorsData.MonthlyStats[month] = p.visitCount
	p.mutex.RUnlock()

	data, err := json.MarshalIndent(visitorsData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	dataFile := filepath.Join(p.dataPath, "visitors.json")
	if err := os.MkdirAll(filepath.Dir(dataFile), 0755); err != nil {
		return fmt.Errorf("create dir error: %w", err)
	}

	// Serialize the whole write. Saves fire from several goroutines (per visit,
	// day rollover, the periodic saver); the old code shared one fixed
	// "visitors.json.tmp" across them, so their writes interleaved into that one
	// file and their renames raced — which is how the file got corrupted and the
	// history was lost. One writer at a time, each with its own temp file.
	p.saveMu.Lock()
	defer p.saveMu.Unlock()

	// Keep the last good copy as a backup before overwriting, so a future
	// corruption is always recoverable (see loadVisitorsData). Only rotate when
	// the current file still parses, so we never overwrite a good backup with a
	// bad primary.
	if cur, err := os.ReadFile(dataFile); err == nil {
		var check VisitorsData
		if json.Unmarshal(cur, &check) == nil {
			_ = os.WriteFile(dataFile+".bak", cur, 0644)
		}
	}

	tmp, err := os.CreateTemp(p.dataPath, "visitors-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp file error: %w", err)
	}
	tempName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tempName)
		return fmt.Errorf("write temp file error: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tempName)
		return fmt.Errorf("sync temp file error: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("close temp file error: %w", err)
	}

	if err := os.Rename(tempName, dataFile); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("rename file error: %w", err)
	}

	return nil
}

type visitorsBar struct {
	Title  string
	Height int // percent of the busiest day
	Today  bool
}

type visitorsTrend struct {
	Bars       []visitorsBar
	FromLabel  string
	Peak       string
	PeakDate   string
	TrendLabel string
	TrendDir   string // "up", "down", "flat" or "" when there is no previous week to compare
}

// buildTrendLocked charts unique visitors per day for the last `days` days. Unique rather
// than raw visits: a single client polling the page used to add ~720 "visits" a day, which
// flattened every real day to nothing. History starts at the first day with data.
func (p *VisitorsPlugin) buildTrendLocked(days int) *visitorsTrend {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	on := func(d time.Time) (visits, unique int64) {
		date := d.Format("2006-01-02")
		if date == p.currentDay {
			return p.todayCount, int64(len(p.uniqueToday))
		}
		return p.dailyStats[date], p.dailyUnique[date]
	}

	start := today.AddDate(0, 0, -(days - 1))
	for !start.After(today) {
		if _, u := on(start); u > 0 {
			break
		}
		start = start.AddDate(0, 0, 1)
	}
	if start.After(today) {
		return nil
	}

	var peak int64
	var peakDay time.Time
	for d := start; !d.After(today); d = d.AddDate(0, 0, 1) {
		if _, u := on(d); u > peak {
			peak, peakDay = u, d
		}
	}

	t := &visitorsTrend{
		FromLabel: start.Format("Jan 2"),
		Peak:      formatNumberWithCommas(peak),
		PeakDate:  peakDay.Format("Jan 2"),
	}

	var week, prev int64
	for i := 0; i < 14; i++ {
		_, u := on(today.AddDate(0, 0, -i))
		if i < 7 {
			week += u
		} else {
			prev += u
		}
	}
	// Only compare against a full previous week of data.
	if prev > 0 && !start.After(today.AddDate(0, 0, -13)) {
		pct := int(math.Round(float64(week-prev) / float64(prev) * 100))
		switch {
		case week >= 3*prev:
			t.TrendDir = "up"
			t.TrendLabel = fmt.Sprintf("▲ ×%d", int(math.Round(float64(week)/float64(prev))))
		case pct > 0:
			t.TrendDir = "up"
			t.TrendLabel = fmt.Sprintf("▲ %d%%", pct)
		case pct < 0:
			t.TrendDir = "down"
			t.TrendLabel = fmt.Sprintf("▼ %d%%", -pct)
		default:
			t.TrendDir = "flat"
			t.TrendLabel = "= 0%"
		}
	}

	for d := start; !d.After(today); d = d.AddDate(0, 0, 1) {
		v, u := on(d)
		h := 0
		if peak > 0 {
			h = int(math.Round(float64(u) / float64(peak) * 100))
		}
		isToday := d.Equal(today)
		title := d.Format("Mon, Jan 2") + ": " + formatNumberWithCommas(u) + " unique · " + formatNumberWithCommas(v) + " visits"
		if isToday {
			title += " (so far)"
		}
		t.Bars = append(t.Bars, visitorsBar{Title: title, Height: h, Today: isToday})
	}
	return t
}

func (p *VisitorsPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	showVisitors := p.getConfigBool(settings, "ui.showVisitors", true)
	if !showVisitors {
		return "", nil
	}

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Visitors")
	showTotal := p.getConfigBool(settings, "ui.showTotal", true)
	showToday := p.getConfigBool(settings, "ui.showToday", true)
	showUnique := p.getConfigBool(settings, "ui.showUnique", true)
	showHeatmap := p.getConfigBool(settings, "ui.showHeatmap", p.getConfigBool(settings, "ui.showChart", true))
	showRegions := p.getConfigBool(settings, "ui.showRegions", true)

	p.mutex.RLock()
	totalVisits := p.visitCount
	todayVisits := p.todayCount
	uniqueVisits := int64(len(p.uniqueToday))
	var trend *visitorsTrend
	if showHeatmap {
		trend = p.buildTrendLocked(visitorsTrendDays)
	}
	var countryCopy map[string]int64
	if showRegions && len(p.countryStats) > 0 {
		countryCopy = make(map[string]int64, len(p.countryStats))
		for k, v := range p.countryStats {
			if v > 0 {
				countryCopy[k] = v
			}
		}
	}
	p.mutex.RUnlock()

	countriesJSON := "{}"
	if len(countryCopy) > 0 {
		if b, err := json.Marshal(countryCopy); err == nil {
			countriesJSON = string(b)
		}
	}

	tmpl := `
<section class="visitors-section section plugin" data-w="1">
	<header class="plugin-header">
		<h2 class="plugin-title">{{.SectionTitle}}</h2>
	</header>
	<div class="plugin__inner">
		<div class="visitors-card">
			{{if or .ShowTotal .ShowToday .ShowUnique}}
			<div class="visitors-stats">
				{{if .ShowTotal}}
				<div class="visitor-stat" title="{{.TotalExact}} visits all time">
					<div class="visitor-number" data-stat="total">{{.TotalFormatted}}</div>
					<div class="visitor-label">Total</div>
				</div>
				{{end}}
				{{if .ShowToday}}
				<div class="visitor-stat" title="{{.TodayExact}} visits today">
					<div class="visitor-number" data-stat="today">{{.TodayFormatted}}</div>
					<div class="visitor-label">Today</div>
				</div>
				{{end}}
				{{if .ShowUnique}}
				<div class="visitor-stat" title="Unique visitors today">
					<div class="visitor-number" data-stat="unique">{{.UniqueFormatted}}</div>
					<div class="visitor-label">Unique</div>
				</div>
				{{end}}
			</div>
			{{end}}
			{{with .Trend}}
			<div class="visitors-chart">
				<div class="visitors-chart-head">
					<span class="visitors-block-label">Unique visitors / day</span>
					{{if .TrendDir}}<span class="visitors-trend" data-dir="{{.TrendDir}}" title="Unique visitors, last 7 days vs the 7 days before">{{.TrendLabel}}</span>{{end}}
				</div>
				<div class="visitors-bars" role="img" aria-label="Unique visitors per day since {{.FromLabel}}, peak {{.Peak}} on {{.PeakDate}}">
					{{range .Bars}}<span class="visitors-bar{{if .Today}} is-today{{end}}{{if eq .Height 0}} is-empty{{end}}" style="--h: {{.Height}}%" title="{{.Title}}"></span>{{end}}
				</div>
				<div class="visitors-bars-axis"><span>{{.FromLabel}}</span><span class="visitors-axis-peak">peak {{.Peak}} · {{.PeakDate}}</span><span>today</span></div>
			</div>
			{{end}}
		</div>
		{{if .ShowRegions}}
		<div class="visitors-regions visitors-regions--map">
			<div class="visitors-chart-head">
				<span class="visitors-block-label">Regions</span>
				<div class="visitors-heat-scale">Less<span class="region-scale" data-level="1"></span><span class="region-scale" data-level="2"></span><span class="region-scale" data-level="3"></span><span class="region-scale" data-level="4"></span>More</div>
			</div>
			<div class="visitors-map" data-countries="{{.CountriesJSON}}"></div>
		</div>
		{{end}}
	</div>
</section>`

	data := struct {
		SectionTitle    string
		ShowTotal       bool
		ShowToday       bool
		ShowUnique      bool
		ShowRegions     bool
		TotalFormatted  string
		TotalExact      string
		TodayFormatted  string
		TodayExact      string
		UniqueFormatted string
		Trend           *visitorsTrend
		CountriesJSON   string
	}{
		SectionTitle:    sectionTitle,
		ShowTotal:       showTotal,
		ShowToday:       showToday,
		ShowUnique:      showUnique,
		ShowRegions:     showRegions,
		TotalFormatted:  formatNumber(totalVisits),
		TotalExact:      formatNumberWithCommas(totalVisits),
		TodayFormatted:  formatNumber(todayVisits),
		TodayExact:      formatNumberWithCommas(todayVisits),
		UniqueFormatted: formatNumber(uniqueVisits),
		Trend:           trend,
		CountriesJSON:   countriesJSON,
	}

	t, err := template.New("visitors").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (p *VisitorsPlugin) RecordVisit(userAgent, ip string) {
	countBots := p.getConfigBool(p.storage.GetPluginConfig(p.Name()).Settings, "ui.countBots", false)
	ip = normalizeVisitorIP(ip)
	// Real visitors always arrive through nginx with their own address; a loopback
	// client is something on the server polling the page (~720 hits a day).
	bot := isBotUA(userAgent) || isLoopbackIP(ip)

	p.mutex.Lock()

	today := time.Now().Format("2006-01-02")
	if today != p.currentDay {
		log.Printf("[Visitors] Day change detected in RecordVisit: %s -> %s", p.currentDay, today)
		p.rolloverLocked(today)
	}

	if bot && !countBots {
		p.botCount++
		p.mutex.Unlock()
		return
	}

	p.visitCount++
	p.todayCount++

	newUnique := false
	h := visitorHash(ip, userAgent)
	if _, seen := p.uniqueToday[h]; !seen {
		p.uniqueToday[h] = struct{}{}
		newUnique = true
	}

	currentTotal := p.visitCount
	currentToday := p.todayCount
	p.mutex.Unlock()

	if newUnique {
		p.enqueueGeo(ip)
	}

	log.Printf("[Visitors] Visit recorded: total=%d, today=%d, IP=%s", currentTotal, currentToday, ip)

	go func() {
		p.broadcastUpdate()

		if time.Since(p.lastSaveTime) > 30*time.Second {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			if err := p.saveVisitorsDataAsync(ctx); err != nil {
				log.Printf("[Visitors] ERROR: Failed to save after visit: %v", err)
			} else {
				p.mutex.Lock()
				p.lastSaveTime = time.Now()
				p.mutex.Unlock()
			}
		}
	}()
}

func (p *VisitorsPlugin) broadcastUpdate() {
	p.mutex.RLock()
	total := p.visitCount
	today := p.todayCount
	unique := int64(len(p.uniqueToday))
	p.mutex.RUnlock()

	p.hub.Broadcast("visitors_update", map[string]interface{}{
		"total":     total,
		"today":     today,
		"unique":    unique,
		"timestamp": time.Now().Unix(),
	})
}

func (p *VisitorsPlugin) UpdateData(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	saveDone := make(chan error, 1)
	go func() {
		saveCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		saveDone <- p.saveVisitorsDataAsync(saveCtx)
	}()

	select {
	case err := <-saveDone:
		if err != nil {
			log.Printf("[Visitors] UpdateData save error: %v", err)
		}
		return nil
	case <-ctx.Done():
		return nil
	case <-time.After(2 * time.Second):
		log.Printf("[Visitors] UpdateData save timeout, continuing anyway")
		return nil
	}
}

// decodeVisitorsFile reads and parses a visitors data file. A missing file
// returns an os.IsNotExist error; a present-but-unparseable one returns a
// non-nil error the caller treats as corruption.
func decodeVisitorsFile(path string) (VisitorsData, error) {
	var d VisitorsData
	data, err := os.ReadFile(path)
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return d, fmt.Errorf("parse %s: %w", path, err)
	}
	return d, nil
}

func (p *VisitorsPlugin) loadVisitorsData() {
	dataFile := filepath.Join(p.dataPath, "visitors.json")

	visitorsData, err := decodeVisitorsFile(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return // fresh install
		}

		// A corrupt or unreadable primary must NOT fall through to empty maps:
		// the periodic save would then overwrite the (recoverable) file with
		// zeros — exactly how the prod history was wiped. Preserve the bad file
		// and load the last good backup instead.
		log.Printf("[Visitors] ERROR loading %s, attempting backup recovery: %v", dataFile, err)
		if data, rerr := os.ReadFile(dataFile); rerr == nil {
			corruptName := fmt.Sprintf("%s.corrupt-%d", dataFile, time.Now().UnixNano())
			if werr := os.WriteFile(corruptName, data, 0644); werr == nil {
				log.Printf("[Visitors] preserved corrupt file as %s", corruptName)
			}
		}

		backup, berr := decodeVisitorsFile(dataFile + ".bak")
		if berr != nil {
			log.Printf("[Visitors] no usable backup (%v); starting empty but keeping the corrupt copy", berr)
			p.currentDay = time.Now().Format("2006-01-02")
			return
		}
		log.Printf("[Visitors] recovered from backup: total=%d, countries=%d", backup.TotalVisits, len(backup.CountryStats))
		visitorsData = backup
	}

	p.visitCount = visitorsData.TotalVisits
	if visitorsData.DailyStats != nil {
		p.dailyStats = visitorsData.DailyStats
	}
	if visitorsData.DailyUnique != nil {
		p.dailyUnique = visitorsData.DailyUnique
	}
	if visitorsData.CountryStats != nil {
		p.countryStats = visitorsData.CountryStats
	}

	today := time.Now().Format("2006-01-02")
	if visitorsData.CurrentDay == today {
		p.todayCount = visitorsData.TodayVisits
		p.botCount = visitorsData.TodayBots
		for _, h := range visitorsData.UniqueHashes {
			p.uniqueToday[h] = struct{}{}
		}
	} else {
		if visitorsData.CurrentDay != "" && visitorsData.TodayVisits > 0 {
			p.dailyStats[visitorsData.CurrentDay] = visitorsData.TodayVisits
		}
		if visitorsData.CurrentDay != "" && len(visitorsData.UniqueHashes) > 0 {
			p.dailyUnique[visitorsData.CurrentDay] = int64(len(visitorsData.UniqueHashes))
		}
	}

	p.currentDay = today
	p.pruneHistoryLocked(visitorsHistoryDays)
}

func (p *VisitorsPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *VisitorsPlugin) SetSettings(settings map[string]interface{}) error {
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

func (p *VisitorsPlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(string); ok {
				return value
			}
			return defaultValue
		}
		next, ok := current[k].(map[string]interface{})
		if !ok {
			return defaultValue
		}
		current = next
	}

	return defaultValue
}

func (p *VisitorsPlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(bool); ok {
				return value
			}
			return defaultValue
		}
		next, ok := current[k].(map[string]interface{})
		if !ok {
			return defaultValue
		}
		current = next
	}

	return defaultValue
}

func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	} else if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}

func (p *VisitorsPlugin) weekTotalLocked() int64 {
	now := time.Now()
	var week int64
	for i := 0; i < 7; i++ {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		if date == p.currentDay {
			week += p.todayCount
		} else {
			week += p.dailyStats[date]
		}
	}
	return week
}

func (p *VisitorsPlugin) RenderText(ctx context.Context) (string, error) {
	p.mutex.RLock()
	total := p.visitCount
	today := p.todayCount
	unique := int64(len(p.uniqueToday))
	p.mutex.RUnlock()

	return fmt.Sprintf("Visitors: %s total, %s today (%s unique)",
		formatNumber(total), formatNumber(today), formatNumber(unique)), nil
}

func (p *VisitorsPlugin) GetMetrics() map[string]interface{} {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return map[string]interface{}{
		"total_visits":    p.visitCount,
		"today_visits":    p.todayCount,
		"today_unique":    int64(len(p.uniqueToday)),
		"today_bots":      p.botCount,
		"week_visits":     p.weekTotalLocked(),
		"countries_count": len(p.countryStats),
	}
}
