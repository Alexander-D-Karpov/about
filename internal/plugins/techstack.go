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
		<h2 class="plugin-title">{{.SectionTitle}}</h2>
	</header>

	<div class="plugin__inner">
		<div class="tech-groups">
			{{range .Groups}}
			<div class="tech-group">
				<div class="tech-group__label">{{.Name}}</div>
				<div class="tech-grid">
					{{range .Items}}
					<button type="button" class="tech-item" title="Show projects using {{.Name}}">
						<img src="{{.IconURL}}" alt="" class="icon icon-tech" width="18" height="18" loading="lazy" decoding="async">
						<span class="tech-name">{{.Name}}</span>
					</button>
					{{end}}
				</div>
			</div>
			{{end}}
		</div>
	</div>
</section>`

	type tech struct {
		Name     string
		IconURL  string
		Category string
	}

	var technologies []tech
	for _, t := range techs {
		techMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := techMap["name"].(string)

		iconURL := techIconURL(techMap)

		category, _ := techMap["category"].(string)
		category = strings.TrimSpace(category)
		if category == "" {
			category = defaultTechCategory(name)
		}

		technologies = append(technologies, tech{
			Name:     name,
			IconURL:  iconURL,
			Category: category,
		})
	}

	// Groups keep the configured order of their first member, so a category
	// set in the config lands where its first technology was listed.
	type group struct {
		Name  string
		Items []tech
	}
	var groups []group
	groupIdx := map[string]int{}
	for _, cat := range techCategoryOrder {
		groupIdx[cat] = len(groups)
		groups = append(groups, group{Name: cat})
	}
	for _, t := range technologies {
		i, ok := groupIdx[t.Category]
		if !ok {
			i = len(groups)
			groupIdx[t.Category] = i
			groups = append(groups, group{Name: t.Category})
		}
		groups[i].Items = append(groups[i].Items, t)
	}
	nonEmpty := groups[:0]
	for _, g := range groups {
		if len(g.Items) > 0 {
			nonEmpty = append(nonEmpty, g)
		}
	}
	groups = nonEmpty

	tmplParsed, err := template.New("tech").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = tmplParsed.Execute(&buf, struct {
		SectionTitle string
		Groups       []group
	}{
		SectionTitle: sectionTitle,
		Groups:       groups,
	})
	return buf.String(), err
}

// techIconURL resolves a technology entry's icon: an explicit iconPath, a
// full path/URL or filename in "icon", or a plain slug under /static/icons.
func techIconURL(techMap map[string]interface{}) string {
	if v, ok := techMap["iconPath"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	raw, _ := techMap["icon"].(string)
	icon := strings.TrimSpace(raw)
	// Older configs reused the database/container logos for extensions. Keep
	// explicit custom icons intact while migrating those legacy defaults.
	name, _ := techMap["name"].(string)
	if strings.EqualFold(strings.TrimSpace(name), "pgvector") && icon == "postgresql" {
		icon = "pgvector"
	}
	if strings.EqualFold(strings.TrimSpace(name), "Docker Compose") && icon == "docker" {
		icon = "docker-compose"
	}
	if (strings.EqualFold(strings.TrimSpace(name), "DRF") || strings.EqualFold(strings.TrimSpace(name), "Django REST Framework")) && icon == "django" {
		icon = "drf"
	}
	switch {
	case icon == "":
		return "/static/icons/stack.svg"
	case strings.HasPrefix(icon, "/"), strings.HasPrefix(icon, "http://"), strings.HasPrefix(icon, "https://"):
		return icon
	case strings.Contains(icon, "."):
		return "/static/icons/" + icon
	default:
		return "/static/icons/" + icon + ".svg"
	}
}

// techIconAliases maps other spellings used in project tags to the name a
// technology has on the Technologies card.
var techIconAliases = map[string]string{
	"golang": "go", "postgres": "postgresql", "js": "javascript", "ts": "typescript",
	"tailwind": "tailwind css", "k8s": "kubernetes", "c plus plus": "c++",
	"github action": "github actions", "compose": "docker compose", "openai": "openai api",
	"html5": "html", "css3": "css", "node": "javascript", "nodejs": "javascript", "pg": "postgresql",
}

// TechIconIndex maps lower-cased technology names (from the Technologies card
// config) to their icon URLs, so other cards can draw a tag as its icon.
func TechIconIndex(st *storage.Storage) map[string]string {
	out := map[string]string{}
	techs, _ := st.GetPluginConfig("techstack").Settings["technologies"].([]interface{})
	for _, t := range techs {
		m, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			out[name] = techIconURL(m)
		}
	}
	return out
}

func lookupTechIcon(index map[string]string, name string) string {
	k := strings.ToLower(strings.TrimSpace(name))
	if u, ok := index[k]; ok {
		return u
	}
	if a, ok := techIconAliases[k]; ok {
		return index[a]
	}
	// Project-only technologies need not be added to the curated tech stack.
	return projectTechIcons[k]
}

var projectTechIcons = map[string]string{
	"bubbletea":             "/static/icons/bubbletea.png",
	"chromium":              "/static/icons/chromium-transparent.svg",
	"drf":                   "/static/icons/drf.svg",
	"django rest framework": "/static/icons/drf.svg",
	"sqlite":                "/static/icons/sqlite.svg",
	"numpy":                 "/static/icons/numpy.svg",
	"pandas":                "/static/icons/pandas.svg",
	"opencv":                "/static/icons/opencv.svg",
	"webrtc":                "/static/icons/webrtc.svg",
	"telethon":              "/static/icons/telethon.svg",
	"python-telegram-bot":   "/static/icons/python-telegram-bot.svg",
	"ffmpeg":                "/static/icons/ffmpeg.svg",
	"fyne":                  "/static/icons/fyne.png",
	"jupyter":               "/static/icons/jupyter.svg",
	"osm":                   "/static/icons/openstreetmap.svg",
	"overpassturbo":         "/static/icons/overpassturbo.ico",
	"webassembly":           "/static/icons/webassembly.svg",
	"platformio":            "/static/icons/platformio.svg",
	"esp8266":               "/static/icons/espressif.svg",
	"raspberry pi":          "/static/icons/raspberrypi.svg",
}

// techCategoryOrder is the display order of the built-in categories; any other
// category named in the config follows them.
var techCategoryOrder = []string{"Backend", "Frontend", "Data", "DevOps", "Other"}

// defaultTechCategory files a technology under a category when its config entry
// does not name one ("category": "..." overrides this).
func defaultTechCategory(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "python", "go", "golang", "django", "fastapi", "flask", "graphql", "websockets", "websocket", "node", "node.js", "nodejs", "celery", "grpc", "aiogram", "sqlalchemy":
		return "Backend"
	case "javascript", "typescript", "react", "vue", "svelte", "html", "css", "tailwind", "tailwind css", "sass", "electron":
		return "Frontend"
	case "postgresql", "postgres", "redis", "mysql", "sqlite", "mongodb", "clickhouse", "elasticsearch", "rabbitmq", "kafka", "pgvector", "pytorch", "transformers", "openai api":
		return "Data"
	case "linux", "docker", "nginx", "git", "bash", "kubernetes", "k8s", "ansible", "terraform", "github actions", "gitlab ci", "docker compose", "prometheus", "opentelemetry", "sentry", "grafana":
		return "DevOps"
	default:
		return "Other"
	}
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

func (p *TechStackPlugin) GetMetrics() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	techs, ok := config.Settings["technologies"].([]interface{})

	metrics := map[string]interface{}{
		"total_technologies": 0,
	}

	if ok {
		metrics["total_technologies"] = len(techs)
	}

	return metrics
}
