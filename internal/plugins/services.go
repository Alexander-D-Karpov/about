package plugins

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type ServicesPlugin struct {
	storage         *storage.Storage
	hub             *stream.Hub
	serviceStatuses map[string]ServiceStatus
	lastCheck       time.Time
	mutex           sync.RWMutex
	httpClient      *http.Client
}

type ServiceStatus struct {
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	Status       string    `json:"status"`
	ResponseTime int64     `json:"response_time"`
	Description  string    `json:"description"`
	Icon         string    `json:"icon"`
	LastChecked  time.Time `json:"last_checked"`
	StatusCode   int       `json:"status_code"`
}

func NewServicesPlugin(storage *storage.Storage, hub *stream.Hub) *ServicesPlugin {
	return &ServicesPlugin{
		storage:         storage,
		hub:             hub,
		serviceStatuses: make(map[string]ServiceStatus),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *ServicesPlugin) Name() string {
	return "services"
}

func (p *ServicesPlugin) Render(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	services, ok := settings["services"].([]interface{})
	if !ok || len(services) == 0 {
		return p.renderNoServices(), nil
	}

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Local Services")
	showStatus := p.getConfigBool(settings, "ui.showStatus", true)
	showResponseTime := p.getConfigBool(settings, "ui.showResponseTime", true)

	tmpl := `
	<div class="services-section section" data-w="2">
		<div class="plugin-header">
			<h3 class="plugin-title">{{.SectionTitle}}</h3>
		</div>
		<div class="plugin__inner">
			<div class="services-grid">
				{{range .Services}}
				<div class="service-item" data-url="{{.URL}}" data-status="{{.Status}}">
					<div class="service-header">
						{{if .Icon}}
						<span class="service-icon">{{.Icon}}</span>
						{{else}}
						<span class="service-icon">⚙️</span>
						{{end}}
						<div class="service-name">
							{{if .URL}}
							<a href="{{.URL}}" target="_blank" rel="noopener" class="service-link">{{.Name}}</a>
							{{else}}
							<span class="service-title">{{.Name}}</span>
							{{end}}
						</div>
						{{if $.ShowStatus}}
						<span class="status-indicator status-{{.Status}}" 
							  title="{{.StatusText}} ({{.ResponseTime}}ms)"
							  data-tooltip="{{.StatusText}}"></span>
						{{end}}
					</div>
					{{if .Description}}
					<div class="service-description">{{.Description}}</div>
					{{end}}
					<div class="service-stats">
						{{if and $.ShowResponseTime .ResponseTime}}
						<span class="service-response-time">{{.ResponseTime}}ms</span>
						{{end}}
						{{if .LastChecked}}
						<span class="service-last-check">{{.LastCheckedText}}</span>
						{{end}}
						{{if .StatusCode}}
						<span class="service-status-code">{{.StatusCode}}</span>
						{{end}}
					</div>
				</div>
				{{end}}
			</div>
			
			<div class="services-summary">
				<div class="summary-item">
					<span class="summary-label">Online</span>
					<span class="summary-count online-count">{{.OnlineCount}}</span>
				</div>
				<div class="summary-item">
					<span class="summary-label">Offline</span>
					<span class="summary-count offline-count">{{.OfflineCount}}</span>
				</div>
				<div class="summary-item">
					<span class="summary-label">Total</span>
					<span class="summary-count total-count">{{.TotalCount}}</span>
				</div>
			</div>
		</div>
	</div>`

	type serviceData struct {
		Name            string
		URL             string
		Icon            string
		Description     string
		Status          string
		StatusText      string
		ResponseTime    int64
		StatusCode      int
		LastChecked     time.Time
		LastCheckedText string
	}

	var serviceList []serviceData
	onlineCount := 0
	offlineCount := 0

	p.mutex.RLock()
	defer p.mutex.RUnlock()

	for _, service := range services {
		serviceMap, ok := service.(map[string]interface{})
		if !ok {
			continue
		}

		name := p.getStringFromMap(serviceMap, "name", "Service")
		url := p.getStringFromMap(serviceMap, "url", "")
		icon := p.getStringFromMap(serviceMap, "icon", "")
		description := p.getStringFromMap(serviceMap, "description", "")

		status := p.serviceStatuses[name]

		statusText := "Unknown"
		switch status.Status {
		case "online":
			statusText = "Online"
			onlineCount++
		case "offline":
			statusText = "Offline"
			offlineCount++
		case "unknown":
			statusText = "Unknown"
		}

		lastCheckedText := ""
		if !status.LastChecked.IsZero() {
			lastCheckedText = p.formatTimeAgo(status.LastChecked)
		}

		serviceList = append(serviceList, serviceData{
			Name:            name,
			URL:             url,
			Icon:            icon,
			Description:     description,
			Status:          status.Status,
			StatusText:      statusText,
			ResponseTime:    status.ResponseTime,
			StatusCode:      status.StatusCode,
			LastChecked:     status.LastChecked,
			LastCheckedText: lastCheckedText,
		})
	}

	data := struct {
		SectionTitle     string
		Services         []serviceData
		ShowStatus       bool
		ShowResponseTime bool
		OnlineCount      int
		OfflineCount     int
		TotalCount       int
	}{
		SectionTitle:     sectionTitle,
		Services:         serviceList,
		ShowStatus:       showStatus,
		ShowResponseTime: showResponseTime,
		OnlineCount:      onlineCount,
		OfflineCount:     offlineCount,
		TotalCount:       len(serviceList),
	}

	t, err := template.New("services").Parse(tmpl)
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

func (p *ServicesPlugin) renderNoServices() string {
	return `<div class="services-section section">
		<div class="plugin-header">
			<h3 class="plugin-title">Services</h3>
		</div>
		<div class="plugin__inner">
			<div class="no-services">
				<p class="text-muted">No services configured</p>
				<p class="text-muted">Add services in the admin panel to monitor their status</p>
			</div>
		</div>
	</div>`
}

func (p *ServicesPlugin) UpdateData(ctx context.Context) error {
	if time.Since(p.lastCheck) < 2*time.Minute {
		return nil
	}

	config := p.storage.GetPluginConfig(p.Name())
	services, ok := config.Settings["services"].([]interface{})
	if !ok {
		return nil
	}

	var wg sync.WaitGroup
	for _, service := range services {
		serviceMap, ok := service.(map[string]interface{})
		if !ok {
			continue
		}

		name := p.getStringFromMap(serviceMap, "name", "")
		url := p.getStringFromMap(serviceMap, "url", "")
		description := p.getStringFromMap(serviceMap, "description", "")
		icon := p.getStringFromMap(serviceMap, "icon", "")

		if name == "" || url == "" {
			continue
		}

		wg.Add(1)
		go func(serviceName, serviceUrl, desc, icn string) {
			defer wg.Done()
			p.checkServiceStatus(serviceName, serviceUrl, desc, icn)
		}(name, url, description, icon)
	}

	wg.Wait()
	p.lastCheck = time.Now()

	p.broadcastStatusUpdate()

	return nil
}

func (p *ServicesPlugin) checkServiceStatus(name, url, description, icon string) {
	start := time.Now()

	status := ServiceStatus{
		Name:        name,
		URL:         url,
		Status:      "unknown",
		Description: description,
		Icon:        icon,
		LastChecked: time.Now(),
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		status.Status = "offline"
		p.updateServiceStatus(name, status)
		return
	}

	req.Header.Set("User-Agent", "AboutPage-ServiceMonitor/1.0")
	req.Header.Set("Accept", "*/*")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		status.Status = "offline"
		status.ResponseTime = time.Since(start).Milliseconds()
	} else {
		resp.Body.Close()
		status.ResponseTime = time.Since(start).Milliseconds()
		status.StatusCode = resp.StatusCode

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			status.Status = "online"
		} else if resp.StatusCode >= 400 && resp.StatusCode < 600 {
			status.Status = "offline"
		} else {
			status.Status = "unknown"
		}
	}

	p.updateServiceStatus(name, status)
}

func (p *ServicesPlugin) updateServiceStatus(name string, status ServiceStatus) {
	p.mutex.Lock()
	oldStatus := p.serviceStatuses[name]
	p.serviceStatuses[name] = status
	p.mutex.Unlock()

	if oldStatus.Status != status.Status {
		p.hub.Broadcast("service_status_update", map[string]interface{}{
			"name":          name,
			"status":        status.Status,
			"response_time": status.ResponseTime,
			"status_code":   status.StatusCode,
			"timestamp":     time.Now().Unix(),
		})
	}
}

func (p *ServicesPlugin) broadcastStatusUpdate() {
	p.mutex.RLock()
	onlineCount := 0
	offlineCount := 0
	totalCount := len(p.serviceStatuses)

	statusMap := make(map[string]interface{})
	for name, status := range p.serviceStatuses {
		if status.Status == "online" {
			onlineCount++
		} else if status.Status == "offline" {
			offlineCount++
		}

		statusMap[name] = map[string]interface{}{
			"status":        status.Status,
			"response_time": status.ResponseTime,
			"status_code":   status.StatusCode,
		}
	}
	p.mutex.RUnlock()

	p.hub.Broadcast("services_summary_update", map[string]interface{}{
		"online_count":  onlineCount,
		"offline_count": offlineCount,
		"total_count":   totalCount,
		"services":      statusMap,
		"timestamp":     time.Now().Unix(),
	})
}

func (p *ServicesPlugin) formatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return "just now"
	} else if duration < time.Hour {
		mins := int(duration.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	} else if duration < 24*time.Hour {
		hours := int(duration.Hours())
		return fmt.Sprintf("%dh ago", hours)
	} else {
		days := int(duration.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
}

func (p *ServicesPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *ServicesPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings

	err := p.storage.SetPluginConfig(p.Name(), config)
	if err != nil {
		return err
	}

	p.lastCheck = time.Time{}

	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(),
		"action": "settings_changed",
	})

	return nil
}

func (p *ServicesPlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
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

func (p *ServicesPlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
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

func (p *ServicesPlugin) getStringFromMap(m map[string]interface{}, key string, defaultValue string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return defaultValue
}

func (p *ServicesPlugin) RenderText(ctx context.Context) (string, error) {
	p.mutex.RLock()
	onlineCount := 0
	offlineCount := 0
	totalCount := len(p.serviceStatuses)

	for _, status := range p.serviceStatuses {
		if status.Status == "online" {
			onlineCount++
		} else if status.Status == "offline" {
			offlineCount++
		}
	}
	p.mutex.RUnlock()

	if totalCount == 0 {
		return "Services: No services configured", nil
	}

	return fmt.Sprintf("Services: %d online, %d offline (%d total)", onlineCount, offlineCount, totalCount), nil
}
