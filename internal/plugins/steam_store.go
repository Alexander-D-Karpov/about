package plugins

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	steamLibraryVersion = 1
	// Bump when the game/software classification rules change.
	steamTypesVersion = 2
	// How long a cached per-game achievement entry stays valid.
	steamAchTTL = 7 * 24 * time.Hour
	// Enriched games buffered before the store is flushed to disk. Bounds how much work an
	// interrupted process can lose.
	steamAchFlushEvery = 25
)

type steamYearSnapshot struct {
	Year     int         `json:"year"`
	Playtime map[int]int `json:"playtime"`
	TakenAt  int64       `json:"taken_at"`
}

type steamLibraryFile struct {
	Version      int                   `json:"version"`
	Games        []SteamGame           `json:"games"`
	GamesUpdated int64                 `json:"games_updated"`
	FirstSeen    map[int]int64         `json:"first_seen"`
	Achievements map[int]steamAchEntry `json:"achievements"`
	YearSnapshot steamYearSnapshot     `json:"year_snapshot"`
	PendingAch   []int                 `json:"pending_ach"`
	FamilyValid  bool                  `json:"family_valid"`
	LastFullSync int64                 `json:"last_full_sync"`
	// AppTypes maps appid -> store type ("game", "dlc", "software", "tool", "demo", ...).
	// GetOwnedGames does not say what an app is, so this is resolved separately and cached
	// forever; only "game" entries are shown.
	AppTypes map[int]string `json:"app_types"`
	// AppHeaders holds real banner URLs for apps whose art lives under a content-hashed path
	// that cannot be derived from the appid.
	AppHeaders  map[int]string `json:"app_headers"`
	PendingType []int          `json:"pending_type"`
	// TypesVersion lets the classification be redone without discarding the far more expensive
	// achievement cache, which a full store version bump would throw away.
	TypesVersion int `json:"types_version"`
	// TokenCheckedAt is zero until a sync has actually tried the current token, which lets the
	// admin page say "not checked yet" instead of wrongly calling a fresh token expired.
	TokenCheckedAt int64 `json:"token_checked_at"`
}

// SteamStore is the crash-safe side-car for the Steam library. It lives beside config.json rather
// than inside it: the library is hundreds of entries and SetPluginConfig rewrites the whole config
// file (plus spawns a backup marshal) on every call.
type SteamStore struct {
	path    string
	mu      sync.Mutex
	data    steamLibraryFile
	dirty   bool
	pending int
}

func NewSteamStore(dataPath string) *SteamStore {
	s := &SteamStore{path: filepath.Join(dataPath, "steam_library.json")}
	s.load()
	return s
}

func (s *SteamStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reset()

	b, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[Steam] library store read failed, starting fresh: %v", err)
		}
		return
	}

	var f steamLibraryFile
	if err := json.Unmarshal(b, &f); err != nil {
		log.Printf("[Steam] library store parse failed, starting fresh: %v", err)
		return
	}
	if f.Version != steamLibraryVersion {
		log.Printf("[Steam] library store version %d != %d, starting fresh", f.Version, steamLibraryVersion)
		return
	}

	s.data = f
	if s.data.FirstSeen == nil {
		s.data.FirstSeen = make(map[int]int64)
	}
	if s.data.Achievements == nil {
		s.data.Achievements = make(map[int]steamAchEntry)
	}
	if s.data.YearSnapshot.Playtime == nil {
		s.data.YearSnapshot.Playtime = make(map[int]int)
	}
	if s.data.AppTypes == nil {
		s.data.AppTypes = make(map[int]string)
	}
	if s.data.AppHeaders == nil {
		s.data.AppHeaders = make(map[int]string)
	}
	if s.data.TypesVersion != steamTypesVersion {
		// The rules for what counts as a game changed; drop the verdicts (not the achievements)
		// and let the worker re-resolve them.
		log.Printf("[Steam] app classification version %d != %d, re-resolving %d apps",
			s.data.TypesVersion, steamTypesVersion, len(s.data.AppTypes))
		s.data.AppTypes = make(map[int]string)
		s.data.TypesVersion = steamTypesVersion
		s.dirty = true
	}
}

func (s *SteamStore) reset() {
	s.data = steamLibraryFile{
		Version:      steamLibraryVersion,
		FirstSeen:    make(map[int]int64),
		Achievements: make(map[int]steamAchEntry),
		YearSnapshot: steamYearSnapshot{Playtime: make(map[int]int)},
		AppTypes:     make(map[int]string),
		AppHeaders:   make(map[int]string),
		TypesVersion: steamTypesVersion,
	}
}

// Flush writes the store atomically. It is a no-op when nothing changed.
func (s *SteamStore) Flush() {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	s.data.Version = steamLibraryVersion
	b, err := json.Marshal(s.data)
	s.dirty = false
	s.mu.Unlock()

	if err != nil {
		log.Printf("[Steam] library store marshal failed: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		log.Printf("[Steam] library store mkdir failed: %v", err)
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		log.Printf("[Steam] library store write failed: %v", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		log.Printf("[Steam] library store rename failed: %v", err)
	}
}

// --- games ---

func (s *SteamStore) Games() ([]SteamGame, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SteamGame, len(s.data.Games))
	copy(out, s.data.Games)
	return out, time.Unix(s.data.GamesUpdated, 0)
}

func (s *SteamStore) GameCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data.Games)
}

func (s *SteamStore) SetGames(games []SteamGame) {
	s.mu.Lock()
	s.data.Games = make([]SteamGame, len(games))
	copy(s.data.Games, games)
	s.data.GamesUpdated = time.Now().Unix()
	s.dirty = true
	s.mu.Unlock()
	s.Flush()
}

// --- first seen (the "date added" fallback for owned games) ---

// StampFirstSeen records now as the first-seen time for any appid not already known, and returns
// how many were newly recorded.
func (s *SteamStore) StampFirstSeen(appIDs []int) int {
	now := time.Now().Unix()
	added := 0
	s.mu.Lock()
	for _, id := range appIDs {
		if _, ok := s.data.FirstSeen[id]; !ok {
			s.data.FirstSeen[id] = now
			added++
		}
	}
	if added > 0 {
		s.dirty = true
	}
	s.mu.Unlock()
	return added
}

// FirstSeenAll returns a copy of the whole first-seen map.
func (s *SteamStore) FirstSeenAll() map[int]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[int]int64, len(s.data.FirstSeen))
	for k, v := range s.data.FirstSeen {
		out[k] = v
	}
	return out
}

// IsFirstRun reports whether we have never stamped anything, meaning first-seen values are the
// import date rather than a real acquisition date.
func (s *SteamStore) IsFirstRun() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.LastFullSync == 0
}

// --- achievements ---

func (s *SteamStore) Achievement(appID int) (steamAchEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data.Achievements[appID]
	if !ok {
		return steamAchEntry{}, false
	}
	if time.Since(time.Unix(e.FetchedAt, 0)) > steamAchTTL {
		return e, false
	}
	return e, true
}

// SetAchievement stores one game's achievement state, flushing in batches so an interrupt loses at
// most steamAchFlushEvery games of work.
func (s *SteamStore) SetAchievement(appID int, e steamAchEntry) {
	s.mu.Lock()
	e.FetchedAt = time.Now().Unix()
	s.data.Achievements[appID] = e
	s.dirty = true
	s.pending++
	flush := s.pending >= steamAchFlushEvery
	if flush {
		s.pending = 0
	}
	s.mu.Unlock()
	if flush {
		s.Flush()
	}
}

// AllAchievements returns a copy of every cached achievement entry.
func (s *SteamStore) AllAchievements() map[int]steamAchEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[int]steamAchEntry, len(s.data.Achievements))
	for k, v := range s.data.Achievements {
		out[k] = v
	}
	return out
}

// --- resumable enrichment cursor ---

func (s *SteamStore) SetPending(appIDs []int) {
	s.mu.Lock()
	s.data.PendingAch = make([]int, len(appIDs))
	copy(s.data.PendingAch, appIDs)
	s.dirty = true
	s.mu.Unlock()
	s.Flush()
}

func (s *SteamStore) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data.PendingAch)
}

// PopPending removes and returns up to n appids from the head of the queue. The removal is
// persisted by the caller's subsequent flush; a crash before that simply retries those games.
func (s *SteamStore) PopPending(n int) []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.data.PendingAch) {
		n = len(s.data.PendingAch)
	}
	if n <= 0 {
		return nil
	}
	out := make([]int, n)
	copy(out, s.data.PendingAch[:n])
	s.data.PendingAch = s.data.PendingAch[n:]
	s.dirty = true
	return out
}

// --- app types (games vs software/dlc/tools) ---

func (s *SteamStore) AppTypes() map[int]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[int]string, len(s.data.AppTypes))
	for k, v := range s.data.AppTypes {
		out[k] = v
	}
	return out
}

// AppHeaders returns cached banner URLs keyed by appid.
func (s *SteamStore) AppHeaders() map[int]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[int]string, len(s.data.AppHeaders))
	for k, v := range s.data.AppHeaders {
		out[k] = v
	}
	return out
}

func (s *SteamStore) SetAppInfo(appID int, t, header string) {
	s.mu.Lock()
	s.data.AppTypes[appID] = t
	if header != "" {
		s.data.AppHeaders[appID] = header
	}
	s.dirty = true
	s.pending++
	flush := s.pending >= steamAchFlushEvery
	if flush {
		s.pending = 0
	}
	s.mu.Unlock()
	if flush {
		s.Flush()
	}
}

func (s *SteamStore) SetPendingTypes(appIDs []int) {
	s.mu.Lock()
	s.data.PendingType = make([]int, len(appIDs))
	copy(s.data.PendingType, appIDs)
	s.dirty = true
	s.mu.Unlock()
	s.Flush()
}

func (s *SteamStore) PendingTypeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data.PendingType)
}

// RequeueType puts an appid back at the head of the queue, for when the API throttles us.
func (s *SteamStore) RequeueType(appID int) {
	s.mu.Lock()
	s.data.PendingType = append([]int{appID}, s.data.PendingType...)
	s.dirty = true
	s.mu.Unlock()
}

func (s *SteamStore) PopPendingTypes(n int) []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.data.PendingType) {
		n = len(s.data.PendingType)
	}
	if n <= 0 {
		return nil
	}
	out := make([]int, n)
	copy(out, s.data.PendingType[:n])
	s.data.PendingType = s.data.PendingType[n:]
	s.dirty = true
	return out
}

// --- yearly snapshot ---

func (s *SteamStore) YearSnapshot() steamYearSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := steamYearSnapshot{
		Year:     s.data.YearSnapshot.Year,
		TakenAt:  s.data.YearSnapshot.TakenAt,
		Playtime: make(map[int]int, len(s.data.YearSnapshot.Playtime)),
	}
	for k, v := range s.data.YearSnapshot.Playtime {
		out.Playtime[k] = v
	}
	return out
}

func (s *SteamStore) SetYearSnapshot(snap steamYearSnapshot) {
	s.mu.Lock()
	s.data.YearSnapshot = snap
	s.dirty = true
	s.mu.Unlock()
	s.Flush()
}

// --- sync bookkeeping ---

func (s *SteamStore) LastFullSync() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.LastFullSync == 0 {
		return time.Time{}
	}
	return time.Unix(s.data.LastFullSync, 0)
}

func (s *SteamStore) MarkFullSync() {
	s.mu.Lock()
	s.data.LastFullSync = time.Now().Unix()
	s.dirty = true
	s.mu.Unlock()
	s.Flush()
}

// ClearFullSync forces the next cycle to re-pull the whole library.
func (s *SteamStore) ClearFullSync() {
	s.mu.Lock()
	s.data.LastFullSync = 0
	s.dirty = true
	s.mu.Unlock()
	s.Flush()
}

func (s *SteamStore) FamilyValid() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.FamilyValid
}

func (s *SteamStore) SetFamilyValid(v bool) {
	s.mu.Lock()
	if s.data.FamilyValid != v {
		s.data.FamilyValid = v
		s.dirty = true
	}
	s.mu.Unlock()
}

// TokenChecked reports whether a sync has tried the current token yet.
func (s *SteamStore) TokenChecked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.TokenCheckedAt > 0
}

// MarkTokenChecked records that a sync exercised the current token.
func (s *SteamStore) MarkTokenChecked() {
	s.mu.Lock()
	s.data.TokenCheckedAt = time.Now().Unix()
	s.dirty = true
	s.mu.Unlock()
}

// ResetTokenCheck forgets the previous verification, for when the token changes.
func (s *SteamStore) ResetTokenCheck() {
	s.mu.Lock()
	s.data.TokenCheckedAt = 0
	s.dirty = true
	s.mu.Unlock()
}
