package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
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
	visitorsHistoryDays  = 90
	visitorsGeoQueueSize = 256
	visitorsGeoCacheCap  = 10000
	visitorsHeatmapWeeks = 12
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
		"http://ip-api.com/json/"+ip+"?fields=status,countryCode", nil)
	if err != nil {
		return ""
	}

	resp, err := p.geoClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var body struct {
		Status      string `json:"status"`
		CountryCode string `json:"countryCode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ""
	}
	if body.Status != "success" || len(body.CountryCode) != 2 {
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
		return
	}
	select {
	case p.geoQueue <- ip:
	default:
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

	tempFile := dataFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("write temp file error: %w", err)
	}

	if err := os.Rename(tempFile, dataFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("rename file error: %w", err)
	}

	return nil
}

type visitorsHeatDay struct {
	Date   string
	Count  int64
	Unique int64
	Level  int
}

func (p *VisitorsPlugin) buildHeatmapLocked(weeks int) []visitorsHeatDay {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var out []visitorsHeatDay
	var maxV int64
	var minV int64 = -1

	for w := weeks - 1; w >= 0; w-- {
		end := today.AddDate(0, 0, -w*7)
		start := end.AddDate(0, 0, -6)

		var count, uniq int64
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			date := d.Format("2006-01-02")
			if date == p.currentDay {
				count += p.todayCount
				uniq += int64(len(p.uniqueToday))
			} else {
				count += p.dailyStats[date]
				uniq += p.dailyUnique[date]
			}
		}

		if count > 0 {
			if count > maxV {
				maxV = count
			}
			if minV < 0 || count < minV {
				minV = count
			}
		}

		out = append(out, visitorsHeatDay{
			Date:   start.Format("Jan 2") + " – " + end.Format("Jan 2"),
			Count:  count,
			Unique: uniq,
		})
	}

	spread := maxV - minV
	for i := range out {
		c := out[i].Count
		if c <= 0 {
			continue
		}
		if spread <= 0 {
			out[i].Level = 2
			continue
		}
		level := 1 + int(float64(c-minV)/float64(spread)*3+0.5)
		if level < 1 {
			level = 1
		}
		if level > 4 {
			level = 4
		}
		out[i].Level = level
	}

	return out
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
	var heatmap []visitorsHeatDay
	if showHeatmap {
		heatmap = p.buildHeatmapLocked(visitorsHeatmapWeeks)
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
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
	</header>
	<div class="plugin__inner">
		<div class="visitors-stats">
			{{if .ShowTotal}}
			<div class="visitor-stat" data-tooltip="Exact: {{.TotalExact}}">
				<div class="visitor-number" data-stat="total">{{.TotalFormatted}}</div>
				<div class="visitor-label">Total</div>
			</div>
			{{end}}
			{{if .ShowToday}}
			<div class="visitor-stat" data-tooltip="Exact: {{.TodayExact}}">
				<div class="visitor-number" data-stat="today">{{.TodayFormatted}}</div>
				<div class="visitor-label">Today</div>
			</div>
			{{end}}
			{{if .ShowUnique}}
			<div class="visitor-stat" data-tooltip="Unique visitors today">
				<div class="visitor-number" data-stat="unique">{{.UniqueFormatted}}</div>
				<div class="visitor-label">Unique</div>
			</div>
			{{end}}
		</div>
		{{if .Heatmap}}
		<div class="visitors-chart">
			<span class="visitors-block-label">Visits · last {{.HeatmapWeeks}} weeks</span>
			<div class="visitors-heatmap">
				{{range .Heatmap}}<span class="visitors-day" data-level="{{.Level}}" title="{{.Date}}: {{.Count}} visits{{if .Unique}} / {{.Unique}} unique{{end}}"></span>{{end}}
			</div>
			<div class="visitors-heat-scale">Less<span class="visitors-day" data-level="1"></span><span class="visitors-day" data-level="2"></span><span class="visitors-day" data-level="3"></span><span class="visitors-day" data-level="4"></span>More</div>
		</div>
		{{end}}
		{{if .ShowRegions}}
		<div class="visitors-regions visitors-regions--map">
			<span class="visitors-block-label">Regions · heatmap</span>
			<div class="visitors-map" data-countries="{{.CountriesJSON}}"></div>
			<div class="visitors-heat-scale">Less<span class="region-scale" data-level="1"></span><span class="region-scale" data-level="2"></span><span class="region-scale" data-level="3"></span><span class="region-scale" data-level="4"></span>More</div>
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
		Heatmap         []visitorsHeatDay
		HeatmapWeeks    int
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
		Heatmap:         heatmap,
		HeatmapWeeks:    visitorsHeatmapWeeks,
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
	bot := isBotUA(userAgent)
	ip = normalizeVisitorIP(ip)

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

func (p *VisitorsPlugin) loadVisitorsData() {
	dataFile := filepath.Join(p.dataPath, "visitors.json")

	data, err := os.ReadFile(dataFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[Visitors] ERROR reading data file: %v", err)
		}
		return
	}

	var visitorsData VisitorsData
	if err := json.Unmarshal(data, &visitorsData); err != nil {
		log.Printf("[Visitors] ERROR parsing JSON: %v", err)
		return
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
