package plugins

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const artistTagTTL = 30 * 24 * time.Hour

type artistTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type artistTagEntry struct {
	Tags      []artistTag `json:"tags"`
	FetchedAt int64       `json:"fetched_at"`
}

type ArtistTagCache struct {
	path    string
	mu      sync.Mutex
	data    map[string]artistTagEntry
	dirty   bool
	pending int
}

func NewArtistTagCache(dataPath string) *ArtistTagCache {
	c := &ArtistTagCache{
		path: filepath.Join(dataPath, "music_artist_tags.json"),
		data: make(map[string]artistTagEntry),
	}
	if b, err := os.ReadFile(c.path); err == nil {
		if err := json.Unmarshal(b, &c.data); err != nil {
			log.Printf("[Music] artist tag cache parse failed, starting fresh: %v", err)
			c.data = make(map[string]artistTagEntry)
		}
	}
	log.Printf("[Music] artist tag cache loaded: %d artists", len(c.data))
	return c
}

func artistTagKey(artist string) string {
	return strings.ToLower(strings.TrimSpace(artist))
}

func (c *ArtistTagCache) Get(artist string) ([]artistTag, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[artistTagKey(artist)]
	if !ok {
		return nil, false
	}
	if time.Since(time.Unix(e.FetchedAt, 0)) > artistTagTTL {
		return nil, false
	}
	return e.Tags, true
}

func (c *ArtistTagCache) Set(artist string, tags []artistTag) {
	c.mu.Lock()
	c.data[artistTagKey(artist)] = artistTagEntry{Tags: tags, FetchedAt: time.Now().Unix()}
	c.dirty = true
	c.pending++
	flush := c.pending >= 200
	if flush {
		c.pending = 0
	}
	c.mu.Unlock()
	if flush {
		c.Flush()
	}
}

func (c *ArtistTagCache) Flush() {
	c.mu.Lock()
	if !c.dirty {
		c.mu.Unlock()
		return
	}
	b, err := json.Marshal(c.data)
	c.dirty = false
	c.mu.Unlock()
	if err != nil {
		log.Printf("[Music] artist tag cache marshal failed: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0755); err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		log.Printf("[Music] artist tag cache write failed: %v", err)
		return
	}
	if err := os.Rename(tmp, c.path); err != nil {
		os.Remove(tmp)
		log.Printf("[Music] artist tag cache rename failed: %v", err)
	}
}
