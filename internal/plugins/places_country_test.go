package plugins

import (
	"os"
	"testing"
)

func TestCountryAt(t *testing.T) {
	geo, err := os.ReadFile("../../static/libs/geo/countries.geo.json")
	if err != nil {
		t.Skip("no geojson:", err)
	}
	CountryShapesJSON = geo
	for _, c := range []struct {
		lat, lng float64
		want     string
	}{
		{25.229634, 55.289585, "United Arab Emirates"},
		{27.864442, 34.295447, "Egypt"},
		{36.800207, 10.185776, "Tunisia"},
		{35.67686, 139.763895, "Japan"},
		{35.776768, 140.318338, "Japan"},
		{23.128843, 113.258958, "China"},
		{41.693459, 44.801449, "Georgia"},
		{59.960674, 30.158655, "Russia"},
		{51.507446, -0.127765, "United Kingdom"},
		{43.585482, 39.723109, "Russia"},
	} {
		if got := countryAt(c.lat, c.lng); got != c.want {
			t.Errorf("countryAt(%v, %v) = %q, want %q", c.lat, c.lng, got, c.want)
		}
	}
}
