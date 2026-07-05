package plugins

import (
	"context"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

var socialColors = map[string]string{
	"telegram":   "#2aabee",
	"github":     "#e6edf3",
	"vk":         "#4a76a8",
	"linkedin":   "#0a66c2",
	"email":      "#e8557a",
	"discord":    "#5865f2",
	"ctftime":    "#f0a040",
	"codewars":   "#ad2c27",
	"lastfm":     "#d51007",
	"steam":      "#66c0f4",
	"beatleader": "#b055ff",
}

func (p *SocialPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	links, ok := cfg.Settings["links"].([]interface{})
	if !ok {
		return nil
	}

	for _, raw := range links {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		icon := p.getStringFromMap(m, "icon", "link")
		color := socialColors[icon]
		if color == "" {
			color = "#9fb4c6"
		}
		vm.Profile.Socials = append(vm.Profile.Socials, view.SocialVM{
			Label: p.getStringFromMap(m, "name", "Link"),
			Icon:  icon,
			Href:  p.getStringFromMap(m, "url", "#"),
			Color: color,
		})
	}
	return nil
}
