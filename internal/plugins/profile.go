package plugins

import (
	"context"
	"html/template"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type ProfilePlugin struct {
	storage *storage.Storage
	hub     *stream.Hub
}

func NewProfilePlugin(storage *storage.Storage, hub *stream.Hub) *ProfilePlugin {
	return &ProfilePlugin{
		storage: storage,
		hub:     hub,
	}
}

func (p *ProfilePlugin) Name() string {
	return "profile"
}

func (p *ProfilePlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	name := p.getConfigValue(settings, "name", "sanspie")
	title := p.getConfigValue(settings, "title", "Web FullStack Developer")
	bio := p.getConfigValue(settings, "bio", "Your bio description here.")
	profileImage := p.getConfigValue(settings, "profileImage", "")

	// split title: first word ("Web") + the rest ("FullStack Developer")
	var titleFirst, titleRest string
	if parts := strings.Fields(title); len(parts) > 0 {
		titleFirst = parts[0]
		if len(parts) > 1 {
			titleRest = strings.Join(parts[1:], " ")
		}
	}

	subtitle := p.getConfigValue(settings, "subtitle", "DevSecOps")

	var subDev, subSec, subOps string
	if subtitle == "DevSecOps" {
		subDev, subSec, subOps = "Dev", "Sec", "Ops"
	} else if parts := strings.Fields(subtitle); len(parts) >= 3 {
		subDev, subSec, subOps = parts[0], parts[1], parts[2]
	}

	tmpl := `
<section class="profile-section section plugin" data-w="2">
	<div class="plugin__inner">
		<div class="profile-content">
			<div class="profile-text">
				<h1 class="profile-name">{{.Name}}</h1>

				<div class="profile-titles">
					{{if .Title}}
					<div class="profile-title">
						{{if .TitleFirst}}
						<span class="profile-title-web">{{.TitleFirst}}</span>
						{{end}}
						{{if .TitleRest}}
						<span class="profile-title-rest">{{.TitleRest}}</span>
						{{end}}
						{{if not .TitleFirst}}
						{{.Title}}
						{{end}}
					</div>
					{{end}}

					{{if .Subtitle}}
					<div class="profile-subtitle">
						{{if .SubtitleDev}}
						<span class="subtitle-dev">{{.SubtitleDev}}</span><!--
						--><span class="subtitle-sec">{{.SubtitleSec}}</span><!--
						--><span class="subtitle-ops">{{.SubtitleOps}}</span>
						{{else}}
						{{.Subtitle}}
						{{end}}
					</div>
					{{end}}
				</div>

				{{if .Bio}}
				<div class="profile-bio">{{.Bio}}</div>
				{{end}}
			</div>

			{{if .ProfileImage}}
			<div class="profile-image">
				<img src="{{.ProfileImage}}" alt="{{.Name}}" loading="lazy">
			</div>
			{{end}}
		</div>
	</div>
</section>`

	data := struct {
		Name         string
		Title        string
		Subtitle     string
		Bio          string
		ProfileImage string

		TitleFirst string
		TitleRest  string

		SubtitleDev string
		SubtitleSec string
		SubtitleOps string
	}{
		Name:         name,
		Title:        title,
		Subtitle:     subtitle,
		Bio:          bio,
		ProfileImage: profileImage,

		TitleFirst: titleFirst,
		TitleRest:  titleRest,

		SubtitleDev: subDev,
		SubtitleSec: subSec,
		SubtitleOps: subOps,
	}

	t, err := template.New("profile").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (p *ProfilePlugin) UpdateData(ctx context.Context) error {
	// Profile data is static, no external updates needed
	return nil
}

func (p *ProfilePlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *ProfilePlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings

	// Save configuration
	err := p.storage.SetPluginConfig(p.Name(), config)
	if err != nil {
		return err
	}

	// Broadcast update to all connected clients
	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(),
		"action": "settings_changed",
	})

	return nil
}

func (p *ProfilePlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
	// Handle nested keys like "ui.titleLabel"
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			// Last key, get the value
			if value, ok := current[k].(string); ok {
				return value
			}
			return defaultValue
		} else {
			// Intermediate key, navigate deeper
			if next, ok := current[k].(map[string]interface{}); ok {
				current = next
			} else {
				return defaultValue
			}
		}
	}

	return defaultValue
}

func (p *ProfilePlugin) RenderText(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	name := p.getConfigValue(settings, "name", "User")
	title := p.getConfigValue(settings, "title", "")
	subtitle := p.getConfigValue(settings, "subtitle", "")

	var parts []string
	parts = append(parts, name)
	if title != "" {
		parts = append(parts, title)
	}
	if subtitle != "" {
		parts = append(parts, subtitle)
	}

	return strings.Join(parts, " - "), nil
}
