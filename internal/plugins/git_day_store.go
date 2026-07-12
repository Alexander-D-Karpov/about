package plugins

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const gitDayStoreVersion = 2

type gitDayEntry struct {
	Items     []GitActivityItem `json:"items"`
	Private   int               `json:"private"`
	FetchedAt int64             `json:"fetched_at"`
}

type gitDayFile struct {
	Version int                    `json:"version"`
	Days    map[string]gitDayEntry `json:"days"`
}

type GitDayStore struct {
	path  string
	mu    sync.Mutex
	data  map[string]gitDayEntry
	dirty bool
}

func NewGitDayStore(dataPath string) *GitDayStore {
	s := &GitDayStore{
		path: filepath.Join(dataPath, "git_days.json"),
		data: make(map[string]gitDayEntry),
	}
	if b, err := os.ReadFile(s.path); err == nil {
		var f gitDayFile
		if err := json.Unmarshal(b, &f); err == nil && f.Version == gitDayStoreVersion && f.Days != nil {
			s.data = f.Days
		} else {
			log.Printf("[Git] day store outdated or unreadable, starting fresh")
			s.dirty = true
		}
	}
	log.Printf("[Git] day store loaded: %d days", len(s.data))
	return s
}

func (s *GitDayStore) Get(date string) (gitDayEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[date]
	return e, ok
}

func (s *GitDayStore) Set(date string, items []GitActivityItem, private int) {
	s.mu.Lock()
	s.data[date] = gitDayEntry{Items: items, Private: private, FetchedAt: time.Now().Unix()}
	s.dirty = true
	s.mu.Unlock()
}

func (s *GitDayStore) Flush() {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	b, err := json.Marshal(gitDayFile{Version: gitDayStoreVersion, Days: s.data})
	s.dirty = false
	s.mu.Unlock()
	if err != nil {
		log.Printf("[Git] day store marshal failed: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		log.Printf("[Git] day store write failed: %v", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		log.Printf("[Git] day store rename failed: %v", err)
	}
}
