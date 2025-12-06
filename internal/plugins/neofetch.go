package plugins

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type NeofetchPlugin struct {
	storage *storage.Storage
	hub     *stream.Hub
}

type NeofetchMachine struct {
	Name   string `json:"name"`
	Output string `json:"output"`
}

func NewNeofetchPlugin(st *storage.Storage, hub *stream.Hub) *NeofetchPlugin {
	return &NeofetchPlugin{storage: st, hub: hub}
}
func (p *NeofetchPlugin) Name() string { return "neofetch" }

func (p *NeofetchPlugin) Render(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	cfg := p.storage.GetPluginConfig(p.Name())
	rawMachines, ok := cfg.Settings["machines"].([]interface{})
	if !ok || len(rawMachines) == 0 {
		return p.renderNoMachines(), nil
	}

	type vm struct {
		Name   string
		Output string
	}
	machines := make([]vm, 0, len(rawMachines))

	for _, v := range rawMachines {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		name := p.get(m, "name", "Machine")
		out := p.get(m, "output", "")
		machines = append(machines, vm{Name: name, Output: out})
	}

	if len(machines) == 0 {
		return p.renderNoMachines(), nil
	}

	const tmpl = `
<section class="neofetch-section section plugin" data-w="2">
  <header class="plugin-header">
    <h3 class="plugin-title">System Information</h3>
  </header>

  <div class="plugin__inner">
    {{if gt (len .Machines) 1}}
    <div class="machine-buttons">
      {{range $i, $m := .Machines}}
      <button class="btn machine-btn" type="button" data-machine="{{$i}}" {{if eq $i 0}}data-active="true"{{end}}>{{$m.Name}}</button>
      {{end}}
    </div>
    {{end}}

    <div class="neofetch-outputs">
      {{range $i, $m := .Machines}}
      <div class="neofetch-output" id="neofetch-{{$i}}" {{if ne $i 0}}style="display:none"{{end}}>
        <div class="terminal">
          <div class="terminal-body">
            <pre class="neofetch-pre">{{$m.Output}}</pre>
          </div>
        </div>
      </div>
      {{end}}
    </div>
  </div>
</section>`

	data := struct{ Machines []vm }{Machines: machines}
	funcs := template.FuncMap{
		"len": func(v interface{}) int {
			switch s := v.(type) {
			case []vm:
				return len(s)
			default:
				return 0
			}
		},
		"gt": func(a, b int) bool { return a > b },
		"eq": func(a, b int) bool { return a == b },
		"ne": func(a, b int) bool { return a != b },
	}
	tpl, err := template.New("neofetch").Funcs(funcs).Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := tpl.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

func (p *NeofetchPlugin) renderNoMachines() string {
	return `<section class="neofetch-section section plugin" data-w="2">
  <header class="plugin-header">
    <h3 class="plugin-title">System Information</h3>
  </header>
  <div class="plugin__inner">
    <p class="text-muted">No machines configured</p>
  </div>
</section>`
}

func (p *NeofetchPlugin) UpdateData(ctx context.Context) error { return nil }
func (p *NeofetchPlugin) GetSettings() map[string]interface{} {
	return p.storage.GetPluginConfig(p.Name()).Settings
}
func (p *NeofetchPlugin) SetSettings(s map[string]interface{}) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	cfg.Settings = s
	return p.storage.SetPluginConfig(p.Name(), cfg)
}
func (p *NeofetchPlugin) RenderText(ctx context.Context) (string, error) {
	cfg := p.storage.GetPluginConfig(p.Name())
	raw, ok := cfg.Settings["machines"].([]interface{})
	if !ok || len(raw) == 0 {
		return "System: No machines configured", nil
	}
	names := make([]string, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]interface{}); ok {
			if n, ok := m["name"].(string); ok {
				names = append(names, n)
			}
		}
	}
	if len(names) == 0 {
		return "System: No valid machines", nil
	}
	return fmt.Sprintf("System: %s (%d machines)", strings.Join(names, ", "), len(names)), nil
}

func (p *NeofetchPlugin) get(m map[string]interface{}, k, d string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return d
}
