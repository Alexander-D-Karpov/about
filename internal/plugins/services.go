package plugins

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
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
	Status       string    `json:"status"` // "online" | "offline" | "unknown"
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

func (p *ServicesPlugin) Name() string { return "services" }

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
	compact := p.getConfigBool(settings, "ui.compact", false)
	sortBy := p.getConfigValue(settings, "ui.sort", "configured") // "configured" | "status" | "latency"

	p.mutex.RLock()
	lastUpdated := p.lastCheck
	p.mutex.RUnlock()
	lastUpdatedText := "never"
	stale := true
	if !lastUpdated.IsZero() {
		lastUpdatedText = p.formatTimeAgo(lastUpdated)
		stale = time.Since(lastUpdated) > 5*time.Minute
	}

	const tmpl = `
<section class="services-section section plugin {{if .Compact}}is-compact{{end}}" data-w="2" aria-labelledby="services-title" {{if .Stale}}data-stale="true"{{end}}>
	<header class="plugin-header services-header">
		<h3 id="services-title" class="plugin-title">{{.SectionTitle}}</h3>
		<div class="services-header__meta">
			<span class="services-updated" aria-live="polite" title="Last update">{{.LastUpdatedText}}</span>
		</div>
	</header>

	<div class="plugin__inner">
		<div class="services-grid" role="list">
			{{range .Services}}
			<article class="service-item" role="listitem"
			         data-url="{{.URL}}" data-status="{{.Status}}" data-rt="{{.ResponseTime}}" data-code="{{.StatusCode}}">
				{{if .URL}}
				<a class="card-overlay" href="{{.URL}}" target="_blank" rel="noopener noreferrer" aria-label="Open {{.Name}}"></a>
				{{end}}
				<div class="service-head">
					<span class="service-icon" aria-hidden="true">
						{{- if .IsImage -}}
							<img src="{{.Icon}}" alt="" loading="lazy" decoding="async" />
						{{- else if .IsInlineSVG -}}
							{{.IconHTML}}
						{{- else if .Icon -}}
							{{.Icon}}
						{{- else -}}
							⚙️
						{{- end -}}
					</span>
					<div class="service-id">
						{{if .URL}}
							<span class="service-title service-title-link">{{.Name}}</span>
						{{else}}
							<span class="service-title">{{.Name}}</span>
						{{end}}
						{{if $.ShowStatus}}
						<span class="service-statuschip status-{{.Status}}" title="{{.StatusText}}{{if .ResponseTime}} • {{.ResponseTime}}ms{{end}}">
							<span class="status-dot {{if eq .Status "online"}}status-online{{else if eq .Status "offline"}}status-offline{{end}}" aria-hidden="true"></span>
							<span class="sr-only">{{.StatusText}}</span>
							<span class="status-label">{{.StatusText}}</span>
						</span>
						{{end}}
					</div>
				</div>


				{{if .Description}}
				<p class="service-description">{{.Description}}</p>
				{{end}}

				<div class="service-stats">
					{{if and $.ShowResponseTime .ResponseTime}}
						<span class="chip chip-rt" title="Response time">{{.ResponseTime}}ms</span>
					{{else if eq .Status "unknown"}}
						<span class="chip chip-skeleton" title="Waiting for first check…">—</span>
					{{end}}
					{{if .StatusCode}}
						<span class="chip chip-code" data-code="{{.StatusCode}}" title="HTTP status">{{.StatusCode}}</span>
					{{end}}
					{{if .LastCheckedText}}
						<span class="chip chip-last" title="Last checked">{{.LastCheckedText}}</span>
					{{end}}
				</div>
			</article>
			{{end}}
		</div>

		<footer class="services-summary" aria-label="Summary">
			<div class="summary-item" title="Number of online services">
				<span class="summary-count online-count">{{.OnlineCount}}</span>
				<span class="summary-label">Online</span>
			</div>
			<div class="summary-item" title="Number of offline services">
				<span class="summary-count offline-count">{{.OfflineCount}}</span>
				<span class="summary-label">Offline</span>
			</div>
			<div class="summary-item" title="Total services">
				<span class="summary-count total-count">{{.TotalCount}}</span>
				<span class="summary-label">Total</span>
			</div>
		</footer>
	</div>
</section>`

	type serviceData struct {
		Name            string
		URL             string
		Icon            string
		IconHTML        template.HTML
		IsImage         bool
		IsInlineSVG     bool
		Description     string
		Status          string
		StatusText      string
		ResponseTime    int64
		StatusCode      int
		LastChecked     time.Time
		LastCheckedText string
	}

	var list []serviceData
	onlineCount := 0
	offlineCount := 0

	p.mutex.RLock()
	for _, raw := range services {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name := p.getStringFromMap(m, "name", "Service")
		url := p.getStringFromMap(m, "url", "")
		iconStr := p.getIconString(m) // <— use new helper
		description := p.getStringFromMap(m, "description", "")

		isImg := looksLikeImagePath(iconStr)
		isSVG := false
		var iconHTML template.HTML
		if !isImg && isInlineSVGMarkup(iconStr) {
			isSVG = true
			iconHTML = template.HTML(iconStr) // admin-only content; treated as trusted
		}

		st := p.serviceStatuses[name]
		statusText := "Unknown"
		switch st.Status {
		case "online":
			statusText = "Online"
			onlineCount++
		case "offline":
			statusText = "Offline"
			offlineCount++
		}

		lastCheckedText := ""
		if !st.LastChecked.IsZero() {
			lastCheckedText = p.formatTimeAgo(st.LastChecked)
		}

		list = append(list, serviceData{
			Name:            name,
			URL:             url,
			Icon:            iconStr,
			IconHTML:        iconHTML,
			IsImage:         isImg,
			IsInlineSVG:     isSVG,
			Description:     description,
			Status:          st.Status,
			StatusText:      statusText,
			ResponseTime:    st.ResponseTime,
			StatusCode:      st.StatusCode,
			LastChecked:     st.LastChecked,
			LastCheckedText: lastCheckedText,
		})
	}
	p.mutex.RUnlock()

	switch sortBy {
	case "status":
		sort.SliceStable(list, func(i, j int) bool {
			pri := func(s string) int {
				switch s {
				case "online":
					return 0
				case "unknown":
					return 1
				case "offline":
					return 2
				default:
					return 3
				}
			}
			pi, pj := pri(list[i].Status), pri(list[j].Status)
			if pi != pj {
				return pi < pj
			}
			return list[i].Name < list[j].Name
		})
	case "latency":
		sort.SliceStable(list, func(i, j int) bool {
			ri := list[i].ResponseTime
			rj := list[j].ResponseTime
			if ri == 0 && rj != 0 {
				return false
			}
			if rj == 0 && ri != 0 {
				return true
			}
			if ri == rj {
				return list[i].Name < list[j].Name
			}
			return ri < rj
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
		LastUpdatedText  string
		Stale            bool
		Compact          bool
	}{
		SectionTitle:     sectionTitle,
		Services:         list,
		ShowStatus:       showStatus,
		ShowResponseTime: showResponseTime,
		OnlineCount:      onlineCount,
		OfflineCount:     offlineCount,
		TotalCount:       len(list),
		LastUpdatedText:  lastUpdatedText,
		Stale:            stale,
		Compact:          compact,
	}

	t, err := template.New("services").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (p *ServicesPlugin) renderNoServices() string {
	return `<section class="services-section section plugin" data-w="2" aria-labelledby="services-title">
		<header class="plugin-header services-header">
			<h3 id="services-title" class="plugin-title">Services</h3>
		</header>
		<div class="plugin__inner">
			<div class="no-services">
				<p class="text-muted">No services configured</p>
				<p class="text-muted">Add services in the admin panel to monitor their status</p>
			</div>
		</div>
	</section>`
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
		go func(serviceName, serviceURL, desc, icn string) {
			defer wg.Done()
			p.checkServiceStatus(serviceName, serviceURL, desc, icn)
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
	req = req.WithContext(context.Background())
	req.Header.Set("User-Agent", "AboutPage-ServiceMonitor/1.0")
	req.Header.Set("Accept", "*/*")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		status.Status = "offline"
		status.ResponseTime = time.Since(start).Milliseconds()
	} else {
		_ = resp.Body.Close()
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
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func (p *ServicesPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *ServicesPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings

	if err := p.storage.SetPluginConfig(p.Name(), config); err != nil {
		return err
	}

	p.lastCheck = time.Time{}
	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(),
		"action": "settings_changed",
	})
	return nil
}

func (p *ServicesPlugin) getConfigValue(settings map[string]interface{}, key string, def string) string {
	keys := strings.Split(key, ".")
	cur := settings
	for i, k := range keys {
		if i == len(keys)-1 {
			if v, ok := cur[k].(string); ok {
				return v
			}
			return def
		}
		next, ok := cur[k].(map[string]interface{})
		if !ok {
			return def
		}
		cur = next
	}
	return def
}

func (p *ServicesPlugin) getConfigBool(settings map[string]interface{}, key string, def bool) bool {
	keys := strings.Split(key, ".")
	cur := settings
	for i, k := range keys {
		if i == len(keys)-1 {
			if v, ok := cur[k].(bool); ok {
				return v
			}
			return def
		}
		next, ok := cur[k].(map[string]interface{})
		if !ok {
			return def
		}
		cur = next
	}
	return def
}

func (p *ServicesPlugin) getStringFromMap(m map[string]interface{}, key, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

func (p *ServicesPlugin) RenderText(ctx context.Context) (string, error) {
	p.mutex.RLock()
	online := 0
	offline := 0
	total := len(p.serviceStatuses)
	for _, st := range p.serviceStatuses {
		switch st.Status {
		case "online":
			online++
		case "offline":
			offline++
		}
	}
	p.mutex.RUnlock()

	if total == 0 {
		return "Services: No services configured", nil
	}
	return fmt.Sprintf("Services: %d online, %d offline (%d total)", online, offline, total), nil
}

func (p *ServicesPlugin) getIconString(m map[string]interface{}) string {
	// Accept several common keys from admin payloads
	for _, k := range []string{"icon", "iconPath", "icon_url", "logo", "image"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func looksLikeImagePath(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "/") || strings.HasPrefix(s, "data:image/") {
		return true
	}
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg", ".ico", ".bmp"} {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}

func isInlineSVGMarkup(s string) bool {
	trim := strings.TrimSpace(s)
	// Simple check; admin-provided content only
	return strings.HasPrefix(trim, "<svg") && strings.Contains(trim, "</svg>")
}
