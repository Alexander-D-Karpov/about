package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type WebringPlugin struct {
	storage     *storage.Storage
	hub         *stream.Hub
	webringData *WebringData
	siteCount   int
	lastUpdate  time.Time
	httpClient  *http.Client
	mutex       sync.RWMutex
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
			Timeout: 15 * time.Second,
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

	p.mutex.RLock()
	data := p.webringData
	p.mutex.RUnlock()

	if data != nil {
		prevName = data.Prev.Name
		nextName = data.Next.Name
		prevURL = data.Prev.URL
		nextURL = data.Next.URL

		if data.Prev.Favicon != "" {
			prevFavicon = fmt.Sprintf("%s/media/%s", strings.TrimRight(base, "/"), data.Prev.Favicon)
			hasPrevFavicon = true
		}
		if data.Next.Favicon != "" {
			nextFavicon = fmt.Sprintf("%s/media/%s", strings.TrimRight(base, "/"), data.Next.Favicon)
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
<section class="webring-section section plugin"
         data-w="2"
         data-base-url="{{.BaseURL}}"
         data-username="{{.Username}}">
	<header class="plugin-header">
		<h3 class="plugin-title">Webring</h3>
	</header>

	<div class="plugin__inner">
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

	templateData := struct {
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
	if err := t.Execute(&sb, templateData); err != nil {
		return "", err
	}

	return sb.String(), nil
}

func (p *WebringPlugin) UpdateData(ctx context.Context) error {
	// Update every hour
	if !p.lastUpdate.IsZero() && time.Since(p.lastUpdate) < time.Hour {
		return nil
	}

	cfg := p.storage.GetPluginConfig(p.Name())
	settings := cfg.Settings

	base := getString(settings, "webring_url", "https://webring.otomir23.me")
	user := getString(settings, "username", "sanspie")
	base = strings.TrimRight(base, "/")

	if user == "" {
		return fmt.Errorf("webring username not configured")
	}

	dataURL := fmt.Sprintf("%s/%s/data", base, user)
	req, err := http.NewRequestWithContext(ctx, "GET", dataURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "AboutPage/1.0")
		req.Header.Set("Accept", "application/json")

		resp, err := p.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var newData WebringData
				if err := json.NewDecoder(resp.Body).Decode(&newData); err == nil {
					p.mutex.Lock()
					p.webringData = &newData
					p.mutex.Unlock()

					prevFavicon := fmt.Sprintf("%s/media/%s", base, newData.Prev.Favicon)
					nextFavicon := fmt.Sprintf("%s/media/%s", base, newData.Next.Favicon)

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
			}
		}
	}

	// 2. Fetch Sites Count
	sitesURL := fmt.Sprintf("%s/sites/", base)
	sitesReq, err := http.NewRequestWithContext(ctx, "GET", sitesURL, nil)
	if err == nil {
		sitesReq.Header.Set("User-Agent", "AboutPage/1.0")
		sitesReq.Header.Set("Accept", "application/json")

		sitesResp, err := p.httpClient.Do(sitesReq)
		if err == nil {
			defer sitesResp.Body.Close()
			if sitesResp.StatusCode == http.StatusOK {
				var sites []interface{}
				if err := json.NewDecoder(sitesResp.Body).Decode(&sites); err == nil {
					p.mutex.Lock()
					p.siteCount = len(sites)
					p.mutex.Unlock()
				}
			}
		}
	}

	p.mutex.Lock()
	p.lastUpdate = time.Now()
	p.mutex.Unlock()

	return nil
}

func (p *WebringPlugin) GetSettings() map[string]interface{} {
	return p.storage.GetPluginConfig(p.Name()).Settings
}

func (p *WebringPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings
	p.mutex.Lock()
	p.lastUpdate = time.Time{}
	p.mutex.Unlock()
	return p.storage.SetPluginConfig(p.Name(), config)
}

func getString(m map[string]interface{}, key, def string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

func (p *WebringPlugin) RenderText(ctx context.Context) (string, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if p.webringData == nil {
		return "Webring: No data", nil
	}
	return fmt.Sprintf("Webring: %s ← current → %s (%d sites)",
		p.webringData.Prev.Name, p.webringData.Next.Name, p.siteCount), nil
}

func (p *WebringPlugin) GetMetrics() map[string]interface{} {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	status := 0
	if p.webringData != nil {
		status = 1
	}
	return map[string]interface{}{
		"status_ok":   status,
		"sites_count": p.siteCount,
	}
}
