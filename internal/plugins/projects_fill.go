package plugins

import (
	"context"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *ProjectsPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	raw, ok := cfg.Settings["projects"].([]interface{})
	if !ok {
		return nil
	}

	out := make([]view.ProjectVM, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		pv := view.ProjectVM{
			Name:   strFromMap(m, "name"),
			Desc:   strFromMap(m, "description"),
			Image:  strFromMap(m, "image"),
			Source: strFromMap(m, "github"),
			Demo:   strFromMap(m, "live"),
		}
		if techs, ok := m["technologies"].([]interface{}); ok {
			for _, t := range techs {
				if ts, ok := t.(string); ok {
					pv.Tags = append(pv.Tags, ts)
				}
			}
		}
		out = append(out, pv)
	}

	vm.Projects = out
	return nil
}
