package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSteamStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()

	s := NewSteamStore(dir)
	s.SetGames([]SteamGame{
		{AppID: 1, Name: "One", PlaytimeAll: 120},
		{AppID: 2, Name: "Two", PlaytimeAll: 60},
	})
	s.StampFirstSeen([]int{1, 2})
	s.SetPending([]int{2, 1})
	s.SetAchievement(1, steamAchEntry{Available: true, Achieved: 3, Total: 4, Percent: 75})
	s.MarkFullSync()
	s.Flush()

	reloaded := NewSteamStore(dir)

	games, _ := reloaded.Games()
	if len(games) != 2 {
		t.Fatalf("expected 2 games after reload, got %d", len(games))
	}
	if got := reloaded.PendingCount(); got != 2 {
		t.Fatalf("expected 2 pending after reload, got %d", got)
	}
	if entry, fresh := reloaded.Achievement(1); !fresh || entry.Achieved != 3 {
		t.Fatalf("achievement did not survive reload: %+v fresh=%v", entry, fresh)
	}
	if len(reloaded.FirstSeenAll()) != 2 {
		t.Fatalf("first-seen map did not survive reload")
	}
	if reloaded.LastFullSync().IsZero() {
		t.Fatalf("last full sync did not survive reload")
	}
}

func TestSteamStoreVersionMismatchStartsFresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "steam_library.json")

	blob, _ := json.Marshal(steamLibraryFile{
		Version: steamLibraryVersion + 1,
		Games:   []SteamGame{{AppID: 9, Name: "Stale"}},
	})
	if err := os.WriteFile(path, blob, 0644); err != nil {
		t.Fatal(err)
	}

	s := NewSteamStore(dir)
	if games, _ := s.Games(); len(games) != 0 {
		t.Fatalf("expected fresh store on version mismatch, got %d games", len(games))
	}
}

func TestSteamStoreCorruptFileStartsFresh(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "steam_library.json"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewSteamStore(dir)
	if games, _ := s.Games(); len(games) != 0 {
		t.Fatalf("expected fresh store on corrupt file, got %d games", len(games))
	}
	// The store must still be usable afterwards.
	s.SetGames([]SteamGame{{AppID: 1}})
	if s.GameCount() != 1 {
		t.Fatalf("store unusable after recovering from corrupt file")
	}
}

// PopPending is the resumable cursor: an interrupted run must continue, not restart.
func TestSteamStorePopPendingIsResumable(t *testing.T) {
	dir := t.TempDir()
	s := NewSteamStore(dir)
	s.SetPending([]int{1, 2, 3, 4, 5})

	first := s.PopPending(2)
	if len(first) != 2 || first[0] != 1 || first[1] != 2 {
		t.Fatalf("unexpected first batch: %v", first)
	}
	if s.PendingCount() != 3 {
		t.Fatalf("expected 3 remaining, got %d", s.PendingCount())
	}
	s.Flush()

	// Simulate a restart: the queue must resume at 3, not back at 1.
	reloaded := NewSteamStore(dir)
	next := reloaded.PopPending(5)
	if len(next) != 3 || next[0] != 3 {
		t.Fatalf("queue did not resume after restart: %v", next)
	}

	if got := reloaded.PopPending(1); got != nil {
		t.Fatalf("expected nil from drained queue, got %v", got)
	}
}

func TestSteamStoreAchievementTTL(t *testing.T) {
	dir := t.TempDir()
	s := NewSteamStore(dir)

	s.SetAchievement(7, steamAchEntry{Available: true, Total: 10})
	if _, fresh := s.Achievement(7); !fresh {
		t.Fatalf("freshly written entry should be fresh")
	}

	// Backdate past the TTL.
	s.mu.Lock()
	e := s.data.Achievements[7]
	e.FetchedAt = time.Now().Add(-steamAchTTL - time.Hour).Unix()
	s.data.Achievements[7] = e
	s.mu.Unlock()

	entry, fresh := s.Achievement(7)
	if fresh {
		t.Fatalf("expired entry should not be fresh")
	}
	if entry.Total != 10 {
		t.Fatalf("expired entry should still be returned as a fallback, got %+v", entry)
	}
}

func TestSteamStoreStampFirstSeenOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	s := NewSteamStore(dir)

	if added := s.StampFirstSeen([]int{1, 2, 3}); added != 3 {
		t.Fatalf("expected 3 newly stamped, got %d", added)
	}
	original := s.FirstSeenAll()[1]

	if added := s.StampFirstSeen([]int{1, 2, 3, 4}); added != 1 {
		t.Fatalf("expected only appid 4 to be new, got %d", added)
	}
	if s.FirstSeenAll()[1] != original {
		t.Fatalf("existing first-seen timestamp must not be overwritten")
	}
}
