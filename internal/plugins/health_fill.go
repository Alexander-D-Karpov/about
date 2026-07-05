package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *HealthPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	s := cfg.Settings
	if p.username == "" {
		return nil
	}

	showSteps := p.getConfigBool(s, "ui.showSteps", true)
	showCalories := p.getConfigBool(s, "ui.showCalories", true)
	showWorkouts := p.getConfigBool(s, "ui.showWorkouts", true)
	showSleep := p.getConfigBool(s, "ui.showSleep", true)
	showHeartRate := p.getConfigBool(s, "ui.showHeartRate", true)
	showHydration := p.getConfigBool(s, "ui.showHydration", true)

	p.mutex.RLock()
	d := *p.healthData
	steps7 := p.last7Locked(func(a *DailyAverage) float64 { return float64(a.Steps) })
	cal7 := p.last7Locked(func(a *DailyAverage) float64 { return a.Calories })
	sleep7 := p.last7Locked(func(a *DailyAverage) float64 { return a.SleepHours })
	work7 := p.last7Locked(func(a *DailyAverage) float64 { return float64(a.WorkoutMinutes) })
	hyd7 := p.last7Locked(func(a *DailyAverage) float64 { return a.HydrationML })
	p.mutex.RUnlock()

	var cards []view.HealthCardVM
	if showSteps {
		cards = append(cards, view.HealthCardVM{
			Label: "Steps", Value: formatNumberCommas(d.StepsToday),
			Sub:   fmt.Sprintf("%s this week", formatNumberCommas(d.StepsWeek)),
			Color: "#4d9fff", Icon: "steps", Kind: "bars", Data: steps7,
		})
	}
	if showCalories {
		cards = append(cards, view.HealthCardVM{
			Label: "Calories", Value: fmt.Sprintf("%.0f", d.CaloriesToday), Unit: "kcal",
			Sub:   fmt.Sprintf("%.0f this week", d.CaloriesWeek),
			Color: "#f0a040", Icon: "fire", Kind: "bars", Data: cal7,
		})
	}
	if showHeartRate {
		cards = append(cards, view.HealthCardVM{
			Label: "Heart Rate", Value: fmt.Sprintf("%d", d.CurrentHeartRate), Unit: "bpm",
			Sub:   fmt.Sprintf("resting %d", d.RestingHeartRate),
			Color: "#e8557a", Icon: "heart", Kind: "ecg", Data: []float64{1},
		})
	}
	if showSleep {
		cards = append(cards, view.HealthCardVM{
			Label: "Sleep", Value: fmt.Sprintf("%.1f", d.SleepHoursLastNight), Unit: "h",
			Sub:   fmt.Sprintf("%.1fh avg", d.SleepAvgWeek),
			Color: "#b055ff", Icon: "moon", Kind: "tiles", Data: sleep7,
		})
	}
	if showWorkouts {
		cards = append(cards, view.HealthCardVM{
			Label: "Workout", Value: fmt.Sprintf("%d", d.WorkoutMinutesToday), Unit: "min",
			Sub:   fmt.Sprintf("%d min this week", d.WorkoutMinutesWeek),
			Color: "#10d060", Icon: "dumbbell", Kind: "bars", Data: work7,
		})
	}
	if showHydration {
		cards = append(cards, view.HealthCardVM{
			Label: "Hydration", Value: fmt.Sprintf("%.0f", d.HydrationToday), Unit: "ml",
			Sub: "today", Color: "#2fd4c4", Icon: "water", Kind: "bars", Data: hyd7,
		})
	}

	vm.Health = cards
	return nil
}

func (p *HealthPlugin) last7Locked(extract func(*DailyAverage) float64) []float64 {
	now := time.Now()
	out := make([]float64, 0, 7)
	for i := 6; i >= 0; i-- {
		key := now.AddDate(0, 0, -i).Format("2006-01-02")
		if avg, ok := p.dailyAverages[key]; ok {
			out = append(out, extract(avg))
		} else {
			out = append(out, 0)
		}
	}
	return out
}
