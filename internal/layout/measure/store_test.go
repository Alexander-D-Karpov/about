package measure

import (
	"path/filepath"
	"testing"
)

func TestHeightStoreSetGet(t *testing.T) {
	s := NewHeightStore(t.TempDir())
	if _, ok := s.Get("tech", "bp0", 1); ok {
		t.Fatal("expected miss on empty store")
	}
	s.SetPlugin("tech", map[string]map[int]int{"bp0": {1: 640, 2: 420}})
	if h, ok := s.Get("tech", "bp0", 1); !ok || h != 640 {
		t.Fatalf("Get(tech,bp0,1) = %d,%v want 640,true", h, ok)
	}
	if h, ok := s.Get("tech", "bp0", 2); !ok || h != 420 {
		t.Fatalf("Get(tech,bp0,2) = %d,%v want 420,true", h, ok)
	}
	if _, ok := s.Get("tech", "bp0", 3); ok {
		t.Fatal("expected miss for unmeasured span 3")
	}
}

func TestHeightStorePersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewHeightStore(dir)
	s.SetPlugin("music", map[string]map[int]int{"bp1": {1: 800}})
	s.Flush()

	if _, err := filepath.Glob(filepath.Join(dir, "layout_heights.json")); err != nil {
		t.Fatal(err)
	}
	s2 := NewHeightStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if h, ok := s2.Get("music", "bp1", 1); !ok || h != 800 {
		t.Fatalf("after reload Get = %d,%v want 800,true", h, ok)
	}
}

func TestHeightStoreEmptyAndAge(t *testing.T) {
	s := NewHeightStore(t.TempDir())
	if !s.Empty() {
		t.Fatal("new store should be Empty")
	}
	if _, ok := s.Age("tech"); ok {
		t.Fatal("no age for unmeasured plugin")
	}
	s.SetPlugin("tech", map[string]map[int]int{"bp0": {1: 100}})
	if s.Empty() {
		t.Fatal("store should not be Empty after SetPlugin")
	}
	if d, ok := s.Age("tech"); !ok || d < 0 {
		t.Fatalf("Age = %v,%v want non-negative,true", d, ok)
	}
}
