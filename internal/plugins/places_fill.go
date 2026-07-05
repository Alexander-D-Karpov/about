package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *PlacesPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	s := cfg.Settings

	p.mutex.RLock()
	places := p.places
	p.mutex.RUnlock()

	if len(places) == 0 {
		if raw, ok := s["places"].([]interface{}); ok {
			for _, item := range raw {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				place := Place{
					Name:        p.getStringFromMap(m, "name", ""),
					Lat:         p.getFloatFromMap(m, "lat", 0),
					Lng:         p.getFloatFromMap(m, "lng", 0),
					Country:     p.getStringFromMap(m, "country", ""),
					City:        p.getStringFromMap(m, "city", ""),
					Description: p.getStringFromMap(m, "description", ""),
					VisitedDate: p.getStringFromMap(m, "visited_date", ""),
					Category:    p.getStringFromMap(m, "category", "travel"),
				}
				if place.Lat != 0 && place.Lng != 0 {
					places = append(places, place)
				}
			}
		}
	}

	placesJSON, err := json.Marshal(places)
	if err != nil {
		placesJSON = []byte("[]")
	}
	configJSON, err := json.Marshal(map[string]interface{}{
		"defaultLat":    p.getConfigFloat(s, "ui.defaultLat", 25.0),
		"defaultLng":    p.getConfigFloat(s, "ui.defaultLng", 0.0),
		"defaultZoom":   p.getConfigInt(s, "ui.defaultZoom", 2),
		"heatmapRadius": p.getConfigInt(s, "ui.heatmapRadius", 25),
		"markerRadius":  p.getConfigInt(s, "ui.markerRadius", 8),
	})
	if err != nil {
		configJSON = []byte("{}")
	}

	stats := p.calculateStats(places)
	vm.Travel.PlacesJSON = template.JS(placesJSON)
	vm.Travel.PlacesConfig = template.JS(configJSON)
	vm.Travel.PlacesCount = fmt.Sprintf("%d places · %d countries", stats.TotalPlaces, stats.Countries)
	return nil
}
