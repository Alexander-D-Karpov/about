package plugins

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type Plugin interface {
	Name() string
	Render(ctx context.Context) (string, error)
	UpdateData(ctx context.Context) error
	GetSettings() map[string]interface{}
	SetSettings(settings map[string]interface{}) error
}

type Manager struct {
	plugins        map[string]Plugin
	storage        *storage.Storage
	hub            *stream.Hub
	config         *config.Config
	mutex          sync.RWMutex
	renderedCache  map[string]template.HTML
	cacheTimestamp time.Time
	lastUpdate     time.Time
}

func NewManager(storage *storage.Storage, hub *stream.Hub, config *config.Config) *Manager {
	return &Manager{
		plugins:       make(map[string]Plugin),
		storage:       storage,
		hub:           hub,
		config:        config,
		renderedCache: make(map[string]template.HTML),
	}
}

func (m *Manager) LoadAll() error {
	plugins := []Plugin{
		NewProfilePlugin(m.storage, m.hub),
		NewSocialPlugin(m.storage, m.hub),
		NewTechStackPlugin(m.storage, m.hub),
		NewProjectsPlugin(m.storage, m.hub),
		NewNeofetchPlugin(m.storage, m.hub),
		NewWebringPlugin(m.storage, m.hub),
		NewLastFMPlugin(m.storage, m.hub, m.config.LastFMKey),
		NewBeatLeaderPlugin(m.storage, m.hub),
		NewSteamPlugin(m.storage, m.hub, m.config.SteamKey),
		NewVisitorsPlugin(m.storage, m.hub, m.config.DataPath),
		NewServicesPlugin(m.storage, m.hub),
		NewCodePlugin(m.storage, m.hub),
		NewInfoPlugin(m.storage, m.hub),
		NewPersonalPlugin(m.storage, m.hub),
		NewMemePlugin(m.storage, m.hub),
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, plugin := range plugins {
		m.plugins[plugin.Name()] = plugin
	}

	return nil
}

func (m *Manager) PreloadData() error {
	m.mutex.RLock()
	plugins := make([]Plugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	m.mutex.RUnlock()

	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, len(plugins))

	for _, plugin := range plugins {
		wg.Add(1)
		go func(p Plugin) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errors <- fmt.Errorf("plugin %s panic: %v", p.Name(), r)
				}
			}()

			if err := p.UpdateData(ctx); err != nil {
				errors <- fmt.Errorf("plugin %s: %v", p.Name(), err)
			}
		}(plugin)
	}

	wg.Wait()
	close(errors)

	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Printf("Warning: %v\n", err)
		}
	}

	m.preRenderPlugins(ctx)
	m.lastUpdate = time.Now()
	return nil
}

func (m *Manager) preRenderPlugins(ctx context.Context) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	enabledPlugins := m.getEnabledPluginsLocked()
	m.renderedCache = make(map[string]template.HTML)

	for _, plugin := range enabledPlugins {
		if plugin.Name() == "meme" || plugin.Name() == "info" || plugin.Name() == "visitors" {
			continue
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Error pre-rendering plugin %s: panic: %v\n", plugin.Name(), r)
				}
			}()

			rendered, err := plugin.Render(ctx)
			if err != nil {
				fmt.Printf("Error pre-rendering plugin %s: %v\n", plugin.Name(), err)
				return
			}
			if rendered != "" {
				m.renderedCache[plugin.Name()] = template.HTML(rendered)
			}
		}()
	}

	m.cacheTimestamp = time.Now()
}

func (m *Manager) GetRenderedPlugins(ctx context.Context) []template.HTML {
	m.mutex.RLock()

	if time.Since(m.cacheTimestamp) > time.Minute {
		m.mutex.RUnlock()
		go m.preRenderPlugins(ctx)
		m.mutex.RLock()
	}

	enabledPlugins := m.getEnabledPluginsLocked()
	var renderedPlugins []template.HTML

	for _, plugin := range enabledPlugins {
		if plugin.Name() == "meme" || plugin.Name() == "info" || plugin.Name() == "visitors" {
			m.mutex.RUnlock()
			rendered, err := plugin.Render(ctx)
			m.mutex.RLock()
			if err != nil {
				fmt.Printf("Error rendering %s plugin: %v\n", plugin.Name(), err)
				continue
			}
			if rendered != "" {
				renderedPlugins = append(renderedPlugins, template.HTML(rendered))
			}
		} else if rendered, exists := m.renderedCache[plugin.Name()]; exists {
			renderedPlugins = append(renderedPlugins, rendered)
		}
	}

	m.mutex.RUnlock()
	return renderedPlugins
}

func (m *Manager) GetRenderedPluginsFresh(ctx context.Context) []template.HTML {
	m.mutex.RLock()
	enabledPlugins := m.getEnabledPluginsLocked()
	m.mutex.RUnlock()

	var renderedPlugins []template.HTML

	for _, plugin := range enabledPlugins {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Error rendering plugin %s: panic: %v\n", plugin.Name(), r)
				}
			}()

			rendered, err := plugin.Render(ctx)
			if err != nil {
				fmt.Printf("Error rendering plugin %s: %v\n", plugin.Name(), err)
				return
			}
			if rendered != "" {
				renderedPlugins = append(renderedPlugins, template.HTML(rendered))
			}
		}()
	}

	return renderedPlugins
}

func (m *Manager) getEnabledPluginsLocked() []Plugin {
	var enabled []Plugin

	for _, plugin := range m.plugins {
		config := m.storage.GetPluginConfig(plugin.Name())
		if config.Enabled {
			enabled = append(enabled, plugin)
		}
	}

	sort.Slice(enabled, func(i, j int) bool {
		configI := m.storage.GetPluginConfig(enabled[i].Name())
		configJ := m.storage.GetPluginConfig(enabled[j].Name())
		return configI.Order < configJ.Order
	})

	return enabled
}

func (m *Manager) GetEnabledPlugins() []Plugin {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.getEnabledPluginsLocked()
}

func (m *Manager) GetPlugin(name string) (Plugin, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	plugin, exists := m.plugins[name]
	return plugin, exists
}

func (m *Manager) GetAllPlugins() map[string]Plugin {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]Plugin)
	for name, plugin := range m.plugins {
		result[name] = plugin
	}
	return result
}

func (m *Manager) UpdateExternalData() {
	m.mutex.RLock()
	plugins := make([]Plugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	m.mutex.RUnlock()

	ctx := context.Background()
	var wg sync.WaitGroup

	for _, plugin := range plugins {
		config := m.storage.GetPluginConfig(plugin.Name())
		if config.Enabled {
			wg.Add(1)
			go func(p Plugin) {
				defer wg.Done()
				if err := p.UpdateData(ctx); err != nil {
					fmt.Printf("Error updating plugin %s: %v\n", p.Name(), err)
				}
			}(plugin)
		}
	}

	wg.Wait()

	m.mutex.Lock()
	m.cacheTimestamp = time.Time{}
	m.mutex.Unlock()

	m.hub.Broadcast("plugins_updated", map[string]interface{}{
		"timestamp": time.Now().Unix(),
	})
}

func (m *Manager) UpdatePlugin(pluginName string) {
	if plugin, exists := m.GetPlugin(pluginName); exists {
		ctx := context.Background()
		if err := plugin.UpdateData(ctx); err != nil {
			fmt.Printf("Error updating plugin %s: %v\n", pluginName, err)
		}

		m.mutex.Lock()
		delete(m.renderedCache, pluginName)
		m.mutex.Unlock()

		go func() {
			rendered, err := plugin.Render(ctx)
			if err != nil {
				fmt.Printf("Error re-rendering plugin %s: %v\n", pluginName, err)
				return
			}

			m.mutex.Lock()
			m.renderedCache[pluginName] = template.HTML(rendered)
			m.mutex.Unlock()

			m.hub.Broadcast("plugin_rendered", map[string]interface{}{
				"plugin":    pluginName,
				"rendered":  rendered,
				"timestamp": time.Now().Unix(),
			})
		}()
	}
}

func (m *Manager) BroadcastUpdate(updateType string, data map[string]interface{}) {
	m.hub.Broadcast(updateType, data)
}

func (m *Manager) RefreshMeme() {
	if memePlugin, exists := m.GetPlugin("meme"); exists {
		if meme, ok := memePlugin.(*MemePlugin); ok {
			meme.RefreshMeme()
		}
	}
}

func (m *Manager) SearchAndPlayTrack(query string) error {
	if lastfmPlugin, exists := m.GetPlugin("lastfm"); exists {
		if lastfm, ok := lastfmPlugin.(*LastFMPlugin); ok {
			_, err := lastfm.SearchAndPlayTrack(query)
			return err
		}
	}
	return fmt.Errorf("lastfm plugin not found")
}

func (m *Manager) GetSystemStats() map[string]interface{} {
	stats := map[string]interface{}{
		"total_plugins":   len(m.plugins),
		"enabled_plugins": len(m.GetEnabledPlugins()),
		"cache_timestamp": m.cacheTimestamp.Unix(),
		"last_update":     m.lastUpdate.Unix(),
	}

	if visitorsPlugin, exists := m.GetPlugin("visitors"); exists {
		if visitors, ok := visitorsPlugin.(*VisitorsPlugin); ok {
			visitors.mutex.RLock()
			stats["total_visits"] = visitors.visitCount
			stats["today_visits"] = visitors.todayCount
			visitors.mutex.RUnlock()
		}
	}

	return stats
}
