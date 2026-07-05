package plugins

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *NeofetchPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	raw, ok := cfg.Settings["machines"].([]interface{})
	if !ok || len(raw) == 0 {
		return nil
	}

	machines := make([]view.MachineVM, 0, len(raw))
	for i, v := range raw {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		name := p.get(m, "name", "Machine")
		mv := parseNeofetch(p.get(m, "output", ""))
		mv.Key = neofetchKey(name, i)
		mv.Label = name
		machines = append(machines, mv)
	}
	if len(machines) == 0 {
		return nil
	}

	vm.Machines.Machines = machines
	vm.Machines.Active = machines[0].Key
	return nil
}

func neofetchKey(name string, i int) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	key := strings.Trim(b.String(), "-")
	if key == "" {
		key = fmt.Sprintf("m%d", i)
	}
	return key
}

func parseNeofetch(output string) view.MachineVM {
	lines := strings.Split(output, "\n")

	splitCol := -1
	for _, line := range lines {
		runes := []rune(line)
		at := -1
		for j, r := range runes {
			if r == '@' {
				at = j
				break
			}
		}
		if at < 0 {
			continue
		}
		start := at
		for start > 0 && !unicode.IsSpace(runes[start-1]) {
			start--
		}
		splitCol = start
		break
	}

	var art, info []string
	for _, line := range lines {
		runes := []rune(line)
		if splitCol >= 0 && len(runes) > splitCol {
			art = append(art, strings.TrimRight(string(runes[:splitCol]), " "))
			if right := strings.TrimSpace(string(runes[splitCol:])); right != "" {
				info = append(info, right)
			}
			continue
		}
		t := strings.TrimRight(line, " ")
		if strings.Contains(t, ":") || strings.Contains(t, "@") || strings.Contains(t, "-----") {
			info = append(info, strings.TrimSpace(t))
		} else {
			art = append(art, t)
		}
	}

	mv := view.MachineVM{ASCII: strings.TrimRight(strings.Join(art, "\n"), "\n")}
	for _, line := range info {
		if line == "" || strings.HasPrefix(line, "❯") {
			continue
		}
		if at := strings.Index(line, "@"); at >= 0 && !strings.Contains(line, ":") {
			mv.User = strings.TrimSpace(line[:at])
			mv.Host = strings.TrimSpace(line[at+1:])
			continue
		}
		if strings.Trim(line, "-") == "" {
			mv.Rule = line
			continue
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			mv.Rows = append(mv.Rows, view.KV{K: strings.TrimSpace(line[:idx]), V: strings.TrimSpace(line[idx+1:])})
		}
	}
	return mv
}
