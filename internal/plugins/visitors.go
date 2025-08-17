package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
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
	uniqueToday  map[string]bool
	dataPath     string
	mutex        sync.RWMutex
	lastSaveTime time.Time
	startTime    time.Time
}

type VisitorsData struct {
	TotalVisits  int64            `json:"total_visits"`
	TodayVisits  int64            `json:"today_visits"`
	LastUpdate   time.Time        `json:"last_update"`
	DailyStats   map[string]int64 `json:"daily_stats"`
	MonthlyStats map[string]int64 `json:"monthly_stats"`
}

func NewVisitorsPlugin(storage *storage.Storage, hub *stream.Hub, dataPath string) *VisitorsPlugin {
	plugin := &VisitorsPlugin{
		storage:     storage,
		hub:         hub,
		dataPath:    dataPath,
		uniqueToday: make(map[string]bool),
		startTime:   time.Now(),
	}

	plugin.loadVisitorsData()
	return plugin
}

func (p *VisitorsPlugin) Name() string {
	return "visitors"
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

	var parts []string

	if showTotal {
		parts = append(parts, "Total visits: "+formatNumber(totalVisits))
	}

	if showToday {
		parts = append(parts, "Today: "+formatNumber(todayVisits))
	}

	if showUptime {
		parts = append(parts, "Uptime: "+formatDuration(uptime))
	}

	if len(parts) == 0 {
		return "", nil
	}

	tmpl := `
	<div class="visitors-section">
		{{if .SectionTitle}}
		<h3>{{.SectionTitle}}</h3>
		{{end}}
		<div class="visitors-stats">
			<p class="visitors-info">{{.StatsText}}</p>
		</div>
	</div>`

	data := struct {
		SectionTitle string
		StatsText    string
	}{
		SectionTitle: sectionTitle,
		StatsText:    strings.Join(parts, " • "),
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

func (p *VisitorsPlugin) UpdateData(ctx context.Context) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	now := time.Now()
	if now.Sub(p.lastSaveTime) < 29*time.Minute {
		return nil
	}

	return p.saveVisitorsData()
}

func (p *VisitorsPlugin) RecordVisit(userAgent, ip string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	today := time.Now().Format("2006-01-02")
	uniqueKey := today + ":" + ip

	if !p.uniqueToday[uniqueKey] {
		p.uniqueToday[uniqueKey] = true
		p.todayCount++
		p.visitCount++

		p.hub.Broadcast("visitors_update", map[string]interface{}{
			"total": p.visitCount,
			"today": p.todayCount,
		})
	}

	currentDay := time.Now().Format("2006-01-02")
	for key := range p.uniqueToday {
		if !strings.HasPrefix(key, currentDay+":") {
			delete(p.uniqueToday, key)
		}
	}
}

func (p *VisitorsPlugin) loadVisitorsData() {
	dataFile := filepath.Join(p.dataPath, "visitors.json")

	data, err := ioutil.ReadFile(dataFile)
	if err != nil {
		p.visitCount = 0
		p.todayCount = 0
		return
	}

	var visitorsData VisitorsData
	if err := json.Unmarshal(data, &visitorsData); err != nil {
		p.visitCount = 0
		p.todayCount = 0
		return
	}

	p.visitCount = visitorsData.TotalVisits

	today := time.Now().Format("2006-01-02")
	lastUpdate := visitorsData.LastUpdate.Format("2006-01-02")

	if today == lastUpdate {
		p.todayCount = visitorsData.TodayVisits
	} else {
		p.todayCount = 0
	}
}

func (p *VisitorsPlugin) saveVisitorsData() error {
	dataFile := filepath.Join(p.dataPath, "visitors.json")

	if err := os.MkdirAll(filepath.Dir(dataFile), 0755); err != nil {
		return err
	}

	visitorsData := VisitorsData{
		TotalVisits:  p.visitCount,
		TodayVisits:  p.todayCount,
		LastUpdate:   time.Now(),
		DailyStats:   make(map[string]int64),
		MonthlyStats: make(map[string]int64),
	}

	today := time.Now().Format("2006-01-02")
	month := time.Now().Format("2006-01")

	visitorsData.DailyStats[today] = p.todayCount
	visitorsData.MonthlyStats[month] = p.visitCount

	data, err := json.MarshalIndent(visitorsData, "", "  ")
	if err != nil {
		return err
	}

	p.lastSaveTime = time.Now()
	return ioutil.WriteFile(dataFile, data, 0644)
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
