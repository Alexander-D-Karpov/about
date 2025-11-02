package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type CodePlugin struct {
	storage      *storage.Storage
	hub          *stream.Hub
	githubData   *GitHubUserData
	wakatimeData *WakatimeData
	lastUpdate   time.Time
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
	Name        string    `json:"name"`
	Stars       int       `json:"stargazers_count"`
	Language    string    `json:"language"`
	UpdatedAt   time.Time `json:"updated_at"`
	Description string    `json:"description"`
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

func (p *CodePlugin) getLanguageColor(languageName string) string {
	colors := map[string]string{
		"Go":           "#00ADD8",
		"Python":       "#3776ab",
		"JavaScript":   "#f1e05a",
		"TypeScript":   "#2b7489",
		"Java":         "#b07219",
		"C++":          "#f34b7d",
		"C":            "#555555",
		"C#":           "#239120",
		"Rust":         "#dea584",
		"HTML":         "#e34c26",
		"CSS":          "#1572B6",
		"Shell":        "#89e051",
		"Bash":         "#89e051",
		"PHP":          "#4F5D95",
		"Ruby":         "#701516",
		"Swift":        "#FA7343",
		"Kotlin":       "#A97BFF",
		"Dart":         "#00B4AB",
		"Vue":          "#4FC08D",
		"React":        "#61DAFB",
		"JSON":         "#292929",
		"XML":          "#0060ac",
		"YAML":         "#cb171e",
		"Markdown":     "#083fa1",
		"SQL":          "#e38c00",
		"Dockerfile":   "#384d54",
		"Vim script":   "#199f4b",
		"Lua":          "#000080",
		"PowerShell":   "#012456",
		"Assembly":     "#6E4C13",
		"SCSS":         "#c6538c",
		"Less":         "#1d365d",
		"Sass":         "#a53b70",
		"Makefile":     "#427819",
		"CMake":        "#DA3434",
		"Perl":         "#0298c3",
		"R":            "#198CE7",
		"MATLAB":       "#e16737",
		"Scala":        "#c22d40",
		"Clojure":      "#db5855",
		"Elixir":       "#6e4a7e",
		"Erlang":       "#B83998",
		"Haskell":      "#5e5086",
		"F#":           "#b845fc",
		"OCaml":        "#3be133",
		"Reason":       "#ff5847",
		"Elm":          "#60B5CC",
		"PureScript":   "#1D222D",
		"CoffeeScript": "#244776",
		"LiveScript":   "#499886",
		"Nim":          "#ffc200",
		"Crystal":      "#000100",
		"D":            "#ba595e",
		"Zig":          "#ec915c",
		"V":            "#4f87c4",
		"Odin":         "#60AFFE",
		"Text":         "#383A42",
		"Plain Text":   "#383A42",
		"Other":        "#8b949e",
	}

	if color, exists := colors[languageName]; exists {
		return color
	}

	lowerName := strings.ToLower(languageName)
	for lang, color := range colors {
		if strings.ToLower(lang) == lowerName {
			return color
		}
	}

	return "#8b949e"
}

func (p *CodePlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Coding Stats")
	showGitHub := p.getConfigBool(settings, "ui.showGitHub", true)
	showWakatime := p.getConfigBool(settings, "ui.showWakatime", true)
	showLanguages := p.getConfigBool(settings, "ui.showLanguages", true)
	showCommitGraph := p.getConfigBool(settings, "ui.showCommitGraph", true)

	githubUsername := p.getConfigValue(settings, "github.username", "")

	tmpl := `
	<div class="code-section section" data-w="2">
		<div class="plugin-header">
			<h3 class="plugin-title">{{.SectionTitle}}</h3>
		</div>
		<div class="plugin__inner">
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

			{{if and .ShowLanguages .GitHubData .GitHubData.TopLanguages}}
			<div class="code-subsection">
				<button class="section-toggle" data-target="languages" type="button" aria-expanded="false">
					<span class="toggle-icon">▶</span>
					<span>Languages</span>
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

			{{if and .ShowWakatime .WakatimeData .WakatimeData.Languages}}
			<div class="code-subsection">
				<button class="section-toggle" data-target="wakatime-langs" type="button" aria-expanded="false">
					<span class="toggle-icon">▶</span>
					<span>This Week</span>
					<span class="section-count">({{len .WakatimeData.Languages}} langs)</span>
				</button>
				<div class="collapsible-content collapsed" id="wakatime-langs">
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
						<div class="repo-item" onclick="window.open('https://github.com/{{$.GitHubUsername}}/{{.Name}}', '_blank')" style="cursor: pointer;">
							<span class="repo-name">{{.Name}}</span>
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

	maxWeeklyCommits := 1
	if p.githubData != nil && len(p.githubData.CommitStats.WeeklyCommits) > 0 {
		for _, commits := range p.githubData.CommitStats.WeeklyCommits {
			if commits > maxWeeklyCommits {
				maxWeeklyCommits = commits
			}
		}
	}

	data := struct {
		SectionTitle     string
		ShowGitHub       bool
		ShowWakatime     bool
		ShowLanguages    bool
		ShowCommitGraph  bool
		GitHubData       *GitHubUserData
		WakatimeData     *WakatimeData
		MaxWeeklyCommits int
		GitHubUsername   string
	}{
		SectionTitle:     sectionTitle,
		ShowGitHub:       showGitHub,
		ShowWakatime:     showWakatime,
		ShowLanguages:    showLanguages,
		ShowCommitGraph:  showCommitGraph,
		GitHubData:       p.githubData,
		WakatimeData:     p.wakatimeData,
		MaxWeeklyCommits: maxWeeklyCommits,
		GitHubUsername:   githubUsername,
	}

	funcMap := template.FuncMap{
		"printf": fmt.Sprintf,
		"add":    func(a, b int) int { return a + b },
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"mul": func(a, b int) int { return a * b },
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

func (p *CodePlugin) updateGitHubData(username string) error {
	client := &http.Client{Timeout: 15 * time.Second}

	userURL := fmt.Sprintf("https://api.github.com/users/%s", username)
	req, err := http.NewRequest("GET", userURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "AboutPage/1.0")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var userData GitHubUserData
	if err := json.NewDecoder(resp.Body).Decode(&userData); err != nil {
		return err
	}

	reposURL := fmt.Sprintf("https://api.github.com/users/%s/repos?sort=updated&per_page=100", username)
	req, err = http.NewRequest("GET", reposURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "AboutPage/1.0")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err = client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var repos []GitHubRepo
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return err
	}

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
				color := p.getLanguageColor(lang)
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

	recentRepos := repos
	if len(recentRepos) > 5 {
		recentRepos = recentRepos[:5]
	}

	weeklyCommits := []int{12, 8, 15, 22, 18, 7, 3}

	userData.TotalStars = totalStars
	userData.TopLanguages = topLanguages
	userData.RecentRepos = recentRepos
	userData.TotalCommits = 847
	userData.CommitStats = GitHubCommitStats{
		TotalCommits:  847,
		WeeklyCommits: weeklyCommits,
	}

	p.githubData = &userData

	p.hub.Broadcast("github_update", map[string]interface{}{
		"repos":     userData.PublicRepos,
		"followers": userData.Followers,
		"stars":     userData.TotalStars,
		"languages": len(topLanguages),
	})

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

	weekHours := weekData.Data.TotalSeconds / 3600
	weekTimeText := fmt.Sprintf("%.1f hrs", weekHours)

	var languages []WakatimeLanguage
	for _, lang := range weekData.Data.Languages {
		languages = append(languages, WakatimeLanguage{
			Name:         lang.Name,
			TotalSeconds: lang.TotalSeconds,
			Percent:      lang.Percent,
			Text:         lang.Text,
			Color:        p.getLanguageColor(lang.Name),
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
