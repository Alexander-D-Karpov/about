package plugins

import (
	"context"
	"fmt"
	"html/template"
	"runtime"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type InfoPlugin struct {
	storage   *storage.Storage
	hub       *stream.Hub
	startTime time.Time
	buildTime string
	version   string
	gitCommit string
}

func NewInfoPlugin(storage *storage.Storage, hub *stream.Hub) *InfoPlugin {
	return &InfoPlugin{
		storage:   storage,
		hub:       hub,
		startTime: time.Now(),
		buildTime: "2025-01-15T10:30:00Z",
		version:   "1.0.0",
		gitCommit: "abc123f",
	}
}

func (p *InfoPlugin) Name() string {
	return "info"
}

func (p *InfoPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Page Info")
	showServerInfo := p.getConfigBool(settings, "ui.showServerInfo", true)
	showSourceCode := p.getConfigBool(settings, "ui.showSourceCode", true)

	sourceCodeURL := p.getConfigValue(settings, "sourceCodeURL", "https://github.com/Alexander-D-Karpov/about")

	currentTime := time.Now()
	uptime := currentTime.Sub(p.startTime)
	uptimeStr := p.formatDuration(uptime)
	lastUpdated := currentTime.Format("15:04:05")
	connectedClients := p.hub.GetClientCount()

	tmpl := `
	<div class="info-section section">
		<div class="plugin-header">
			<h3 class="plugin-title">{{.SectionTitle}}</h3>
		</div>
		<div class="plugin__inner">
			{{if .ShowServerInfo}}
			<div class="info-grid">
				<div class="info-item">
					<span class="info-label">Status</span>
					<span class="info-value">
						<span class="status-indicator status-online"></span>
						Online
					</span>
				</div>
				<div class="info-item">
					<span class="info-label">Uptime</span>
					<span class="info-value" data-uptime>{{.Uptime}}</span>
				</div>
				<div class="info-item">
					<span class="info-label">Clients</span>
					<span class="info-value" id="connected-clients">{{.ConnectedClients}}</span>
				</div>
				<div class="info-item">
					<span class="info-label">Updated</span>
					<span class="info-value" id="last-updated">{{.LastUpdated}}</span>
				</div>
			</div>
			{{end}}
			
			{{if .ShowSourceCode}}
			<div class="source-info">
				<div class="source-links">
					<a href="{{.SourceCodeURL}}" target="_blank" rel="noopener" class="btn btn-sm">
						<svg viewBox="0 0 24 24" width="16" height="16">
							<path fill="currentColor" d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
						</svg>
						Source Code
					</a>
				</div>
			</div>
			{{end}}
		</div>
	</div>`

	data := struct {
		SectionTitle     string
		ShowServerInfo   bool
		ShowSourceCode   bool
		Uptime           string
		ConnectedClients int
		LastUpdated      string
		SourceCodeURL    string
	}{
		SectionTitle:     sectionTitle,
		ShowServerInfo:   showServerInfo,
		ShowSourceCode:   showSourceCode,
		Uptime:           uptimeStr,
		ConnectedClients: connectedClients,
		LastUpdated:      lastUpdated,
		SourceCodeURL:    sourceCodeURL,
	}

	t, err := template.New("info").Parse(tmpl)
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

func (p *InfoPlugin) UpdateData(ctx context.Context) error {
	currentTime := time.Now()
	uptime := currentTime.Sub(p.startTime)
	clientCount := p.hub.GetClientCount()

	p.hub.Broadcast("system_update", map[string]interface{}{
		"uptime":            uptime.Seconds(),
		"uptime_text":       p.formatDuration(uptime),
		"goroutines":        runtime.NumGoroutine(),
		"connected_clients": clientCount,
		"last_updated":      currentTime.Format("15:04:05"),
		"timestamp":         currentTime.Unix(),
	})

	return nil
}

func (p *InfoPlugin) formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}

func (p *InfoPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *InfoPlugin) SetSettings(settings map[string]interface{}) error {
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

func (p *InfoPlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
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

func (p *InfoPlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
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
