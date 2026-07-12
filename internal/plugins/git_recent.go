package plugins

import (
	"context"
	"log"
	"sort"
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

	seen := make(map[string]bool)
	out := make([]GitRecentRepo, 0, len(merged))
	sort.Slice(merged, func(i, j int) bool { return merged[i].LastActive.After(merged[j].LastActive) })
	for _, r := range merged {
		key := r.URL
		if key == "" {
			key = r.Source + "/" + r.Name
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
		if len(out) >= gitRecentReposCap {
			break
		}
	}
	return out
}
