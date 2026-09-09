package plugins

import "testing"

func TestHumanizeFloat(t *testing.T) {
	cases := []struct {
		name     string
		in       float64
		decimals int
		want     string
	}{
		{"under a thousand keeps one decimal", 82.04, 1, "82.0"},
		{"exactly three digits is untouched", 999, 0, "999"},
		{"four digits gets one separator", 1884.94, 1, "1,884.9"},
		{"five digits", 23332, 0, "23,332"},
		{"boundary at 1000", 1000, 0, "1,000"},
		{"seven digits gets two separators", 1234567, 0, "1,234,567"},
		{"zero", 0, 1, "0.0"},
		{"negative keeps sign outside separators", -1884.9, 1, "-1,884.9"},
		{"rounds up across the separator", 999.96, 1, "1,000.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := humanizeFloat(c.in, c.decimals); got != c.want {
				t.Errorf("humanizeFloat(%v, %d) = %q, want %q", c.in, c.decimals, got, c.want)
			}
		})
	}
}

func TestFormatBikeDurationHTML(t *testing.T) {
	cases := []struct {
		name string
		min  float64
		want string
	}{
		{"minutes only", 43, `43<span class="bike-stat-unit">m</span>`},
		{"whole hours drop the minutes", 120, `2<span class="bike-stat-unit">h</span>`},
		{"hours and minutes", 7543, `125<span class="bike-stat-unit">h</span> 43<span class="bike-stat-unit">m</span>`},
		{"hours are separated too", 60000, `1,000<span class="bike-stat-unit">h</span>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(formatBikeDurationHTML(c.min)); got != c.want {
				t.Errorf("formatBikeDurationHTML(%v) = %q, want %q", c.min, got, c.want)
			}
		})
	}
}
