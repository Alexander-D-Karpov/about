package measure

import (
	"testing"
	"time"
)

func TestWorkerNotifyMarksDirty(t *testing.T) {
	w := NewWorker(NewHeightStore(t.TempDir()), "http://127.0.0.1:0")
	w.Notify("music")
	if !w.hasDirty() {
		t.Fatal("Notify should mark worker dirty")
	}
}

func TestStaleThresholdRespected(t *testing.T) {
	s := NewHeightStore(t.TempDir())
	s.SetPlugin("tech", map[string]map[int]int{"bp0": {1: 100}})
	w := NewWorker(s, "http://127.0.0.1:0")
	// fresh store: not stale
	if w.needsMeasure() {
		t.Fatal("fresh store should not need measure")
	}
	// empty store: needs measure
	w2 := NewWorker(NewHeightStore(t.TempDir()), "http://127.0.0.1:0")
	if !w2.needsMeasure() {
		t.Fatal("empty store should need measure")
	}
}

func TestNewestAge(t *testing.T) {
	s := NewHeightStore(t.TempDir())
	if _, ok := s.NewestAge(); ok {
		t.Fatal("empty store should have no NewestAge")
	}
	s.SetPlugin("tech", map[string]map[int]int{"bp0": {1: 100}})
	if age, ok := s.NewestAge(); !ok || age < 0 || age > time.Minute {
		t.Fatalf("NewestAge after fresh set = %v,%v want small non-negative", age, ok)
	}
}
