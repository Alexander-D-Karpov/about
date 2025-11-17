package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type WebringPlugin struct {
	storage     *storage.Storage
	hub         *stream.Hub
	webringData *WebringData
	lastUpdate  time.Time
	httpClient  *http.Client
}

type WebringData struct {
	Prev struct {
		ID      int    `json:"id"`
		Slug    string `json:"slug"`
		Name    string `json:"name"`
		URL     string `json:"url"`
		Favicon string `json:"favicon"`
	} `json:"prev"`
	Curr struct {
		ID      int    `json:"id"`
		Slug    string `json:"slug"`
		Name    string `json:"name"`
		URL     string `json:"url"`
		Favicon string `json:"favicon"`
	} `json:"curr"`
	Next struct {
		ID      int    `json:"id"`
		Slug    string `json:"slug"`
		Name    string `json:"name"`
		URL     string `json:"url"`
		Favicon string `json:"favicon"`
	} `json:"next"`
}

func NewWebringPlugin(storage *storage.Storage, hub *stream.Hub) *WebringPlugin {
	return &WebringPlugin{
		storage: storage,
		hub:     hub,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (p *WebringPlugin) Name() string { return "webring" }

func (p *WebringPlugin) Render(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	cfg := p.storage.GetPluginConfig(p.Name())
	settings := cfg.Settings

	base := getString(settings, "webring_url", "https://webring.otomir23.me")
	user := getString(settings, "username", "sanspie")

	var prevName, nextName, prevURL, nextURL, prevFavicon, nextFavicon string
	var hasPrevFavicon, hasNextFavicon bool

	if p.webringData != nil {
		prevName = p.webringData.Prev.Name
		nextName = p.webringData.Next.Name
		prevURL = p.webringData.Prev.URL
		nextURL = p.webringData.Next.URL

		if p.webringData.Prev.Favicon != "" {
			prevFavicon = fmt.Sprintf("%s/media/%s", strings.TrimRight(base, "/"), p.webringData.Prev.Favicon)
			hasPrevFavicon = true
		}
		if p.webringData.Next.Favicon != "" {
			nextFavicon = fmt.Sprintf("%s/media/%s", strings.TrimRight(base, "/"), p.webringData.Next.Favicon)
			hasNextFavicon = true
		}
	} else {
		prevName = "Loading..."
		nextName = "Loading..."
		prevURL = fmt.Sprintf("%s/prev/%s", strings.TrimRight(base, "/"), user)
		nextURL = fmt.Sprintf("%s/next/%s", strings.TrimRight(base, "/"), user)
	}

	homeURL := base

	tmpl := `
<section class="webring-section plugin" data-base-url="{{.BaseURL}}" data-username="{{.Username}}">
	<div class="plugin__inner">
		<div class="plugin-header">
			<h3 class="plugin-title">Webring</h3>
		</div>
		<nav class="webring-nav" role="navigation" aria-label="Webring navigation">
			<a href="{{.PrevURL}}" 
			   class="webring-prev" 
			   rel="prev external nofollow noopener"
			   title="Previous: {{.PrevName}}"
			   aria-label="Previous site: {{.PrevName}}">
				{{if .HasPrevFavicon}}
				<img src="{{.PrevFavicon}}" alt="" width="18" height="18" loading="lazy">
				{{else}}
				<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
					<path d="M15.41 7.41L14 6l-6 6 6 6 1.41-1.41L10.83 12z"/>
				</svg>
				{{end}}
				<span class="webring-text">← {{.PrevName}}</span>
			</a>
			
			<a href="{{.HomeURL}}" 
			   class="webring-home" 
			   rel="external nofollow noopener"
			   title="Webring Home"
			   aria-label="Webring home">
				<span class="webring-text">webring</span>
			</a>
			
			<a href="{{.NextURL}}" 
			   class="webring-next" 
			   rel="next external nofollow noopener"
			   title="Next: {{.NextName}}"
			   aria-label="Next site: {{.NextName}}">
				<span class="webring-text">{{.NextName}} →</span>
				{{if .HasNextFavicon}}
				<img src="{{.NextFavicon}}" alt="" width="18" height="18" loading="lazy">
				{{else}}
				<svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
					<path d="M8.59 16.59L10 18l6-6-6-6-1.41 1.41L13.17 12z"/>
				</svg>
				{{end}}
			</a>
		</nav>
	</div>
</section>`

	data := struct {
		PrevName       string
		NextName       string
		PrevURL        string
		NextURL        string
		PrevFavicon    string
		NextFavicon    string
		HasPrevFavicon bool
		HasNextFavicon bool
		HomeURL        string
		Username       string
		BaseURL        string
	}{
		PrevName:       prevName,
		NextName:       nextName,
		PrevURL:        prevURL,
		NextURL:        nextURL,
		PrevFavicon:    prevFavicon,
		NextFavicon:    nextFavicon,
		HasPrevFavicon: hasPrevFavicon,
		HasNextFavicon: hasNextFavicon,
		HomeURL:        homeURL,
		Username:       user,
		BaseURL:        strings.TrimRight(base, "/"),
	}

	t, err := template.New("webring").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	if err := t.Execute(&sb, data); err != nil {
		return "", err
	}

	return sb.String(), nil
}

func (p *WebringPlugin) UpdateData(ctx context.Context) error {
	// Update every hour, or immediately if no data exists
	if p.webringData != nil && time.Since(p.lastUpdate) < time.Hour {
		return nil
	}

	cfg := p.storage.GetPluginConfig(p.Name())
	settings := cfg.Settings

	base := getString(settings, "webring_url", "https://webring.otomir23.me")
	user := getString(settings, "username", "sanspie")

	if user == "" {
		return fmt.Errorf("webring username not configured")
	}

	dataURL := fmt.Sprintf("%s/%s/data", strings.TrimRight(base, "/"), user)

	req, err := http.NewRequest("GET", dataURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create webring request: %w", err)
	}

	req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		// Don't return error if we have cached data
		if p.webringData != nil {
			return nil
		}
		return fmt.Errorf("failed to fetch webring data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Don't return error if we have cached data
		if p.webringData != nil {
			return nil
		}
		return fmt.Errorf("webring API returned status %d", resp.StatusCode)
	}

	var newData WebringData
	if err := json.NewDecoder(resp.Body).Decode(&newData); err != nil {
		// Don't return error if we have cached data
		if p.webringData != nil {
			return nil
		}
		return fmt.Errorf("failed to decode webring data: %w", err)
	}

	// Only broadcast if data actually changed
	dataChanged := p.webringData == nil ||
		p.webringData.Prev.Name != newData.Prev.Name ||
		p.webringData.Next.Name != newData.Next.Name ||
		p.webringData.Prev.URL != newData.Prev.URL ||
		p.webringData.Next.URL != newData.Next.URL ||
		p.webringData.Prev.Favicon != newData.Prev.Favicon ||
		p.webringData.Next.Favicon != newData.Next.Favicon

	p.webringData = &newData
	p.lastUpdate = time.Now()

	if dataChanged {
		prevFavicon := fmt.Sprintf("%s/media/%s", strings.TrimRight(base, "/"), newData.Prev.Favicon)
		nextFavicon := fmt.Sprintf("%s/media/%s", strings.TrimRight(base, "/"), newData.Next.Favicon)

		p.hub.Broadcast("webring_update", map[string]interface{}{
			"prev": map[string]interface{}{
				"name":    newData.Prev.Name,
				"url":     newData.Prev.URL,
				"favicon": prevFavicon,
			},
			"next": map[string]interface{}{
				"name":    newData.Next.Name,
				"url":     newData.Next.URL,
				"favicon": nextFavicon,
			},
			"timestamp": time.Now().Unix(),
		})
	}

	return nil
}

func (p *WebringPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *WebringPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings

	p.lastUpdate = time.Time{}

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

func getString(m map[string]interface{}, key, def string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

func (p *WebringPlugin) RenderText(ctx context.Context) (string, error) {
	if p.webringData == nil {
		return "Webring: No data available", nil
	}

	return fmt.Sprintf("Webring: %s ← current → %s",
		p.webringData.Prev.Name, p.webringData.Next.Name), nil
}

func (p *PersonalPlugin) RenderText(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	personalInfo, ok := settings["info"].([]interface{})
	if !ok || len(personalInfo) == 0 {
		return "Personal: No information available", nil
	}

	return fmt.Sprintf("Personal: %d info items available", len(personalInfo)), nil
}
