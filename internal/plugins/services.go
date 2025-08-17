package plugins

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type ServicesPlugin struct {
	storage         *storage.Storage
	hub             *stream.Hub
	serviceStatuses map[string]ServiceStatus
	lastCheck       time.Time
}

type ServiceStatus struct {
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	Status       string    `json:"status"` // "online", "offline", "unknown"
	ResponseTime int64     `json:"response_time"`
	Description  string    `json:"description"`
	Icon         string    `json:"icon"`
	LastChecked  time.Time `json:"last_checked"`
}

func NewServicesPlugin(storage *storage.Storage, hub *stream.Hub) *ServicesPlugin {
	return &ServicesPlugin{
		storage:         storage,
		hub:             hub,
		serviceStatuses: make(map[string]ServiceStatus),
	}
}

func (p *ServicesPlugin) Name() string {
	return "services"
}

func (p *ServicesPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	services, ok := settings["services"].([]interface{})
	if !ok || len(services) == 0 {
		return "", nil
	}

	// Get configurable UI text
	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "🛠️ Local Services")
	showStatus := p.getConfigBool(settings, "ui.showStatus", true)
	showResponseTime := p.getConfigBool(settings, "ui.showResponseTime", true)

	tmpl := `
	<div class="services-section section">
		<h3>{{.SectionTitle}}</h3>
		<div class="services-grid">
			{{range .Services}}
			<div class="service-item">
				<div class="service-header">
					{{if .Icon}}
					<span class="service-icon">{{.Icon}}</span>
					{{end}}
					<div class="service-name">
						{{if .URL}}
						<a href="{{.URL}}" target="_blank" rel="noopener">{{.Name}}</a>
						{{else}}
						{{.Name}}
						{{end}}
					</div>
					{{if $.ShowStatus}}
					<span class="status-indicator status-{{.Status}}" data-tooltip="{{.StatusText}}"></span>
					{{end}}
				</div>
				{{if .Description}}
				<div class="service-description">{{.Description}}</div>
				{{end}}
				{{if and $.ShowResponseTime .ResponseTime}}
				<div class="service-response-time">{{.ResponseTime}}ms</div>
				{{end}}
			</div>
			{{end}}
		</div>
	</div>`

	type serviceData struct {
		Name         string
		URL          string
		Icon         string
		Description  string
		Status       string
		StatusText   string
		ResponseTime int64
	}

	var serviceList []serviceData
	for _, service := range services {
		serviceMap, ok := service.(map[string]interface{})
		if !ok {
			continue
		}

		name := p.getStringFromMap(serviceMap, "name", "Service")
		url := p.getStringFromMap(serviceMap, "url", "")
		icon := p.getStringFromMap(serviceMap, "icon", "🔧")
		description := p.getStringFromMap(serviceMap, "description", "")

		// Get cached status
		status := p.serviceStatuses[name]
		statusText := "Unknown"
		switch status.Status {
		case "online":
			statusText = "Online"
		case "offline":
			statusText = "Offline"
		case "unknown":
			statusText = "Unknown"
		}

		serviceList = append(serviceList, serviceData{
			Name:         name,
			URL:          url,
			Icon:         icon,
			Description:  description,
			Status:       status.Status,
			StatusText:   statusText,
			ResponseTime: status.ResponseTime,
		})
	}

	data := struct {
		SectionTitle     string
		Services         []serviceData
		ShowStatus       bool
		ShowResponseTime bool
	}{
		SectionTitle:     sectionTitle,
		Services:         serviceList,
		ShowStatus:       showStatus,
		ShowResponseTime: showResponseTime,
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

func (p *ServicesPlugin) UpdateData(ctx context.Context) error {
	// Check services every 5 minutes
	if time.Since(p.lastCheck) < 5*time.Minute {
		return nil
	}

	config := p.storage.GetPluginConfig(p.Name())
	services, ok := config.Settings["services"].([]interface{})
	if !ok {
		return nil
	}

	for _, service := range services {
		serviceMap, ok := service.(map[string]interface{})
		if !ok {
			continue
		}

		name := p.getStringFromMap(serviceMap, "name", "")
		url := p.getStringFromMap(serviceMap, "url", "")

		if name == "" || url == "" {
			continue
		}

		go p.checkServiceStatus(name, url)
	}

	p.lastCheck = time.Now()
	return nil
}

func (p *ServicesPlugin) checkServiceStatus(name, url string) {
	start := time.Now()
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	status := ServiceStatus{
		Name:        name,
		URL:         url,
		Status:      "unknown",
		LastChecked: time.Now(),
	}

	resp, err := client.Get(url)
	if err != nil {
		status.Status = "offline"
	} else {
		resp.Body.Close()
		status.ResponseTime = time.Since(start).Milliseconds()

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			status.Status = "online"
		} else {
			status.Status = "offline"
		}
	}

	// Update cached status
	p.serviceStatuses[name] = status

	// Broadcast update
	p.hub.Broadcast("service_status_update", map[string]interface{}{
		"name":          name,
		"status":        status.Status,
		"response_time": status.ResponseTime,
	})
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

	// Broadcast update
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
