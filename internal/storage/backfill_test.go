package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeConfig lays down a config.json holding just the social links given, in
// the {name,url,icon} shape the plugin reads.
func writeConfig(t *testing.T, dir string, links []map[string]interface{}) {
	t.Helper()

	entries := make([]interface{}, 0, len(links))
	for _, l := range links {
		entries = append(entries, l)
	}
	cfg := map[string]interface{}{
		"plugins": map[string]interface{}{
			"social": map[string]interface{}{
				"enabled":  true,
				"order":    2,
				"settings": map[string]interface{}{"links": entries},
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func loadLinkNames(t *testing.T, dir string) []string {
	t.Helper()

	st := New(dir)
	t.Cleanup(st.Wait)
	if err := st.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	links, ok := st.GetPluginConfig("social").Settings["links"].([]interface{})
	if !ok {
		t.Fatalf("social links missing or not a list")
	}
	names := make([]string, 0, len(links))
	for _, l := range links {
		names = append(names, linkField(l, "name"))
	}
	return names
}

func indexOfName(names []string, want string) int {
	for i, n := range names {
		if n == want {
			return i
		}
	}
	return -1
}

// A config written before a link joined the defaults gets it added, next to the
// defaults that surround it rather than tacked onto the end.
func TestBackfillAddsNewDefaultInPosition(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, []map[string]interface{}{
		{"name": "Telegram", "url": "https://t.me/sanspie", "icon": "telegram"},
		{"name": "GitHub", "url": "https://github.com/Alexander-D-Karpov", "icon": "github"},
		{"name": "VK", "url": "https://vk.com/al.karpov", "icon": "vk"},
		{"name": "Steam", "url": "https://steamcommunity.com/id/sanspie", "icon": "steam"},
	})

	names := loadLinkNames(t, dir)

	x := indexOfName(names, "X")
	if x < 0 {
		t.Fatalf("X was not backfilled, got %v", names)
	}
	if vk := indexOfName(names, "VK"); x != vk+1 {
		t.Errorf("X should follow VK as it does in the defaults, got %v", names)
	}
	// Links the config never had are backfilled too - the whole point is that
	// nothing in the defaults is special-cased.
	for _, want := range []string{"LinkedIn", "Email", "Discord"} {
		if indexOfName(names, want) < 0 {
			t.Errorf("%s was not backfilled, got %v", want, names)
		}
	}
}

// Backfilling twice must not duplicate anything.
func TestBackfillIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, []map[string]interface{}{
		{"name": "GitHub", "url": "https://github.com/Alexander-D-Karpov", "icon": "github"},
	})

	first := loadLinkNames(t, dir)
	second := loadLinkNames(t, dir)

	if len(first) != len(second) {
		t.Fatalf("second load changed the list: %v then %v", first, second)
	}
	seen := map[string]bool{}
	for _, n := range second {
		if seen[n] {
			t.Errorf("duplicate link %q in %v", n, second)
		}
		seen[n] = true
	}
}

// Once a default has been offered it is recorded, so deleting the link through
// the admin UI sticks instead of being undone on the next boot.
func TestBackfillDoesNotResurrectRemovedLink(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, []map[string]interface{}{
		{"name": "GitHub", "url": "https://github.com/Alexander-D-Karpov", "icon": "github"},
	})

	if names := loadLinkNames(t, dir); indexOfName(names, "X") < 0 {
		t.Fatalf("X was not backfilled on the first load, got %v", names)
	}

	st := New(dir)
	t.Cleanup(st.Wait)
	if err := st.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	config := st.GetPluginConfig("social")
	links := config.Settings["links"].([]interface{})
	kept := make([]interface{}, 0, len(links))
	for _, l := range links {
		if linkField(l, "name") != "X" {
			kept = append(kept, l)
		}
	}
	config.Settings["links"] = kept
	if err := st.SetPluginConfig("social", config); err != nil {
		t.Fatalf("save: %v", err)
	}
	st.Wait()

	if names := loadLinkNames(t, dir); indexOfName(names, "X") >= 0 {
		t.Errorf("removed link came back on reload: %v", names)
	}
}
