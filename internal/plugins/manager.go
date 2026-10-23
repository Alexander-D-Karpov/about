package plugins

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/metrics"
	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type Plugin interface {
	Name() string
	Render(ctx context.Context) (string, error)
	RenderText(ctx context.Context) (string, error)
	UpdateData(ctx context.Context) error
	GetSettings() map[string]interface{}
	SetSettings(settings map[string]interface{}) error
}

// LayoutNotifier is notified when a plugin's rendered content changes, so a
// layout-measurement pass can be scheduled. Implemented by the measure worker.
type LayoutNotifier interface {
	Notify(plugin string)
}

type Manager struct {
	plugins          map[string]Plugin
	storage          *storage.Storage
	hub              *stream.Hub
	config           *config.Config
	mutex            sync.RWMutex
	renderedCache    map[string]template.HTML
	cacheTimestamp   time.Time
	lastUpdate       time.Time
	appStartTime     time.Time
	prometheusClient *metrics.PrometheusClient
	layoutNotifier   LayoutNotifier
	// last broadcast HTML hash per plugin, to suppress identical re-renders
	lastBroadcastHash map[string]string
	// when each plugin last got a frame, so volatile-only changes still
	// refresh eventually instead of never
	lastBroadcastAt map[string]time.Time
}

func (m *Manager) SetLayoutNotifier(n LayoutNotifier) {
	m.mutex.Lock()
	m.layoutNotifier = n
	m.mutex.Unlock()
}

func (m *Manager) notifyLayout(plugin string) {
	m.mutex.RLock()
	n := m.layoutNotifier
	m.mutex.RUnlock()
	if n != nil {
		n.Notify(plugin)
	}
}

func NewManager(storage *storage.Storage, hub *stream.Hub, config *config.Config, appStartTime time.Time) *Manager {
	prometheusClient := metrics.NewPrometheusClient(
		config.PrometheusPushGateway,
		config.PrometheusQueryURL,
		config.PrometheusUser,
		config.PrometheusPassword,
		config.PrometheusJobName,
	)

	return &Manager{
		plugins:          make(map[string]Plugin),
		storage:          storage,
		hub:              hub,
		config:           config,
		renderedCache:    make(map[string]template.HTML),
		appStartTime:     appStartTime,
		prometheusClient: prometheusClient,
	}
}

func (m *Manager) LoadAll() error {
	beatLeaderPlugin := NewBeatLeaderPlugin(m.storage, m.hub, m.config.MediaPath)
	beatLeaderPlugin.SetCacheInvalidator(func() {
		m.InvalidatePluginCache("beatleader")
	})

	musicPlugin := NewMusicPlugin(m.storage, m.hub, m.config)
	musicPlugin.SetPluginManager(m)

	codePlugin := NewCodePlugin(m.storage, m.hub, m.config)
	codePlugin.SetCacheInvalidator(func() {
		m.InvalidatePluginCache("code")
	})

	steamPlugin := NewSteamPlugin(m.storage, m.hub, m.config.SteamKey, m.config.DataPath)
	steamPlugin.SetCacheInvalidator(func() {
		m.InvalidatePluginCache("steam")
	})

	plugins := []Plugin{
		NewProfilePlugin(m.storage, m.hub),
		NewSocialPlugin(m.storage, m.hub),
		NewTechStackPlugin(m.storage, m.hub),
		NewProjectsPlugin(m.storage, m.hub),
		NewNeofetchPlugin(m.storage, m.hub),
		NewWebringPlugin(m.storage, m.hub),
		musicPlugin,
		beatLeaderPlugin,
		steamPlugin,
		NewVisitorsPlugin(m.storage, m.hub, m.config.DataPath),
		NewServicesPlugin(m.storage, m.hub, m.config.ServicesProxyURL),
		codePlugin,
		NewInfoPlugin(m.storage, m.hub, m.appStartTime),
		NewPersonalPlugin(m.storage, m.hub),
		NewMemePlugin(m.storage, m.hub),
		NewPlacesPlugin(m.storage, m.hub),
		NewHealthPlugin(m.storage, m.hub, m.config.HCGatewayURL, m.config.HCGatewayUser, m.config.HCGatewayPassword),
		NewPhotosPlugin(m.storage, m.hub),
		NewBikePlugin(m.storage, m.hub),
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, plugin := range plugins {
		m.plugins[plugin.Name()] = plugin

		if provider, ok := plugin.(metrics.MetricsProvider); ok {
			m.prometheusClient.RegisterProvider(provider)
		}
	}

	return nil
}

func (m *Manager) StartPrometheusMetrics(ctx context.Context) {
	if m.prometheusClient.IsEnabled() {
		interval := time.Duration(m.config.PrometheusPushInterval) * time.Second
		go m.prometheusClient.StartPeriodicPush(ctx, interval)
	}
}

func (m *Manager) GetPrometheusClient() *metrics.PrometheusClient {
	return m.prometheusClient
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

// preRenderPlugins warms the render cache.
//
// It renders WITHOUT holding the manager lock, then swaps the result in at the
// end. It used to hold the write lock for the whole loop - up to 30 seconds -
// during which every request blocked on the read lock in GetRenderedPlugins.
// That is what made a cold start feel like the site was down: the listener was
// up but every request sat waiting for the warm-up to finish.
func (m *Manager) preRenderPlugins(ctx context.Context) {
	m.mutex.RLock()
	enabledPlugins := m.getEnabledPluginsLocked()
	m.mutex.RUnlock()

	fresh := make(map[string]template.HTML)

	renderCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for _, plugin := range enabledPlugins {
		if plugin.Name() == "meme" || plugin.Name() == "info" || plugin.Name() == "visitors" || plugin.Name() == "services" {
			continue
		}

		select {
		case <-renderCtx.Done():
			fmt.Printf("Pre-rendering timeout for remaining plugins\n")
			// Publish whatever finished rather than discarding it.
			m.publishRenderCache(fresh)
			return
		default:
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Error pre-rendering plugin %s: panic: %v\n", plugin.Name(), r)
				}
			}()

			pluginCtx, pluginCancel := context.WithTimeout(renderCtx, 5*time.Second)
			defer pluginCancel()

			rendered, err := plugin.Render(pluginCtx)
			if err != nil {
				fmt.Printf("Error pre-rendering plugin %s: %v\n", plugin.Name(), err)
				return
			}
			if rendered != "" {
				fresh[plugin.Name()] = template.HTML(rendered)
			}
		}()
	}

	m.publishRenderCache(fresh)
}

// publishRenderCache swaps a freshly built cache in under a brief write lock.
func (m *Manager) publishRenderCache(fresh map[string]template.HTML) {
	m.mutex.Lock()
	m.renderedCache = fresh
	m.cacheTimestamp = time.Now()
	m.mutex.Unlock()
}

func (m *Manager) GetRenderedPlugins(ctx context.Context) []template.HTML {
	select {
	case <-ctx.Done():
		return []template.HTML{}
	default:
	}

	m.mutex.RLock()
	enabledPlugins := m.getEnabledPluginsLocked()
	m.mutex.RUnlock()

	var renderedPlugins []template.HTML
	renderCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	// A plugin that can't render in time (its data source hanging behind a
	// dead proxy, say) must not take the rest of the page with it: each render
	// has its own timeout, a failed or slow render falls back to that plugin's
	// last good HTML, and once the page budget is spent the remaining plugins
	// come from that cache instead of being dropped.
	for _, plugin := range enabledPlugins {
		pluginName := plugin.Name()

		m.mutex.RLock()
		cachedRender, hasCached := m.renderedCache[pluginName]
		cacheAge := time.Since(m.cacheTimestamp)
		m.mutex.RUnlock()

		if renderCtx.Err() != nil {
			if hasCached {
				renderedPlugins = append(renderedPlugins, cachedRender)
			}
			continue
		}

		alwaysFresh := pluginName == "meme" || pluginName == "info" || pluginName == "visitors" || pluginName == "services"
		if !alwaysFresh && hasCached && !m.cacheTimestamp.IsZero() && cacheAge < 3*time.Minute {
			renderedPlugins = append(renderedPlugins, cachedRender)
			continue
		}

		timeout := 2000 * time.Millisecond
		if alwaysFresh {
			timeout = 1500 * time.Millisecond
		}
		rendered := m.renderPluginWithTimeout(plugin, renderCtx, timeout)
		if rendered == "" {
			if hasCached {
				renderedPlugins = append(renderedPlugins, cachedRender)
			}
			continue
		}

		renderedHTML := template.HTML(rendered)
		renderedPlugins = append(renderedPlugins, renderedHTML)

		m.mutex.Lock()
		m.renderedCache[pluginName] = renderedHTML
		if m.cacheTimestamp.IsZero() {
			m.cacheTimestamp = time.Now()
		}
		m.mutex.Unlock()
	}

	if renderCtx.Err() != nil {
		fmt.Printf("Rendering budget spent, filled the rest of %d plugins from cache\n", len(renderedPlugins))
	}
	return renderedPlugins
}

func (m *Manager) renderPluginWithTimeout(plugin Plugin, ctx context.Context, timeout time.Duration) string {
	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				errorChan <- fmt.Errorf("panic: %v", r)
			}
		}()

		pluginCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		rendered, err := plugin.Render(pluginCtx)
		if err != nil {
			errorChan <- err
		} else {
			resultChan <- rendered
		}
	}()

	// Wait for this plugin's own timeout, not just the page deadline, so one
	// hung plugin can't eat the whole render budget.
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case rendered := <-resultChan:
		return rendered
	case err := <-errorChan:
		fmt.Printf("Error rendering %s plugin: %v\n", plugin.Name(), err)
		return ""
	case <-timer.C:
		fmt.Printf("Timeout rendering %s plugin\n", plugin.Name())
		return ""
	case <-ctx.Done():
		fmt.Printf("Timeout rendering %s plugin\n", plugin.Name())
		return ""
	}
}

func (m *Manager) GetRenderedPluginsFresh(ctx context.Context) []template.HTML {
	m.mutex.RLock()
	enabledPlugins := m.getEnabledPluginsLocked()
	m.mutex.RUnlock()

	var renderedPlugins []template.HTML
	renderCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for _, plugin := range enabledPlugins {
		select {
		case <-renderCtx.Done():
			fmt.Printf("Fresh rendering timeout, returning partial results\n")
			return renderedPlugins
		default:
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Error rendering plugin %s: panic: %v\n", plugin.Name(), r)
				}
			}()

			pluginCtx, pluginCancel := context.WithTimeout(renderCtx, 3*time.Second)
			defer pluginCancel()

			rendered, err := plugin.Render(pluginCtx)
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
	order := make(map[string]int, len(m.plugins))

	for _, plugin := range m.plugins {
		config := m.storage.GetPluginConfig(plugin.Name())
		if config.Enabled && !hiddenPlugins[plugin.Name()] {
			enabled = append(enabled, plugin)
			order[plugin.Name()] = config.Order
		}
	}

	// m.plugins is a map, so `enabled` starts in a random order and a stable
	// sort would hand ties whatever order this particular request happened to
	// draw. Two plugins sharing an Order then swap places between page loads,
	// which reshuffles the whole packed layout. Break ties by name so a given
	// config always renders the same page.
	//
	// Between the fixed edges (webring first, then profile; projects last) the
	// page reads as groups: who I am, what's happening now, stats,
	// infrastructure, gallery. The configured order sorts within a group.
	sort.Slice(enabled, func(i, j int) bool {
		nameI, nameJ := enabled[i].Name(), enabled[j].Name()
		if gi, gj := pluginGroup(nameI), pluginGroup(nameJ); gi != gj {
			return gi < gj
		}
		if order[nameI] != order[nameJ] {
			return order[nameI] < order[nameJ]
		}
		return nameI < nameJ
	})

	var webring []Plugin
	var profile []Plugin
	var middle []Plugin
	var projects []Plugin

	for _, plugin := range enabled {
		switch plugin.Name() {
		case "webring":
			webring = append(webring, plugin)
		case "profile":
			profile = append(profile, plugin)
		case "projects":
			projects = append(projects, plugin)
		default:
			middle = append(middle, plugin)
		}
	}

	result := make([]Plugin, 0, len(enabled))
	result = append(result, webring...)
	result = append(result, profile...)
	result = append(result, middle...)
	result = append(result, projects...)

	return result
}

// hiddenPlugins stay off the page whatever their config says. Health is
// hidden for now; drop it from here to bring the card back.
var hiddenPlugins = map[string]bool{
	"health": true,
}

// pluginGroups ranks the middle of the page into sections.
var pluginGroups = map[string]int{
	"social": 0, "techstack": 0,
	"music": 1, "steam": 1, "health": 1,
	"code": 2, "bike": 2, "beatleader": 2, "visitors": 2,
	"services": 3, "neofetch": 3, "info": 3,
	"photos": 4, "places": 4, "meme": 4, "personal": 4,
}

func pluginGroup(name string) int {
	if g, ok := pluginGroups[name]; ok {
		return g
	}
	return 4
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
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	m.mutex.RLock()
	plugins := make([]Plugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	m.mutex.RUnlock()

	type pluginUpdate struct {
		plugin Plugin
		err    error
	}

	updateChan := make(chan pluginUpdate, len(plugins))
	activeUpdates := 0
	var failedPlugins []string

	for _, plugin := range plugins {
		config := m.storage.GetPluginConfig(plugin.Name())
		if !config.Enabled {
			continue
		}

		activeUpdates++
		go func(p Plugin) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Plugin %s update panic: %v", p.Name(), r)
					updateChan <- pluginUpdate{p, fmt.Errorf("panic: %v", r)}
				}
			}()

			pluginCtx, pluginCancel := context.WithTimeout(ctx, 30*time.Second)
			defer pluginCancel()

			err := p.UpdateData(pluginCtx)
			updateChan <- pluginUpdate{p, err}
		}(plugin)
	}

	completed := 0
	timeout := time.NewTimer(3 * time.Minute)
	defer timeout.Stop()

	for completed < activeUpdates {
		select {
		case update := <-updateChan:
			completed++
			if update.err != nil {
				log.Printf("Error updating plugin %s: %v", update.plugin.Name(), update.err)
				failedPlugins = append(failedPlugins, update.plugin.Name())
			}

		case <-timeout.C:
			log.Printf("Plugin updates timed out, %d/%d completed, %d failed", completed, activeUpdates, len(failedPlugins))
			if len(failedPlugins) > 0 {
				log.Printf("Failed plugins: %s", strings.Join(failedPlugins, ", "))
			}
			cancel()
			goto cleanup

		case <-ctx.Done():
			log.Printf("Plugin updates cancelled, %d/%d completed, %d failed", completed, activeUpdates, len(failedPlugins))
			if len(failedPlugins) > 0 {
				log.Printf("Failed plugins: %s", strings.Join(failedPlugins, ", "))
			}
			goto cleanup
		}
	}

cleanup:
	breakLoop := false
	for completed < activeUpdates && !breakLoop {
		select {
		case update := <-updateChan:
			completed++
			if update.err != nil {
				failedPlugins = append(failedPlugins, update.plugin.Name())
			}
		case <-time.After(100 * time.Millisecond):
			breakLoop = true
		}
	}

	m.mutex.Lock()
	m.cacheTimestamp = time.Time{}
	m.mutex.Unlock()

	select {
	case <-ctx.Done():
		log.Printf("Skipping WebSocket broadcast due to context cancellation")
	default:
		m.hub.Broadcast("plugins_updated", map[string]interface{}{
			"timestamp": time.Now().Unix(),
			"completed": completed,
			"total":     activeUpdates,
			"failed":    len(failedPlugins),
		})
	}

	if len(failedPlugins) > 0 {
		log.Printf("Plugin update summary: %d completed, %d failed: %s",
			completed-len(failedPlugins), len(failedPlugins), strings.Join(failedPlugins, ", "))
	} else {
		log.Printf("Plugin update summary: all %d plugins completed successfully", completed)
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
	if musicPlugin, exists := m.GetPlugin("music"); exists {
		if music, ok := musicPlugin.(*MusicPlugin); ok {
			_, err := music.SearchAndPlayTrack(query)
			return err
		}
	}
	return fmt.Errorf("music plugin not found")
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

	if servicesPlugin, exists := m.GetPlugin("services"); exists {
		if services, ok := servicesPlugin.(*ServicesPlugin); ok {
			services.mutex.RLock()
			onlineCount := 0
			offlineCount := 0
			for _, status := range services.serviceStatuses {
				if status.Status == "online" {
					onlineCount++
				} else if status.Status == "offline" {
					offlineCount++
				}
			}
			stats["services_online"] = onlineCount
			stats["services_offline"] = offlineCount
			stats["services_total"] = len(services.serviceStatuses)
			services.mutex.RUnlock()
		}
	}

	return stats
}

func (m *Manager) InvalidateCache() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.renderedCache = make(map[string]template.HTML)
	m.cacheTimestamp = time.Time{}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		m.UpdateExternalData()
		m.preRenderPlugins(ctx)
		m.notifyLayout("*")

		m.hub.Broadcast("cache_refreshed", map[string]interface{}{
			"timestamp": time.Now().Unix(),
			"action":    "invalidated_and_refreshed",
		})
	}()
}

func (m *Manager) UpdateExternalDataBackground() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	m.mutex.RLock()
	plugins := make([]Plugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	m.mutex.RUnlock()

	type pluginUpdate struct {
		plugin Plugin
		err    error
	}

	updateChan := make(chan pluginUpdate, len(plugins))
	activeUpdates := 0
	var failedPlugins []string
	var successfulPlugins []string

	for _, plugin := range plugins {
		config := m.storage.GetPluginConfig(plugin.Name())
		if !config.Enabled {
			continue
		}

		if plugin.Name() == "info" || plugin.Name() == "visitors" {
			continue
		}

		activeUpdates++
		go func(p Plugin) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Background plugin %s update panic: %v", p.Name(), r)
					updateChan <- pluginUpdate{p, fmt.Errorf("panic: %v", r)}
				}
			}()

			pluginCtx, pluginCancel := context.WithTimeout(ctx, 20*time.Second)
			defer pluginCancel()

			err := p.UpdateData(pluginCtx)
			updateChan <- pluginUpdate{p, err}
		}(plugin)
	}

	if activeUpdates == 0 {
		log.Printf("No plugins to update in background")
		return
	}

	completed := 0
	timeout := time.NewTimer(2*time.Minute + 30*time.Second)
	defer timeout.Stop()

	for completed < activeUpdates {
		select {
		case update := <-updateChan:
			completed++
			if update.err != nil {
				log.Printf("Background update error for plugin %s: %v", update.plugin.Name(), update.err)
				failedPlugins = append(failedPlugins, update.plugin.Name())
			} else {
				successfulPlugins = append(successfulPlugins, update.plugin.Name())
			}

		case <-timeout.C:
			log.Printf("Background plugin updates timed out, %d/%d completed, %d failed", completed, activeUpdates, len(failedPlugins))
			if len(failedPlugins) > 0 {
				log.Printf("Background failed plugins: %s", strings.Join(failedPlugins, ", "))
			}
			if len(successfulPlugins) > 0 {
				log.Printf("Background successful plugins: %s", strings.Join(successfulPlugins, ", "))
			}
			cancel()
			goto cleanup

		case <-ctx.Done():
			log.Printf("Background plugin updates cancelled, %d/%d completed", completed, activeUpdates)
			goto cleanup
		}
	}

cleanup:
	drainTimeout := time.After(2 * time.Second)
drain:
	for completed < activeUpdates {
		select {
		case update := <-updateChan:
			completed++
			if update.err != nil {
				failedPlugins = append(failedPlugins, update.plugin.Name())
			} else {
				successfulPlugins = append(successfulPlugins, update.plugin.Name())
			}
		case <-drainTimeout:
			break drain
		}
	}

	if len(successfulPlugins) > 0 {
		go func() {
			bgCtx, bgCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer bgCancel()

			m.mutex.Lock()
			for _, pluginName := range successfulPlugins {
				delete(m.renderedCache, pluginName)
			}
			m.mutex.Unlock()

			m.preRenderPluginsSelective(bgCtx, successfulPlugins)
		}()
	}

	select {
	case <-ctx.Done():
		log.Printf("Skipping background update WebSocket broadcast due to context cancellation")
	default:
		if len(successfulPlugins) > 2 {
			m.hub.Broadcast("plugins_updated", map[string]interface{}{
				"timestamp":  time.Now().Unix(),
				"background": true,
				"completed":  len(successfulPlugins),
				"failed":     len(failedPlugins),
			})
		}
	}

	log.Printf("Background update summary: %d successful, %d failed", len(successfulPlugins), len(failedPlugins))
	if len(failedPlugins) > 0 {
		log.Printf("Background failed plugins: %s", strings.Join(failedPlugins, ", "))
	}
}

type selectiveRender struct {
	name string
	html string
}

// How long a plugin may go without a broadcast when only volatile fields
// (progress bars, "9m ago" style timestamps) have changed. Bounds staleness of
// those fields while still collapsing the every-20s churn.
const volatileRefreshInterval = 5 * time.Minute

// Markup that changes on every render without representing new data. Blanking
// these before hashing is what lets broadcastRenderedIfChanged tell "the track
// changed" apart from "the progress bar moved 7%".
//
// The music plugin re-renders every 20s off the Last.fm ticker; comparing two
// consecutive 27KB payloads showed only four differing lines - the progress
// fill width, the elapsed counter, and two "N minutes ago" labels. The browser
// already animates the first two locally (plugin-music.js drives
// [data-music-fill] and [data-music-elapsed]), so re-sending the whole section
// for them was pure churn: clients replaced the node and re-packed the mosaic
// three times a minute.
var volatileMarkup = []*regexp.Regexp{
	// inline progress widths: style="width:62%"
	regexp.MustCompile(`width:\s*[\d.]+%`),
	// relative times anywhere in the text, not just as a whole element:
	// "16m ago", "2 hours ago", and prefixed forms like
	// "Last played 14m ago" (data-music-status), which an element-anchored
	// pattern missed - that single line kept re-broadcasting the section
	// every minute.
	regexp.MustCompile(`\b\d+\s*(?:s|m|h|d|sec|secs|min|mins|hour|hours|day|days)\s+ago\b`),
	// elapsed/duration counters: >2:54<, >1:03:22<
	regexp.MustCompile(`>\s*\d{1,2}:\d{2}(?::\d{2})?\s*<`),
}

func normalizeForDedup(html string) string {
	for _, re := range volatileMarkup {
		html = re.ReplaceAllString(html, "")
	}
	return html
}

// broadcastRenderedIfChanged pushes a plugin's new HTML to clients, but only
// when it actually differs from what was last sent.
//
// The music plugin re-renders every 20s off the Last.fm ticker and was
// broadcasting an identical ~25KB payload each time. Clients replace the whole
// section and re-pack the mosaic on every frame, so the page visibly rebuilt
// three times a minute - which is what "the site keeps reloading" looked like.
// Skipping unchanged renders removes that churn without delaying real updates.
func (m *Manager) broadcastRenderedIfChanged(pluginName, html string) bool {
	if m.hub == nil || html == "" {
		return false
	}

	sum := sha1.Sum([]byte(normalizeForDedup(html)))
	hash := hex.EncodeToString(sum[:])

	m.mutex.Lock()
	if m.lastBroadcastHash == nil {
		m.lastBroadcastHash = make(map[string]string)
	}
	if m.lastBroadcastAt == nil {
		m.lastBroadcastAt = make(map[string]time.Time)
	}
	// Volatile-only changes are suppressed, but not forever: relative
	// timestamps ("9m ago") would otherwise sit stale until the underlying
	// data changed. Let one through every so often as a floor.
	stale := time.Since(m.lastBroadcastAt[pluginName]) > volatileRefreshInterval
	if m.lastBroadcastHash[pluginName] == hash && !stale {
		m.mutex.Unlock()
		return false
	}
	m.lastBroadcastHash[pluginName] = hash
	m.lastBroadcastAt[pluginName] = time.Now()
	m.mutex.Unlock()

	m.hub.Broadcast("plugin_rendered", map[string]interface{}{
		"plugin":    pluginName,
		"rendered":  html,
		"timestamp": time.Now().Unix(),
	})
	return true
}

// preRenderPluginsSelective re-renders the given plugins and pushes the fresh
// HTML to connected clients.
//
// It previously only refreshed m.renderedCache, so after a background update
// every open page still showed stale content. The only thing telling clients
// anything had changed was the "plugins_updated" broadcast, which the frontend
// turned into a full location.reload() - so every visitor's page reloaded on
// each background cycle. Emitting "plugin_rendered" per plugin lets the client
// swap just that section instead.
func (m *Manager) preRenderPluginsSelective(ctx context.Context, pluginNames []string) {
	for _, r := range m.renderSelective(ctx, pluginNames) {
		m.broadcastRenderedIfChanged(r.name, r.html)
	}
}

// renderSelective renders under the manager lock and returns what changed, so
// the caller can broadcast without holding the lock.
func (m *Manager) renderSelective(ctx context.Context, pluginNames []string) []selectiveRender {
	var rendered []selectiveRender
	if len(pluginNames) == 0 {
		return rendered
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	pluginMap := make(map[string]Plugin)
	for _, plugin := range m.plugins {
		pluginMap[plugin.Name()] = plugin
	}

	renderCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	for _, pluginName := range pluginNames {
		plugin, exists := pluginMap[pluginName]
		if !exists {
			continue
		}

		if plugin.Name() == "meme" || plugin.Name() == "info" || plugin.Name() == "visitors" || plugin.Name() == "services" {
			continue
		}

		select {
		case <-renderCtx.Done():
			fmt.Printf("Selective pre-rendering timeout for remaining plugins\n")
			return rendered
		default:
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Error selective pre-rendering plugin %s: panic: %v\n", plugin.Name(), r)
				}
			}()

			pluginCtx, pluginCancel := context.WithTimeout(renderCtx, 3*time.Second)
			defer pluginCancel()

			html, err := plugin.Render(pluginCtx)
			if err != nil {
				fmt.Printf("Error selective pre-rendering plugin %s: %v\n", plugin.Name(), err)
				return
			}
			if html != "" {
				m.renderedCache[plugin.Name()] = template.HTML(html)
				rendered = append(rendered, selectiveRender{name: plugin.Name(), html: html})
				fmt.Printf("Selective pre-rendered plugin: %s\n", plugin.Name())
			}
		}()
	}

	return rendered
}

func (m *Manager) UpdatePlugin(pluginName string) error {
	plugin, exists := m.GetPlugin(pluginName)
	if !exists {
		return fmt.Errorf("plugin %s not found", pluginName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	updateErr := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				updateErr <- fmt.Errorf("plugin %s update panic: %v", pluginName, r)
			}
		}()
		updateErr <- plugin.UpdateData(ctx)
	}()

	select {
	case err := <-updateErr:
		if err != nil {
			return fmt.Errorf("failed to update plugin %s: %w", pluginName, err)
		}
	case <-ctx.Done():
		return fmt.Errorf("plugin %s update timeout", pluginName)
	}

	m.mutex.Lock()
	delete(m.renderedCache, pluginName)
	m.mutex.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Error re-rendering plugin %s: panic: %v\n", pluginName, r)
			}
		}()

		renderCtx, renderCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer renderCancel()

		rendered, err := plugin.Render(renderCtx)
		if err != nil {
			fmt.Printf("Error re-rendering plugin %s: %v\n", pluginName, err)
			return
		}

		m.mutex.Lock()
		m.renderedCache[pluginName] = template.HTML(rendered)
		m.mutex.Unlock()

		// Only push when the markup actually changed - this path runs off the
		// 20s Last.fm ticker and was re-sending an identical payload.
		m.broadcastRenderedIfChanged(pluginName, rendered)
	}()

	return nil
}

func (m *Manager) GetTextRenderedPlugins(ctx context.Context) []string {
	select {
	case <-ctx.Done():
		return []string{}
	default:
	}

	m.mutex.RLock()
	enabledPlugins := m.getEnabledPluginsLocked()
	m.mutex.RUnlock()

	var renderedPlugins []string
	renderCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	for _, plugin := range enabledPlugins {
		select {
		case <-renderCtx.Done():
			return renderedPlugins
		default:
		}

		rendered := m.renderPluginTextWithTimeout(plugin, ctx, 500*time.Millisecond)
		if rendered != "" {
			renderedPlugins = append(renderedPlugins, rendered)
		}
	}

	return renderedPlugins
}

func (m *Manager) renderPluginTextWithTimeout(plugin Plugin, ctx context.Context, timeout time.Duration) string {
	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				errorChan <- fmt.Errorf("panic: %v", r)
			}
		}()

		pluginCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		rendered, err := plugin.RenderText(pluginCtx)
		if err != nil {
			errorChan <- err
		} else {
			resultChan <- rendered
		}
	}()

	select {
	case rendered := <-resultChan:
		return rendered
	case <-errorChan:
		return ""
	case <-ctx.Done():
		return ""
	}
}

func (m *Manager) GetSystemTextSummary() string {
	m.mutex.RLock()
	totalPlugins := len(m.plugins)
	enabledCount := len(m.getEnabledPluginsLocked())
	m.mutex.RUnlock()

	return fmt.Sprintf("System: %d/%d plugins enabled", enabledCount, totalPlugins)
}

func (m *Manager) InvalidatePluginCache(pluginName string) {
	m.mutex.Lock()
	delete(m.renderedCache, pluginName)
	m.mutex.Unlock()

	log.Printf("Invalidated cache for plugin: %s", pluginName)

	m.notifyLayout(pluginName)
}

func (m *Manager) GetClientCount() int {
	if m.hub == nil {
		return 0
	}
	return m.hub.GetClientCount()
}
