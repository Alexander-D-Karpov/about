package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *BikePlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	settings := cfg.Settings
	rides := p.loadRides(settings)
	if len(rides) == 0 {
		return nil
	}

	totalKm := p.getFloat(settings, "total_km_override", 0)
	if totalKm <= 0 {
		for _, r := range rides {
			totalKm += r.DistanceKm
		}
	}

	var totalElev, totalDur, longest float64
	for _, r := range rides {
		totalElev += r.ElevationGainM
		totalDur += r.DurationMin
		if r.DistanceKm > longest {
			longest = r.DistanceKm
		}
	}
	avg := totalKm / float64(len(rides))

	display := make([]BikeRide, len(rides))
	copy(display, rides)
	for i := range display {
		if display[i].HideFirstKm > 0 && len(display[i].Coordinates) > 1 {
			display[i].Coordinates = trimFirstKm(display[i].Coordinates, display[i].HideFirstKm)
		}
		if display[i].HideLastKm > 0 && len(display[i].Coordinates) > 1 {
			display[i].Coordinates = trimLastKm(display[i].Coordinates, display[i].HideLastKm)
		}
	}
	sort.Slice(display, func(i, j int) bool { return display[i].Date > display[j].Date })

	colors := []string{"#f0a040", "#f0c040", "#e8557a", "#b055ff", "#4d9fff", "#10d060"}
	rideVMs := make([]view.RideVM, 0, len(display))
	for i, r := range display {
		speed := 0.0
		if r.DurationMin > 0 {
			speed = r.DistanceKm / (r.DurationMin / 60.0)
		}
		rideVMs = append(rideVMs, view.RideVM{
			Name:  r.Name,
			Date:  r.Date,
			Km:    fmt.Sprintf("%.1f km", r.DistanceKm),
			Elev:  fmt.Sprintf("↑%.0fm", r.ElevationGainM),
			Time:  formatBikeDuration(r.DurationMin),
			Speed: fmt.Sprintf("%.1f км/ч", speed),
			Color: colors[i%len(colors)],
			Prof:  bikeProfile(r.Coordinates, 40),
		})
	}

	ridesJSON, _ := json.Marshal(display)

	vm.Travel.Totals = view.BikeTotalsVM{
		Distance:  fmt.Sprintf("%.1f km", totalKm),
		Rides:     len(rides),
		Elevation: fmt.Sprintf("%sm", formatNumberWithCommas(int64(totalElev))),
		Time:      formatBikeDuration(totalDur),
		Avg:       fmt.Sprintf("%.1f km", avg),
		Longest:   fmt.Sprintf("%.1f km", longest),
	}
	vm.Travel.Rides = rideVMs
	vm.Travel.BikeJSON = template.JS(ridesJSON)
	return nil
}

func bikeProfile(coords [][]float64, n int) []float64 {
	elev := make([]float64, 0, len(coords))
	for _, c := range coords {
		if len(c) >= 3 && c[2] != 0 {
			elev = append(elev, c[2])
		}
	}
	if len(elev) < 2 {
		return nil
	}
	if len(elev) <= n {
		return elev
	}
	out := make([]float64, n)
	step := float64(len(elev)-1) / float64(n-1)
	for i := 0; i < n; i++ {
		out[i] = elev[int(float64(i)*step+0.5)]
	}
	return out
}
