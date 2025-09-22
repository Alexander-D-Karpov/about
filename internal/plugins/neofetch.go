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
	Name     string            `json:"name"`
	Hostname string            `json:"hostname"`
	Username string            `json:"username"`
	ASCII    []string          `json:"ascii"`
	Info     map[string]string `json:"info"`
	Colors   []string          `json:"colors"`
}

func NewNeofetchPlugin(storage *storage.Storage, hub *stream.Hub) *NeofetchPlugin {
	return &NeofetchPlugin{
		storage: storage,
		hub:     hub,
	}
}

func (p *NeofetchPlugin) Name() string { return "neofetch" }

func (p *NeofetchPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())

	machinesData, ok := config.Settings["machines"].([]interface{})
	if !ok || len(machinesData) == 0 {
		return p.renderNoMachines(), nil
	}

	var machines []NeofetchMachine
	for _, machineData := range machinesData {
		if machineMap, ok := machineData.(map[string]interface{}); ok {
			machine := p.parseMachine(machineMap)
			machines = append(machines, machine)
		}
	}

	if len(machines) == 0 {
		return p.renderNoMachines(), nil
	}

	tmpl := `
	<div class="neofetch-section section">
		<h3>System Information</h3>

		{{if gt (len .Machines) 1}}
		<div class="machine-buttons">
			{{range $index, $machine := .Machines}}
			<button class="btn machine-btn" type="button" data-machine="{{$index}}" {{if eq $index 0}}data-active="true"{{end}}>
				{{$machine.Name}}
			</button>
			{{end}}
		</div>
		{{end}}

		<div class="neofetch-outputs">
			{{range $index, $machine := .Machines}}
			<div class="neofetch-output" id="neofetch-{{$index}}" {{if ne $index 0}}style="display: none;"{{end}}>
				<pre class="terminal">{{$machine.RenderedOutput}}</pre>
			</div>
			{{end}}
		</div>
	</div>`

	var processedMachines []struct {
		Name           string
		RenderedOutput template.HTML
	}

	for _, machine := range machines {
		output := p.renderMachineOutput(machine)
		processedMachines = append(processedMachines, struct {
			Name           string
			RenderedOutput template.HTML
		}{
			Name:           machine.Name,
			RenderedOutput: template.HTML(output),
		})
	}

	data := struct {
		Machines []struct {
			Name           string
			RenderedOutput template.HTML
		}
	}{
		Machines: processedMachines,
	}

	funcMap := template.FuncMap{
		"len": func(slice interface{}) int {
			switch s := slice.(type) {
			case []struct {
				Name           string
				RenderedOutput template.HTML
			}:
				return len(s)
			default:
				return 0
			}
		},
		"gt": func(a, b int) bool { return a > b },
		"eq": func(a, b int) bool { return a == b },
		"ne": func(a, b int) bool { return a != b },
	}

	tmplParsed, err := template.New("neofetch").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = tmplParsed.Execute(&buf, data)
	return buf.String(), err
}

func (p *NeofetchPlugin) renderNoMachines() string {
	return `<div class="neofetch-section section">
		<h3>System Information</h3>
		<p class="text-muted">No machines configured</p>
	</div>`
}

func (p *NeofetchPlugin) renderMachineOutput(machine NeofetchMachine) string {
	var output strings.Builder

	maxLines := len(machine.ASCII)
	infoLines := p.formatInfoLines(machine)
	if len(infoLines) > maxLines {
		maxLines = len(infoLines)
	}

	for i := 0; i < maxLines; i++ {
		var asciiLine, infoLine string

		if i < len(machine.ASCII) {
			asciiLine = machine.ASCII[i]
		} else {
			asciiLine = strings.Repeat(" ", 40)
		}

		if i < len(infoLines) {
			infoLine = infoLines[i]
		}

		if infoLine != "" {
			output.WriteString(fmt.Sprintf(`<span class="ascii-art">%s</span>%s`, asciiLine, infoLine))
		} else {
			output.WriteString(fmt.Sprintf(`<span class="ascii-art">%s</span>`, asciiLine))
		}
		output.WriteString("\n")
	}

	if len(machine.Colors) > 0 {
		output.WriteString("\n<div class=\"color-palette\">\n")
		for _, color := range machine.Colors {
			output.WriteString(fmt.Sprintf(`  <span class="color-block" style="background-color: %s;"></span>`, color))
			output.WriteString("\n")
		}
		output.WriteString("</div>")
	}

	return output.String()
}

func (p *NeofetchPlugin) formatInfoLines(machine NeofetchMachine) []string {
	var lines []string

	if machine.Username != "" && machine.Hostname != "" {
		userHost := fmt.Sprintf(`<span class="user-host">%s@</span><span class="hostname">%s</span>`,
			machine.Username, machine.Hostname)
		lines = append(lines, userHost)

		separator := strings.Repeat("-", len(machine.Username)+1+len(machine.Hostname))
		lines = append(lines, fmt.Sprintf(`<span class="separator">%s</span>`, separator))
	}

	infoOrder := []string{"OS", "Host", "Kernel", "Uptime", "Packages", "Shell", "Resolution",
		"DE", "WM", "WM Theme", "Theme", "Icons", "Terminal", "CPU", "GPU", "Memory", "Disk", "Battery"}

	for _, key := range infoOrder {
		if value, exists := machine.Info[key]; exists && value != "" {
			line := fmt.Sprintf(`<span class="info-key">%s:</span> %s`, key, value)
			lines = append(lines, line)
		}
	}

	return lines
}

func (p *NeofetchPlugin) parseMachine(machineMap map[string]interface{}) NeofetchMachine {
	machine := NeofetchMachine{
		Info:   make(map[string]string),
		ASCII:  []string{},
		Colors: []string{},
	}

	if name, ok := machineMap["name"].(string); ok {
		machine.Name = name
	}
	if hostname, ok := machineMap["hostname"].(string); ok {
		machine.Hostname = hostname
	}
	if username, ok := machineMap["username"].(string); ok {
		machine.Username = username
	}

	if asciiData, ok := machineMap["ascii"].([]interface{}); ok {
		for _, line := range asciiData {
			if lineStr, ok := line.(string); ok {
				machine.ASCII = append(machine.ASCII, lineStr)
			}
		}
	}

	if infoData, ok := machineMap["info"].(map[string]interface{}); ok {
		for key, value := range infoData {
			if valueStr, ok := value.(string); ok {
				machine.Info[key] = valueStr
			}
		}
	}

	if colorsData, ok := machineMap["colors"].([]interface{}); ok {
		for _, color := range colorsData {
			if colorStr, ok := color.(string); ok {
				machine.Colors = append(machine.Colors, colorStr)
			}
		}
	}

	return machine
}

func (p *NeofetchPlugin) UpdateData(ctx context.Context) error { return nil }

func (p *NeofetchPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *NeofetchPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings
	return p.storage.SetPluginConfig(p.Name(), config)
}

func (p *NeofetchPlugin) RenderText(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	machinesData, ok := config.Settings["machines"].([]interface{})
	if !ok || len(machinesData) == 0 {
		return "System: No machines configured", nil
	}

	var machineNames []string
	for _, machineData := range machinesData {
		if machineMap, ok := machineData.(map[string]interface{}); ok {
			if name, ok := machineMap["name"].(string); ok {
				machineNames = append(machineNames, name)
			}
		}
	}

	if len(machineNames) == 0 {
		return "System: No valid machines", nil
	}

	return fmt.Sprintf("System: %s (%d machines)", strings.Join(machineNames, ", "), len(machineNames)), nil
}
