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

func (p *SocialPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Links")

	links, ok := settings["links"].([]interface{})
	if !ok {
		return "", nil
	}
	tmpl := `
<section class="social-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
	</header>

	<div class="plugin__inner">
		<div class="social-links">
			{{range .Links}}
			<a href="{{.URL}}" title="{{.Name}}" target="_blank" rel="noopener" class="social-link">
				<img src="{{.IconURL}}" alt="{{.Name}} icon" class="icon icon-social" loading="lazy" decoding="async">
			</a>
			{{end}}
		</div>
	</div>
</section>`

	type socialLink struct {
		Name    string
		URL     string
		IconURL string
	}

	var socialLinks []socialLink

	for _, link := range links {
		linkMap, ok := link.(map[string]interface{})
		if !ok {
			continue
		}

		name := p.getStringFromMap(linkMap, "name", "Link")
		url := p.getStringFromMap(linkMap, "url", "#")

		// Allow optional direct icon path override
		iconURL := p.getStringFromMap(linkMap, "iconPath", "")
		if strings.TrimSpace(iconURL) == "" {
			iconField := p.getStringFromMap(linkMap, "icon", "link")
			iconField = strings.TrimSpace(iconField)

			switch {
			case iconField == "":
				iconURL = "/static/icons/link.svg"

			case strings.HasPrefix(iconField, "/"),
				strings.HasPrefix(iconField, "http://"),
				strings.HasPrefix(iconField, "https://"):
				// already a full path or URL
				iconURL = iconField

			case strings.Contains(iconField, "."):
				// has extension → keep it
				iconURL = "/static/icons/" + iconField

			default:
				// plain slug → default to .svg
				iconURL = "/static/icons/" + iconField + ".svg"
			}
		}
		socialLinks = append(socialLinks, socialLink{
			Name:    name,
			URL:     url,
			IconURL: iconURL,
		})
	}

	data := struct {
		SectionTitle string
		Links        []socialLink
	}{
		SectionTitle: sectionTitle,
		Links:        socialLinks,
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
