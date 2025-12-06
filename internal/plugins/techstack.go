package plugins

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type TechStackPlugin struct {
	storage *storage.Storage
	hub     *stream.Hub
}

func NewTechStackPlugin(storage *storage.Storage, hub *stream.Hub) *TechStackPlugin {
	return &TechStackPlugin{
		storage: storage,
		hub:     hub,
	}
}

func (p *TechStackPlugin) Name() string {
	return "techstack"
}

func (p *TechStackPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	techs, ok := settings["technologies"].([]interface{})
	if !ok {
		return "", nil
	}

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Technologies")

	// Icons are served from /static. Each entry can have:
	// - icon: slug -> /static/icons/<slug>.svg
	// - iconPath: direct path under static (e.g. /static/icons/tech/django.svg)
	tmpl := `
<section class="tech-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
	</header>

	<div class="plugin__inner">
		<div class="tech-grid">
			{{range .Technologies}}
			<div class="tech-item" title="{{.Name}}">
				<img src="{{.IconURL}}" alt="{{.Name}} logo" class="icon icon-tech" loading="lazy" decoding="async">
				<span class="tech-name">{{.Name}}</span>
			</div>
			{{end}}
		</div>
	</div>
</section>`

	type tech struct {
		Name    string
		IconURL string
	}

	var technologies []tech
	for _, t := range techs {
		techMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := techMap["name"].(string)

		iconURL := ""
		if v, ok := techMap["iconPath"].(string); ok && strings.TrimSpace(v) != "" {
			// explicit direct path
			iconURL = strings.TrimSpace(v)
		} else {
			// allow full path/URL in "icon" OR fall back to slug under /static/icons
			if raw, ok := techMap["icon"].(string); ok && strings.TrimSpace(raw) != "" {
				icon := strings.TrimSpace(raw)

				switch {
				case strings.HasPrefix(icon, "/"),
					strings.HasPrefix(icon, "http://"),
					strings.HasPrefix(icon, "https://"):
					// already a full path or URL
					iconURL = icon

				case strings.Contains(icon, "."):
					// looks like "foo.png" or "vendor/foo.svg" → keep extension as provided
					iconURL = "/static/icons/" + icon

				default:
					// plain slug → default to .svg in /static/icons
					iconURL = "/static/icons/" + icon + ".svg"
				}
			} else {
				// safe fallback
				iconURL = "/static/icons/stack.svg"
			}
		}

		technologies = append(technologies, tech{
			Name:    name,
			IconURL: iconURL,
		})
	}

	tmplParsed, err := template.New("tech").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = tmplParsed.Execute(&buf, struct {
		SectionTitle string
		Technologies []tech
	}{
		SectionTitle: sectionTitle,
		Technologies: technologies,
	})
	return buf.String(), err
}

func (p *TechStackPlugin) UpdateData(ctx context.Context) error {
	return nil
}

func (p *TechStackPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *TechStackPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings
	return p.storage.SetPluginConfig(p.Name(), config)
}

func (p *TechStackPlugin) RenderText(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	techs, ok := config.Settings["technologies"].([]interface{})
	if !ok || len(techs) == 0 {
		return "Tech: No technologies configured", nil
	}

	var techNames []string
	for _, t := range techs {
		techMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := techMap["name"].(string); ok {
			techNames = append(techNames, name)
		}
	}

	if len(techNames) == 0 {
		return "Tech: No valid technologies", nil
	}

	if len(techNames) > 8 {
		return fmt.Sprintf("Tech: %s and %d more", strings.Join(techNames[:8], ", "), len(techNames)-8), nil
	}

	return fmt.Sprintf("Tech: %s", strings.Join(techNames, ", ")), nil
}

func (p *TechStackPlugin) getConfigValue(settings map[string]interface{}, s string, s2 string) string {
	keys := strings.Split(s, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k]; ok {
				if strValue, ok := value.(string); ok {
					return strValue
				}
			}
			return s2
		} else {
			if next, ok := current[k].(map[string]interface{}); ok {
				current = next
			} else {
				return s2
			}
		}
	}
	return s2
}
