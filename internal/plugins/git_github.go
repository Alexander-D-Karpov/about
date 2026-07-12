package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

type GitHubProvider struct {
	cfg      gitSourceConfig
	client   *http.Client
	api      string
	gql      string
	htmlBase string
	store    *CodeStatsStore
}

func NewGitHubProvider(cfg gitSourceConfig, client *http.Client) *GitHubProvider {
	api := "https://api.github.com"
	gql := "https://api.github.com/graphql"
	htmlBase := "https://github.com"

	if cfg.BaseURL != "" && !strings.Contains(cfg.BaseURL, "github.com") {
		api = cfg.BaseURL + "/api/v3"
		gql = cfg.BaseURL + "/api/graphql"
		htmlBase = cfg.BaseURL
	}

	return &GitHubProvider{cfg: cfg, client: client, api: api, gql: gql, htmlBase: htmlBase}
}

func (p *GitHubProvider) Key() string   { return "github:" + p.cfg.Name }
func (p *GitHubProvider) Label() string { return p.cfg.Name }
func (p *GitHubProvider) Color() string { return p.cfg.Color }
func (p *GitHubProvider) Private() bool { return p.cfg.Private }

func (p *GitHubProvider) SetStatsStore(s *CodeStatsStore) { p.store = s }

func (p *GitHubProvider) headers() map[string]string {
	h := map[string]string{"X-GitHub-Api-Version": "2022-11-28"}
	if p.cfg.Token != "" {
		h["Authorization"] = "Bearer " + p.cfg.Token
	}
	return h
}

func (p *GitHubProvider) FetchCalendar(ctx context.Context, from, to time.Time) (map[string]int, error) {
	if p.cfg.Token == "" {
		return nil, fmt.Errorf("github calendar requires a token")
	}

	out := make(map[string]int)
	chunkEnd := to
	for chunkEnd.After(from) {
		chunkStart := chunkEnd.AddDate(0, 0, -364)
		if chunkStart.Before(from) {
			chunkStart = from
		}
		cal, err := p.fetchCalendarChunk(ctx, chunkStart, chunkEnd)
		if err != nil {
			if len(out) == 0 {
				return nil, err
			}
			break
		}
		for d, c := range cal {
			out[d] = c
		}
		chunkEnd = chunkStart.AddDate(0, 0, -1)
	}
	return out, nil
}

type ghEvent struct {
	Type string `json:"type"`
	Repo struct {
		Name string `json:"name"`
	} `json:"repo"`
	Public    bool            `json:"public"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

func (p *GitHubProvider) fetchEventsPage(ctx context.Context, endpoint string) ([]ghEvent, error) {
	var events []ghEvent
	err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &events)
	return events, err
}

func (p *GitHubProvider) FetchActivities(ctx context.Context, since time.Time, limit int) ([]GitActivityItem, error) {
	var events []ghEvent
	for page := 1; page <= 3; page++ {
		endpoint := fmt.Sprintf("%s/users/%s/events?per_page=100&page=%d", p.api, p.cfg.Username, page)
		chunk, err := p.fetchEventsPage(ctx, endpoint)
		if err != nil {
			if page == 1 {
				endpoint = fmt.Sprintf("%s/users/%s/events/public?per_page=100&page=%d", p.api, p.cfg.Username, page)
				chunk, err = p.fetchEventsPage(ctx, endpoint)
				if err != nil {
					log.Printf("[Git] github events fetch failed: %v", err)
				}
			}
		}
		if len(chunk) == 0 {
			break
		}
		events = append(events, chunk...)
		if len(chunk) < 100 || chunk[len(chunk)-1].CreatedAt.Before(since) {
			break
		}
	}
	log.Printf("[Git] github raw events: %d", len(events))

	items := make([]GitActivityItem, 0, len(events))
	pushSeen := make(map[string]bool)

	for _, ev := range events {
		if ev.CreatedAt.Before(since) {
			continue
		}

		item := GitActivityItem{
			Time:    ev.CreatedAt,
			Repo:    gitShortRepo(ev.Repo.Name),
			URL:     p.htmlBase + "/" + ev.Repo.Name,
			Source:  p.Label(),
			Color:   p.Color(),
			Private: p.cfg.Private || !ev.Public,
		}

		switch ev.Type {
		case "PushEvent":
			var pd struct {
				Ref          string `json:"ref"`
				Size         int    `json:"size"`
				DistinctSize int    `json:"distinct_size"`
				Head         string `json:"head"`
				Before       string `json:"before"`
				Commits      []struct {
					SHA     string `json:"sha"`
					Message string `json:"message"`
				} `json:"commits"`
			}
			json.Unmarshal(ev.Payload, &pd)
			n := pd.DistinctSize
			if n == 0 {
				n = pd.Size
			}
			if n == 0 {
				continue
			}
			item.Type = "push"
			item.Commits = n
			item.Ref = gitShortRef(pd.Ref)
			item.Title = fmt.Sprintf("Pushed %d commit%s to %s", n, gitPlural(n), item.Repo)
			if n == 1 && len(pd.Commits) > 0 {
				if msg := gitFirstLine(pd.Commits[len(pd.Commits)-1].Message); msg != "" {
					item.Title = msg
				}
			}
			switch {
			case n > 1 && pd.Before != "" && pd.Head != "":
				item.URL = p.htmlBase + "/" + ev.Repo.Name + "/compare/" + pd.Before + "..." + pd.Head
			case pd.Head != "":
				item.URL = p.htmlBase + "/" + ev.Repo.Name + "/commit/" + pd.Head
			case len(pd.Commits) > 0:
				item.URL = p.htmlBase + "/" + ev.Repo.Name + "/commit/" + pd.Commits[len(pd.Commits)-1].SHA
			}
			pushSeen[ev.Repo.Name+"|"+ev.CreatedAt.Format("2006-01-02")] = true

		case "WatchEvent":
			item.Type = "star"
			item.Title = "Starred " + ev.Repo.Name

		case "PullRequestEvent":
			var pd struct {
				Action      string `json:"action"`
				PullRequest struct {
					Number    int    `json:"number"`
					Title     string `json:"title"`
					Merged    bool   `json:"merged"`
					HTMLURL   string `json:"html_url"`
					Additions int    `json:"additions"`
					Deletions int    `json:"deletions"`
				} `json:"pull_request"`
			}
			json.Unmarshal(ev.Payload, &pd)
			item.Additions = pd.PullRequest.Additions
			item.Deletions = pd.PullRequest.Deletions
			if pd.PullRequest.HTMLURL != "" {
				item.URL = pd.PullRequest.HTMLURL
			}
			switch {
			case pd.Action == "opened":
				item.Type = "pr"
				item.Title = fmt.Sprintf("Opened PR #%d: %s", pd.PullRequest.Number, pd.PullRequest.Title)
			case pd.Action == "closed" && pd.PullRequest.Merged:
				item.Type = "merge"
				item.Title = fmt.Sprintf("Merged PR #%d: %s", pd.PullRequest.Number, pd.PullRequest.Title)
			case pd.Action == "closed":
				item.Type = "pr"
				item.Title = fmt.Sprintf("Closed PR #%d: %s", pd.PullRequest.Number, pd.PullRequest.Title)
			default:
				continue
			}

		case "IssuesEvent":
			var pd struct {
				Action string `json:"action"`
				Issue  struct {
					Number  int    `json:"number"`
					Title   string `json:"title"`
					HTMLURL string `json:"html_url"`
				} `json:"issue"`
			}
			json.Unmarshal(ev.Payload, &pd)
			if pd.Action != "opened" && pd.Action != "closed" {
				continue
			}
			if pd.Issue.HTMLURL != "" {
				item.URL = pd.Issue.HTMLURL
			}
			item.Type = "issue"
			verb := "Opened"
			if pd.Action == "closed" {
				verb = "Closed"
			}
			item.Title = fmt.Sprintf("%s issue #%d: %s", verb, pd.Issue.Number, pd.Issue.Title)

		case "CreateEvent":
			var pd struct {
				RefType string `json:"ref_type"`
			}
			json.Unmarshal(ev.Payload, &pd)
			if pd.RefType != "repository" {
				continue
			}
			item.Type = "create"
			item.Title = "Created repository " + item.Repo

		case "ReleaseEvent":
			var pd struct {
				Action  string `json:"action"`
				Release struct {
					TagName string `json:"tag_name"`
					HTMLURL string `json:"html_url"`
				} `json:"release"`
			}
			json.Unmarshal(ev.Payload, &pd)
			if pd.Action != "published" {
				continue
			}
			if pd.Release.HTMLURL != "" {
				item.URL = pd.Release.HTMLURL
			}
			item.Type = "release"
			item.Title = fmt.Sprintf("Released %s in %s", pd.Release.TagName, item.Repo)

		default:
			continue
		}

		items = append(items, item)
	}

	searchItems := p.pushesFromCommitSearch(ctx, since, pushSeen)
	items = append(items, searchItems...)
	log.Printf("[Git] github feed: %d from events, %d from commit search", len(items)-len(searchItems), len(searchItems))

	sort.Slice(items, func(i, j int) bool { return items[i].Time.After(items[j].Time) })
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (p *GitHubProvider) pushesFromCommitSearch(ctx context.Context, since time.Time, pushSeen map[string]bool) []GitActivityItem {
	q := fmt.Sprintf("author:%s committer-date:>=%s", p.cfg.Username, since.Format("2006-01-02"))
	var res struct {
		Items []struct {
			HTMLURL   string `json:"html_url"`
			Committer *struct {
				Login string `json:"login"`
			} `json:"committer"`
			Commit struct {
				Message   string `json:"message"`
				Committer struct {
					Name string    `json:"name"`
					Date time.Time `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
			Repository struct {
				Name     string `json:"name"`
				FullName string `json:"full_name"`
				HTMLURL  string `json:"html_url"`
				Private  bool   `json:"private"`
			} `json:"repository"`
		} `json:"items"`
	}
	endpoint := fmt.Sprintf("%s/search/commits?q=%s&sort=committer-date&order=desc&per_page=100",
		p.api, url.QueryEscape(q))
	if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &res); err != nil {
		log.Printf("[Git] github commit search for feed failed: %v", err)
		return nil
	}

	type agg struct {
		count           int
		latest          time.Time
		url             string
		latestCommitURL string
		latestMsg       string
		repo            string
		private         bool
	}
	groups := map[string]*agg{}
	order := []string{}

	for _, it := range res.Items {
		committerLogin := ""
		if it.Committer != nil {
			committerLogin = it.Committer.Login
		}
		if isBotCommit(committerLogin, it.Commit.Committer.Name) {
			continue
		}
		day := it.Commit.Committer.Date.Format("2006-01-02")
		key := it.Repository.FullName + "|" + day
		if pushSeen[key] {
			continue
		}
		g, ok := groups[key]
		if !ok {
			g = &agg{
				url:     it.Repository.HTMLURL,
				repo:    it.Repository.Name,
				private: it.Repository.Private,
			}
			groups[key] = g
			order = append(order, key)
		}
		g.count++
		if it.Commit.Committer.Date.After(g.latest) {
			g.latest = it.Commit.Committer.Date
			g.latestCommitURL = it.HTMLURL
			g.latestMsg = gitFirstLine(it.Commit.Message)
		}
	}

	out := make([]GitActivityItem, 0, len(order))
	for _, key := range order {
		g := groups[key]
		itemURL := g.url
		if g.latestCommitURL != "" {
			itemURL = g.latestCommitURL
		}
		title := fmt.Sprintf("Pushed %d commit%s to %s", g.count, gitPlural(g.count), g.repo)
		if g.count == 1 && g.latestMsg != "" {
			title = g.latestMsg
		}
		out = append(out, GitActivityItem{
			Type:    "push",
			Title:   title,
			Repo:    g.repo,
			URL:     itemURL,
			Time:    g.latest,
			Commits: g.count,
			Source:  p.Label(),
			Color:   p.Color(),
			Private: p.cfg.Private || g.private,
		})
	}
	return out
}

func (p *GitHubProvider) FetchStats(ctx context.Context) (*GitSourceStats, error) {
	stats := &GitSourceStats{}

	type ghRepo struct {
		FullName string `json:"full_name"`
		Fork     bool   `json:"fork"`
	}

	var repos []ghRepo
	for page := 1; ; page++ {
		endpoint := fmt.Sprintf("%s/users/%s/repos?per_page=100&page=%d&type=owner", p.api, p.cfg.Username, page)
		if p.cfg.Token != "" {
			endpoint = fmt.Sprintf("%s/user/repos?per_page=100&page=%d&affiliation=owner,collaborator,organization_member", p.api, page)
		}
		var chunk []ghRepo
		if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &chunk); err != nil {
			if page == 1 {
				return nil, err
			}
			stats.Partial = true
			break
		}
		if len(chunk) == 0 {
			break
		}
		repos = append(repos, chunk...)
		if len(chunk) < 100 {
			break
		}
	}
	stats.Repos = len(repos)
	log.Printf("[Git] %s: %d repos to process", p.Key(), len(repos))

	gqlCommits, gqlOK := p.fetchTotalCommitsGraphQL(ctx)
	if gqlOK {
		stats.Commits = gqlCommits
		log.Printf("[Git] %s: total commits via graphql (full history): %d", p.Key(), gqlCommits)
	}

	login := strings.ToLower(p.cfg.Username)
	var restCommits int64
	failed := 0
	timedOut := false
	fromCache := 0
	fetched := 0

	addRepo := func(c RepoStats) {
		restCommits += c.Commits
		stats.Additions += c.Additions
		stats.Deletions += c.Deletions
	}

	for i, r := range repos {
		fmt.Fprintf(os.Stderr, "\r[Git] %s stats: %d/%d repos processed", p.Key(), i, len(repos))

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
			timedOut = true
			stats.Partial = true
			if c, ok := gitCachedRepoStats(p.store, p.Key(), r.FullName, 0); ok {
				addRepo(c)
			}
			continue
		}

		var contribs []struct {
			Total int `json:"total"`
			Weeks []struct {
				A int `json:"a"`
				D int `json:"d"`
			} `json:"weeks"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
		}
		endpoint := fmt.Sprintf("%s/repos/%s/stats/contributors", p.api, r.FullName)
		var err error
		for attempt := 0; attempt < 12; attempt++ {
			err = gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &contribs)
			if err != errGitStatsPending {
				break
			}
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
			break
		}
		if err != nil {
			failed++
			stats.Partial = true
			if c, ok := gitCachedRepoStats(p.store, p.Key(), r.FullName, 0); ok {
				addRepo(c)
			}
			continue
		}

		var repoStats RepoStats
		for _, c := range contribs {
			if strings.ToLower(c.Author.Login) != login {
				continue
			}
			repoStats.Commits += int64(c.Total)
			for _, w := range c.Weeks {
				repoStats.Additions += int64(w.A)
				repoStats.Deletions += int64(w.D)
			}
		}
		repoStats.UpdatedAt = time.Now().Unix()
		addRepo(repoStats)
		fetched++
		if p.store != nil {
			p.store.SetRepoStats(p.Key(), r.FullName, repoStats)
		}
	}
	fmt.Fprintf(os.Stderr, "\r[Git] %s stats: %d/%d repos processed\n", p.Key(), len(repos), len(repos))
	log.Printf("[Git] %s: repo stats resolved: %d from cache, %d fetched, %d failed, timedOut=%t",
		p.Key(), fromCache, fetched, failed, timedOut)

	if !gqlOK {
		stats.Commits = restCommits
	}
	if failed > 0 {
		log.Printf("[Git] %s: %d repo(s) failed contributor stats, lines changed is partial", p.Key(), failed)
	}
	if gqlOK && failed == 0 && !timedOut {
		stats.Partial = false
	}
	return stats, nil
}

func (p *GitHubProvider) FetchDayDetails(ctx context.Context, date string) ([]GitActivityItem, error) {
	day, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return nil, err
	}
	fromStr := day.AddDate(0, 0, -1).Format("2006-01-02")
	toStr := day.AddDate(0, 0, 1).Format("2006-01-02")

	q := fmt.Sprintf("author:%s committer-date:%s..%s", p.cfg.Username, fromStr, toStr)
	endpoint := fmt.Sprintf("%s/search/commits?q=%s&sort=committer-date&order=desc&per_page=100",
		p.api, url.QueryEscape(q))
	var res struct {
		Items []struct {
			SHA       string `json:"sha"`
			HTMLURL   string `json:"html_url"`
			Committer *struct {
				Login string `json:"login"`
			} `json:"committer"`
			Commit struct {
				Message   string `json:"message"`
				Committer struct {
					Name string    `json:"name"`
					Date time.Time `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
			Repository struct {
				FullName string `json:"full_name"`
				Private  bool   `json:"private"`
			} `json:"repository"`
		} `json:"items"`
	}
	if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &res); err != nil {
		return nil, err
	}
	items := make([]GitActivityItem, 0, len(res.Items))
	detailed := 0
	for _, it := range res.Items {
		committerLogin := ""
		if it.Committer != nil {
			committerLogin = it.Committer.Login
		}
		if isBotCommit(committerLogin, it.Commit.Committer.Name) {
			continue
		}
		if it.Commit.Committer.Date.Local().Format("2006-01-02") != date {
			continue
		}
		title := gitFirstLine(it.Commit.Message)
		item := GitActivityItem{
			Type:    "push",
			Title:   title,
			Repo:    gitShortRepo(it.Repository.FullName),
			URL:     it.HTMLURL,
			Time:    it.Commit.Committer.Date,
			Commits: 1,
			Source:  p.Label(),
			Color:   p.Color(),
			Private: p.cfg.Private || it.Repository.Private,
		}
		if detailed < 12 && !item.Private {
			var detail struct {
				Stats struct {
					Additions int `json:"additions"`
					Deletions int `json:"deletions"`
				} `json:"stats"`
			}
			commitEndpoint := fmt.Sprintf("%s/repos/%s/commits/%s", p.api, it.Repository.FullName, it.SHA)
			if err := gitDoJSON(ctx, p.client, "GET", commitEndpoint, p.headers(), nil, &detail); err == nil {
				item.Additions = detail.Stats.Additions
				item.Deletions = detail.Stats.Deletions
			}
			detailed++
		}
		items = append(items, item)
	}
	return items, nil
}

func (p *GitHubProvider) FetchRecentRepos(ctx context.Context, since time.Time) ([]GitRecentRepo, error) {
	q := fmt.Sprintf("author:%s committer-date:>=%s", p.cfg.Username, since.Format("2006-01-02"))

	type agg struct {
		repo    GitRecentRepo
		full    string
		private bool
	}
	byName := map[string]*agg{}
	order := []string{}

	for page := 1; page <= 3; page++ {
		var res struct {
			Items []struct {
				Committer *struct {
					Login string `json:"login"`
				} `json:"committer"`
				Commit struct {
					Committer struct {
						Name string    `json:"name"`
						Date time.Time `json:"date"`
					} `json:"committer"`
				} `json:"commit"`
				Repository struct {
					Name     string `json:"name"`
					FullName string `json:"full_name"`
					HTMLURL  string `json:"html_url"`
					Private  bool   `json:"private"`
					Language string `json:"language"`
				} `json:"repository"`
			} `json:"items"`
		}
		endpoint := fmt.Sprintf("%s/search/commits?q=%s&sort=committer-date&order=desc&per_page=100&page=%d",
			p.api, url.QueryEscape(q), page)
		if err := gitDoJSON(ctx, p.client, "GET", endpoint, p.headers(), nil, &res); err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		if len(res.Items) == 0 {
			break
		}
		for _, it := range res.Items {
			committerLogin := ""
			if it.Committer != nil {
				committerLogin = it.Committer.Login
			}
			if isBotCommit(committerLogin, it.Commit.Committer.Name) {
				continue
			}
			r := it.Repository
			a, ok := byName[r.FullName]
			if !ok {
				a = &agg{
					repo: GitRecentRepo{
						Name:        r.Name,
						URL:         r.HTMLURL,
						MainLang:    r.Language,
						Source:      p.Label(),
						SourceColor: p.Color(),
					},
					full:    r.FullName,
					private: r.Private,
				}
				byName[r.FullName] = a
				order = append(order, r.FullName)
			}
			a.repo.Commits++
			if it.Commit.Committer.Date.After(a.repo.LastActive) {
				a.repo.LastActive = it.Commit.Committer.Date
			}
		}
		if len(res.Items) < 100 {
			break
		}
	}

	out := make([]GitRecentRepo, 0, gitRecentReposCap)
	for _, fullName := range order {
		a := byName[fullName]
		if a.private || p.cfg.Private {
			continue
		}
		repo := a.repo

		var meta struct {
			StargazersCount int    `json:"stargazers_count"`
			Language        string `json:"language"`
		}
		if err := gitDoJSON(ctx, p.client, "GET",
			fmt.Sprintf("%s/repos/%s", p.api, a.full), p.headers(), nil, &meta); err == nil {
			repo.Stars = meta.StargazersCount
			if repo.MainLang == "" {
				repo.MainLang = meta.Language
			}
		}

		var langBytes map[string]int64
		if err := gitDoJSON(ctx, p.client, "GET",
			fmt.Sprintf("%s/repos/%s/languages", p.api, a.full), p.headers(), nil, &langBytes); err == nil && len(langBytes) > 0 {
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
		if len(out) >= gitRecentReposCap {
			break
		}
	}
	return out, nil
}

func (p *GitHubProvider) fetchCalendarChunk(ctx context.Context, from, to time.Time) (map[string]int, error) {
	query := `query($login:String!,$from:DateTime!,$to:DateTime!){user(login:$login){contributionsCollection(from:$from,to:$to){contributionCalendar{weeks{contributionDays{date contributionCount}}}}}}`
	payload, _ := json.Marshal(map[string]interface{}{
		"query": query,
		"variables": map[string]string{
			"login": p.cfg.Username,
			"from":  from.Format(time.RFC3339),
			"to":    to.Format(time.RFC3339),
		},
	})

	var resp struct {
		Data struct {
			User struct {
				ContributionsCollection struct {
					ContributionCalendar struct {
						Weeks []struct {
							ContributionDays []struct {
								Date              string `json:"date"`
								ContributionCount int    `json:"contributionCount"`
							} `json:"contributionDays"`
						} `json:"weeks"`
					} `json:"contributionCalendar"`
				} `json:"contributionsCollection"`
			} `json:"user"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := gitDoJSON(ctx, p.client, "POST", p.gql, p.headers(), bytes.NewReader(payload), &resp); err != nil {
		return nil, err
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("github graphql: %s", resp.Errors[0].Message)
	}

	out := make(map[string]int)
	for _, w := range resp.Data.User.ContributionsCollection.ContributionCalendar.Weeks {
		for _, d := range w.ContributionDays {
			if d.ContributionCount > 0 {
				out[d.Date] = d.ContributionCount
			}
		}
	}
	return out, nil
}

func (p *GitHubProvider) fetchTotalCommitsGraphQL(ctx context.Context) (int64, bool) {
	if p.cfg.Token == "" {
		return 0, false
	}

	yearsQuery := `query($login:String!){user(login:$login){contributionsCollection{contributionYears}}}`
	payload, _ := json.Marshal(map[string]interface{}{
		"query":     yearsQuery,
		"variables": map[string]string{"login": p.cfg.Username},
	})
	var yearsResp struct {
		Data struct {
			User struct {
				ContributionsCollection struct {
					ContributionYears []int `json:"contributionYears"`
				} `json:"contributionsCollection"`
			} `json:"user"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := gitDoJSON(ctx, p.client, "POST", p.gql, p.headers(), bytes.NewReader(payload), &yearsResp); err != nil {
		log.Printf("[Git] %s: graphql contribution years failed: %v", p.Key(), err)
		return 0, false
	}
	if len(yearsResp.Errors) > 0 {
		log.Printf("[Git] %s: graphql contribution years error: %s", p.Key(), yearsResp.Errors[0].Message)
		return 0, false
	}

	years := yearsResp.Data.User.ContributionsCollection.ContributionYears
	if len(years) == 0 {
		return 0, false
	}
	sort.Ints(years)
	log.Printf("[Git] %s: contribution years on record: %v", p.Key(), years)

	commitsQuery := `query($login:String!,$from:DateTime!,$to:DateTime!){user(login:$login){contributionsCollection(from:$from,to:$to){totalCommitContributions restrictedContributionsCount}}}`
	var total int64
	for _, year := range years {
		select {
		case <-ctx.Done():
			return 0, false
		default:
		}

		from := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC)
		payload, _ := json.Marshal(map[string]interface{}{
			"query": commitsQuery,
			"variables": map[string]interface{}{
				"login": p.cfg.Username,
				"from":  from.Format(time.RFC3339),
				"to":    to.Format(time.RFC3339),
			},
		})
		var resp struct {
			Data struct {
				User struct {
					ContributionsCollection struct {
						TotalCommitContributions     int64 `json:"totalCommitContributions"`
						RestrictedContributionsCount int64 `json:"restrictedContributionsCount"`
					} `json:"contributionsCollection"`
				} `json:"user"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := gitDoJSON(ctx, p.client, "POST", p.gql, p.headers(), bytes.NewReader(payload), &resp); err != nil {
			log.Printf("[Git] %s: graphql commits for %d failed: %v", p.Key(), year, err)
			return 0, false
		}
		if len(resp.Errors) > 0 {
			log.Printf("[Git] %s: graphql commits for %d error: %s", p.Key(), year, resp.Errors[0].Message)
			return 0, false
		}
		cc := resp.Data.User.ContributionsCollection
		total += cc.TotalCommitContributions + cc.RestrictedContributionsCount
		log.Printf("[Git] %s: %d: commits=%d restricted=%d (running total %d)",
			p.Key(), year, cc.TotalCommitContributions, cc.RestrictedContributionsCount, total)
	}
	return total, true
}
