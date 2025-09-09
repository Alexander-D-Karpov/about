package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type VisitorsPlugin struct {
	storage       *storage.Storage
	hub           *stream.Hub
	visitCount    int64
	todayCount    int64
	currentDay    string
	dataPath      string
	mutex         sync.RWMutex
	lastSaveTime  time.Time
	startTime     time.Time
	lastDayCheck  time.Time
	dailyStats    map[string]int64
	lastBroadcast time.Time
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
		currentDay: time.Now().Format("29.02.2006"),
		startTime:  time.Now(),
	}

	plugin.loadVisitorsData()
	plugin.checkDayTransition()

	go plugin.startDayChecker()

	return plugin
}

func (p *VisitorsPlugin) Name() string {
	return "visitors"
}

func (p *VisitorsPlugin) startDayChecker() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		p.checkDayTransition()
	}
}

func (p *VisitorsPlugin) checkDayTransition() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	today := time.Now().Format("29.02.2006")
	if today != p.currentDay {
		if p.currentDay != "" && p.todayCount > 0 {
			p.dailyStats[p.currentDay] = p.todayCount
		}

		p.currentDay = today
		p.todayCount = 0

		p.saveVisitorsDataUnsafe()
		p.broadcastUpdate()
	}
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
	uptime := time.Since(p.startTime)
	p.mutex.RUnlock()

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Visitors")
	showTotal := p.getConfigBool(settings, "ui.showTotal", true)
	showToday := p.getConfigBool(settings, "ui.showToday", true)
	showUptime := p.getConfigBool(settings, "ui.showUptime", true)

	tmpl := `
	<div class="visitors-section section">
		<div class="plugin-header">
			<h3 class="plugin-title">{{.SectionTitle}}</h3>
		</div>
		<div class="plugin__inner">
			<div class="visitors-stats">
				{{if .ShowTotal}}
				<div class="visitor-stat">
					<div class="visitor-number" data-stat="total">{{.TotalVisits}}</div>
					<div class="visitor-label">Total</div>
				</div>
				{{end}}
				{{if .ShowToday}}
				<div class="visitor-stat">
					<div class="visitor-number" data-stat="today">{{.TodayVisits}}</div>
					<div class="visitor-label">Today</div>
				</div>
				{{end}}
				{{if .ShowUptime}}
				<div class="visitor-stat">
					<div class="visitor-uptime" data-uptime>{{.Uptime}}</div>
					<div class="visitor-label">Uptime</div>
				</div>
				{{end}}
			</div>
		</div>
	</div>`

	data := struct {
		SectionTitle string
		ShowTotal    bool
		ShowToday    bool
		ShowUptime   bool
		TotalVisits  string
		TodayVisits  string
		Uptime       string
	}{
		SectionTitle: sectionTitle,
		ShowTotal:    showTotal,
		ShowToday:    showToday,
		ShowUptime:   showUptime,
		TotalVisits:  formatNumber(totalVisits),
		TodayVisits:  formatNumber(todayVisits),
		Uptime:       formatDuration(uptime),
	}

	t, err := template.New("visitors").Parse(tmpl)
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

func (p *VisitorsPlugin) RecordVisit(userAgent, ip string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.checkDayTransitionUnsafe()

	// Always increment both counters on every visit
	p.visitCount++
	p.todayCount++

	// Always broadcast immediately for real-time updates
	go p.broadcastUpdate()

	// Save immediately to ensure persistence
	go func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		p.saveVisitorsDataUnsafe()
	}()
}

func (p *VisitorsPlugin) checkDayTransitionUnsafe() {
	today := time.Now().Format("29.02.2006")
	if today != p.currentDay {
		if p.currentDay != "" && p.todayCount > 0 {
			p.dailyStats[p.currentDay] = p.todayCount
		}

		p.currentDay = today
		p.todayCount = 0
	}
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
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Always save when requested to ensure data persistence
	return p.saveVisitorsDataUnsafe()
}

func (p *VisitorsPlugin) loadVisitorsData() {
	dataFile := filepath.Join(p.dataPath, "visitors.json")

	data, err := os.ReadFile(dataFile)
	if err != nil {
		p.visitCount = 0
		p.todayCount = 0
		p.dailyStats = make(map[string]int64)
		return
	}

	var visitorsData VisitorsData
	if err := json.Unmarshal(data, &visitorsData); err != nil {
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

	today := time.Now().Format("29.02.2006")
	if visitorsData.CurrentDay == today {
		p.todayCount = visitorsData.TodayVisits
	} else {
		if visitorsData.CurrentDay != "" && visitorsData.TodayVisits > 0 {
			p.dailyStats[visitorsData.CurrentDay] = visitorsData.TodayVisits
		}
		p.todayCount = 0
	}

	p.currentDay = today
}

func (p *VisitorsPlugin) saveVisitorsDataUnsafe() error {
	dataFile := filepath.Join(p.dataPath, "visitors.json")

	if err := os.MkdirAll(filepath.Dir(dataFile), 0755); err != nil {
		return err
	}

	visitorsData := VisitorsData{
		TotalVisits:  p.visitCount,
		TodayVisits:  p.todayCount,
		CurrentDay:   p.currentDay,
		LastUpdate:   time.Now(),
		DailyStats:   p.dailyStats,
		MonthlyStats: make(map[string]int64),
	}

	month := time.Now().Format("2006-01")
	visitorsData.MonthlyStats[month] = p.visitCount

	data, err := json.MarshalIndent(visitorsData, "", "  ")
	if err != nil {
		return err
	}

	p.lastSaveTime = time.Now()
	return os.WriteFile(dataFile, data, 0644)
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

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}
