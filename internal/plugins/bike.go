package plugins

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type BikePlugin struct {
	storage *storage.Storage
	hub     *stream.Hub
	mutex   sync.RWMutex
}

type BikeRide struct {
	Name           string      `json:"name"`
	Date           string      `json:"date"`
	DistanceKm     float64     `json:"distance_km"`
	ElevationGainM float64     `json:"elevation_gain_m"`
	DurationMin    float64     `json:"duration_minutes"`
	Coordinates    [][]float64 `json:"coordinates"`
	HideLastKm     float64     `json:"hide_last_km"`
	HideFirstKm    float64     `json:"hide_first_km"`
}

type GPXFile struct {
	XMLName xml.Name   `xml:"gpx"`
	Tracks  []GPXTrack `xml:"trk"`
}

type GPXTrack struct {
	Name     string       `xml:"name"`
	Segments []GPXSegment `xml:"trkseg"`
}

type GPXSegment struct {
	Points []GPXPoint `xml:"trkpt"`
}

type GPXPoint struct {
	Lat  float64 `xml:"lat,attr"`
	Lon  float64 `xml:"lon,attr"`
	Ele  float64 `xml:"ele"`
	Time string  `xml:"time"`
}

func NewBikePlugin(st *storage.Storage, hub *stream.Hub) *BikePlugin {
	return &BikePlugin{storage: st, hub: hub}
}

func (p *BikePlugin) Name() string { return "bike" }

func (p *BikePlugin) Render(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	cfg := p.storage.GetPluginConfig(p.Name())
	settings := cfg.Settings
	sectionTitle := p.getStr(settings, "ui.sectionTitle", "Bike Rides")
	rides := p.loadRides(settings)

	if len(rides) == 0 {
		return "", nil
	}

	totalKm := p.getFloat(settings, "total_km_override", 0)
	if totalKm <= 0 {
		for _, r := range rides {
			totalKm += r.DistanceKm
		}
	}

	var totalElev, totalDur float64
	for _, r := range rides {
		totalElev += r.ElevationGainM
		totalDur += r.DurationMin
	}
	avgDist := totalKm / float64(len(rides))

	displayRides := make([]BikeRide, len(rides))
	copy(displayRides, rides)
	for i := range displayRides {
		if displayRides[i].HideFirstKm > 0 && len(displayRides[i].Coordinates) > 1 {
			displayRides[i].Coordinates = trimFirstKm(displayRides[i].Coordinates, displayRides[i].HideFirstKm)
		}
		if displayRides[i].HideLastKm > 0 && len(displayRides[i].Coordinates) > 1 {
			displayRides[i].Coordinates = trimLastKm(displayRides[i].Coordinates, displayRides[i].HideLastKm)
		}
	}

	sort.Slice(displayRides, func(i, j int) bool {
		return displayRides[i].Date > displayRides[j].Date
	})

	ridesJSON, _ := json.Marshal(displayRides)

	const tmpl = `
<section class="bike-section section plugin" id="bike-plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
	</header>
	<div class="plugin__inner">
		<div class="bike-stats">
			<div class="bike-stat"><div class="bike-stat-value">{{.TotalKm}}</div><div class="bike-stat-label">Total KM</div></div>
			<div class="bike-stat"><div class="bike-stat-value">{{.TotalRides}}</div><div class="bike-stat-label">Rides</div></div>
			<div class="bike-stat"><div class="bike-stat-value">{{.TotalElevation}}</div><div class="bike-stat-label">Elevation ↑</div></div>
			<div class="bike-stat"><div class="bike-stat-value">{{.AvgDistance}}</div><div class="bike-stat-label">Avg KM</div></div>
			<div class="bike-stat"><div class="bike-stat-value">{{.TotalTime}}</div><div class="bike-stat-label">Total Time</div></div>
		</div>

		<div class="bike-map-container" id="bike-map-container">
			<div class="bike-map" id="bike-map"></div>
			<div class="map-controls">
				<button class="map-control-btn" id="bike-fit-bounds" title="Fit all rides">
					<svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor">
						<path d="M15 3l2.3 2.3-2.89 2.87 1.42 1.42L18.7 6.7 21 9V3h-6zM3 9l2.3-2.3 2.87 2.89 1.42-1.42L6.7 5.3 9 3H3v6zm6 12l-2.3-2.3 2.89-2.87-1.42-1.42L5.3 17.3 3 15v6h6zm12-6l-2.3 2.3-2.87-2.89-1.42 1.42 2.89 2.87L15 21h6v-6z"/>
					</svg>
				</button>
			</div>
			<div class="map-loading" id="bike-map-loading">
				<div class="loading"></div>
				<span>Loading map...</span>
			</div>
		</div>

		<div class="bike-rides-list" id="bike-rides-list">
			{{range $i, $r := .Rides}}
			<div class="bike-ride-item" data-ride="{{$i}}">
				<div class="bike-ride-color" data-ride-color="{{$i}}"></div>
				<div class="bike-ride-info">
					<div class="bike-ride-name">{{$r.Name}}</div>
					<div class="bike-ride-date">{{$r.Date}}</div>
				</div>
				<div class="bike-ride-chips">
					<span class="bike-chip">{{printf "%.1f" $r.DistanceKm}} km</span>
					<span class="bike-chip">↑{{printf "%.0f" $r.ElevationGainM}}m</span>
					<span class="bike-chip">{{fmtDur $r.DurationMin}}</span>
				</div>
			</div>
			{{end}}
		</div>
	</div>
	<script id="bike-data" type="application/json">{{.RidesJSON}}</script>
</section>`

	funcMap := template.FuncMap{
		"printf": fmt.Sprintf,
		"fmtDur": func(min float64) string {
			if min < 60 {
				return fmt.Sprintf("%.0fm", min)
			}
			h, m := int(min)/60, int(min)%60
			if m == 0 {
				return fmt.Sprintf("%dh", h)
			}
			return fmt.Sprintf("%dh %dm", h, m)
		},
	}

	t, err := template.New("bike").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, struct {
		SectionTitle, TotalKm, TotalElevation, AvgDistance, TotalTime string
		TotalRides                                                    int
		Rides                                                         []BikeRide
		RidesJSON                                                     template.JS
	}{
		SectionTitle:   sectionTitle,
		TotalKm:        fmt.Sprintf("%.1f", totalKm),
		TotalRides:     len(rides),
		TotalElevation: fmt.Sprintf("%.0fm", totalElev),
		AvgDistance:    fmt.Sprintf("%.1f", avgDist),
		TotalTime:      formatBikeDuration(totalDur),
		Rides:          displayRides,
		RidesJSON:      template.JS(ridesJSON),
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (p *BikePlugin) UpdateData(ctx context.Context) error { return nil }

func (p *BikePlugin) GetSettings() map[string]interface{} {
	return p.storage.GetPluginConfig(p.Name()).Settings
}

func (p *BikePlugin) SetSettings(settings map[string]interface{}) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	cfg.Settings = settings
	if err := p.storage.SetPluginConfig(p.Name(), cfg); err != nil {
		return err
	}
	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(), "action": "settings_changed",
	})
	return nil
}

func (p *BikePlugin) RenderText(ctx context.Context) (string, error) {
	cfg := p.storage.GetPluginConfig(p.Name())
	rides := p.loadRides(cfg.Settings)
	if len(rides) == 0 {
		return "Bike: No rides", nil
	}
	total := 0.0
	for _, r := range rides {
		total += r.DistanceKm
	}
	return fmt.Sprintf("Bike: %d rides, %.1f km", len(rides), total), nil
}

func (p *BikePlugin) GetMetrics() map[string]interface{} {
	cfg := p.storage.GetPluginConfig(p.Name())
	rides := p.loadRides(cfg.Settings)
	total := 0.0
	for _, r := range rides {
		total += r.DistanceKm
	}
	return map[string]interface{}{"total_rides": len(rides), "total_km": total}
}

func (p *BikePlugin) loadRides(settings map[string]interface{}) []BikeRide {
	raw, ok := settings["rides"].([]interface{})
	if !ok {
		return nil
	}
	var rides []BikeRide
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		ride := BikeRide{
			Name:           strFromMap(m, "name"),
			Date:           strFromMap(m, "date"),
			DistanceKm:     floatFromMap(m, "distance_km"),
			ElevationGainM: floatFromMap(m, "elevation_gain_m"),
			DurationMin:    floatFromMap(m, "duration_minutes"),
			HideLastKm:     floatFromMap(m, "hide_last_km"),
			HideFirstKm:    floatFromMap(m, "hide_first_km"),
		}
		if coords, ok := m["coordinates"].([]interface{}); ok {
			for _, c := range coords {
				if pair, ok := c.([]interface{}); ok && len(pair) >= 2 {
					lat, _ := toFloat64(pair[0])
					lng, _ := toFloat64(pair[1])
					coord := []float64{lat, lng}
					if len(pair) >= 3 {
						if ele, ok := toFloat64(pair[2]); ok {
							coord = append(coord, ele)
						}
					}
					ride.Coordinates = append(ride.Coordinates, coord)
				}
			}
		}
		rides = append(rides, ride)
	}
	return rides
}

func trimFirstKm(coords [][]float64, km float64) [][]float64 {
	if km <= 0 || len(coords) < 2 {
		return coords
	}
	cum := 0.0
	for i := 1; i < len(coords); i++ {
		cum += haversineKm(coords[i-1][0], coords[i-1][1], coords[i][0], coords[i][1])
		if cum >= km {
			return coords[i:]
		}
	}
	return coords[len(coords)-1:]
}

func (p *BikePlugin) getStr(settings map[string]interface{}, key, def string) string {
	parts := strings.Split(key, ".")
	cur := settings
	for i, k := range parts {
		if i == len(parts)-1 {
			if v, ok := cur[k].(string); ok {
				return v
			}
			return def
		}
		if next, ok := cur[k].(map[string]interface{}); ok {
			cur = next
		} else {
			return def
		}
	}
	return def
}

func (p *BikePlugin) getFloat(settings map[string]interface{}, key string, def float64) float64 {
	parts := strings.Split(key, ".")
	cur := settings
	for i, k := range parts {
		if i == len(parts)-1 {
			f, ok := toFloat64(cur[k])
			if !ok {
				return def
			}
			return f
		}
		if next, ok := cur[k].(map[string]interface{}); ok {
			cur = next
		} else {
			return def
		}
	}
	return def
}

func strFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func floatFromMap(m map[string]interface{}, key string) float64 {
	f, _ := toFloat64(m[key])
	return f
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func trimLastKm(coords [][]float64, km float64) [][]float64 {
	if km <= 0 || len(coords) < 2 {
		return coords
	}
	cum := 0.0
	for i := len(coords) - 1; i > 0; i-- {
		cum += haversineKm(coords[i][0], coords[i][1], coords[i-1][0], coords[i-1][1])
		if cum >= km {
			return coords[:i]
		}
	}
	return coords[:1]
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func formatBikeDuration(min float64) string {
	h, m := int(min)/60, int(min)%60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func ParseGPX(data []byte) (*BikeRide, error) {
	var gpx GPXFile
	if err := xml.Unmarshal(data, &gpx); err != nil {
		return nil, fmt.Errorf("invalid GPX: %w", err)
	}

	var points []GPXPoint
	name := ""
	for _, trk := range gpx.Tracks {
		if name == "" && trk.Name != "" {
			name = trk.Name
		}
		for _, seg := range trk.Segments {
			points = append(points, seg.Points...)
		}
	}
	if len(points) < 2 {
		return nil, fmt.Errorf("GPX has fewer than 2 points")
	}

	totalDist := 0.0
	elevGain := 0.0
	var coords [][]float64

	for i, pt := range points {
		coords = append(coords, []float64{pt.Lat, pt.Lon, pt.Ele})
		if i > 0 {
			totalDist += haversineKm(points[i-1].Lat, points[i-1].Lon, pt.Lat, pt.Lon)
			if diff := pt.Ele - points[i-1].Ele; diff > 0 {
				elevGain += diff
			}
		}
	}

	var durMin float64
	layouts := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05+00:00",
		"2006-01-02T15:04:05-07:00",
	}
	var tStart, tEnd time.Time
	var okStart, okEnd bool
	for _, l := range layouts {
		if !okStart {
			if t, err := time.Parse(l, points[0].Time); err == nil {
				tStart = t
				okStart = true
			}
		}
		if !okEnd {
			if t, err := time.Parse(l, points[len(points)-1].Time); err == nil {
				tEnd = t
				okEnd = true
			}
		}
	}
	if okStart && okEnd {
		durMin = tEnd.Sub(tStart).Minutes()
	}

	date := ""
	if okStart {
		date = tStart.Format("2006-01-02")
	}

	if len(coords) > 300 {
		coords = downsampleCoords(coords, 300)
	}

	if name == "" {
		name = "Ride"
	}

	return &BikeRide{
		Name:           name,
		Date:           date,
		DistanceKm:     math.Round(totalDist*100) / 100,
		ElevationGainM: math.Round(elevGain*10) / 10,
		DurationMin:    math.Round(durMin*10) / 10,
		Coordinates:    coords,
	}, nil
}

func downsampleCoords(coords [][]float64, max int) [][]float64 {
	if len(coords) <= max {
		return coords
	}
	out := make([][]float64, 0, max)
	step := float64(len(coords)-1) / float64(max-1)
	for i := 0; i < max-1; i++ {
		out = append(out, coords[int(math.Round(float64(i)*step))])
	}
	out = append(out, coords[len(coords)-1])
	return out
}
