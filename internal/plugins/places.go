package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type PlacesPlugin struct {
	storage    *storage.Storage
	hub        *stream.Hub
	places     []Place
	lastUpdate time.Time
	mutex      sync.RWMutex
}

type Place struct {
	Name        string   `json:"name"`
	Lat         float64  `json:"lat"`
	Lng         float64  `json:"lng"`
	Country     string   `json:"country"`
	City        string   `json:"city"`
	Description string   `json:"description"`
	VisitedDate string   `json:"visited_date"`
	Photos      []string `json:"photos"`
	Category    string   `json:"category"`
}

type PlaceStats struct {
	TotalPlaces   int            `json:"total_places"`
	Countries     int            `json:"countries"`
	Cities        int            `json:"cities"`
	CountryList   []string       `json:"country_list"`
	CategoryCount map[string]int `json:"category_count"`
}

func NewPlacesPlugin(storage *storage.Storage, hub *stream.Hub) *PlacesPlugin {
	return &PlacesPlugin{
		storage: storage,
		hub:     hub,
		places:  []Place{},
	}
}

func (p *PlacesPlugin) Name() string {
	return "places"
}

func (p *PlacesPlugin) Render(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Visited Places")
	defaultZoom := p.getConfigInt(settings, "ui.defaultZoom", 2)
	defaultLat := p.getConfigFloat(settings, "ui.defaultLat", 25.0)
	defaultLng := p.getConfigFloat(settings, "ui.defaultLng", 0.0)
	heatmapIntensity := p.getConfigFloat(settings, "ui.heatmapIntensity", 0.6)
	heatmapRadius := p.getConfigInt(settings, "ui.heatmapRadius", 25)
	markerRadius := p.getConfigInt(settings, "ui.markerRadius", 8)

	p.mutex.RLock()
	places := p.places
	p.mutex.RUnlock()

	if len(places) == 0 {
		placesRaw, ok := settings["places"].([]interface{})
		if ok {
			for _, placeRaw := range placesRaw {
				placeMap, ok := placeRaw.(map[string]interface{})
				if !ok {
					continue
				}
				place := Place{
					Name:        p.getStringFromMap(placeMap, "name", ""),
					Lat:         p.getFloatFromMap(placeMap, "lat", 0),
					Lng:         p.getFloatFromMap(placeMap, "lng", 0),
					Country:     p.getStringFromMap(placeMap, "country", ""),
					City:        p.getStringFromMap(placeMap, "city", ""),
					Description: p.getStringFromMap(placeMap, "description", ""),
					VisitedDate: p.getStringFromMap(placeMap, "visited_date", ""),
					Category:    p.getStringFromMap(placeMap, "category", "travel"),
				}
				if place.Lat != 0 && place.Lng != 0 {
					places = append(places, place)
				}
			}
			p.mutex.Lock()
			p.places = places
			p.mutex.Unlock()
		}
	}

	placesJSON, err := json.Marshal(places)
	if err != nil {
		placesJSON = []byte("[]")
	}

	tmpl := `
<section class="places-section section" id="places-plugin" data-w="2">
    <h3>{{.SectionTitle}}</h3>

    <div class="places-map-container" id="places-map-container">
        <div class="places-map" id="places-map"></div>
        <div class="map-controls">
            <button class="map-control-btn" id="toggle-heatmap" title="Toggle heatmap/markers">
                <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
                    <path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7zm0 9.5c-1.38 0-2.5-1.12-2.5-2.5s1.12-2.5 2.5-2.5 2.5 1.12 2.5 2.5-1.12 2.5-2.5 2.5z"/>
                </svg>
            </button>
            <button class="map-control-btn" id="fit-bounds" title="Fit all places">
                <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
                    <path d="M15 3l2.3 2.3-2.89 2.87 1.42 1.42L18.7 6.7 21 9V3h-6zM3 9l2.3-2.3 2.87 2.89 1.42-1.42L6.7 5.3 9 3H3v6zm6 12l-2.3-2.3 2.89-2.87-1.42-1.42L5.3 17.3 3 15v6h6zm12-6l-2.3 2.3-2.87-2.89-1.42 1.42 2.89 2.87L15 21h6v-6z"/>
                </svg>
            </button>
        </div>
        <div class="map-loading" id="map-loading">
            <div class="loading"></div>
            <span>Loading map...</span>
        </div>
    </div>

    <script id="places-data" type="application/json">{{.PlacesJSON}}</script>
    <script id="places-config" type="application/json">{
        "defaultZoom": {{.DefaultZoom}},
        "defaultLat": {{.DefaultLat}},
        "defaultLng": {{.DefaultLng}},
        "heatmapIntensity": {{.HeatmapIntensity}},
        "heatmapRadius": {{.HeatmapRadius}},
        "markerRadius": {{.MarkerRadius}}
    }</script>
</section>`

	data := struct {
		SectionTitle     string
		PlacesJSON       template.JS
		DefaultZoom      int
		DefaultLat       float64
		DefaultLng       float64
		HeatmapIntensity float64
		HeatmapRadius    int
		MarkerRadius     int
	}{
		SectionTitle:     sectionTitle,
		PlacesJSON:       template.JS(placesJSON),
		DefaultZoom:      defaultZoom,
		DefaultLat:       defaultLat,
		DefaultLng:       defaultLng,
		HeatmapIntensity: heatmapIntensity,
		HeatmapRadius:    heatmapRadius,
		MarkerRadius:     markerRadius,
	}

	t, err := template.New("places").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = t.Execute(&buf, data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (p *PlacesPlugin) calculateStats(places []Place) PlaceStats {
	stats := PlaceStats{
		TotalPlaces:   len(places),
		CategoryCount: make(map[string]int),
	}

	countriesMap := make(map[string]bool)
	citiesMap := make(map[string]bool)

	for _, place := range places {
		if place.Country != "" {
			countriesMap[place.Country] = true
		}
		if place.City != "" {
			citiesMap[place.City] = true
		}
		if place.Category != "" {
			stats.CategoryCount[place.Category]++
		}
	}

	stats.Countries = len(countriesMap)
	stats.Cities = len(citiesMap)

	for country := range countriesMap {
		stats.CountryList = append(stats.CountryList, country)
	}

	return stats
}

func (p *PlacesPlugin) UpdateData(ctx context.Context) error {
	return nil
}

func (p *PlacesPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *PlacesPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings

	p.mutex.Lock()
	p.places = []Place{}
	p.mutex.Unlock()

	err := p.storage.SetPluginConfig(p.Name(), config)
	if err != nil {
		return err
	}

	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(),
		"action": "settings_changed",
	})

	return nil
}

func (p *PlacesPlugin) RenderText(ctx context.Context) (string, error) {
	p.mutex.RLock()
	places := p.places
	p.mutex.RUnlock()

	if len(places) == 0 {
		return "Places: No places configured", nil
	}

	stats := p.calculateStats(places)
	return fmt.Sprintf("Places: %d places in %d countries", stats.TotalPlaces, stats.Countries), nil
}

func (p *PlacesPlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(string); ok {
				return value
			}
			return defaultValue
		}
		if next, ok := current[k].(map[string]interface{}); ok {
			current = next
		} else {
			return defaultValue
		}
	}
	return defaultValue
}

func (p *PlacesPlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(bool); ok {
				return value
			}
			return defaultValue
		}
		if next, ok := current[k].(map[string]interface{}); ok {
			current = next
		} else {
			return defaultValue
		}
	}
	return defaultValue
}

func (p *PlacesPlugin) getConfigInt(settings map[string]interface{}, key string, defaultValue int) int {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(float64); ok {
				return int(value)
			}
			if value, ok := current[k].(int); ok {
				return value
			}
			return defaultValue
		}
		if next, ok := current[k].(map[string]interface{}); ok {
			current = next
		} else {
			return defaultValue
		}
	}
	return defaultValue
}

func (p *PlacesPlugin) getConfigFloat(settings map[string]interface{}, key string, defaultValue float64) float64 {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(float64); ok {
				return value
			}
			return defaultValue
		}
		if next, ok := current[k].(map[string]interface{}); ok {
			current = next
		} else {
			return defaultValue
		}
	}
	return defaultValue
}

func (p *PlacesPlugin) getStringFromMap(m map[string]interface{}, key string, defaultValue string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return defaultValue
}

func (p *PlacesPlugin) getFloatFromMap(m map[string]interface{}, key string, defaultValue float64) float64 {
	if value, ok := m[key].(float64); ok {
		return value
	}
	return defaultValue
}
