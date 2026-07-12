package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type GiteaProvider struct {
	cfg    gitSourceConfig
	client *http.Client
	base   string
	store  *CodeStatsStore
}

func NewGiteaProvider(cfg gitSourceConfig, client *http.Client) *GiteaProvider {
	base := cfg.BaseURL
	if base == "" {
		base = "https://gitea.com"
	}
	return &GiteaProvider{cfg: cfg, client: client, base: base}
}

func (p *GiteaProvider) Key() string   { return "gitea:" + p.cfg.Name }
func (p *GiteaProvider) Label() string { return p.cfg.Name }
func (p *GiteaProvider) Color() string { return p.cfg.Color }
func (p *GiteaProvider) Private() bool { return p.cfg.Private }

func (p *GiteaProvider) SetStatsStore(s *CodeStatsStore) { p.store = s }

func (p *GiteaProvider) headers() map[string]string {
	h := map[string]string{}
	if p.cfg.Token != "" {
		h["Authorization"] = "token " + p.cfg.Token
	}
	return h
}

func (p *GiteaProvider) FetchCalendar(ctx context.Context, from, to time.Time) (map[string]int, error) {
	var hm []struct {
		Timestamp     int64 `json:"timestamp"`
		Contributions int   `json:"contributions"`
	}
	endpoint := fmt.Sprintf("%s/api/v1/users/%s/heatmap", p.base, url.PathEscape(p.cfg.Username))
	if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &hm); err != nil {
		return nil, err
	}

	out := make(map[string]int)
	for _, e := range hm {
		t := time.Unix(e.Timestamp, 0)
		if t.Before(from) || t.After(to.AddDate(0, 0, 1)) {
			continue
		}
		out[t.Format("2006-01-02")] += e.Contributions
	}
	return out, nil
}

type giteaFeed struct {
	OpType  string    `json:"op_type"`
	RefName string    `json:"ref_name"`
	Content string    `json:"content"`
	Created time.Time `json:"created"`
	Repo    struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
		Private  bool   `json:"private"`
	} `json:"repo"`
}

func (p *GiteaProvider) FetchActivities(ctx context.Context, since time.Time, limit int) ([]GitActivityItem, error) {
	var feeds []giteaFeed
	endpoint := fmt.Sprintf("%s/api/v1/users/%s/activities/feeds?only-performed-by=true&limit=%d",
		p.base, url.PathEscape(p.cfg.Username), min(limit, 100))
	if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &feeds); err != nil {
		return nil, err
	}

	items := make([]GitActivityItem, 0, len(feeds))
	for _, f := range feeds {
		if f.Created.Before(since) {
			continue
		}

		item := GitActivityItem{
			Time:    f.Created,
			Repo:    gitShortRepo(f.Repo.FullName),
			URL:     f.Repo.HTMLURL,
			Source:  p.Label(),
			Color:   p.Color(),
			Private: p.cfg.Private || f.Repo.Private,
		}

		issueTitle := func() (int, string) {
			parts := strings.SplitN(f.Content, "|", 2)
			idx := 0
			title := f.Content
			if len(parts) == 2 {
				fmt.Sscanf(parts[0], "%d", &idx)
				title = parts[1]
			}
			return idx, title
		}

		switch f.OpType {
		case "commit_repo", "mirror_sync_push":
			var pc struct {
				Len     int `json:"Len"`
				Commits []struct {
					Sha1    string `json:"Sha1"`
					Message string `json:"Message"`
				} `json:"Commits"`
				HeadCommit struct {
					Sha1    string `json:"Sha1"`
					Message string `json:"Message"`
				} `json:"HeadCommit"`
			}
			json.Unmarshal([]byte(f.Content), &pc)
			n := pc.Len
			if n == 0 {
				n = len(pc.Commits)
			}
			if n == 0 {
				n = 1
			}
			item.Type = "push"
			item.Commits = n
			item.Ref = gitShortRef(f.RefName)
			item.Title = fmt.Sprintf("Pushed %d commit%s to %s", n, gitPlural(n), item.Repo)

			headSha := pc.HeadCommit.Sha1
			headMsg := pc.HeadCommit.Message
			if headSha == "" && len(pc.Commits) > 0 {
				headSha = pc.Commits[0].Sha1
				headMsg = pc.Commits[0].Message
			}
			if n == 1 {
				if msg := gitFirstLine(headMsg); msg != "" {
					item.Title = msg
				}
			}
			if headSha != "" && f.Repo.HTMLURL != "" {
				item.URL = f.Repo.HTMLURL + "/commit/" + headSha
			}

		case "create_repo":
			item.Type = "create"
			item.Title = "Created repository " + item.Repo

		case "star_repo":
			item.Type = "star"
			item.Repo = f.Repo.FullName
			item.Title = "Starred " + f.Repo.FullName

		case "create_issue":
			idx, title := issueTitle()
			item.Type = "issue"
			item.Title = fmt.Sprintf("Opened issue #%d: %s", idx, title)
			if f.Repo.HTMLURL != "" && idx > 0 {
				item.URL = fmt.Sprintf("%s/issues/%d", f.Repo.HTMLURL, idx)
			}

		case "close_issue":
			idx, title := issueTitle()
			item.Type = "issue"
			item.Title = fmt.Sprintf("Closed issue #%d: %s", idx, title)
			if f.Repo.HTMLURL != "" && idx > 0 {
				item.URL = fmt.Sprintf("%s/issues/%d", f.Repo.HTMLURL, idx)
			}

		case "create_pull_request":
			idx, title := issueTitle()
			item.Type = "pr"
			item.Title = fmt.Sprintf("Opened PR #%d: %s", idx, title)
			if f.Repo.HTMLURL != "" && idx > 0 {
				item.URL = fmt.Sprintf("%s/pulls/%d", f.Repo.HTMLURL, idx)
			}

		case "merge_pull_request", "auto_merge_pull_request":
			idx, title := issueTitle()
			item.Type = "merge"
			item.Title = fmt.Sprintf("Merged PR #%d: %s", idx, title)
			if f.Repo.HTMLURL != "" && idx > 0 {
				item.URL = fmt.Sprintf("%s/pulls/%d", f.Repo.HTMLURL, idx)
			}

		case "publish_release":
			item.Type = "release"
			item.Title = fmt.Sprintf("Released %s in %s", gitShortRef(f.RefName), item.Repo)
			if f.Repo.HTMLURL != "" && f.RefName != "" {
				item.URL = f.Repo.HTMLURL + "/releases/tag/" + gitShortRef(f.RefName)
			}

		default:
			continue
		}

		items = append(items, item)
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func (p *GiteaProvider) FetchStats(ctx context.Context) (*GitSourceStats, error) {
	type giteaRepo struct {
		FullName string `json:"full_name"`
		Fork     bool   `json:"fork"`
	}

	var repos []giteaRepo
	for page := 1; ; page++ {
		var chunk []giteaRepo
		endpoint := fmt.Sprintf("%s/api/v1/users/%s/repos?limit=50&page=%d", p.base, url.PathEscape(p.cfg.Username), page)
		if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &chunk); err != nil {
			if page == 1 {
				return nil, err
			}
			return &GitSourceStats{Repos: len(repos), Partial: true}, nil
		}
		if len(chunk) == 0 {
			break
		}
		repos = append(repos, chunk...)
		if len(chunk) < 50 {
			break
		}
	}

	stats := &GitSourceStats{Repos: len(repos)}
	login := strings.ToLower(p.cfg.Username)

	addRepo := func(c RepoStats) {
		stats.Commits += c.Commits
		stats.Additions += c.Additions
		stats.Deletions += c.Deletions
	}

	fromCache := 0
	fetched := 0
	failed := 0

	for _, r := range repos {
		if r.Fork {
			continue
		}

		if c, ok := gitCachedRepoStats(p.store, p.Key(), r.FullName, gitStatsInterval); ok {
			addRepo(c)
			fromCache++
			continue
		}

		expired := false
		select {
		case <-ctx.Done():
			expired = true
		default:
		}
		if expired {
			stats.Partial = true
			if c, ok := gitCachedRepoStats(p.store, p.Key(), r.FullName, 0); ok {
				addRepo(c)
			}
			continue
		}

		var repoStats RepoStats
		repoFailed := false

		for page := 1; ; page++ {
			select {
			case <-ctx.Done():
				repoFailed = true
			default:
			}
			if repoFailed {
				break
			}

			var commits []struct {
				Author *struct {
					Login string `json:"login"`
				} `json:"author"`
				Commit struct {
					Author struct {
						Name string `json:"name"`
					} `json:"author"`
				} `json:"commit"`
				Stats struct {
					Additions int `json:"additions"`
					Deletions int `json:"deletions"`
				} `json:"stats"`
			}
			endpoint := fmt.Sprintf("%s/api/v1/repos/%s/commits?stat=true&limit=50&page=%d", p.base, r.FullName, page)
			if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &commits); err != nil {
				repoFailed = true
				break
			}
			if len(commits) == 0 {
				break
			}
			for _, c := range commits {
				mine := false
				if c.Author != nil && strings.ToLower(c.Author.Login) == login {
					mine = true
				} else if c.Author == nil && strings.EqualFold(c.Commit.Author.Name, p.cfg.Username) {
					mine = true
				}
				if !mine {
					continue
				}
				repoStats.Commits++
				repoStats.Additions += int64(c.Stats.Additions)
				repoStats.Deletions += int64(c.Stats.Deletions)
			}
			if len(commits) < 50 {
				break
			}
		}

		if repoFailed {
			failed++
			stats.Partial = true
			if c, ok := gitCachedRepoStats(p.store, p.Key(), r.FullName, 0); ok {
				addRepo(c)
			}
			continue
		}

		repoStats.UpdatedAt = time.Now().Unix()
		addRepo(repoStats)
		fetched++
		if p.store != nil {
			p.store.SetRepoStats(p.Key(), r.FullName, repoStats)
		}
	}
	log.Printf("[Git] %s: repo stats resolved: %d from cache, %d fetched, %d failed", p.Key(), fromCache, fetched, failed)
	return stats, nil
}

func (p *GiteaProvider) FetchRecentRepos(ctx context.Context, since time.Time) ([]GitRecentRepo, error) {
	if p.cfg.Private {
		return nil, nil
	}

	type giteaRepo struct {
		Name       string    `json:"name"`
		FullName   string    `json:"full_name"`
		HTMLURL    string    `json:"html_url"`
		Private    bool      `json:"private"`
		Fork       bool      `json:"fork"`
		StarsCount int       `json:"stars_count"`
		Language   string    `json:"language"`
		UpdatedAt  time.Time `json:"updated_at"`
	}

	var repos []giteaRepo
	for page := 1; page <= 3; page++ {
		var chunk []giteaRepo
		endpoint := fmt.Sprintf("%s/api/v1/users/%s/repos?limit=50&page=%d", p.base, url.PathEscape(p.cfg.Username), page)
		if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &chunk); err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		if len(chunk) == 0 {
			break
		}
		repos = append(repos, chunk...)
		if len(chunk) < 50 {
			break
		}
	}

	candidates := make([]giteaRepo, 0, len(repos))
	for _, r := range repos {
		if r.Private || r.Fork || r.UpdatedAt.Before(since) {
			continue
		}
		candidates = append(candidates, r)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt) })
	if len(candidates) > gitRecentReposCap {
		candidates = candidates[:gitRecentReposCap]
	}

	sinceStr := since.Format(time.RFC3339)
	out := make([]GitRecentRepo, 0, len(candidates))

	for _, r := range candidates {
		repo := GitRecentRepo{
			Name:        r.Name,
			URL:         r.HTMLURL,
			MainLang:    r.Language,
			Stars:       r.StarsCount,
			LastActive:  r.UpdatedAt,
			Source:      p.Label(),
			SourceColor: p.Color(),
		}

		var commits []struct {
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
			Commit struct {
				Author struct {
					Name string `json:"name"`
				} `json:"author"`
				Committer struct {
					Date time.Time `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
		}
		endpoint := fmt.Sprintf("%s/api/v1/repos/%s/commits?limit=50&since=%s", p.base, r.FullName, url.QueryEscape(sinceStr))
		if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &commits); err == nil {
			for _, c := range commits {
				mine := false
				if c.Author != nil && strings.EqualFold(c.Author.Login, p.cfg.Username) {
					mine = true
				} else if c.Author == nil && strings.EqualFold(c.Commit.Author.Name, p.cfg.Username) {
					mine = true
				}
				if !mine {
					continue
				}
				repo.Commits++
				if c.Commit.Committer.Date.After(repo.LastActive) {
					repo.LastActive = c.Commit.Committer.Date
				}
			}
		}
		if repo.Commits == 0 {
			continue
		}

		var langBytes map[string]int64
		langEndpoint := fmt.Sprintf("%s/api/v1/repos/%s/languages", p.base, r.FullName)
		if err := gitDoJSON(ctx, p.client, "GET", langEndpoint, p.headers(), nil, &langBytes); err == nil && len(langBytes) > 0 {
			var total int64
			for _, b := range langBytes {
				total += b
			}
			for name, b := range langBytes {
				repo.Languages = append(repo.Languages, CodeLangStat{
					Name:    name,
					Color:   GetLanguageColor(name),
					Percent: float64(b) / float64(total) * 100,
				})
			}
			sort.Slice(repo.Languages, func(i, j int) bool {
				return repo.Languages[i].Percent > repo.Languages[j].Percent
			})
			if len(repo.Languages) > 6 {
				repo.Languages = repo.Languages[:6]
			}
			if repo.MainLang == "" {
				repo.MainLang = repo.Languages[0].Name
			}
		}

		out = append(out, repo)
	}
	return out, nil
}
