package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

// parseProjectDate parses a stored project date in ISO "2006-01-02", RFC3339, or
// the old "02 Jan 2006" display form.
func parseProjectDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05Z", "02 Jan 2006", "2 Jan 2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// fmtProjectDate normalizes a stored date into a consistent "02 Jan 2006" string.
// Unparseable values are returned as-is so nothing is silently dropped.
func fmtProjectDate(s string) string {
	if t, ok := parseProjectDate(s); ok {
		return t.Format("02 Jan 2006")
	}
	return strings.TrimSpace(s)
}

var ghRepoRe = regexp.MustCompile(`(?i)github\.com/([^/]+)/([^/#?]+)`)

// parseGitHubRepo extracts owner, repo, and an optional subfolder path from a
// github.com URL. A link like .../tree/<branch>/<dir> yields path=<dir>, so
// dates come from that folder's commits rather than the whole (mono)repo.
func parseGitHubRepo(u string) (owner, repo, path string) {
	m := ghRepoRe.FindStringSubmatch(u)
	if m == nil {
		return "", "", ""
	}
	owner = m[1]
	repo = strings.TrimSuffix(m[2], ".git")
	if i := strings.Index(u, "/tree/"); i >= 0 {
		rest := u[i+len("/tree/"):] // <branch>/<path...>
		if parts := strings.SplitN(rest, "/", 2); len(parts) == 2 {
			path = strings.Trim(parts[1], "/")
		}
	}
	return owner, repo, path
}

type ProjectsPlugin struct {
	storage *storage.Storage
	hub     *stream.Hub
}

func NewProjectsPlugin(storage *storage.Storage, hub *stream.Hub) *ProjectsPlugin {
	return &ProjectsPlugin{
		storage: storage,
		hub:     hub,
	}
}

func (p *ProjectsPlugin) Name() string {
	return "projects"
}

func (p *ProjectsPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	projects, ok := config.Settings["projects"].([]interface{})
	if !ok {
		return "", nil
	}

	tmpl := `
	<div class="projects-section plugin" data-w="3">
		<div class="plugin-header">
			<h3 class="plugin-title">Projects</h3>
		</div>
		<div class="plugin__inner">
			<div class="projects-grid">
				{{range .Projects}}
				<article class="project-card">
					{{if .Image}}
					<div class="project-image">
						<img src="{{.Image}}" alt="{{.Name}}" loading="lazy">
					</div>
					{{end}}
					<div class="project-content">
						<h3 class="project-title">{{.Name}}</h3>
						<p class="project-description">{{.Description}}</p>
						<div class="project-links">
							{{if .GitHub}}
							<a href="{{.GitHub}}" target="_blank" rel="noopener" class="project-link project-link--github">
								<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor">
									<path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/>
								</svg>
								Source
							</a>
							{{end}}
							{{if .Live}}
							<a href="{{.Live}}" target="_blank" rel="noopener" class="project-link project-link--live">
								<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor">
									<path d="M14 3v2h3.59l-9.83 9.83 1.41 1.41L19 6.41V10h2V3m-2 16H5V5h7V3H5c-1.11 0-2 .89-2 2v14c0 1.11.89 2 2 2h14c1.11 0 2-.89 2-2v-7h-2v7Z"/>
								</svg>
								Demo
							</a>
							{{end}}
						</div>
						{{if .Technologies}}
						<div class="project-tech">
							{{range .Technologies}}
							<span class="tech-tag">{{.}}</span>
							{{end}}
						</div>
						{{end}}
					</div>
					{{if or .Created .Updated}}
					<div class="project-footer">
						<div class="project-dates">
							{{if .Created}}<span class="project-date">Created {{.Created}}</span>{{end}}
							{{if .Updated}}<span class="project-date">Updated {{.Updated}}</span>{{end}}
						</div>
					</div>
					{{end}}
				</article>
				{{end}}
			</div>
		</div>
	</div>`

	type project struct {
		Name         string
		Description  string
		Image        string
		GitHub       string
		Live         string
		Technologies []string
		Created      string
		Updated      string
		upd          time.Time // sort key (unexported; ignored by template)
	}

	var projectList []project
	for _, proj := range projects {
		projMap, ok := proj.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := projMap["name"].(string)
		desc, _ := projMap["description"].(string)
		image, _ := projMap["image"].(string)
		github, _ := projMap["github"].(string)
		live, _ := projMap["live"].(string)
		created, _ := projMap["created"].(string)
		updated, _ := projMap["updated"].(string)

		var technologies []string
		if techs, ok := projMap["technologies"].([]interface{}); ok {
			for _, tech := range techs {
				if techStr, ok := tech.(string); ok {
					technologies = append(technologies, techStr)
				}
			}
		}

		updT, _ := parseProjectDate(updated)
		projectList = append(projectList, project{
			Name:         name,
			Description:  desc,
			Image:        image,
			GitHub:       github,
			Live:         live,
			Technologies: technologies,
			Created:      fmtProjectDate(created),
			Updated:      fmtProjectDate(updated),
			upd:          updT,
		})
	}

	// Most recently updated first; undated projects keep their relative order at the end.
	sort.SliceStable(projectList, func(i, j int) bool {
		return projectList[i].upd.After(projectList[j].upd)
	})

	funcMap := template.FuncMap{
		"printf": fmt.Sprintf,
	}

	tmplParsed, err := template.New("projects").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = tmplParsed.Execute(&buf, struct {
		Projects []project
	}{
		Projects: projectList,
	})
	return buf.String(), err
}

// UpdateData refreshes each project's "last updated" (and fills a missing
// "created") from its GitHub repo's pushed_at/created_at. Called daily from the
// background scheduler. Only advances "updated" forward, never regresses it.
func (p *ProjectsPlugin) UpdateData(ctx context.Context) error {
	config := p.storage.GetPluginConfig(p.Name())
	projects, ok := config.Settings["projects"].([]interface{})
	if !ok || len(projects) == 0 {
		return nil
	}

	token := os.Getenv("GITHUB_TOKEN")
	client := &http.Client{Timeout: 15 * time.Second}
	changed := false

	for _, proj := range projects {
		m, ok := proj.(map[string]interface{})
		if !ok {
			continue
		}
		gh, _ := m["github"].(string)
		owner, repo, path := parseGitHubRepo(gh)
		if owner == "" || repo == "" {
			continue
		}

		var created, updated string
		var err error
		if path != "" {
			// Folder link inside a (mono)repo: use that folder's commit history.
			created, updated, err = fetchPathDates(ctx, client, token, owner, repo, path)
		} else {
			created, updated, err = fetchRepoDates(ctx, client, token, owner, repo)
		}
		if err != nil {
			continue
		}

		if updated != "" {
			if cur, _ := m["updated"].(string); cur != updated {
				m["updated"] = updated
				changed = true
			}
		}
		if created != "" {
			cur, _ := m["created"].(string)
			// For folder links the earliest path-commit is authoritative (correct
			// it in either direction); for whole-repo links keep any curated value
			// and only fill when missing.
			if (path != "" && cur != created) || (path == "" && cur == "") {
				m["created"] = created
				changed = true
			}
		}
	}

	if !changed {
		return nil
	}
	config.Settings["projects"] = projects
	return p.storage.SetPluginConfig(p.Name(), config)
}

// ghGet issues an authenticated GET against the GitHub API.
func ghGet(ctx context.Context, client *http.Client, token, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "about-app")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return client.Do(req)
}

func dateOnly(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return ""
}

// fetchRepoDates returns a whole repo's created_at and pushed_at as YYYY-MM-DD.
func fetchRepoDates(ctx context.Context, client *http.Client, token, owner, repo string) (created, updated string, err error) {
	resp, err := ghGet(ctx, client, token, "https://api.github.com/repos/"+owner+"/"+repo)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return "", "", fmt.Errorf("github %s/%s: %s", owner, repo, resp.Status)
	}
	var d struct {
		CreatedAt string `json:"created_at"`
		PushedAt  string `json:"pushed_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return "", "", err
	}
	return dateOnly(d.CreatedAt), dateOnly(d.PushedAt), nil
}

type ghCommit struct {
	Commit struct {
		Committer struct {
			Date string `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

var linkLastRe = regexp.MustCompile(`[?&]page=(\d+)[^>]*>;\s*rel="last"`)

func parseLastPage(link string) int {
	if m := linkLastRe.FindStringSubmatch(link); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 1
}

// fetchPathDates returns the earliest (created) and latest (updated) commit
// dates that touched a folder within a repo, as YYYY-MM-DD. This is what makes a
// project linked to a monorepo subfolder reflect that folder's history rather
// than the whole repo's last push.
func fetchPathDates(ctx context.Context, client *http.Client, token, owner, repo, path string) (created, updated string, err error) {
	base := "https://api.github.com/repos/" + owner + "/" + repo + "/commits?per_page=1&path=" + url.QueryEscape(path)
	resp, err := ghGet(ctx, client, token, base)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return "", "", fmt.Errorf("github commits %s/%s (%s): %s", owner, repo, path, resp.Status)
	}
	var commits []ghCommit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return "", "", err
	}
	if len(commits) == 0 {
		return "", "", nil
	}
	updated = dateOnly(commits[0].Commit.Committer.Date)
	created = updated

	// Oldest commit for the path lives on the last page (per_page=1).
	if last := parseLastPage(resp.Header.Get("Link")); last > 1 {
		resp2, err := ghGet(ctx, client, token, base+"&page="+strconv.Itoa(last))
		if err == nil {
			defer resp2.Body.Close()
			if resp2.StatusCode == http.StatusOK {
				var oldest []ghCommit
				if json.NewDecoder(resp2.Body).Decode(&oldest) == nil && len(oldest) > 0 {
					created = dateOnly(oldest[0].Commit.Committer.Date)
				}
			} else {
				io.Copy(io.Discard, resp2.Body)
			}
		}
	}
	return created, updated, nil
}

func (p *ProjectsPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *ProjectsPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings
	return p.storage.SetPluginConfig(p.Name(), config)
}

func (p *ProjectsPlugin) RenderText(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	projects, ok := config.Settings["projects"].([]interface{})
	if !ok || len(projects) == 0 {
		return "Projects: No projects configured", nil
	}

	var projectNames []string
	for _, proj := range projects {
		projMap, ok := proj.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := projMap["name"].(string); ok {
			projectNames = append(projectNames, name)
		}
	}

	if len(projectNames) == 0 {
		return "Projects: No valid projects", nil
	}

	return fmt.Sprintf("Projects: %s (%d total)", strings.Join(projectNames, ", "), len(projectNames)), nil
}

func (p *ProjectsPlugin) GetMetrics() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	projects, ok := config.Settings["projects"].([]interface{})

	metrics := map[string]interface{}{
		"total_projects":       0,
		"projects_with_github": 0,
		"projects_with_demo":   0,
	}

	if !ok {
		return metrics
	}

	metrics["total_projects"] = len(projects)

	githubCount := 0
	demoCount := 0

	for _, proj := range projects {
		projMap, ok := proj.(map[string]interface{})
		if !ok {
			continue
		}
		if github, ok := projMap["github"].(string); ok && github != "" {
			githubCount++
		}
		if live, ok := projMap["live"].(string); ok && live != "" {
			demoCount++
		}
	}

	metrics["projects_with_github"] = githubCount
	metrics["projects_with_demo"] = demoCount

	return metrics
}
