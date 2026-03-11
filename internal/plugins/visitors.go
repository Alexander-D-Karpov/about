package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type VisitorsPlugin struct {
	storage      *storage.Storage
	hub          *stream.Hub
	visitCount   int64
	todayCount   int64
	currentDay   string
	dataPath     string
	mutex        sync.RWMutex
	lastSaveTime time.Time
	dailyStats   map[string]int64
}

type VisitorsData struct {
	TotalVisits  int64            `json:"total_visits"`
	TodayVisits  int64            `json:"today_visits"`
	CurrentDay   string           `json:"current_day"`
	LastUpdate   time.Time        `json:"last_update"`
	DailyStats   map[string]int64 `json:"daily_stats"`
	MonthlyStats map[string]int64 `json:"monthly_stats"`
}

func NewVisitorsPlugin(storage *storage.Storage, hub *stream.Hub, dataPath string) *VisitorsPlugin {
	plugin := &VisitorsPlugin{
		storage:    storage,
		hub:        hub,
		dataPath:   dataPath,
		dailyStats: make(map[string]int64),
		currentDay: time.Now().Format("2006-01-02"),
	}

	log.Printf("[Visitors] Initializing plugin, data path: %s", dataPath)

	plugin.loadVisitorsData()

	log.Printf("[Visitors] Loaded data: total=%d, today=%d, current_day=%s",
		plugin.visitCount, plugin.todayCount, plugin.currentDay)

	plugin.checkDayTransition()

	if os.Getenv("VISITORS_DEBUG") == "true" {
		log.Printf("[Visitors] DEBUG MODE: Testing day transition immediately")
		plugin.testDayTransition()
	}

	go plugin.startDayChecker()

	return plugin
}

func (p *VisitorsPlugin) Name() string {
	return "visitors"
}

func (p *VisitorsPlugin) startDayChecker() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	log.Printf("[Visitors] Day checker started, will check every minute")

	for range ticker.C {
		log.Printf("[Visitors] Day checker tick at %s", time.Now().Format("15:04:05"))
		p.checkDayTransition()
	}
}

func (p *VisitorsPlugin) checkDayTransition() {
	p.mutex.Lock()
	today := time.Now().Format("2006-01-02")
	needsSave := false
	var oldDay string
	var oldCount int64

	if today != p.currentDay {
		log.Printf("[Visitors] Day transition detected: %s -> %s (today count: %d)", p.currentDay, today, p.todayCount)

		if p.currentDay != "" && p.todayCount > 0 {
			p.dailyStats[p.currentDay] = p.todayCount
			oldDay = p.currentDay
			oldCount = p.todayCount
		}

		p.currentDay = today
		p.todayCount = 0
		needsSave = true
	}
	p.mutex.Unlock()

	if needsSave {
		log.Printf("[Visitors] Saving day transition data for %s with %d visits", oldDay, oldCount)

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := p.saveVisitorsDataAsync(ctx); err != nil {
				log.Printf("[Visitors] ERROR: Failed to save day transition data: %v", err)
			} else {
				log.Printf("[Visitors] Successfully saved day transition data")
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
		CurrentDay:   p.currentDay,
		LastUpdate:   time.Now(),
		DailyStats:   make(map[string]int64),
		MonthlyStats: make(map[string]int64),
	}

	for k, v := range p.dailyStats {
		visitorsData.DailyStats[k] = v
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

	log.Printf("[Visitors] Data saved: total=%d, today=%d, day=%s",
		visitorsData.TotalVisits, visitorsData.TodayVisits, visitorsData.CurrentDay)

	return nil
}

func (p *VisitorsPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	showVisitors := p.getConfigBool(settings, "ui.showVisitors", true)
	if !showVisitors {
		return "", nil
	}

	p.mutex.RLock()
	totalVisits := p.visitCount
	todayVisits := p.todayCount
	p.mutex.RUnlock()

	totalFormatted := formatNumber(totalVisits)
	totalExact := formatNumberWithCommas(totalVisits)
	todayFormatted := formatNumber(todayVisits)
	todayExact := formatNumberWithCommas(todayVisits)

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Visitors")
	showTotal := p.getConfigBool(settings, "ui.showTotal", true)
	showToday := p.getConfigBool(settings, "ui.showToday", true)

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
		</div>
	</div>
</section>`

	data := struct {
		SectionTitle   string
		ShowTotal      bool
		ShowToday      bool
		TotalFormatted string
		TotalExact     string
		TodayFormatted string
		TodayExact     string
	}{
		SectionTitle:   sectionTitle,
		ShowTotal:      showTotal,
		ShowToday:      showToday,
		TotalFormatted: totalFormatted,
		TotalExact:     totalExact,
		TodayFormatted: todayFormatted,
		TodayExact:     todayExact,
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
	p.mutex.Lock()

	today := time.Now().Format("2006-01-02")
	if today != p.currentDay {
		log.Printf("[Visitors] Day change detected in RecordVisit: %s -> %s", p.currentDay, today)
		if p.currentDay != "" && p.todayCount > 0 {
			p.dailyStats[p.currentDay] = p.todayCount
		}
		p.currentDay = today
		p.todayCount = 0
	}

	p.visitCount++
	p.todayCount++

	currentTotal := p.visitCount
	currentToday := p.todayCount
	p.mutex.Unlock()

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
	p.mutex.RUnlock()

	p.hub.Broadcast("visitors_update", map[string]interface{}{
		"total":     total,
		"today":     today,
		"timestamp": time.Now().Unix(),
	})
}

func (p *VisitorsPlugin) UpdateData(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	log.Printf("[Visitors] UpdateData called, attempting save...")

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
			return nil
		}
		log.Printf("[Visitors] UpdateData completed successfully")
		return nil
	case <-ctx.Done():
		log.Printf("[Visitors] UpdateData cancelled by context")
		return nil
	case <-time.After(2 * time.Second):
		log.Printf("[Visitors] UpdateData save timeout, continuing anyway")
		return nil
	}
}

func (p *VisitorsPlugin) loadVisitorsData() {
	dataFile := filepath.Join(p.dataPath, "visitors.json")

	log.Printf("[Visitors] Loading data from %s", dataFile)

	data, err := os.ReadFile(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[Visitors] No existing data file, starting fresh")
		} else {
			log.Printf("[Visitors] ERROR reading data file: %v", err)
		}
		p.visitCount = 0
		p.todayCount = 0
		p.dailyStats = make(map[string]int64)
		return
	}

	log.Printf("[Visitors] Loaded %d bytes from data file", len(data))

	var visitorsData VisitorsData
	if err := json.Unmarshal(data, &visitorsData); err != nil {
		log.Printf("[Visitors] ERROR parsing JSON: %v", err)
		p.visitCount = 0
		p.todayCount = 0
		p.dailyStats = make(map[string]int64)
		return
	}

	p.visitCount = visitorsData.TotalVisits
	if visitorsData.DailyStats != nil {
		p.dailyStats = visitorsData.DailyStats
	} else {
		p.dailyStats = make(map[string]int64)
	}

	today := time.Now().Format("2006-01-02")
	if visitorsData.CurrentDay == today {
		p.todayCount = visitorsData.TodayVisits
		log.Printf("[Visitors] Loaded today's count: %d", p.todayCount)
	} else {
		if visitorsData.CurrentDay != "" && visitorsData.TodayVisits > 0 {
			p.dailyStats[visitorsData.CurrentDay] = visitorsData.TodayVisits
			log.Printf("[Visitors] Saved previous day %s count: %d", visitorsData.CurrentDay, visitorsData.TodayVisits)
		}
		p.todayCount = 0
	}

	p.currentDay = today

	log.Printf("[Visitors] Data loaded successfully: total=%d, today=%d, day=%s, dailyStats=%d days",
		p.visitCount, p.todayCount, p.currentDay, len(p.dailyStats))
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

func (p *VisitorsPlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
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

func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	} else if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	} else {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
}

func (p *VisitorsPlugin) RenderText(ctx context.Context) (string, error) {
	p.mutex.RLock()
	total := p.visitCount
	today := p.todayCount
	p.mutex.RUnlock()

	return fmt.Sprintf("Visitors: %s total, %s today",
		formatNumber(total), formatNumber(today)), nil
}

func (p *VisitorsPlugin) testDayTransition() {
	log.Printf("[Visitors] TEST: Starting day transition test")

	p.mutex.Lock()
	initialTotal := p.visitCount
	initialToday := p.todayCount
	initialDay := p.currentDay

	p.todayCount = 42
	p.currentDay = "2025-01-01"
	log.Printf("[Visitors] TEST: Set test values - day: %s, today: %d", p.currentDay, p.todayCount)
	p.mutex.Unlock()

	time.Sleep(100 * time.Millisecond)

	p.checkDayTransition()

	time.Sleep(500 * time.Millisecond)

	p.mutex.RLock()
	newDay := p.currentDay
	newToday := p.todayCount
	savedCount := p.dailyStats["2025-01-01"]
	p.mutex.RUnlock()

	log.Printf("[Visitors] TEST RESULTS:")
	log.Printf("  - Initial: day=%s, today=%d, total=%d", initialDay, initialToday, initialTotal)
	log.Printf("  - After transition: day=%s, today=%d", newDay, newToday)
	log.Printf("  - Saved in dailyStats[2025-01-01]: %d", savedCount)

	if savedCount != 42 {
		log.Printf("[Visitors] TEST FAILED: Expected 42 in dailyStats, got %d", savedCount)
	} else if newToday != 0 {
		log.Printf("[Visitors] TEST FAILED: Expected today count to reset to 0, got %d", newToday)
	} else {
		log.Printf("[Visitors] TEST PASSED: Day transition working correctly")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.saveVisitorsDataAsync(ctx); err != nil {
		log.Printf("[Visitors] TEST: Save failed: %v", err)
	} else {
		log.Printf("[Visitors] TEST: Save succeeded")

		dataFile := filepath.Join(p.dataPath, "visitors.json")
		if data, err := os.ReadFile(dataFile); err == nil {
			log.Printf("[Visitors] TEST: Saved file size: %d bytes", len(data))
		}
	}
}

func (p *VisitorsPlugin) GetMetrics() map[string]interface{} {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return map[string]interface{}{
		"total_visits": p.visitCount,
		"today_visits": p.todayCount,
	}
}
