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
	codeStatsVersion    = 2
	codeStatsHistoryMax = 500
	codeStatsHistoryGap = 6 * time.Hour
)

type gitStatsCacheEntry struct {
	Stats     GitSourceStats `json:"stats"`
	UpdatedAt int64          `json:"updated_at"`
}

type GitStatsPoint struct {
	At        int64 `json:"at"`
	Commits   int64 `json:"commits"`
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	Repos     int   `json:"repos"`
	Partial   bool  `json:"partial"`
}

type RepoStats struct {
	Commits   int64 `json:"commits"`
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	PushedAt  int64 `json:"pushed_at,omitempty"`
	UpdatedAt int64 `json:"updated_at"`
}

type codeStatsFile struct {
	Version            int                             `json:"version"`
	GitHub             *GitHubCodeStats                `json:"github,omitempty"`
	GitHubUpdated      int64                           `json:"github_updated,omitempty"`
	Wakatime           *WakatimeStats                  `json:"wakatime,omitempty"`
	WakatimeUpdated    int64                           `json:"wakatime_updated,omitempty"`
	Git                map[string]gitStatsCacheEntry   `json:"git"`
	GitHistory         map[string][]GitStatsPoint      `json:"git_history,omitempty"`
	RepoStats          map[string]map[string]RepoStats `json:"repo_stats,omitempty"`
	RecentRepos        []GitRecentRepo                 `json:"recent_repos,omitempty"`
	RecentReposUpdated int64                           `json:"recent_repos_updated,omitempty"`
}

type CodeStatsStore struct {
	path string
	mu   sync.Mutex
	data codeStatsFile
}

func NewCodeStatsStore(dataPath string) *CodeStatsStore {
	s := &CodeStatsStore{path: filepath.Join(dataPath, "code_stats.json")}
	s.load()
	return s
}

func (s *CodeStatsStore) Dir() string {
	return filepath.Dir(s.path)
}

func (s *CodeStatsStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Git = make(map[string]gitStatsCacheEntry)
	s.data.GitHistory = make(map[string][]GitStatsPoint)
	s.data.RepoStats = make(map[string]map[string]RepoStats)
	s.data.Version = codeStatsVersion

	b, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[CodeStore] read %s failed: %v", s.path, err)
		} else {
			log.Printf("[CodeStore] no stats file yet, will collect from scratch")
		}
		return
	}

	var parsed codeStatsFile
	if err := json.Unmarshal(b, &parsed); err != nil {
		log.Printf("[CodeStore] parse %s failed, starting fresh: %v", s.path, err)
		return
	}
	if parsed.Git == nil {
		parsed.Git = make(map[string]gitStatsCacheEntry)
	}
	if parsed.GitHistory == nil {
		parsed.GitHistory = make(map[string][]GitStatsPoint)
	}
	if parsed.RepoStats == nil {
		parsed.RepoStats = make(map[string]map[string]RepoStats)
	}
	s.data = parsed

	if s.data.Version != codeStatsVersion {
		log.Printf("[CodeStore] stats version %d -> %d, keeping per-repo cache", s.data.Version, codeStatsVersion)
		s.data.Version = codeStatsVersion
		s.saveLocked()
	}

	repoCached := 0
	for _, m := range s.data.RepoStats {
		repoCached += len(m)
	}
	log.Printf("[CodeStore] loaded %s: git sources=%d, per-repo stats=%d, history series=%d, github=%v, wakatime=%v",
		s.path, len(s.data.Git), repoCached, len(s.data.GitHistory), s.data.GitHub != nil, s.data.Wakatime != nil)
}

func (s *CodeStatsStore) saveLocked() {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		log.Printf("[CodeStore] marshal failed: %v", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		log.Printf("[CodeStore] mkdir failed: %v", err)
		return
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		log.Printf("[CodeStore] write failed: %v", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		log.Printf("[CodeStore] rename failed: %v", err)
	}
}

func (s *CodeStatsStore) GetGitHub() (*GitHubCodeStats, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.GitHub == nil {
		return nil, time.Time{}
	}
	cp := *s.data.GitHub
	return &cp, time.Unix(s.data.GitHubUpdated, 0)
}

func (s *CodeStatsStore) SetGitHub(v *GitHubCodeStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.GitHub = v
	s.data.GitHubUpdated = time.Now().Unix()
	s.saveLocked()
}

func (s *CodeStatsStore) GetWakatime() (*WakatimeStats, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Wakatime == nil {
		return nil, time.Time{}
	}
	cp := *s.data.Wakatime
	return &cp, time.Unix(s.data.WakatimeUpdated, 0)
}

func (s *CodeStatsStore) SetWakatime(v *WakatimeStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Wakatime = v
	s.data.WakatimeUpdated = time.Now().Unix()
	s.saveLocked()
}

func (s *CodeStatsStore) GetGitStats(key string) (GitSourceStats, time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data.Git[key]
	if !ok {
		return GitSourceStats{}, time.Time{}, false
	}
	return e.Stats, time.Unix(e.UpdatedAt, 0), true
}

func (s *CodeStatsStore) SetGitStats(key string, st GitSourceStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	s.data.Git[key] = gitStatsCacheEntry{Stats: st, UpdatedAt: now}
	s.appendHistoryLocked(key, st, now)
	s.saveLocked()
}

func (s *CodeStatsStore) MergeGitStats(key string, fresh GitSourceStats) GitSourceStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	merged := fresh
	if prev, ok := s.data.Git[key]; ok && fresh.Partial {
		if prev.Stats.Commits > merged.Commits {
			merged.Commits = prev.Stats.Commits
		}
		if prev.Stats.Additions > merged.Additions {
			merged.Additions = prev.Stats.Additions
		}
		if prev.Stats.Deletions > merged.Deletions {
			merged.Deletions = prev.Stats.Deletions
		}
		if prev.Stats.Repos > merged.Repos {
			merged.Repos = prev.Stats.Repos
		}
	}

	now := time.Now().Unix()
	s.data.Git[key] = gitStatsCacheEntry{Stats: merged, UpdatedAt: now}
	s.appendHistoryLocked(key, merged, now)
	s.saveLocked()
	return merged
}

func (s *CodeStatsStore) appendHistoryLocked(key string, st GitSourceStats, now int64) {
	if s.data.GitHistory == nil {
		s.data.GitHistory = make(map[string][]GitStatsPoint)
	}

	series := s.data.GitHistory[key]
	point := GitStatsPoint{
		At:        now,
		Commits:   st.Commits,
		Additions: st.Additions,
		Deletions: st.Deletions,
		Repos:     st.Repos,
		Partial:   st.Partial,
	}

	if n := len(series); n > 0 {
		last := series[n-1]
		sameValues := last.Commits == point.Commits &&
			last.Additions == point.Additions &&
			last.Deletions == point.Deletions &&
			last.Repos == point.Repos
		if sameValues && time.Since(time.Unix(last.At, 0)) < codeStatsHistoryGap {
			series[n-1].At = now
			series[n-1].Partial = point.Partial
			s.data.GitHistory[key] = series
			return
		}
	}

	series = append(series, point)
	if len(series) > codeStatsHistoryMax {
		series = series[len(series)-codeStatsHistoryMax:]
	}
	s.data.GitHistory[key] = series
}

func (s *CodeStatsStore) GitHistory(key string) []GitStatsPoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]GitStatsPoint(nil), s.data.GitHistory[key]...)
}

func (s *CodeStatsStore) AllGitStats() map[string]gitStatsCacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]gitStatsCacheEntry, len(s.data.Git))
	for k, v := range s.data.Git {
		out[k] = v
	}
	return out
}

func (s *CodeStatsStore) GetRepoStats(source, repo string) (RepoStats, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.RepoStats == nil {
		return RepoStats{}, false
	}
	e, ok := s.data.RepoStats[source][repo]
	return e, ok
}

func (s *CodeStatsStore) RepoStatsFor(source string) map[string]RepoStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]RepoStats, len(s.data.RepoStats[source]))
	for k, v := range s.data.RepoStats[source] {
		out[k] = v
	}
	return out
}

func (s *CodeStatsStore) SetRepoStats(source, repo string, st RepoStats) {
	s.SetRepoStatsBatch(source, map[string]RepoStats{repo: st})
}

func (s *CodeStatsStore) SetRepoStatsBatch(source string, batch map[string]RepoStats) {
	if len(batch) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.RepoStats == nil {
		s.data.RepoStats = make(map[string]map[string]RepoStats)
	}
	if s.data.RepoStats[source] == nil {
		s.data.RepoStats[source] = make(map[string]RepoStats)
	}
	for repo, st := range batch {
		s.data.RepoStats[source][repo] = st
	}
	s.saveLocked()
}

func (s *CodeStatsStore) PruneRepoStats(source string, keep map[string]bool) {
	if len(keep) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := s.data.RepoStats[source]
	if len(cur) == 0 {
		return
	}

	removed := 0
	for repo := range cur {
		if !keep[repo] {
			delete(cur, repo)
			removed++
		}
	}
	if removed > 0 {
		log.Printf("[CodeStore] pruned %d stale repo stats for %s", removed, source)
		s.saveLocked()
	}
}

func (s *CodeStatsStore) GetRecentRepos() ([]GitRecentRepo, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]GitRecentRepo(nil), s.data.RecentRepos...), time.Unix(s.data.RecentReposUpdated, 0)
}

func (s *CodeStatsStore) SetRecentRepos(v []GitRecentRepo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.RecentRepos = v
	s.data.RecentReposUpdated = time.Now().Unix()
	s.saveLocked()
}
