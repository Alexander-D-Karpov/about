package plugins

import (
	"context"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *TechStackPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	techs, ok := cfg.Settings["technologies"].([]interface{})
	if !ok {
		return nil
	}

	out := make([]view.TechVM, 0, len(techs))
	for _, raw := range techs {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		icon, _ := m["icon"].(string)
		out = append(out, view.TechVM{Name: name, Icon: icon})
	}

	vm.Tech = out
	return nil
}
