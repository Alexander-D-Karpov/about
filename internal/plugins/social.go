package plugins

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type SocialPlugin struct {
	storage *storage.Storage
	hub     *stream.Hub
}

func NewSocialPlugin(storage *storage.Storage, hub *stream.Hub) *SocialPlugin {
	return &SocialPlugin{
		storage: storage,
		hub:     hub,
	}
}

func (p *SocialPlugin) Name() string {
	return "social"
}

// socialIconSlug reduces an icon reference (or the link's name, when no icon is
// configured) to a stable [a-z0-9-] token. It is emitted as data-icon so CSS can
// target individual brands - some logos are pure-white artwork meant for dark
// backgrounds and need correcting in light mode.
func socialIconSlug(iconField, name string) string {
	s := strings.ToLower(strings.TrimSpace(iconField))
	if s != "" {
		// An icon reference may be a path or filename: keep the basename and
		// drop the extension. Only do this here - a display name like
		// "Last.fm" must not be truncated at its dot.
		if i := strings.LastIndexByte(s, '/'); i >= 0 {
			s = s[i+1:]
		}
		if i := strings.LastIndexByte(s, '.'); i >= 0 {
			s = s[:i]
		}
	} else {
		s = strings.ToLower(strings.TrimSpace(name))
	}

	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '_', r == '-':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func (p *SocialPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Links")

	links, ok := settings["links"].([]interface{})
	if !ok {
		return "", nil
	}

	pages := []struct {
		Name    string
		URL     string
		IconSVG template.HTML
	}{
		{
			Name:    "Tier Rankings",
			URL:     "/ranking",
			IconSVG: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="12" width="4" height="9"/><rect x="10" y="3" width="4" height="18"/><rect x="17" y="8" width="4" height="13"/></svg>`,
		},
	}

	tmpl := `
<section class="social-section section plugin" data-w="1">
	<header class="plugin-header">
		<h2 class="plugin-title">{{.SectionTitle}}</h2>
	</header>

	<div class="plugin__inner">
		<div class="social-links">
			{{range .Links}}
			<a href="{{.URL}}" title="{{.Name}}" target="_blank" rel="noopener" class="social-link" data-icon="{{.Slug}}">
				<img src="{{.IconURL}}" alt="{{.Name}} icon" class="icon icon-social" width="20" height="20" loading="lazy" decoding="async">
			</a>
			{{end}}
		</div>

		{{if .Pages}}
		<div class="social-pages">
			{{range .Pages}}
			<a href="{{.URL}}" title="{{.Name}}" class="social-page-link">
				<span class="social-page-icon">{{.IconSVG}}</span>
				<span class="social-page-name">{{.Name}}</span>
			</a>
			{{end}}
		</div>
		{{end}}
	</div>
</section>`

	type socialLink struct {
		Name    string
		URL     string
		IconURL string
		Slug    string
	}

	type pageLink struct {
		Name    string
		URL     string
		IconSVG template.HTML
	}

	var socialLinks []socialLink

	for _, link := range links {
		linkMap, ok := link.(map[string]interface{})
		if !ok {
			continue
		}

		name := p.getStringFromMap(linkMap, "name", "Link")
		url := p.getStringFromMap(linkMap, "url", "#")

		iconField := strings.TrimSpace(p.getStringFromMap(linkMap, "icon", ""))

		iconURL := p.getStringFromMap(linkMap, "iconPath", "")
		if strings.TrimSpace(iconURL) == "" {
			switch {
			case iconField == "":
				iconURL = "/static/icons/link.svg"
			case strings.HasPrefix(iconField, "/"),
				strings.HasPrefix(iconField, "http://"),
				strings.HasPrefix(iconField, "https://"):
				iconURL = iconField
			case strings.Contains(iconField, "."):
				iconURL = "/static/icons/" + iconField
			default:
				iconURL = "/static/icons/" + iconField + ".svg"
			}
		}
		socialLinks = append(socialLinks, socialLink{
			Name:    name,
			URL:     url,
			IconURL: iconURL,
			Slug:    socialIconSlug(iconField, name),
		})
	}

	var pageLinks []pageLink
	for _, pg := range pages {
		pageLinks = append(pageLinks, pageLink{
			Name:    pg.Name,
			URL:     pg.URL,
			IconSVG: pg.IconSVG,
		})
	}

	data := struct {
		SectionTitle string
		Links        []socialLink
		Pages        []pageLink
	}{
		SectionTitle: sectionTitle,
		Links:        socialLinks,
		Pages:        pageLinks,
	}

	t, err := template.New("social").Parse(tmpl)
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

func (p *SocialPlugin) UpdateData(ctx context.Context) error {
	return nil
}

func (p *SocialPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *SocialPlugin) SetSettings(settings map[string]interface{}) error {
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

func (p *SocialPlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
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

func (p *SocialPlugin) getStringFromMap(m map[string]interface{}, key string, defaultValue string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return defaultValue
}

func (p *SocialPlugin) RenderText(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	links, ok := settings["links"].([]interface{})
	if !ok || len(links) == 0 {
		return "Social: No links configured", nil
	}

	var linkNames []string
	for i, link := range links {
		linkMap, ok := link.(map[string]interface{})
		if !ok {
			continue
		}
		name := p.getStringFromMap(linkMap, "name", "Link")

		if i == 0 {
			url := p.getStringFromMap(linkMap, "url", "")
			if url != "" {
				name = fmt.Sprintf("%s (%s)", name, url)
			}
		}
		linkNames = append(linkNames, name)

		if i >= 4 {
			break
		}
	}

	if len(linkNames) == 0 {
		return "Social: No valid links", nil
	}

	s := fmt.Sprintf("Social: %s", strings.Join(linkNames, ", "))
	if len(links) > 5 {
		s += fmt.Sprintf(" and %d more", len(links)-5)
	}
	return s, nil
}

func (p *SocialPlugin) GetMetrics() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	links, ok := config.Settings["links"].([]interface{})

	metrics := map[string]interface{}{
		"total_links": 0,
	}

	if ok {
		metrics["total_links"] = len(links)
	}

	return metrics
}
