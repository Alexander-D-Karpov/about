package plugins

import (
	"context"
	"html/template"
	"log"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

type SectionFiller interface {
	Fill(ctx context.Context, vm *view.PageVM) error
}

type namedFiller struct {
	name   string
	filler SectionFiller
}

type sectionGroup struct {
	template string
	plugins  []string
	pick     func(*view.PageVM) any
}

var sectionGroups = []sectionGroup{
	{"topbar", []string{"webring", "visitors", "info"}, func(vm *view.PageVM) any { return vm.Top }},
	{"section_profile", []string{"profile", "social"}, func(vm *view.PageVM) any { return vm.Profile }},
	{"section_health", []string{"health"}, func(vm *view.PageVM) any { return vm.Health }},
	{"section_music", []string{"lastfm"}, func(vm *view.PageVM) any { return vm.Music }},
	{"section_code", []string{"code"}, func(vm *view.PageVM) any { return vm.Code }},
	{"section_tech", []string{"techstack"}, func(vm *view.PageVM) any { return vm.Tech }},
	{"section_games", []string{"steam", "beatleader"}, func(vm *view.PageVM) any { return vm.Games }},
	{"section_travel", []string{"bike", "places", "photos"}, func(vm *view.PageVM) any { return vm.Travel }},
	{"section_hosting", []string{"services"}, func(vm *view.PageVM) any { return vm.Hosting }},
	{"section_machines", []string{"neofetch", "meme"}, func(vm *view.PageVM) any { return vm.Machines }},
	{"section_projects", []string{"projects"}, func(vm *view.PageVM) any { return vm.Projects }},
}

func groupForPlugin(name string) (sectionGroup, bool) {
	for _, g := range sectionGroups {
		for _, p := range g.plugins {
			if p == name {
				return g, true
			}
		}
	}
	return sectionGroup{}, false
}

func (m *Manager) orderedFillersLocked() []namedFiller {
	enabled := m.getEnabledPluginsLocked()
	out := make([]namedFiller, 0, len(enabled))
	for _, p := range enabled {
		if f, ok := p.(SectionFiller); ok {
			out = append(out, namedFiller{p.Name(), f})
		}
	}
	return out
}

func (m *Manager) fillGuarded(ctx context.Context, name string, f SectionFiller, vm *view.PageVM) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[render] fill panic in %s: %v", name, r)
		}
	}()
	fctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := f.Fill(fctx, vm); err != nil {
		log.Printf("[render] fill error in %s: %v", name, err)
	}
}

func (m *Manager) BuildPage(ctx context.Context, theme string) (template.HTML, error) {
	vm := view.PageVM{Theme: theme}

	m.mutex.RLock()
	fillers := m.orderedFillersLocked()
	m.mutex.RUnlock()

	for _, nf := range fillers {
		m.fillGuarded(ctx, nf.name, nf.filler, &vm)
	}

	return view.RenderPage(vm)
}

func (m *Manager) RenderSectionLive(ctx context.Context, changedPlugin string) {
	g, ok := groupForPlugin(changedPlugin)
	if !ok {
		return
	}

	m.mutex.RLock()
	group := make([]namedFiller, 0, len(g.plugins))
	for _, pn := range g.plugins {
		p, exists := m.plugins[pn]
		if !exists {
			continue
		}
		if !m.storage.GetPluginConfig(pn).Enabled {
			continue
		}
		if f, isFiller := p.(SectionFiller); isFiller {
			group = append(group, namedFiller{pn, f})
		}
	}
	m.mutex.RUnlock()

	if len(group) == 0 {
		return
	}

	vm := view.PageVM{}
	for _, nf := range group {
		m.fillGuarded(ctx, nf.name, nf.filler, &vm)
	}

	html, err := view.RenderSection(g.template, g.pick(&vm))
	if err != nil {
		log.Printf("[render] section %s render error: %v", g.template, err)
		return
	}

	m.hub.Broadcast("section_rendered", map[string]interface{}{
		"name": g.template,
		"html": html,
	})
}
