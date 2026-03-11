package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type CodePlugin struct {
	storage          *storage.Storage
	hub              *stream.Hub
	githubData       *GitHubUserData
	wakatimeData     *WakatimeData
	allRepoLanguages []LanguageStat
	lastUpdate       time.Time
	mutex            sync.RWMutex
}

type RepoLanguageBreakdown struct {
	Name       string
	Percentage float64
	Color      string
}

type GitHubUserData struct {
	Login        string            `json:"login"`
	Name         string            `json:"name"`
	PublicRepos  int               `json:"public_repos"`
	Followers    int               `json:"followers"`
	Following    int               `json:"following"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Bio          string            `json:"bio"`
	Location     string            `json:"location"`
	TotalCommits int               `json:"-"`
	TotalStars   int               `json:"-"`
	TopLanguages []LanguageStat    `json:"-"`
	RecentRepos  []GitHubRepo      `json:"-"`
	CommitStats  GitHubCommitStats `json:"-"`
}

type GitHubRepo struct {
	Name         string                  `json:"name"`
	Stars        int                     `json:"stargazers_count"`
	Language     string                  `json:"language"`
	UpdatedAt    time.Time               `json:"updated_at"`
	Description  string                  `json:"description"`
	LanguagesURL string                  `json:"languages_url"`
	Languages    []RepoLanguageBreakdown `json:"-"`
}

type GitHubCommitStats struct {
	TotalCommits    int            `json:"total_commits"`
	WeeklyCommits   []int          `json:"weekly_commits"`
	LanguageStats   map[string]int `json:"language_stats"`
	ContributionMap map[string]int `json:"contribution_map"`
}

type LanguageStat struct {
	Name       string  `json:"name"`
	Percentage float64 `json:"percentage"`
	Color      string  `json:"color"`
	Bytes      int     `json:"bytes"`
}

type WakatimeData struct {
	TotalTime struct {
		Seconds float64 `json:"seconds"`
		Text    string  `json:"text"`
	} `json:"total_time"`
	LastWeek struct {
		Seconds float64 `json:"seconds"`
		Text    string  `json:"text"`
	} `json:"last_week"`
	Languages []WakatimeLanguage `json:"languages"`
	Editors   []struct {
		Name         string  `json:"name"`
		TotalSeconds float64 `json:"total_seconds"`
		Percent      float64 `json:"percent"`
		Text         string  `json:"text"`
	} `json:"editors"`
	OperatingSystems []struct {
		Name         string  `json:"name"`
		TotalSeconds float64 `json:"total_seconds"`
		Percent      float64 `json:"percent"`
		Text         string  `json:"text"`
	} `json:"operating_systems"`
}

type WakatimeLanguage struct {
	Name         string  `json:"name"`
	TotalSeconds float64 `json:"total_seconds"`
	Percent      float64 `json:"percent"`
	Text         string  `json:"text"`
	Color        string  `json:"color"`
}

func NewCodePlugin(storage *storage.Storage, hub *stream.Hub) *CodePlugin {
	return &CodePlugin{
		storage: storage,
		hub:     hub,
	}
}

func (p *CodePlugin) Name() string {
	return "code"
}

func (p *CodePlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Coding Stats")
	showGitHub := p.getConfigBool(settings, "ui.showGitHub", true)
	showWakatime := p.getConfigBool(settings, "ui.showWakatime", true)
	showLanguages := p.getConfigBool(settings, "ui.showLanguages", true)

	githubUsername := p.getConfigValue(settings, "github.username", "")

	p.mutex.RLock()
	allRepoLanguages := p.allRepoLanguages
	p.mutex.RUnlock()

	tmpl := `
	<div class="code-section section" data-w="2">
		<div class="plugin-header">
			<h3 class="plugin-title">{{.SectionTitle}}</h3>
		</div>
		<div class="plugin__inner">
			{{if .AllRepoLanguages}}
			<div class="code-lang-summary">
				<div class="lang-summary-label">Languages across all repositories</div>
				<div class="lang-summary-bar">
					{{range .AllRepoLanguages}}
					<div class="lang-segment" style="flex: {{printf "%.2f" .Percentage}}; background: {{.Color}};" title="{{.Name}} {{printf "%.1f" .Percentage}}%"></div>
					{{end}}
				</div>
				<div class="lang-summary-legend">
					{{range .AllRepoLanguages}}
					<span class="lang-legend-item"><span class="lang-dot" style="background: {{.Color}};"></span>{{.Name}} <span class="lang-pct">{{printf "%.1f" .Percentage}}%</span></span>
					{{end}}
				</div>
			</div>
			{{end}}

			{{if and .ShowGitHub .GitHubData}}
			<div class="stats-overview">
				<div class="stat-card">
					<div class="stat-number">{{.GitHubData.PublicRepos}}</div>
					<div class="stat-label">Repos</div>
				</div>
				<div class="stat-card">
					<div class="stat-number">{{.GitHubData.TotalStars}}</div>
					<div class="stat-label">Stars</div>
				</div>
				<div class="stat-card">
					<div class="stat-number">{{.GitHubData.Followers}}</div>
					<div class="stat-label">Followers</div>
				</div>
				<div class="stat-card">
					<div class="stat-number">{{.GitHubData.TotalCommits}}</div>
					<div class="stat-label">Commits</div>
				</div>
			</div>
			{{end}}

			{{if and .ShowWakatime .WakatimeData}}
			<div class="time-summary">
				<div class="time-card">
					<span class="time-value">{{.WakatimeData.LastWeek.Text}}</span>
					<span class="time-label">this week</span>
				</div>
				<div class="time-card">
					<span class="time-value">{{.WakatimeData.TotalTime.Text}}</span>
					<span class="time-label">all time</span>
				</div>
			</div>
			{{end}}

			{{if and .ShowWakatime .WakatimeData .WakatimeData.Languages}}
			<div class="code-subsection">
				<button class="section-toggle" data-target="wakatime-langs" type="button" aria-expanded="true">
					<span class="toggle-icon">▼</span>
					<span>This Week</span>
					<span class="section-count">({{len .WakatimeData.Languages}} langs)</span>
				</button>
				<div class="collapsible-content" id="wakatime-langs">
					<div class="wakatime-list">
						{{range .WakatimeData.Languages}}
						{{if gt .Percent 1.0}}
						<div class="waka-item">
							<span class="waka-lang">{{.Name}}</span>
							<div class="waka-bar">
								<div class="waka-fill" style="width: {{.Percent}}%; background-color: {{.Color}};"></div>
							</div>
							<span class="waka-time">{{.Text}}</span>
						</div>
						{{end}}
						{{end}}
					</div>

					{{if .WakatimeData.Editors}}
					<div class="editor-list">
						{{range .WakatimeData.Editors}}
						{{if gt .Percent 5.0}}
						<span class="editor-chip">{{.Name}} {{printf "%.0f" .Percent}}%</span>
						{{end}}
						{{end}}
					</div>
					{{end}}
				</div>
			</div>
			{{end}}

			{{if and .ShowLanguages .GitHubData .GitHubData.TopLanguages}}
			<div class="code-subsection">
				<button class="section-toggle" data-target="languages" type="button" aria-expanded="false">
					<span class="toggle-icon">▶</span>
					<span>Top Languages</span>
					<span class="section-count">({{len .GitHubData.TopLanguages}})</span>
				</button>
				<div class="collapsible-content collapsed" id="languages">
					<div class="language-chart">
						{{range .GitHubData.TopLanguages}}
						<div class="language-item">
							<div class="lang-info">
								<span class="lang-name">{{.Name}}</span>
								<span class="lang-percent">{{printf "%.1f" .Percentage}}%</span>
							</div>
							<div class="lang-bar">
								<div class="lang-fill" style="width: {{.Percentage}}%; background-color: {{.Color}};"></div>
							</div>
						</div>
						{{end}}
					</div>
				</div>
			</div>
			{{end}}

			{{if and .ShowGitHub .GitHubData .GitHubData.RecentRepos}}
			<div class="code-subsection">
				<button class="section-toggle" data-target="repos" type="button" aria-expanded="false">
					<span class="toggle-icon">▶</span>
					<span>Recent Repos</span>
					<span class="section-count">({{len .GitHubData.RecentRepos}})</span>
				</button>
				<div class="collapsible-content collapsed" id="repos">
					<div class="repo-list">
						{{range .GitHubData.RecentRepos}}
						<div class="repo-item" onclick="window.open('https://github.com/{{$.GitHubUsername}}/{{.Name}}', '_blank')">
							<div class="repo-content">
								<div class="repo-name">{{.Name}}</div>
								{{if .Languages}}
								{{$total := 0.0}}
								{{range .Languages}}{{$total = add $total .Percentage}}{{end}}
								<div class="repo-lang-bar">
									{{range .Languages}}
									<div style="flex: {{.Percentage}}; background-color: {{.Color}};" title="{{.Name}} {{printf "%.1f" .Percentage}}%"></div>
									{{end}}
									{{if lt $total 100.0}}
									<div style="flex: {{printf "%.6f" (sub 100.0 $total)}}; background-color: #8b949e;" title="Other {{printf "%.1f" (sub 100.0 $total)}}%"></div>
									{{end}}
								</div>
								{{else if .Language}}
								<div class="repo-lang-bar">
									<div style="flex: 100; background-color: {{call $.GetLanguageColor .Language}};" title="{{.Language}} 100%"></div>
								</div>
								{{end}}
							</div>
							<div class="repo-tags">
								{{if .Language}}<span class="repo-lang">{{.Language}}</span>{{end}}
								{{if gt .Stars 0}}<span class="repo-stars">★{{.Stars}}</span>{{end}}
							</div>
						</div>
						{{end}}
					</div>
				</div>
			</div>
			{{end}}

			{{if not (or .GitHubData .WakatimeData)}}
			<div class="no-data">
				<p class="text-muted">Configure GitHub username or WakaTime API key to see coding statistics.</p>
			</div>
			{{end}}
		</div>
	</div>`

	data := struct {
		SectionTitle     string
		ShowGitHub       bool
		ShowWakatime     bool
		ShowLanguages    bool
		GitHubData       *GitHubUserData
		WakatimeData     *WakatimeData
		AllRepoLanguages []LanguageStat
		GitHubUsername   string
		GetLanguageColor func(string) string
	}{
		SectionTitle:     sectionTitle,
		ShowGitHub:       showGitHub,
		ShowWakatime:     showWakatime,
		ShowLanguages:    showLanguages,
		GitHubData:       p.githubData,
		WakatimeData:     p.wakatimeData,
		AllRepoLanguages: allRepoLanguages,
		GitHubUsername:   githubUsername,
		GetLanguageColor: GetLanguageColor,
	}

	funcMap := template.FuncMap{
		"printf": fmt.Sprintf,
		"add":    func(a, b float64) float64 { return a + b },
		"sub":    func(a, b float64) float64 { return a - b },
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"mul": func(a, b int) int { return a * b },
		"lt":  func(a, b float64) bool { return a < b },
	}

	t, err := template.New("code").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = t.Execute(&buf, data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (p *CodePlugin) checkRecentCommits(client *http.Client, username, repoName string, since time.Time) bool {
	commitsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits?since=%s&author=%s&per_page=1",
		username, repoName, since.Format(time.RFC3339), username)

	req, err := http.NewRequest("GET", commitsURL, nil)
	if err != nil {
		return false
	}

	config := p.storage.GetPluginConfig(p.Name())
	if token := p.getConfigValue(config.Settings, "github.token", ""); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	req.Header.Set("User-Agent", "AboutPage/1.0")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false
	}

	var commits []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return false
	}

	return len(commits) > 0
}

func (p *CodePlugin) UpdateData(ctx context.Context) error {
	if time.Since(p.lastUpdate) < 6*time.Hour {
		return nil
	}

	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	if githubUsername := p.getConfigValue(settings, "github.username", ""); githubUsername != "" {
		if err := p.updateGitHubData(githubUsername); err != nil {
			fmt.Printf("Warning: Failed to update GitHub data: %v\n", err)
		}
	}

	if wakatimeKey := p.getConfigValue(settings, "wakatime.api_key", ""); wakatimeKey != "" {
		if err := p.updateWakatimeData(wakatimeKey); err != nil {
			fmt.Printf("Warning: Failed to update WakaTime data: %v\n", err)
		}
	}

	p.lastUpdate = time.Now()
	return nil
}

func (p *CodePlugin) fetchAllRepoLanguages(client *http.Client, username string, repos []GitHubRepo) []LanguageStat {
	config := p.storage.GetPluginConfig(p.Name())
	token := p.getConfigValue(config.Settings, "github.token", "")

	allLanguageBytes := make(map[string]int)

	for _, repo := range repos {
		if repo.LanguagesURL == "" {
			continue
		}

		req, err := http.NewRequest("GET", repo.LanguagesURL, nil)
		if err != nil {
			continue
		}

		if token != "" {
			req.Header.Set("Authorization", "token "+token)
		}
		req.Header.Set("User-Agent", "AboutPage/1.0")
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			continue
		}

		var languages map[string]int
		if err := json.NewDecoder(resp.Body).Decode(&languages); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		for lang, bytes := range languages {
			allLanguageBytes[lang] += bytes
		}

		time.Sleep(50 * time.Millisecond)
	}

	totalBytes := 0
	for _, bytes := range allLanguageBytes {
		totalBytes += bytes
	}

	var avgLanguages []LanguageStat
	if totalBytes > 0 {
		for lang, bytes := range allLanguageBytes {
			percentage := float64(bytes) / float64(totalBytes) * 100
			if percentage >= 0.5 {
				avgLanguages = append(avgLanguages, LanguageStat{
					Name:       lang,
					Percentage: percentage,
					Color:      GetLanguageColor(lang),
					Bytes:      bytes,
				})
			}
		}

		for i := 0; i < len(avgLanguages); i++ {
			for j := i + 1; j < len(avgLanguages); j++ {
				if avgLanguages[i].Percentage < avgLanguages[j].Percentage {
					avgLanguages[i], avgLanguages[j] = avgLanguages[j], avgLanguages[i]
				}
			}
		}

		if len(avgLanguages) > 12 {
			avgLanguages = avgLanguages[:12]
		}
	}

	return avgLanguages
}

func (p *CodePlugin) updateGitHubData(username string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	fmt.Println("Updating github info...")

	userURL := fmt.Sprintf("https://api.github.com/users/%s", username)
	req, err := http.NewRequest("GET", userURL, nil)
	if err != nil {
		return err
	}

	config := p.storage.GetPluginConfig(p.Name())
	if token := p.getConfigValue(config.Settings, "github.token", ""); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	req.Header.Set("User-Agent", "AboutPage/1.0")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch user data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("GitHub API returned status %d for user %s", resp.StatusCode, username)
	}

	var userData GitHubUserData
	if err := json.NewDecoder(resp.Body).Decode(&userData); err != nil {
		return fmt.Errorf("failed to decode user data: %w", err)
	}

	reposURL := fmt.Sprintf("https://api.github.com/users/%s/repos?sort=updated&per_page=100", username)
	req, err = http.NewRequest("GET", reposURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create repos request: %w", err)
	}

	if token := p.getConfigValue(config.Settings, "github.token", ""); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	req.Header.Set("User-Agent", "AboutPage/1.0")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch repos: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var errorResp struct {
			Message          string `json:"message"`
			DocumentationURL string `json:"documentation_url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
			return fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, errorResp.Message)
		}
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var repos []GitHubRepo
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return fmt.Errorf("failed to decode repos (status %d): %w", resp.StatusCode, err)
	}

	allRepoLanguages := p.fetchAllRepoLanguages(client, username, repos)

	totalStars := 0
	languageBytes := make(map[string]int)
	for _, repo := range repos {
		totalStars += repo.Stars
		if repo.Language != "" {
			languageBytes[repo.Language] += 1000
		}
	}

	totalBytes := 0
	for _, bytes := range languageBytes {
		totalBytes += bytes
	}

	var topLanguages []LanguageStat
	for lang, bytes := range languageBytes {
		if totalBytes > 0 {
			percentage := float64(bytes) / float64(totalBytes) * 100
			if percentage >= 1.0 {
				color := GetLanguageColor(lang)
				topLanguages = append(topLanguages, LanguageStat{
					Name:       lang,
					Percentage: percentage,
					Color:      color,
					Bytes:      bytes,
				})
			}
		}
	}

	for i := 0; i < len(topLanguages); i++ {
		for j := i + 1; j < len(topLanguages); j++ {
			if topLanguages[i].Percentage < topLanguages[j].Percentage {
				topLanguages[i], topLanguages[j] = topLanguages[j], topLanguages[i]
			}
		}
	}

	if len(topLanguages) > 8 {
		topLanguages = topLanguages[:8]
	}

	timeSince := time.Now().AddDate(0, -3, 0)
	var recentActiveRepos []GitHubRepo

	for _, repo := range repos {
		if repo.UpdatedAt.After(timeSince) {
			hasRecentCommits := p.checkRecentCommits(client, username, repo.Name, timeSince)
			if hasRecentCommits {
				recentActiveRepos = append(recentActiveRepos, repo)
			}
		}
	}

	for i := 0; i < len(recentActiveRepos); i++ {
		for j := i + 1; j < len(recentActiveRepos); j++ {
			if recentActiveRepos[i].UpdatedAt.Before(recentActiveRepos[j].UpdatedAt) {
				recentActiveRepos[i], recentActiveRepos[j] = recentActiveRepos[j], recentActiveRepos[i]
			}
		}
	}

	for i := range recentActiveRepos {
		if recentActiveRepos[i].LanguagesURL != "" {
			recentActiveRepos[i].Languages = p.fetchRepoLanguages(client, recentActiveRepos[i].LanguagesURL)
		}
	}

	commitStats := p.fetchCommitStats(client, username, repos)

	totalCommitsLastYear := p.fetchTotalCommits(client, username)
	if totalCommitsLastYear > 0 {
		commitStats.TotalCommits = totalCommitsLastYear
	}

	userData.TotalStars = totalStars
	userData.TopLanguages = topLanguages
	userData.RecentRepos = recentActiveRepos
	userData.TotalCommits = commitStats.TotalCommits
	userData.CommitStats = commitStats

	p.githubData = &userData

	p.mutex.Lock()
	p.allRepoLanguages = allRepoLanguages
	p.mutex.Unlock()

	p.hub.Broadcast("github_update", map[string]interface{}{
		"repos":     userData.PublicRepos,
		"followers": userData.Followers,
		"stars":     userData.TotalStars,
		"languages": len(topLanguages),
	})

	fmt.Println("Updating github info... done")
	return nil
}

func (p *CodePlugin) updateWakatimeData(apiKey string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	weekURL := "https://wakatime.com/api/v1/users/current/stats/last_7_days"
	req, err := http.NewRequest("GET", weekURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Basic "+apiKey)
	req.Header.Set("User-Agent", "AboutPage/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var weekData struct {
		Data struct {
			TotalSeconds float64 `json:"total_seconds"`
			Languages    []struct {
				Name         string  `json:"name"`
				TotalSeconds float64 `json:"total_seconds"`
				Percent      float64 `json:"percent"`
				Text         string  `json:"text"`
			} `json:"languages"`
			Editors []struct {
				Name         string  `json:"name"`
				TotalSeconds float64 `json:"total_seconds"`
				Percent      float64 `json:"percent"`
				Text         string  `json:"text"`
			} `json:"editors"`
			OperatingSystems []struct {
				Name         string  `json:"name"`
				TotalSeconds float64 `json:"total_seconds"`
				Percent      float64 `json:"percent"`
				Text         string  `json:"text"`
			} `json:"operating_systems"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&weekData); err != nil {
		return err
	}

	allTimeURL := "https://wakatime.com/api/v1/users/current/all_time_since_today"
	req, err = http.NewRequest("GET", allTimeURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Basic "+apiKey)
	req.Header.Set("User-Agent", "AboutPage/1.0")

	resp, err = client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var allTimeData struct {
		Data struct {
			TotalSeconds float64 `json:"total_seconds"`
			Text         string  `json:"text"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&allTimeData); err != nil {
		return err
	}

	if len(weekData.Data.Languages) == 0 && weekData.Data.TotalSeconds == 0 && p.wakatimeData != nil {
		p.hub.Broadcast("wakatime_update", map[string]interface{}{
			"week_hours": p.wakatimeData.LastWeek.Seconds / 3600,
			"total_text": p.wakatimeData.TotalTime.Text,
			"languages":  len(p.wakatimeData.Languages),
		})
		return nil
	}

	weekHours := weekData.Data.TotalSeconds / 3600
	weekTimeText := fmt.Sprintf("%.1f hrs", weekHours)

	var languages []WakatimeLanguage
	for _, lang := range weekData.Data.Languages {
		languages = append(languages, WakatimeLanguage{
			Name:         lang.Name,
			TotalSeconds: lang.TotalSeconds,
			Percent:      lang.Percent,
			Text:         lang.Text,
			Color:        GetLanguageColor(lang.Name),
		})
	}

	wakatimeData := &WakatimeData{
		TotalTime: struct {
			Seconds float64 `json:"seconds"`
			Text    string  `json:"text"`
		}{
			Seconds: allTimeData.Data.TotalSeconds,
			Text:    allTimeData.Data.Text,
		},
		LastWeek: struct {
			Seconds float64 `json:"seconds"`
			Text    string  `json:"text"`
		}{
			Seconds: weekData.Data.TotalSeconds,
			Text:    weekTimeText,
		},
		Languages:        languages,
		Editors:          weekData.Data.Editors,
		OperatingSystems: weekData.Data.OperatingSystems,
	}

	p.wakatimeData = wakatimeData

	p.hub.Broadcast("wakatime_update", map[string]interface{}{
		"week_hours": weekHours,
		"total_text": allTimeData.Data.Text,
		"languages":  len(weekData.Data.Languages),
	})

	return nil
}

func (p *CodePlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *CodePlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings

	err := p.storage.SetPluginConfig(p.Name(), config)
	if err != nil {
		return err
	}

	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(),
		"action": "settings_changed",
	})

	return nil
}

func (p *CodePlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(string); ok {
				return value
			}
			return defaultValue
		} else {
			if next, ok := current[k].(map[string]interface{}); ok {
				current = next
			} else {
				return defaultValue
			}
		}
	}

	return defaultValue
}

func (p *CodePlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
	keys := strings.Split(key, ".")
	current := settings

	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(bool); ok {
				return value
			}
			return defaultValue
		} else {
			if next, ok := current[k].(map[string]interface{}); ok {
				current = next
			} else {
				return defaultValue
			}
		}
	}

	return defaultValue
}

func (p *CodePlugin) RenderText(ctx context.Context) (string, error) {
	if p.githubData == nil && p.wakatimeData == nil {
		return "Code: No data available", nil
	}

	var parts []string

	if p.githubData != nil {
		parts = append(parts, fmt.Sprintf("%d repos, %d stars", p.githubData.PublicRepos, p.githubData.TotalStars))
	}

	if p.wakatimeData != nil {
		parts = append(parts, fmt.Sprintf("%s this week", p.wakatimeData.LastWeek.Text))
	}

	if len(parts) == 0 {
		return "Code: No stats available", nil
	}

	return fmt.Sprintf("Code: %s", strings.Join(parts, ", ")), nil
}

func (p *CodePlugin) fetchRepoLanguages(client *http.Client, languagesURL string) []RepoLanguageBreakdown {
	req, err := http.NewRequest("GET", languagesURL, nil)
	if err != nil {
		return nil
	}

	config := p.storage.GetPluginConfig(p.Name())
	if token := p.getConfigValue(config.Settings, "github.token", ""); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	req.Header.Set("User-Agent", "AboutPage/1.0")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	var languages map[string]int
	if err := json.NewDecoder(resp.Body).Decode(&languages); err != nil {
		return nil
	}

	total := 0
	for _, bytes := range languages {
		total += bytes
	}

	if total == 0 {
		return nil
	}

	var breakdown []RepoLanguageBreakdown
	for lang, bytes := range languages {
		percentage := float64(bytes) / float64(total) * 100
		if percentage >= 0.5 {
			breakdown = append(breakdown, RepoLanguageBreakdown{
				Name:       lang,
				Percentage: percentage,
				Color:      GetLanguageColor(lang),
			})
		}
	}

	for i := 0; i < len(breakdown); i++ {
		for j := i + 1; j < len(breakdown); j++ {
			if breakdown[i].Percentage < breakdown[j].Percentage {
				breakdown[i], breakdown[j] = breakdown[j], breakdown[i]
			}
		}
	}

	return breakdown
}

func (p *CodePlugin) fetchCommitStats(client *http.Client, username string, repos []GitHubRepo) GitHubCommitStats {
	stats := GitHubCommitStats{
		WeeklyCommits: make([]int, 7),
	}

	oneWeekAgo := time.Now().AddDate(0, 0, -7)

	maxRepos := len(repos)
	if maxRepos > 20 {
		maxRepos = 20
	}

	config := p.storage.GetPluginConfig(p.Name())
	token := p.getConfigValue(config.Settings, "github.token", "")

	for _, repo := range repos[:maxRepos] {
		commitsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits?since=%s&author=%s",
			username, repo.Name, oneWeekAgo.Format(time.RFC3339), username)

		req, err := http.NewRequest("GET", commitsURL, nil)
		if err != nil {
			continue
		}

		if token != "" {
			req.Header.Set("Authorization", "token "+token)
		}

		req.Header.Set("User-Agent", "AboutPage/1.0")
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			continue
		}

		var commits []struct {
			Commit struct {
				Author struct {
					Date time.Time `json:"date"`
				} `json:"author"`
			} `json:"commit"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		for _, commit := range commits {
			daysSince := int(time.Since(commit.Commit.Author.Date).Hours() / 24)
			if daysSince >= 0 && daysSince < 7 {
				stats.WeeklyCommits[6-daysSince]++
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	for _, count := range stats.WeeklyCommits {
		stats.TotalCommits += count
	}

	if stats.TotalCommits == 0 {
		stats.TotalCommits = p.estimateTotalCommits(client, username, repos[:maxRepos])
	}

	return stats
}

func (p *CodePlugin) estimateTotalCommits(client *http.Client, username string, repos []GitHubRepo) int {
	totalCommits := 0

	for _, repo := range repos {
		statsURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/stats/contributors", username, repo.Name)

		req, err := http.NewRequest("GET", statsURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "AboutPage/1.0")
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == 202 {
			resp.Body.Close()
			continue
		}

		var contributors []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Total int `json:"total"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&contributors); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		for _, contrib := range contributors {
			if contrib.Author.Login == username {
				totalCommits += contrib.Total
				break
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	return totalCommits
}

func (p *CodePlugin) fetchTotalCommits(client *http.Client, username string) int {
	config := p.storage.GetPluginConfig(p.Name())
	token := p.getConfigValue(config.Settings, "github.token", "")

	if token == "" {
		return 0
	}

	totalCommits := 0
	now := time.Now()

	for year := 0; year < 10; year++ {
		yearStart := now.AddDate(-year-1, 0, 0)
		yearEnd := now.AddDate(-year, 0, 0)

		query := fmt.Sprintf(`{
			user(login: "%s") {
				contributionsCollection(from: "%s", to: "%s") {
					contributionCalendar {
						totalContributions
					}
				}
			}
		}`, username, yearStart.Format(time.RFC3339), yearEnd.Format(time.RFC3339))

		requestBody := map[string]string{
			"query": query,
		}

		jsonData, err := json.Marshal(requestBody)
		if err != nil {
			break
		}

		req, err := http.NewRequest("POST", "https://api.github.com/graphql", bytes.NewBuffer(jsonData))
		if err != nil {
			break
		}

		req.Header.Set("Authorization", "bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "AboutPage/1.0")

		resp, err := client.Do(req)
		if err != nil {
			break
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			break
		}

		var result struct {
			Data struct {
				User struct {
					ContributionsCollection struct {
						ContributionCalendar struct {
							TotalContributions int `json:"totalContributions"`
						} `json:"contributionCalendar"`
					} `json:"contributionsCollection"`
				} `json:"user"`
			} `json:"data"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			break
		}
		resp.Body.Close()

		yearContributions := result.Data.User.ContributionsCollection.ContributionCalendar.TotalContributions
		totalCommits += yearContributions

		if yearContributions == 0 {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	return totalCommits
}

func (p *CodePlugin) GetMetrics() map[string]interface{} {
	metrics := map[string]interface{}{
		"github_repos":         0,
		"github_stars":         0,
		"github_commits":       0,
		"github_followers":     0,
		"github_following":     0,
		"github_languages":     0,
		"wakatime_hours_7d":    0.0,
		"wakatime_hours_total": 0.0,
		"wakatime_languages":   0,
		"wakatime_editors":     0,
		"recent_repos_count":   0,
		"top_languages_count":  0,
	}

	if p.githubData != nil {
		metrics["github_repos"] = p.githubData.PublicRepos
		metrics["github_stars"] = p.githubData.TotalStars
		metrics["github_commits"] = p.githubData.TotalCommits
		metrics["github_followers"] = p.githubData.Followers
		metrics["github_following"] = p.githubData.Following
		metrics["github_languages"] = len(p.githubData.TopLanguages)
		metrics["recent_repos_count"] = len(p.githubData.RecentRepos)
		metrics["top_languages_count"] = len(p.githubData.TopLanguages)
	}

	if p.wakatimeData != nil {
		metrics["wakatime_hours_7d"] = p.wakatimeData.LastWeek.Seconds / 3600.0
		metrics["wakatime_hours_total"] = p.wakatimeData.TotalTime.Seconds / 3600.0
		metrics["wakatime_languages"] = len(p.wakatimeData.Languages)
		metrics["wakatime_editors"] = len(p.wakatimeData.Editors)
	}

	p.mutex.RLock()
	if len(p.allRepoLanguages) > 0 {
		metrics["github_languages"] = len(p.allRepoLanguages)
	}
	p.mutex.RUnlock()

	return metrics
}
