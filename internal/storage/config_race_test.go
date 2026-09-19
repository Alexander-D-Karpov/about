package storage

import (
	"sync"
	"testing"
)

// GetPluginConfig used to hand callers the live settings map out of s.data, so
// two plugins persisting a setting at the same time wrote to the same map and
// the process died with "fatal error: concurrent map writes" (seen in
// MusicPlugin.saveStats). The config a caller receives must be its own.
func TestPluginConfigSettingsAreNotSharedAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, []map[string]interface{}{{"name": "tg", "url": "u", "icon": "i"}})

	st := New(dir)
	t.Cleanup(st.Wait)
	if err := st.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	a := st.GetPluginConfig("social")
	b := st.GetPluginConfig("social")
	a.Settings["scratch"] = "written by a"
	if _, leaked := b.Settings["scratch"]; leaked {
		t.Fatal("two GetPluginConfig calls share one settings map")
	}

	// And a stored config must not stay writable through the caller's map.
	c := st.GetPluginConfig("social")
	c.Settings["kept"] = 1
	if err := st.SetPluginConfig("social", c); err != nil {
		t.Fatalf("set: %v", err)
	}
	c.Settings["after"] = 2
	if _, leaked := st.GetPluginConfig("social").Settings["after"]; leaked {
		t.Fatal("caller can still mutate settings after SetPluginConfig stored them")
	}
	if _, ok := st.GetPluginConfig("social").Settings["kept"]; !ok {
		t.Fatal("SetPluginConfig dropped a setting written before the call")
	}
}

// The read-modify-write cycle every plugin uses to persist a setting, run
// concurrently. Under -race this fails on the old aliasing behaviour.
func TestConcurrentPluginConfigUpdates(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, []map[string]interface{}{{"name": "tg", "url": "u", "icon": "i"}})

	st := New(dir)
	t.Cleanup(st.Wait)
	if err := st.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for n := 0; n < 40; n++ {
				cfg := st.GetPluginConfig("social")
				cfg.Settings["cache"] = map[string]interface{}{"worker": i, "n": n}
				if err := st.SetPluginConfig("social", cfg); err != nil {
					t.Errorf("set: %v", err)
					return
				}
			}
		}(i)
	}
	// Readers racing the writers, the way a page render does.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 40; n++ {
				for range st.GetPluginConfig("social").Settings {
				}
			}
		}()
	}
	wg.Wait()
}
