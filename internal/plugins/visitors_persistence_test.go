package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newVisitorsForTest(dir string) *VisitorsPlugin {
	return &VisitorsPlugin{
		dataPath:     dir,
		dailyStats:   map[string]int64{},
		dailyUnique:  map[string]int64{},
		uniqueToday:  map[string]struct{}{},
		countryStats: map[string]int64{},
	}
}

func readVisitorsFile(t *testing.T, path string) VisitorsData {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var d VisitorsData
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("visitors file is not valid JSON (%v):\n%s", err, string(b))
	}
	return d
}

// Every save writes to a fixed temp name and renames it, so concurrent saves
// (day rollover, the periodic saver, and per-visit saves all fire their own
// goroutine) interleave into one temp file and race on the rename. On prod that
// produced a malformed visitors.json ("invalid character '}' after top-level
// value") and a stream of rename errors. A save must always leave a complete,
// valid file behind, whatever else is saving at the same time.
func TestVisitorsConcurrentSavesNeverCorrupt(t *testing.T) {
	dir := t.TempDir()
	p := newVisitorsForTest(dir)
	p.visitCount = 139863
	p.currentDay = "2026-09-28"
	p.todayCount = 42
	for i := 0; i < 200; i++ {
		day := "2026-09-" + string(rune('0'+i%10))
		p.dailyStats[day] = int64(i)
	}
	p.countryStats["RU"] = 90000
	p.countryStats["US"] = 40000

	var wg sync.WaitGroup
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.saveVisitorsDataAsync(context.Background()); err != nil {
				t.Errorf("save failed: %v", err)
			}
		}()
	}
	wg.Wait()

	// The live file must be complete and valid, with the real counter.
	d := readVisitorsFile(t, filepath.Join(dir, "visitors.json"))
	if d.TotalVisits != 139863 {
		t.Fatalf("total_visits = %d, want 139863", d.TotalVisits)
	}

	// No stray temp files left lying around.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

// The catastrophic amplification: when visitors.json fails to parse at startup
// the loader used to fall through with empty maps, and the next auto-save then
// overwrote the (recoverable) file with total=0 — which is exactly how ~140k
// visits and 195 countries were wiped. A corrupt file must instead be preserved
// and the last good backup loaded.
func TestVisitorsLoadRecoversFromBackupOnCorruption(t *testing.T) {
	dir := t.TempDir()

	// A good backup and a corrupted primary (valid JSON + trailing junk, the
	// exact shape prod hit).
	good := VisitorsData{
		TotalVisits:  139863,
		CurrentDay:   "2026-09-27",
		DailyStats:   map[string]int64{"2026-09-26": 500, "2026-09-27": 600},
		CountryStats: map[string]int64{"RU": 90000, "US": 40000},
	}
	gb, _ := json.MarshalIndent(good, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "visitors.json.bak"), gb, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "visitors.json"), append(gb, []byte("}")...), 0644); err != nil {
		t.Fatal(err)
	}

	p := newVisitorsForTest(dir)
	p.loadVisitorsData()

	if p.visitCount != 139863 {
		t.Errorf("recovered total = %d, want 139863 (should have loaded the backup, not wiped to 0)", p.visitCount)
	}
	if p.countryStats["RU"] != 90000 {
		t.Errorf("recovered country data lost: RU=%d", p.countryStats["RU"])
	}

	// The corrupt file must be kept for forensics, not silently discarded.
	entries, _ := os.ReadDir(dir)
	foundCorrupt := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "corrupt") {
			foundCorrupt = true
		}
	}
	if !foundCorrupt {
		t.Error("corrupt visitors.json was not preserved for recovery")
	}
}

// Corrupt primary, no backup: must not crash, and must preserve the corrupt
// file rather than leave nothing to recover from.
func TestVisitorsCorruptWithoutBackupPreservesFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "visitors.json"), []byte(`{"total_visits":5}garbage`), 0644); err != nil {
		t.Fatal(err)
	}
	p := newVisitorsForTest(dir)
	p.loadVisitorsData() // must not panic

	entries, _ := os.ReadDir(dir)
	foundCorrupt := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "corrupt") {
			foundCorrupt = true
		}
	}
	if !foundCorrupt {
		t.Error("corrupt file was not preserved")
	}
}
