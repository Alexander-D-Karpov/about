package plugins

import (
	"context"
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
	techs, ok := config.Settings["technologies"].([]interface{})
	if !ok {
		return "", nil
	}

	// Icons are served from /static. Each entry can have:
	// - icon: slug -> /static/icons/<slug>.svg
	// - iconPath: direct path under static (e.g. /static/icons/tech/django.svg)
	tmpl := `
	<div class="tech-section section">
		<h3>Technologies</h3>
		<div class="tech-grid">
			{{range .Technologies}}
			<div class="tech-item" title="{{.Name}}">
				<img src="{{.IconURL}}" alt="{{.Name}} logo" class="icon icon-tech" loading="lazy">
				<span class="tech-name">{{.Name}}</span>
			</div>
			{{end}}
		</div>
	</div>`

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
		if v, ok := techMap["iconPath"].(string); ok && v != "" {
			iconURL = v
		} else {
			if slug, ok := techMap["icon"].(string); ok && slug != "" {
				iconURL = "/static/icons/" + slug + ".svg"
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
	err = tmplParsed.Execute(&buf, struct{ Technologies []tech }{Technologies: technologies})
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
