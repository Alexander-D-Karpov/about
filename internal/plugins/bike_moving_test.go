package plugins

import (
	"math"
	"testing"
)

// approxKm builds coordinates advancing east along a fixed latitude so that
// each 0.001° step is a known distance, with per-point time offsets (seconds).
func TestMovingTimeMinutes(t *testing.T) {
	t.Run("no timestamps falls back", func(t *testing.T) {
		coords := [][]float64{{59.9, 30.3, 10}, {59.9, 30.31, 12}}
		if _, ok := movingTimeMinutes(coords); ok {
			t.Fatalf("expected ok=false when coords lack time offsets")
		}
	})

	t.Run("excludes stopped time", func(t *testing.T) {
		lat := 59.9
		lon := 30.3
		// Segment A: move ~78m in 10s (~28 km/h) -> counts.
		p0 := []float64{lat, lon, 10, 0}
		p1 := []float64{lat, lon + 0.001, 10, 10}
		// Segment B: stopped for 300s at the same spot -> excluded.
		p2 := []float64{lat, lon + 0.001, 10, 310}
		// Segment C: move again ~78m in 10s -> counts.
		p3 := []float64{lat, lon + 0.002, 10, 320}
		coords := [][]float64{p0, p1, p2, p3}

		moving, ok := movingTimeMinutes(coords)
		if !ok {
			t.Fatalf("expected ok=true")
		}
		// Only the two 10s moving segments should count = 20s.
		if got := moving * 60; math.Abs(got-20) > 0.5 {
			t.Fatalf("moving time = %.2fs, want ~20s (stop excluded)", got)
		}

		// Average over moving time must exceed average over elapsed time.
		dist := haversineKm(lat, lon, lat, lon+0.002)
		elapsedMin := 320.0 / 60
		avgMoving := dist / (moving / 60)
		avgElapsed := dist / elapsedMin
		if !(avgMoving > avgElapsed) {
			t.Fatalf("moving avg %.2f should exceed elapsed avg %.2f", avgMoving, avgElapsed)
		}
	})
}
