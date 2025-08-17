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
	cfg := p.storage.GetPluginConfig(p.Name())
	settings := cfg.Settings

	base := getString(settings, "webring_url", "https://webring.otomir23.me")
	user := getString(settings, "username", "sanspie")

	var prevName, nextName, prevURL, nextURL, prevFavicon, nextFavicon string

	if p.webringData != nil {
		prevName = p.webringData.Prev.Name
		nextName = p.webringData.Next.Name
		prevURL = p.webringData.Prev.URL
		nextURL = p.webringData.Next.URL
		prevFavicon = fmt.Sprintf("%s/media/%s", strings.TrimRight(base, "/"), p.webringData.Prev.Favicon)
		nextFavicon = fmt.Sprintf("%s/media/%s", strings.TrimRight(base, "/"), p.webringData.Next.Favicon)
	} else {
		prevName = "Previous Site"
		nextName = "Next Site"
		prevURL = fmt.Sprintf("%s/prev/%s", strings.TrimRight(base, "/"), user)
		nextURL = fmt.Sprintf("%s/next/%s", strings.TrimRight(base, "/"), user)
		prevFavicon = "https://webring.otomir23.me/static/images/favicon.ico"
		nextFavicon = "https://webring.otomir23.me/static/images/favicon.ico"
	}

	homeURL := base

	tmpl := `
	<div class="webring-section section" data-base-url="{{.BaseURL}}" data-username="{{.Username}}">
		<div class="plugin-header">
			<h3 class="plugin-title">webring</h3>
		</div>
		<div class="plugin__inner">
			<div class="webring-nav">
				<a class="btn btn-ghost webring-prev" href="{{.PrevURL}}" rel="noopener" title="Previous site">
					<img src="{{.PrevFavicon}}" width="16" height="16" alt="">
					<span class="webring-text">← {{.PrevName}}</span>
				</a>
				<a class="btn webring-home" href="{{.HomeURL}}" rel="noopener" title="Ring home">
					<span class="webring-text">webring</span>
				</a>
				<a class="btn btn-ghost webring-next" href="{{.NextURL}}" rel="noopener" title="Next site">
					<span class="webring-text">{{.NextName}} →</span>
					<img src="{{.NextFavicon}}" width="16" height="16" alt="">
				</a>
			</div>
		</div>
	</div>`

	data := struct {
		PrevName    string
		NextName    string
		PrevURL     string
		NextURL     string
		PrevFavicon string
		NextFavicon string
		HomeURL     string
		Username    string
		BaseURL     string
	}{
		PrevName:    prevName,
		NextName:    nextName,
		PrevURL:     prevURL,
		NextURL:     nextURL,
		PrevFavicon: prevFavicon,
		NextFavicon: nextFavicon,
		HomeURL:     homeURL,
		Username:    user,
		BaseURL:     strings.TrimRight(base, "/"),
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
