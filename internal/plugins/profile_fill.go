package plugins

import (
	"context"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *ProfilePlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	s := cfg.Settings

	vm.Profile.Name = p.getConfigValue(s, "name", "sanspie")
	vm.Profile.Avatar = p.getConfigValue(s, "profileImage", "")
	vm.Profile.Bio = p.getConfigValue(s, "bio", "")
	vm.Profile.Role = roleSegs(p.getConfigValue(s, "title", "Web FullStack Developer"))
	vm.Profile.Stack = stackSegs(p.getConfigValue(s, "subtitle", "DevSecOps"))
	if vm.Profile.TierURL == "" {
		vm.Profile.TierURL = "/ranking"
	}
	return nil
}

func roleSegs(title string) []view.Seg {
	parts := strings.Fields(title)
	if len(parts) == 0 {
		return nil
	}
	segs := []view.Seg{{Text: parts[0], Color: "#7db5ff"}}
	if len(parts) > 1 {
		segs = append(segs, view.Seg{Text: strings.Join(parts[1:], " "), Color: "#c5ccd4"})
	}
	return segs
}

func stackSegs(subtitle string) []view.Seg {
	colors := []string{"#10d060", "#f0a040", "#4d9fff"}
	if subtitle == "DevSecOps" {
		return []view.Seg{{Text: "Dev", Color: colors[0]}, {Text: "Sec", Color: colors[1]}, {Text: "Ops", Color: colors[2]}}
	}
	parts := strings.Fields(subtitle)
	segs := make([]view.Seg, 0, len(parts))
	for i, part := range parts {
		segs = append(segs, view.Seg{Text: part, Color: colors[i%len(colors)]})
	}
	return segs
}
