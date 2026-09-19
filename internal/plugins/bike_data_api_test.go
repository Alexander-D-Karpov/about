package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/Alexander-D-Karpov/about/internal/storage"
)

func bikeTestStorage(t *testing.T) *storage.Storage {
	t.Helper()
	st := storage.New(t.TempDir())
	// Backups run in a goroutine; wait for them before TempDir is removed.
	t.Cleanup(st.Wait)
	cfg := st.GetPluginConfig("bike")
	cfg.Settings["rides"] = []interface{}{
		map[string]interface{}{
			"name": "older ride", "date": "2026-08-01",
			"distance_km": 10.0, "elevation_gain_m": 100.0, "duration_minutes": 40.0,
			"coordinates": []interface{}{
				[]interface{}{59.900000, 30.300000, 10.0},
				[]interface{}{59.910000, 30.319999, 12.0},
			},
		},
		map[string]interface{}{
			"name": "newer ride", "date": "2026-08-29",
			"distance_km": 20.0, "elevation_gain_m": 200.0, "duration_minutes": 80.0,
			"coordinates": []interface{}{
				[]interface{}{60.000000, 30.400000, 10.0},
				[]interface{}{60.010000, 30.409999, 12.0},
			},
		},
	}
	if err := st.SetPluginConfig("bike", cfg); err != nil {
		t.Fatal(err)
	}
	return st
}

var bikeDataSrcRe = regexp.MustCompile(`data-src="(/api/bike-data\.json\?v=([0-9a-f]+))"`)

// The GPS tracks are ~470 KB - 53% of the whole page - and they are re-sent on
// every visit while inlined. The markup must only point at the endpoint.
func TestBikeRenderDoesNotInlineTracks(t *testing.T) {
	p := NewBikePlugin(bikeTestStorage(t), nil)
	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "30.319999") {
		t.Error("ride coordinates are still inlined in the page")
	}
	if !bikeDataSrcRe.MatchString(out) {
		t.Errorf("markup does not reference the versioned data endpoint:\n%s", out)
	}
}

// Whatever the endpoint returns must be exactly what the map used to get
// inline: trimmed and sorted newest-first.
func TestBikeDataAPIServesSortedRides(t *testing.T) {
	p := NewBikePlugin(bikeTestStorage(t), nil)
	rec := httptest.NewRecorder()
	p.HandleDataAPI(rec, httptest.NewRequest(http.MethodGet, "/api/bike-data.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var rides []BikeRide
	if err := json.Unmarshal(rec.Body.Bytes(), &rides); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if len(rides) != 2 {
		t.Fatalf("got %d rides, want 2", len(rides))
	}
	if rides[0].Name != "newer ride" {
		t.Errorf("rides[0] = %q, want newest first", rides[0].Name)
	}
	if len(rides[0].Coordinates) == 0 {
		t.Error("coordinates missing from the payload")
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("no ETag, so the browser cannot revalidate")
	}
}

// The version in the URL is a content hash, so that exact URL can never change
// meaning - it is safe to cache forever. This is what makes a repeat visit cost
// zero bytes.
func TestBikeDataAPICachesImmutablyWhenVersionMatches(t *testing.T) {
	st := bikeTestStorage(t)
	p := NewBikePlugin(st, nil)

	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := bikeDataSrcRe.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no data-src in markup:\n%s", out)
	}
	url, version := m[1], m[2]

	rec := httptest.NewRecorder()
	p.HandleDataAPI(rec, httptest.NewRequest(http.MethodGet, url, nil))
	cc := rec.Header().Get("Cache-Control")
	if !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, want immutable for a version-matched URL", cc)
	}

	// Same ETag back -> 304 with no body.
	etag := rec.Header().Get("ETag")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	p.HandleDataAPI(rec2, req)
	if rec2.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304 for matching If-None-Match", rec2.Code)
	}
	if rec2.Body.Len() != 0 {
		t.Errorf("304 must have an empty body, got %d bytes", rec2.Body.Len())
	}
	_ = version
}

// A stale or absent ?v must NOT be cached forever: the body served is current
// data, which would otherwise be pinned under a URL promising something else.
func TestBikeDataAPIDoesNotPinStaleVersion(t *testing.T) {
	p := NewBikePlugin(bikeTestStorage(t), nil)
	for _, url := range []string{"/api/bike-data.json", "/api/bike-data.json?v=deadbeef"} {
		rec := httptest.NewRecorder()
		p.HandleDataAPI(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
			t.Errorf("%s: Cache-Control = %q, must not be immutable", url, cc)
		}
	}
}

// Changing the rides must change the version, or browsers would keep serving
// the old tracks from cache forever.
func TestBikeDataVersionChangesWithContent(t *testing.T) {
	st := bikeTestStorage(t)
	p := NewBikePlugin(st, nil)

	first, err := p.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v1 := bikeDataSrcRe.FindStringSubmatch(first)[2]

	cfg := st.GetPluginConfig("bike")
	rides := cfg.Settings["rides"].([]interface{})
	rides[0].(map[string]interface{})["distance_km"] = 99.0
	cfg.Settings["rides"] = rides
	if err := st.SetPluginConfig("bike", cfg); err != nil {
		t.Fatal(err)
	}

	second, err := p.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v2 := bikeDataSrcRe.FindStringSubmatch(second)[2]

	if v1 == v2 {
		t.Errorf("version did not change after editing a ride (%s)", v1)
	}
}
