package plugins

import (
	"context"
	"html/template"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type PersonalPlugin struct {
	storage *storage.Storage
	hub     *stream.Hub
}

type PersonalInfo struct {
	Title           string `json:"title"`
	Content         string `json:"content"`
	Image           string `json:"image"`
	Icon            string `json:"icon"`
	Category        string `json:"category"`
	RenderedContent string `json:"rendered_content"`
}

func NewPersonalPlugin(storage *storage.Storage, hub *stream.Hub) *PersonalPlugin {
	return &PersonalPlugin{
		storage: storage,
		hub:     hub,
	}
}

func (p *PersonalPlugin) Name() string {
	return "personal"
}

func (p *PersonalPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	personalInfo, ok := settings["info"].([]interface{})
	if !ok || len(personalInfo) == 0 {
		return "", nil
	}

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Personal Info")
	showImages := p.getConfigBool(settings, "ui.showImages", true)
	showCategories := p.getConfigBool(settings, "ui.showCategories", true)
	layout := p.getConfigValue(settings, "ui.layout", "grid") // "grid" or "list"

	tmpl := `
<section class="personal-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
	</header>
	<div class="plugin__inner">
		<div class="personal-{{.Layout}}">
			{{if .ShowCategories}}
			{{range $category, $items := .GroupedInfo}}
			<div class="personal-category">
				<h4 class="category-title">{{$category}}</h4>
				<div class="personal-items">
					{{range $items}}
					<div class="personal-item">
						{{if and $.ShowImages .Image}}
						<div class="personal-image">
							<img src="{{.Image}}" alt="{{.Title}}" loading="lazy">
						</div>
						{{end}}
						<div class="personal-content">
							<div class="personal-header">
								{{if .Icon}}
								<span class="personal-icon">{{.Icon}}</span>
								{{end}}
								<h5 class="personal-title">{{.Title}}</h5>
							</div>
							<div class="personal-text markdown-content">{{.RenderedContent}}</div>
						</div>
					</div>
					{{end}}
				</div>
			</div>
			{{end}}
			{{else}}
			<div class="personal-items">
				{{range .AllInfo}}
				<div class="personal-item">
					{{if and $.ShowImages .Image}}
					<div class="personal-image">
						<img src="{{.Image}}" alt="{{.Title}}" loading="lazy">
					</div>
					{{end}}
					<div class="personal-content">
						<div class="personal-header">
							{{if .Icon}}
							<span class="personal-icon">{{.Icon}}</span>
							{{end}}
							<h5 class="personal-title">{{.Title}}</h5>
						</div>
						<div class="personal-text markdown-content">{{.RenderedContent}}</div>
					</div>
				</div>
				{{end}}
			</div>
			{{end}}
		</div>
	</div>
</section>`

	var allInfo []PersonalInfo
	groupedInfo := make(map[string][]PersonalInfo)

	for _, info := range personalInfo {
		infoMap, ok := info.(map[string]interface{})
		if !ok {
			continue
		}

		personalItem := PersonalInfo{
			Title:    p.getStringFromMap(infoMap, "title", ""),
			Content:  p.getStringFromMap(infoMap, "content", ""),
			Image:    p.getStringFromMap(infoMap, "image", ""),
			Icon:     p.getStringFromMap(infoMap, "icon", ""),
			Category: p.getStringFromMap(infoMap, "category", "General"),
		}

		personalItem.RenderedContent = p.renderMarkdown(personalItem.Content)

		allInfo = append(allInfo, personalItem)

		if showCategories {
			if groupedInfo[personalItem.Category] == nil {
				groupedInfo[personalItem.Category] = []PersonalInfo{}
			}
			groupedInfo[personalItem.Category] = append(groupedInfo[personalItem.Category], personalItem)
		}
	}

	data := struct {
		SectionTitle   string
		ShowImages     bool
		ShowCategories bool
		Layout         string
		AllInfo        []PersonalInfo
		GroupedInfo    map[string][]PersonalInfo
	}{
		SectionTitle:   sectionTitle,
		ShowImages:     showImages,
		ShowCategories: showCategories,
		Layout:         layout,
		AllInfo:        allInfo,
		GroupedInfo:    groupedInfo,
	}

	funcMap := template.FuncMap{
		"html": func(s string) template.HTML {
			return template.HTML(s)
		},
	}

	t, err := template.New("personal").Funcs(funcMap).Parse(tmpl)
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

func (p *PersonalPlugin) renderMarkdown(content string) string {
	// Simple markdown rendering
	lines := strings.Split(content, "\n")
	var result strings.Builder

	inList := false
	inCodeBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				result.WriteString("</pre>\n")
				inCodeBlock = false
			} else {
				result.WriteString("<pre><code>")
				inCodeBlock = true
			}
			continue
		}

		if inCodeBlock {
			result.WriteString(line + "\n")
			continue
		}

		if line == "" {
			if inList {
				result.WriteString("</ul>\n")
				inList = false
			}
			result.WriteString("<br>\n")
			continue
		}

		if strings.HasPrefix(line, "# ") {
			result.WriteString("<h1>" + line[2:] + "</h1>\n")
		} else if strings.HasPrefix(line, "## ") {
			result.WriteString("<h2>" + line[3:] + "</h2>\n")
		} else if strings.HasPrefix(line, "### ") {
			result.WriteString("<h3>" + line[4:] + "</h3>\n")
		} else if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			if !inList {
				result.WriteString("<ul>\n")
				inList = true
			}
			result.WriteString("<li>" + line[2:] + "</li>\n")
		} else {
			if inList {
				result.WriteString("</ul>\n")
				inList = false
			}

			line = p.processInlineMarkdown(line)
			result.WriteString("<p>" + line + "</p>\n")
		}
	}

	if inList {
		result.WriteString("</ul>\n")
	}
	if inCodeBlock {
		result.WriteString("</code></pre>\n")
	}

	return result.String()
}

func (p *PersonalPlugin) processInlineMarkdown(text string) string {
	// Bold **text**
	text = strings.ReplaceAll(text, "**", "</strong>")
	text = strings.ReplaceAll(text, "**", "<strong>")

	// Italic *text*
	text = strings.ReplaceAll(text, "*", "</em>")
	text = strings.ReplaceAll(text, "*", "<em>")

	// Code `text`
	for strings.Contains(text, "`") {
		start := strings.Index(text, "`")
		end := strings.Index(text[start+1:], "`")
		if end == -1 {
			break
		}
		end += start + 1

		before := text[:start]
		code := text[start+1 : end]
		after := text[end+1:]

		text = before + "<code>" + code + "</code>" + after
	}

	// Links [text](url)
	for strings.Contains(text, "[") && strings.Contains(text, "](") {
		start := strings.Index(text, "[")
		middle := strings.Index(text[start:], "](")
		if middle == -1 {
			break
		}
		middle += start
		end := strings.Index(text[middle+2:], ")")
		if end == -1 {
			break
		}
		end += middle + 2

		linkText := text[start+1 : middle]
		linkURL := text[middle+2 : end]

		before := text[:start]
		after := text[end+1:]

		text = before + `<a href="` + linkURL + `" target="_blank" rel="noopener">` + linkText + `</a>` + after
	}

	return text
}

func (p *PersonalPlugin) UpdateData(ctx context.Context) error {
	// Personal info is static, no external updates needed
	return nil
}

func (p *PersonalPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *PersonalPlugin) SetSettings(settings map[string]interface{}) error {
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

func (p *PersonalPlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
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

func (p *PersonalPlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
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

func (p *PersonalPlugin) getStringFromMap(m map[string]interface{}, key string, defaultValue string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return defaultValue
}

func (p *PersonalPlugin) GetMetrics() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	personalInfo, ok := config.Settings["info"].([]interface{})
	count := 0
	if ok {
		count = len(personalInfo)
	}
	return map[string]interface{}{
		"info_items_count": count,
	}
}

func (p *PersonalPlugin) RenderText(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	personalInfo, ok := config.Settings["info"].([]interface{})
	if !ok || len(personalInfo) == 0 {
		return "Personal Info: No items configured", nil
	}

	titles := make([]string, 0, len(personalInfo))
	for _, info := range personalInfo {
		infoMap, ok := info.(map[string]interface{})
		if !ok {
			continue
		}
		title := p.getStringFromMap(infoMap, "title", "")
		if title != "" {
			titles = append(titles, title)
		}
	}
	return "Personal Info: " + strings.Join(titles, ", "), nil
}
