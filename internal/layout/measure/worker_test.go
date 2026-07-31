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
	_ = time.Second
}
