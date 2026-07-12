package plugins

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"sync"
	"time"
)

type GitLabProvider struct {
	cfg    gitSourceConfig
	client *http.Client
	base   string
	store  *CodeStatsStore

	mu       sync.Mutex
	userID   int
	projects map[int]glProjectInfo
}

type glProjectInfo struct {
	name    string
	url     string
	private bool
}

func NewGitLabProvider(cfg gitSourceConfig, client *http.Client) *GitLabProvider {
	base := cfg.BaseURL
	if base == "" {
		base = "https://gitlab.com"
	}
	return &GitLabProvider{
		cfg:      cfg,
		client:   client,
		base:     base,
		projects: make(map[int]glProjectInfo),
	}
}

func (p *GitLabProvider) Key() string   { return "gitlab:" + p.cfg.Name }
func (p *GitLabProvider) Label() string { return p.cfg.Name }
func (p *GitLabProvider) Color() string { return p.cfg.Color }
func (p *GitLabProvider) Private() bool { return p.cfg.Private }

func (p *GitLabProvider) SetStatsStore(s *CodeStatsStore) { p.store = s }

func (p *GitLabProvider) headers() map[string]string {
	h := map[string]string{}
	if p.cfg.Token != "" {
		h["PRIVATE-TOKEN"] = p.cfg.Token
	}
	return h
}

func (p *GitLabProvider) resolveUserID(ctx context.Context) (int, error) {
	p.mu.Lock()
	id := p.userID
	p.mu.Unlock()
	if id != 0 {
		return id, nil
	}

	var users []struct {
		ID int `json:"id"`
	}
	endpoint := fmt.Sprintf("%s/api/v4/users?username=%s", p.base, url.QueryEscape(p.cfg.Username))
	if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &users); err != nil {
		return 0, err
	}
	if len(users) == 0 {
		return 0, fmt.Errorf("gitlab user %s not found on %s", p.cfg.Username, p.base)
	}

	p.mu.Lock()
	p.userID = users[0].ID
	p.mu.Unlock()
	return users[0].ID, nil
}

type glEvent struct {
	ActionName  string    `json:"action_name"`
	TargetType  string    `json:"target_type"`
	TargetTitle string    `json:"target_title"`
	TargetIID   int       `json:"target_iid"`
	ProjectID   int       `json:"project_id"`
	CreatedAt   time.Time `json:"created_at"`
	PushData    *struct {
		CommitCount int    `json:"commit_count"`
		Ref         string `json:"ref"`
		Action      string `json:"action"`
		CommitTo    string `json:"commit_to"`
		CommitTitle string `json:"commit_title"`
	} `json:"push_data"`
}

func (p *GitLabProvider) FetchCalendar(ctx context.Context, from, to time.Time) (map[string]int, error) {
	var cal map[string]int
	endpoint := fmt.Sprintf("%s/users/%s/calendar.json", p.base, url.PathEscape(p.cfg.Username))
	if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &cal); err == nil && len(cal) > 0 {
		out := make(map[string]int, len(cal))
		fromStr := from.Format("2006-01-02")
		toStr := to.Format("2006-01-02")
		for date, count := range cal {
			if date >= fromStr && date <= toStr && count > 0 {
				out[date] = count
			}
		}
		return out, nil
	}

	id, err := p.resolveUserID(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[string]int)
	after := from.AddDate(0, 0, -1).Format("2006-01-02")
	for page := 1; page <= 10; page++ {
		var events []glEvent
		endpoint := fmt.Sprintf("%s/api/v4/users/%d/events?per_page=100&page=%d&after=%s", p.base, id, page, after)
		if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &events); err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		if len(events) == 0 {
			break
		}
		for _, ev := range events {
			out[ev.CreatedAt.Format("2006-01-02")]++
		}
		if len(events) < 100 {
			break
		}
	}
	return out, nil
}

func (p *GitLabProvider) projectInfo(ctx context.Context, id int) (string, string, bool) {
	if id == 0 {
		return "", "", false
	}
	p.mu.Lock()
	if info, ok := p.projects[id]; ok {
		p.mu.Unlock()
		return info.name, info.url, info.private
	}
	p.mu.Unlock()

	var proj struct {
		PathWithNamespace string `json:"path_with_namespace"`
		WebURL            string `json:"web_url"`
		Visibility        string `json:"visibility"`
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%d", p.base, id)
	if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &proj); err != nil {
		return fmt.Sprintf("project-%d", id), "", true
	}

	private := proj.Visibility != "public"
	p.mu.Lock()
	p.projects[id] = glProjectInfo{name: proj.PathWithNamespace, url: proj.WebURL, private: private}
	p.mu.Unlock()
	return proj.PathWithNamespace, proj.WebURL, private
}

func (p *GitLabProvider) FetchActivities(ctx context.Context, since time.Time, limit int) ([]GitActivityItem, error) {
	id, err := p.resolveUserID(ctx)
	if err != nil {
		return nil, err
	}

	var events []glEvent
	after := since.AddDate(0, 0, -1).Format("2006-01-02")
	endpoint := fmt.Sprintf("%s/api/v4/users/%d/events?per_page=100&after=%s", p.base, id, after)
	if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &events); err != nil {
		return nil, err
	}

	items := make([]GitActivityItem, 0, len(events))
	for _, ev := range events {
		if ev.CreatedAt.Before(since) {
			continue
		}

		repoName, repoURL, repoPrivate := p.projectInfo(ctx, ev.ProjectID)
		item := GitActivityItem{
			Time:    ev.CreatedAt,
			Repo:    gitShortRepo(repoName),
			URL:     repoURL,
			Source:  p.Label(),
			Color:   p.Color(),
			Private: p.cfg.Private || repoPrivate,
		}

		switch {
		case ev.PushData != nil && ev.PushData.CommitCount > 0:
			n := ev.PushData.CommitCount
			item.Type = "push"
			item.Commits = n
			item.Ref = gitShortRef(ev.PushData.Ref)
			item.Title = fmt.Sprintf("Pushed %d commit%s to %s", n, gitPlural(n), item.Repo)
			if n == 1 && ev.PushData.CommitTitle != "" {
				item.Title = gitFirstLine(ev.PushData.CommitTitle)
			}
			if ev.PushData.CommitTo != "" && repoURL != "" {
				item.URL = repoURL + "/-/commit/" + ev.PushData.CommitTo
			}

		case ev.TargetType == "MergeRequest":
			switch ev.ActionName {
			case "opened":
				item.Type = "pr"
				item.Title = fmt.Sprintf("Opened MR !%d: %s", ev.TargetIID, ev.TargetTitle)
			case "accepted", "merged":
				item.Type = "merge"
				item.Title = fmt.Sprintf("Merged MR !%d: %s", ev.TargetIID, ev.TargetTitle)
			case "closed":
				item.Type = "pr"
				item.Title = fmt.Sprintf("Closed MR !%d: %s", ev.TargetIID, ev.TargetTitle)
			default:
				continue
			}

		case ev.TargetType == "Issue":
			switch ev.ActionName {
			case "opened":
				item.Type = "issue"
				item.Title = fmt.Sprintf("Opened issue #%d: %s", ev.TargetIID, ev.TargetTitle)
			case "closed":
				item.Type = "issue"
				item.Title = fmt.Sprintf("Closed issue #%d: %s", ev.TargetIID, ev.TargetTitle)
			default:
				continue
			}

		case ev.ActionName == "created" && ev.TargetType == "":
			item.Type = "create"
			item.Title = "Created project " + item.Repo

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

func (p *GitLabProvider) FetchStats(ctx context.Context) (*GitSourceStats, error) {
	projects, partial := p.allProjects(ctx)
	stats := &GitSourceStats{Repos: len(projects), Partial: partial}
	log.Printf("[Git] %s: %d projects to process (incl. membership)", p.Key(), len(projects))
	author := url.QueryEscape(p.cfg.Username)

	addRepo := func(c RepoStats) {
		stats.Commits += c.Commits
		stats.Additions += c.Additions
		stats.Deletions += c.Deletions
	}

	fromCache := 0
	fetched := 0
	failed := 0

	for i, proj := range projects {
		fmt.Fprintf(os.Stderr, "\r[Git] %s stats: %d/%d projects processed", p.Key(), i, len(projects))

		key := proj.PathWithNamespace

		if c, ok := gitCachedRepoStats(p.store, p.Key(), key, gitStatsInterval); ok {
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
			if c, ok := gitCachedRepoStats(p.store, p.Key(), key, 0); ok {
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
				Stats struct {
					Additions int `json:"additions"`
					Deletions int `json:"deletions"`
				} `json:"stats"`
			}
			endpoint := fmt.Sprintf("%s/api/v4/projects/%d/repository/commits?with_stats=true&per_page=100&page=%d&author=%s",
				p.base, proj.ID, page, author)
			if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &commits); err != nil {
				repoFailed = true
				break
			}
			if len(commits) == 0 {
				break
			}
			repoStats.Commits += int64(len(commits))
			for _, c := range commits {
				repoStats.Additions += int64(c.Stats.Additions)
				repoStats.Deletions += int64(c.Stats.Deletions)
			}
			if len(commits) < 100 {
				break
			}
		}

		if repoFailed {
			failed++
			stats.Partial = true
			if c, ok := gitCachedRepoStats(p.store, p.Key(), key, 0); ok {
				addRepo(c)
			}
			continue
		}

		repoStats.UpdatedAt = time.Now().Unix()
		addRepo(repoStats)
		fetched++
		if p.store != nil {
			p.store.SetRepoStats(p.Key(), key, repoStats)
		}
	}
	fmt.Fprintf(os.Stderr, "\r[Git] %s stats: %d/%d projects processed\n", p.Key(), len(projects), len(projects))
	log.Printf("[Git] %s: project stats resolved: %d from cache, %d fetched, %d failed", p.Key(), fromCache, fetched, failed)
	return stats, nil
}

func (p *GitLabProvider) FetchRecentRepos(ctx context.Context, since time.Time) ([]GitRecentRepo, error) {
	if p.cfg.Private {
		return nil, nil
	}

	id, err := p.resolveUserID(ctx)
	if err != nil {
		return nil, err
	}

	var projects []struct {
		ID                int       `json:"id"`
		Path              string    `json:"path"`
		WebURL            string    `json:"web_url"`
		Visibility        string    `json:"visibility"`
		StarCount         int       `json:"star_count"`
		LastActivityAt    time.Time `json:"last_activity_at"`
		PathWithNamespace string    `json:"path_with_namespace"`
	}
	endpoint := fmt.Sprintf("%s/api/v4/users/%d/projects?order_by=last_activity_at&sort=desc&per_page=30", p.base, id)
	if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &projects); err != nil {
		return nil, err
	}

	sinceStr := since.Format(time.RFC3339)
	author := url.QueryEscape(p.cfg.Username)
	out := make([]GitRecentRepo, 0, gitRecentReposCap)

	for _, proj := range projects {
		if proj.Visibility != "public" || proj.LastActivityAt.Before(since) {
			continue
		}

		repo := GitRecentRepo{
			Name:        proj.Path,
			URL:         proj.WebURL,
			Stars:       proj.StarCount,
			LastActive:  proj.LastActivityAt,
			Source:      p.Label(),
			SourceColor: p.Color(),
		}

		var commits []struct {
			CommittedDate time.Time `json:"committed_date"`
		}
		commitsEndpoint := fmt.Sprintf("%s/api/v4/projects/%d/repository/commits?since=%s&author=%s&per_page=100",
			p.base, proj.ID, url.QueryEscape(sinceStr), author)
		if err := gitDoJSON(ctx, p.client, "GET", commitsEndpoint, p.headers(), nil, &commits); err == nil {
			repo.Commits = len(commits)
			for _, c := range commits {
				if c.CommittedDate.After(repo.LastActive) {
					repo.LastActive = c.CommittedDate
				}
			}
		}
		if repo.Commits == 0 {
			continue
		}

		var langs map[string]float64
		langEndpoint := fmt.Sprintf("%s/api/v4/projects/%d/languages", p.base, proj.ID)
		if err := gitDoJSON(ctx, p.client, "GET", langEndpoint, p.headers(), nil, &langs); err == nil && len(langs) > 0 {
			for name, pct := range langs {
				repo.Languages = append(repo.Languages, CodeLangStat{
					Name:    name,
					Color:   GetLanguageColor(name),
					Percent: pct,
				})
			}
			sort.Slice(repo.Languages, func(i, j int) bool {
				return repo.Languages[i].Percent > repo.Languages[j].Percent
			})
			if len(repo.Languages) > 6 {
				repo.Languages = repo.Languages[:6]
			}
			repo.MainLang = repo.Languages[0].Name
		}

		out = append(out, repo)
		if len(out) >= gitRecentReposCap {
			break
		}
	}
	return out, nil
}

type glProj struct {
	ID                int    `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
}

func (p *GitLabProvider) allProjects(ctx context.Context) ([]glProj, bool) {
	seen := map[int]bool{}
	var out []glProj
	partial := false

	add := func(list []glProj) {
		for _, pr := range list {
			if !seen[pr.ID] {
				seen[pr.ID] = true
				out = append(out, pr)
			}
		}
	}

	id, err := p.resolveUserID(ctx)
	if err != nil {
		return nil, true
	}
	for page := 1; ; page++ {
		var chunk []glProj
		endpoint := fmt.Sprintf("%s/api/v4/users/%d/projects?per_page=100&page=%d", p.base, id, page)
		if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &chunk); err != nil {
			partial = true
			break
		}
		if len(chunk) == 0 {
			break
		}
		add(chunk)
		if len(chunk) < 100 {
			break
		}
	}
	if p.cfg.Token != "" {
		for page := 1; ; page++ {
			var chunk []glProj
			endpoint := fmt.Sprintf("%s/api/v4/projects?membership=true&simple=true&per_page=100&page=%d", p.base, page)
			if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &chunk); err != nil {
				partial = true
				break
			}
			if len(chunk) == 0 {
				break
			}
			add(chunk)
			if len(chunk) < 100 {
				break
			}
		}
	}
	return out, partial
}
