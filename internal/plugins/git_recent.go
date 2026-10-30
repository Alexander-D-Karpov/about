package plugins

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	gitRecentReposWindow   = 90 * 24 * time.Hour
	gitRecentReposInterval = 30 * time.Minute
	gitRecentReposCap      = 10
)

type GitRecentRepo struct {
	Name        string         `json:"name"`
	URL         string         `json:"url"`
	MainLang    string         `json:"main_lang"`
	Commits     int            `json:"commits"`
	Stars       int            `json:"stars"`
	Languages   []CodeLangStat `json:"languages"`
	LastActive  time.Time      `json:"last_active"`
	Source      string         `json:"source"`
	SourceColor string         `json:"source_color"`
	// Mirrors is filled when one repo is published on several hosts (a GitHub
	// relay of a Gitea repo): every host it was found on, primary first.
	Mirrors []GitRepoMirror `json:"mirrors,omitempty"`
}

type GitRepoMirror struct {
	Source string `json:"source"`
	Color  string `json:"color"`
	URL    string `json:"url"`
}

type GitRecentRepoLister interface {
	FetchRecentRepos(ctx context.Context, since time.Time) ([]GitRecentRepo, error)
}

func (g *GitActivity) CollectRecentRepos(ctx context.Context) []GitRecentRepo {
	providers := g.Providers()
	since := time.Now().Add(-gitRecentReposWindow)

	var (
		mu     sync.Mutex
		merged []GitRecentRepo
		wg     sync.WaitGroup
	)

	for _, p := range providers {
		lister, ok := p.(GitRecentRepoLister)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(p GitProvider, l GitRecentRepoLister) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Git] recent repos panic for %s: %v", p.Key(), r)
				}
			}()
			pctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			repos, err := l.FetchRecentRepos(pctx, since)
			if err != nil {
				log.Printf("[Git] recent repos fetch failed for %s: %v", p.Key(), err)
				return
			}
			mu.Lock()
			merged = append(merged, repos...)
			mu.Unlock()
		}(p, lister)
	}
	wg.Wait()

	sort.Slice(merged, func(i, j int) bool { return merged[i].LastActive.After(merged[j].LastActive) })
	out := mergeRecentRepoMirrors(merged)
	if len(out) > gitRecentReposCap {
		out = out[:gitRecentReposCap]
	}
	return out
}

// mergeRecentRepoMirrors folds repos that share a name across hosts into one
// entry: the same project pushed to Gitea and relayed to GitHub otherwise
// shows up twice with identical stats. Input order is kept (most recent
// first); the merged entry takes the most active numbers of its copies and
// links to the copy with the most stars, falling back to the most recent.
func mergeRecentRepoMirrors(repos []GitRecentRepo) []GitRecentRepo {
	out := make([]GitRecentRepo, 0, len(repos))
	idx := make(map[string]int, len(repos))
	seenURL := make(map[string]bool, len(repos))
	for _, r := range repos {
		if r.URL != "" {
			if seenURL[r.URL] {
				continue
			}
			seenURL[r.URL] = true
		}
		// An entry may already be merged (the stored list is saved after
		// merging and merged again on render), so carry its hosts along.
		mirrors := r.Mirrors
		if len(mirrors) == 0 && r.Source != "" {
			mirrors = []GitRepoMirror{{Source: r.Source, Color: r.SourceColor, URL: r.URL}}
		}
		key := strings.ToLower(strings.TrimSpace(r.Name))
		i, ok := idx[key]
		if !ok || key == "" {
			r.Mirrors = append([]GitRepoMirror(nil), mirrors...)
			idx[key] = len(out)
			out = append(out, r)
			continue
		}
		m := &out[i]
		for _, mr := range mirrors {
			dup := false
			for _, x := range m.Mirrors {
				if strings.EqualFold(x.Source, mr.Source) {
					dup = true
					break
				}
			}
			if !dup {
				m.Mirrors = append(m.Mirrors, mr)
			}
		}
		if r.Stars > m.Stars {
			m.Stars = r.Stars
			m.URL = r.URL
			m.Source, m.SourceColor = r.Source, r.SourceColor
		}
		if r.Commits > m.Commits {
			m.Commits = r.Commits
		}
		if len(m.Languages) == 0 && len(r.Languages) > 0 {
			m.Languages = r.Languages
		}
		if m.MainLang == "" {
			m.MainLang = r.MainLang
		}
		if r.LastActive.After(m.LastActive) {
			m.LastActive = r.LastActive
		}
	}
	return out
}
