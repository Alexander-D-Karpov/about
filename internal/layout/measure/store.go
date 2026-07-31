package measure

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const heightStoreFile = "layout_heights.json"

type persisted struct {
	// Data[plugin][bucket][span] = heightPx
	Data    map[string]map[string]map[string]int `json:"data"`
	Updated map[string]int64                     `json:"updated"` // plugin -> unix seconds
}

type HeightStore struct {
	mu      sync.RWMutex
	path    string
	data    map[string]map[string]map[int]int
	updated map[string]time.Time
	dirty   bool
}

func NewHeightStore(dataPath string) *HeightStore {
	return &HeightStore{
		path:    filepath.Join(dataPath, heightStoreFile),
		data:    map[string]map[string]map[int]int{},
		updated: map[string]time.Time{},
	}
}

func (s *HeightStore) Load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		log.Printf("[Measure] height store parse failed, starting fresh: %v", err)
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for plugin, buckets := range p.Data {
		s.data[plugin] = map[string]map[int]int{}
		for bucket, spans := range buckets {
			s.data[plugin][bucket] = map[int]int{}
			for spanStr, h := range spans {
				span, err := strconv.Atoi(spanStr)
				if err != nil {
					continue
				}
				s.data[plugin][bucket][span] = h
			}
		}
	}
	for plugin, ts := range p.Updated {
		s.updated[plugin] = time.Unix(ts, 0)
	}
	log.Printf("[Measure] height store loaded: %d plugins", len(s.data))
	return nil
}

func (s *HeightStore) Get(plugin, bucket string, span int) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if b, ok := s.data[plugin]; ok {
		if spans, ok := b[bucket]; ok {
			if h, ok := spans[span]; ok && h > 0 {
				return h, true
			}
		}
	}
	return 0, false
}

func (s *HeightStore) SetPlugin(plugin string, heights map[string]map[int]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.data[plugin]
	if existing == nil {
		existing = map[string]map[int]int{}
	}
	for bucket, spans := range heights {
		m := map[int]int{}
		for span, h := range spans {
			if h > 0 {
				m[span] = h
			}
		}
		if len(m) > 0 {
			existing[bucket] = m
		}
	}
	s.data[plugin] = existing
	s.updated[plugin] = time.Now()
	s.dirty = true
}

func (s *HeightStore) Age(plugin string) (time.Duration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ts, ok := s.updated[plugin]
	if !ok {
		return 0, false
	}
	return time.Since(ts), true
}

// NewestAge returns how long ago the most recently measured plugin was updated.
// ok is false if nothing has been measured.
func (s *HeightStore) NewestAge() (time.Duration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var newest time.Time
	for _, ts := range s.updated {
		if ts.After(newest) {
			newest = ts
		}
	}
	if newest.IsZero() {
		return 0, false
	}
	return time.Since(newest), true
}

func (s *HeightStore) Empty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data) == 0
}

func (s *HeightStore) Flush() {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	p := persisted{
		Data:    map[string]map[string]map[string]int{},
		Updated: map[string]int64{},
	}
	for plugin, buckets := range s.data {
		p.Data[plugin] = map[string]map[string]int{}
		for bucket, spans := range buckets {
			p.Data[plugin][bucket] = map[string]int{}
			for span, h := range spans {
				p.Data[plugin][bucket][strconv.Itoa(span)] = h
			}
		}
	}
	for plugin, ts := range s.updated {
		p.Updated[plugin] = ts.Unix()
	}
	s.dirty = false
	s.mu.Unlock()

	b, err := json.Marshal(p)
	if err != nil {
		log.Printf("[Measure] height store marshal failed: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		log.Printf("[Measure] height store write failed: %v", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		log.Printf("[Measure] height store rename failed: %v", err)
	}
}
