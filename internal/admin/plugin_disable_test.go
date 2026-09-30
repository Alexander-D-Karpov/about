package admin

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

// Toggling a plugin off in the admin posts to updatePluginAPI, and the browser
// re-renders the plugin's form from the settings in the response. For health
// those settings must not carry the runtime blobs the plugin persists
// (current_data, daily_averages, sleep_records, ...) — on a live install those
// maps hold one entry per day/sleep accumulated over months, and rendering them
// as editable fields freezes the admin tab. The persisted config must still
// keep them, though, so a save from the admin doesn't wipe the plugin's data.
func TestDisableHealthReturnsFilteredSettingsButPersistsHidden(t *testing.T) {
	dir := t.TempDir()
	store := storage.New(dir)
	t.Cleanup(store.Wait)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	// Seed health as the running plugin would: visible config plus the hidden
	// runtime blobs, with several accumulated entries.
	sleep := map[string]interface{}{}
	avgs := map[string]interface{}{}
	for _, d := range []string{"2026-09-01", "2026-09-02", "2026-09-03"} {
		sleep[d] = map[string]interface{}{"start_time": d + "T23:00:00Z", "end_time": d + "T07:00:00Z"}
		avgs[d] = map[string]interface{}{"steps": 1000, "calories": 50}
	}
	seed := map[string]interface{}{
		"api":            map[string]interface{}{"baseUrl": "https://api.example", "username": "", "password": ""},
		"ui":             map[string]interface{}{"sectionTitle": "me irl", "showSteps": true},
		"current_data":   map[string]interface{}{"steps_today": 42, "current_heart_rate": 81},
		"daily_averages": avgs,
		"sleep_records":  sleep,
		"last_workout":   map[string]interface{}{"type": "Exercise", "duration": 13},
		"last_persist":   "2026-09-03T01:04:07Z",
	}
	if err := store.SetPluginConfig("health", &storage.PluginConfig{Enabled: true, Order: 15, Settings: seed}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg := &config.Config{AdminUser: "admin", AdminPass: "password"}
	mgr := plugins.NewManager(store, stream.New(), cfg, time.Now())
	h := &Handler{storage: store, pluginManager: mgr, config: cfg}

	// The browser sends only the visible fields (its form was populated from the
	// filtered GET), plus enabled=false.
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("plugin", "health")
	_ = mw.WriteField("enabled", "false")
	_ = mw.WriteField("order", "15")
	visible, _ := json.Marshal(map[string]interface{}{
		"api": seed["api"], "ui": seed["ui"],
	})
	_ = mw.WriteField("settings", string(visible))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/api/plugin", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	h.updatePluginAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success  bool                   `json:"success"`
		Enabled  bool                   `json:"enabled"`
		Settings map[string]interface{} `json:"settings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success || resp.Enabled {
		t.Fatalf("expected success with enabled=false, got success=%v enabled=%v", resp.Success, resp.Enabled)
	}

	// The response the form re-renders from must be clean.
	hidden := []string{"current_data", "daily_averages", "sleep_records", "last_workout", "last_persist"}
	for _, k := range hidden {
		if _, present := resp.Settings[k]; present {
			t.Errorf("response settings still contain hidden runtime key %q; the admin form would render it and freeze", k)
		}
	}
	for _, k := range []string{"api", "ui"} {
		if _, present := resp.Settings[k]; !present {
			t.Errorf("response dropped visible key %q", k)
		}
	}

	// But the persisted config must keep the runtime blobs — a disable must not
	// wipe the plugin's accumulated data.
	persisted := store.GetPluginConfig("health")
	if persisted.Enabled {
		t.Fatal("health should be disabled in storage")
	}
	for _, k := range hidden {
		if _, present := persisted.Settings[k]; !present {
			t.Errorf("persisted config lost hidden runtime key %q on disable", k)
		}
	}
}
