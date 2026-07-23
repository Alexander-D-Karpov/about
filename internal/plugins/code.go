package plugins

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

const (
	codeGithubInterval = 30 * time.Minute
	codeWakaInterval   = 30 * time.Minute
)

type CodePlugin struct {
	storage *storage.Storage
	hub     *stream.Hub
	config  *config.Config
	store   *CodeStatsStore
	git     *GitActivity

	mu         sync.RWMutex
	github     *GitHubCodeStats
	wakatime   *WakatimeStats
	lastGithub time.Time
	lastWaka   time.Time

	recentRepos     []GitRecentRepo
	lastRecentRepos time.Time

	invalidateCache func()
	bgStarted       sync.Once
	updating        int32
}

type codeWeekSeg struct {
	Name    string
	Color   string
	Percent string
	Text    string
}

func NewCodePlugin(st *storage.Storage, hub *stream.Hub, cfg *config.Config) *CodePlugin {
	store := NewCodeStatsStore(cfg.DataPath)
	p := &CodePlugin{
		storage: st,
		hub:     hub,
		config:  cfg,
		store:   store,
	}
	p.git = NewGitActivity(st, hub, "code", store)
	p.github, p.lastGithub = store.GetGitHub()
	p.wakatime, p.lastWaka = store.GetWakatime()
	if p.github != nil {
		log.Printf("[Code] restored github stats from file (age %s)", time.Since(p.lastGithub).Round(time.Minute))
	}
	if p.wakatime != nil {
		log.Printf("[Code] restored wakatime stats from file (age %s)", time.Since(p.lastWaka).Round(time.Minute))
	}
	p.recentRepos, p.lastRecentRepos = store.GetRecentRepos()
	if len(p.recentRepos) > 0 {
		log.Printf("[Code] restored %d recent repos from file (age %s)", len(p.recentRepos), time.Since(p.lastRecentRepos).Round(time.Minute))
	}
	p.startBackground()
	return p
}

func (p *CodePlugin) Name() string      { return "code" }
func (p *CodePlugin) Git() *GitActivity { return p.git }

func (p *CodePlugin) SetCacheInvalidator(fn func()) {
	p.invalidateCache = fn
}

func (p *CodePlugin) startBackground() {
	p.bgStarted.Do(func() {
		go p.backgroundLoop()
	})
}

func (p *CodePlugin) backgroundLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Code] background stats loop panic recovered: %v", r)
			time.Sleep(time.Minute)
			go p.backgroundLoop()
		}
	}()
	time.Sleep(45 * time.Second)
	log.Printf("[Code] background loop started (hourly refresh, %s full recollect, partial retried hourly)", gitStatsInterval)
	p.hourlyCycle()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		p.hourlyCycle()
	}
}

func (p *CodePlugin) hourlyCycle() {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	if _, err := p.git.Update(ctx); err != nil {
		log.Printf("[Code] hourly git refresh failed: %v", err)
	}
	cancel()

	p.git.RefreshToday()
	p.collectHeavyStats()
	p.git.PrefetchDayDetails(gitPrefetchDaysPerRun)

	log.Printf("[Code] hourly cycle done in %v", time.Since(start).Round(time.Second))
}

func (p *CodePlugin) collectHeavyStats() {
	changed := p.git.EnsureStats(false)
	if !changed {
		return
	}
	log.Printf("[Code] git stats changed, invalidating render cache")
	if p.invalidateCache != nil {
		p.invalidateCache()
	}
	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": "code",
		"action": "stats_updated",
	})
}

func (p *CodePlugin) UpdateData(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&p.updating, 0, 1) {
		return nil
	}
	go func() {
		defer atomic.StoreInt32(&p.updating, 0)
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Code] background update panic recovered: %v", r)
			}
		}()
		bctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		start := time.Now()
		changed := p.updateExternal(bctx)
		log.Printf("[Code] background update finished in %v (changed=%t)", time.Since(start).Round(time.Millisecond), changed)
		if changed {
			if p.invalidateCache != nil {
				p.invalidateCache()
			}
			p.hub.Broadcast("plugin_update", map[string]interface{}{
				"plugin": p.Name(),
				"action": "data_updated",
			})
		}
	}()
	return nil
}

func (p *CodePlugin) updateExternal(ctx context.Context) bool {
	changed := false
	settings := p.storage.GetPluginConfig(p.Name()).Settings

	if p.getBool(settings, "ui.showGitHub", true) {
		p.mu.RLock()
		stale := time.Since(p.lastGithub) > codeGithubInterval
		p.mu.RUnlock()
		if stale {
			username := p.getStr(settings, "github.username", "")
			token := p.getStr(settings, "github.token", "")
			if username != "" {
				start := time.Now()
				log.Printf("[Code] fetching github profile stats for %s...", username)
				stats, err := fetchGitHubCodeStats(ctx, p.git.client, username, token)
				if err != nil {
					log.Printf("[Code] github stats fetch failed: %v", err)
				} else {
					p.mu.Lock()
					p.github = stats
					p.lastGithub = time.Now()
					p.mu.Unlock()
					p.store.SetGitHub(stats)
					changed = true
					log.Printf("[Code] github stats fetched in %v: %d repos, %d stars, %d followers",
						time.Since(start).Round(time.Millisecond), stats.PublicRepos, stats.TotalStars, stats.Followers)
				}
			}
		}
	}
	if p.getBool(settings, "ui.showWakatime", true) {
		p.mu.RLock()
		stale := time.Since(p.lastWaka) > codeWakaInterval
		p.mu.RUnlock()
		if stale {
			apiKey := p.getStr(settings, "wakatime.api_key", "")
			if apiKey != "" {
				start := time.Now()
				log.Printf("[Code] fetching wakatime stats...")
				stats, err := fetchWakatimeStats(ctx, p.git.client, apiKey)
				if err != nil {
					log.Printf("[Code] wakatime stats fetch failed: %v", err)
				} else {
					p.mu.Lock()
					if !stats.OSUpToDate && p.wakatime != nil && p.wakatime.OSUpToDate && len(p.wakatime.OSAllTime) > 0 {
						stats.OSAllTime = p.wakatime.OSAllTime
						stats.OSRange = p.wakatime.OSRange
						stats.OSUpToDate = true
					}
					p.wakatime = stats
					p.lastWaka = time.Now()
					p.mu.Unlock()
					p.store.SetWakatime(stats)
					changed = true
					log.Printf("[Code] wakatime stats fetched in %v: %s / 7d, %d languages, os range: %s (up_to_date=%t)",
						time.Since(start).Round(time.Millisecond), stats.Text7d, stats.LangCount, stats.OSRange, stats.OSUpToDate)
				}
			}
		}
	}
	p.mu.RLock()
	reposStale := time.Since(p.lastRecentRepos) > gitRecentReposInterval
	p.mu.RUnlock()
	if reposStale {
		start := time.Now()
		log.Printf("[Code] collecting recent repos...")
		repos := p.git.CollectRecentRepos(ctx)
		if len(repos) > 0 {
			p.mu.Lock()
			p.recentRepos = repos
			p.lastRecentRepos = time.Now()
			p.mu.Unlock()
			p.store.SetRecentRepos(repos)
			changed = true
			log.Printf("[Code] recent repos collected in %v: %d repos", time.Since(start).Round(time.Millisecond), len(repos))
		} else {
			log.Printf("[Code] recent repos collection returned nothing (%v)", time.Since(start).Round(time.Millisecond))
		}
	}
	gitChanged, err := p.git.Update(ctx)
	if err != nil {
		log.Printf("[Code] git activity update failed: %v", err)
	}
	if gitChanged {
		changed = true
	}
	return changed
}

type codeLangView struct {
	Name    string
	Color   string
	Percent string
	Width   string
}

type codeWakaLangView struct {
	Name  string
	Color string
	Text  string
	Width string
}

type codeEditorView struct {
	Name    string
	Percent string
}

type codeRepoView struct {
	Name        string
	URL         string
	MainLang    string
	MainColor   string
	Commits     int
	Langs       []codeLangView
	Stars       int
	Source      string
	SourceColor string
}

func compactWakaText(s string) string {
	s = strings.ReplaceAll(s, " hrs", "h")
	s = strings.ReplaceAll(s, " hr", "h")
	s = strings.ReplaceAll(s, " mins", "m")
	s = strings.ReplaceAll(s, " min", "m")
	s = strings.ReplaceAll(s, " secs", "s")
	s = strings.ReplaceAll(s, " sec", "s")
	return s
}

type codeOSView struct {
	Name    string
	Color   string
	Percent string
	Text    string
}

func osColor(name string) string {
	switch strings.ToLower(name) {
	case "linux":
		return "#f0a010"
	case "windows":
		return "#4d9fff"
	case "mac", "macos", "darwin":
		return "#b0b8c4"
	case "android":
		return "#3ad38b"
	case "freebsd", "openbsd":
		return "#ff5c7a"
	default:
		return "#8B949E"
	}
}

func (p *CodePlugin) Render(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	settings := p.storage.GetPluginConfig(p.Name()).Settings
	sectionTitle := p.getStr(settings, "ui.sectionTitle", "Coding Stats")
	showGitHub := p.getBool(settings, "ui.showGitHub", true)
	showWaka := p.getBool(settings, "ui.showWakatime", true)
	showLangs := p.getBool(settings, "ui.showLanguages", true)
	ghUsername := p.getStr(settings, "github.username", "")

	p.mu.RLock()
	gh := p.github
	wk := p.wakatime
	p.mu.RUnlock()

	gitCommits, _, _, gitRepos, gitPartial := p.git.CurrentStats()

	var langs []codeLangView
	if showLangs && gh != nil && len(gh.Languages) > 0 {
		maxPct := gh.Languages[0].Percent
		for _, l := range gh.Languages {
			if l.Percent > maxPct {
				maxPct = l.Percent
			}
		}
		if maxPct <= 0 {
			maxPct = 1
		}
		for _, l := range gh.Languages {
			langs = append(langs, codeLangView{
				Name:    l.Name,
				Color:   l.Color,
				Percent: fmt.Sprintf("%.1f%%", l.Percent),
				Width:   fmt.Sprintf("%.2f", l.Percent/maxPct*100),
			})
		}
	}

	var weekBar []codeWeekSeg
	if showWaka && wk != nil && wk.Hours7d > 0 {
		totalSec := wk.Hours7d * 3600
		for _, l := range wk.Languages {
			pct := l.Seconds / totalSec * 100
			if pct < 0.5 {
				continue
			}
			weekBar = append(weekBar, codeWeekSeg{
				Name:    l.Name,
				Color:   GetLanguageColor(l.Name),
				Percent: fmt.Sprintf("%.2f%%", pct),
				Text:    compactWakaText(l.Text),
			})
		}
	}

	p.mu.RLock()
	recent := append([]GitRecentRepo(nil), p.recentRepos...)
	p.mu.RUnlock()

	var repos []codeRepoView
	if showGitHub {
		for _, r := range recent {
			rv := codeRepoView{
				Name:        r.Name,
				URL:         r.URL,
				MainLang:    r.MainLang,
				MainColor:   GetLanguageColor(r.MainLang),
				Commits:     r.Commits,
				Stars:       r.Stars,
				Source:      r.Source,
				SourceColor: r.SourceColor,
			}
			for _, l := range r.Languages {
				rv.Langs = append(rv.Langs, codeLangView{
					Name:    l.Name,
					Color:   l.Color,
					Percent: fmt.Sprintf("%.1f%%", l.Percent),
				})
			}
			repos = append(repos, rv)
		}
		if len(repos) == 0 && gh != nil {
			for _, r := range gh.RecentRepos {
				rv := codeRepoView{
					Name:      r.Name,
					URL:       r.URL,
					MainLang:  r.MainLang,
					MainColor: GetLanguageColor(r.MainLang),
					Commits:   r.Commits,
					Stars:     r.Stars,
				}
				for _, l := range r.Languages {
					rv.Langs = append(rv.Langs, codeLangView{Name: l.Name, Color: l.Color, Percent: fmt.Sprintf("%.1f%%", l.Percent)})
				}
				repos = append(repos, rv)
			}
		}
	}

	var wakaLangs []codeWakaLangView
	var editors []codeEditorView
	var osAll []codeOSView
	waka7d, wakaTotal := "", ""
	wakaLangCount := 0
	if showWaka && wk != nil {
		for _, o := range wk.OSAllTime {
			osAll = append(osAll, codeOSView{
				Name:    o.Name,
				Color:   osColor(o.Name),
				Percent: fmt.Sprintf("%.0f%%", o.Percent),
				Text:    compactWakaText(o.Text),
			})
		}

		waka7d = compactWakaText(wk.Text7d)
		if wk.HoursTotal >= 100 {
			wakaTotal = fmt.Sprintf("%sh", formatCount(int(wk.HoursTotal)))
		} else {
			wakaTotal = compactWakaText(wk.TextTotal)
		}
		wakaLangCount = wk.LangCount
		if wakaLangCount == 0 {
			wakaLangCount = len(wk.Languages)
		}
		maxPct := 0.0
		for _, l := range wk.Languages {
			if l.Percent > maxPct {
				maxPct = l.Percent
			}
		}
		if maxPct <= 0 {
			maxPct = 1
		}
		for i, l := range wk.Languages {
			if i >= 10 {
				break
			}
			wakaLangs = append(wakaLangs, codeWakaLangView{
				Name:  l.Name,
				Color: GetLanguageColor(l.Name),
				Text:  l.Text,
				Width: fmt.Sprintf("%.2f", l.Percent/maxPct*100),
			})
		}
		for _, e := range wk.Editors {
			editors = append(editors, codeEditorView{
				Name:    e.Name,
				Percent: fmt.Sprintf("%.0f%%", e.Percent),
			})
		}
	}

	osLabel := ""
	if len(osAll) > 0 {
		osLabel = "OS · all time"
		if wk != nil && wk.OSRange != "" {
			osLabel = "OS · " + wk.OSRange
		}
	}

	totalRepos := gitRepos
	if totalRepos == 0 && gh != nil {
		totalRepos = gh.PublicRepos
	}

	gitCommitsStr := ""
	if gitCommits > 0 {
		prefix := ""
		if gitPartial {
			prefix = "~"
		}
		gitCommitsStr = prefix + gitFormatComma(gitCommits)
	}

	const tmpl = `
<section class="code-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
		{{if .GithubURL}}<a class="btn btn-sm code-header-link" href="{{.GithubURL}}" target="_blank" rel="noopener" aria-label="GitHub profile"><svg viewBox="0 0 24 24" width="15" height="15"><path fill="currentColor" d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg></a>{{end}}
	</header>
	<div class="plugin__inner">
		{{if .HasTopStats}}
		<div class="code-stats">
			{{if .TotalRepos}}
			<div class="code-stat" title="repositories across all git sources">
				<span class="code-stat-value">{{.TotalRepos}}</span>
				<span class="code-stat-label">repos</span>
			</div>
			{{end}}
			{{if .GitCommits}}
			<div class="code-stat" title="commits across all git sources">
				<span class="code-stat-value">{{.GitCommits}}</span>
				<span class="code-stat-label">commits</span>
			</div>
			{{end}}
			{{if .GitHub}}
			<div class="code-stat">
				<span class="code-stat-value">{{.GitHub.TotalStars}}</span>
				<span class="code-stat-label">stars</span>
			</div>
			<div class="code-stat">
				<span class="code-stat-value">{{.GitHub.Followers}}</span>
				<span class="code-stat-label">followers</span>
			</div>
			{{end}}
			{{if .Waka7d}}
			<div class="code-stat">
				<span class="code-stat-value">{{.Waka7d}}</span>
				<span class="code-stat-label">coded / 7d</span>
			</div>
			{{end}}
			{{if .WakaTotal}}
			<div class="code-stat">
				<span class="code-stat-value">{{.WakaTotal}}</span>
				<span class="code-stat-label">coded total</span>
			</div>
			{{end}}
		</div>
		{{end}}
		{{.GitTopHTML}}
		{{if .WakaLangs}}
		<details class="code-block" open>
			<summary><span>This Week</span><span class="code-block-meta">{{if .Waka7d}}{{.Waka7d}} · {{end}}{{.WakaLangCount}} langs</span></summary>
			<div class="code-block-body">
				{{if .WeekBar}}
				<div class="code-langbar">
					{{range .WeekBar}}<i style="width:{{.Percent}};background:{{.Color}}" title="{{.Name}} {{.Text}}"></i>{{end}}
				</div>
				{{end}}
				<div class="code-lang-rows">
					{{range .WakaLangs}}
					<div class="code-lang-row">
						<span class="code-lang-name">{{.Name}}</span>
						<div class="code-lang-bar"><i style="width:{{.Width}}%;background:{{.Color}}"></i></div>
						<span class="code-lang-val">{{.Text}}</span>
					</div>
					{{end}}
				</div>
				{{if .Editors}}
				<div class="code-editors">
					{{range .Editors}}<span class="code-editor">{{.Name}} <em>{{.Percent}}</em></span>{{end}}
				</div>
				{{end}}
			</div>
		</details>
		{{end}}
		{{if .Languages}}
		<details class="code-block" open>
			<summary><span>Top Languages</span><span class="code-block-meta">{{len .Languages}}</span></summary>
			<div class="code-block-body">
				<div class="code-langbar">
					{{range .Languages}}<i style="width:{{.Percent}};background:{{.Color}}" title="{{.Name}} {{.Percent}}"></i>{{end}}
				</div>
				<div class="code-lang-rows">
					{{range .Languages}}
					<div class="code-lang-row">
						<span class="code-lang-name">{{.Name}} <em>{{.Percent}}</em></span>
						<div class="code-lang-bar"><i style="width:{{.Width}}%;background:{{.Color}}"></i></div>
					</div>
					{{end}}
				</div>
			</div>
		</details>
		{{end}}
		{{if .OSAll}}
		<div class="code-os">
			<span class="code-os-label">{{.OSLabel}}</span>
			<div class="code-langbar code-langbar--thin code-os-bar">
				{{range .OSAll}}<i style="width:{{.Percent}};background:{{.Color}}" title="{{.Name}} {{.Text}} ({{.Percent}})"></i>{{end}}
			</div>
			<div class="code-os-chips">
				{{range .OSAll}}<span class="code-editor"><i class="code-os-dot" style="background:{{.Color}}"></i>{{.Name}} <em>{{.Percent}}</em></span>{{end}}
			</div>
		</div>
		{{end}}
		{{if .Repos}}
		<details class="code-block" open>
			<summary><span>Recent Repos</span><span class="code-block-meta">{{len .Repos}} · 3mo</span></summary>
			<div class="code-block-body">
				{{range .Repos}}
				<a class="code-repo code-repo--compact" href="{{.URL}}" target="_blank" rel="noopener">
					<div class="code-repo-row">
						<span class="code-repo-dot" style="background:{{.MainColor}}"></span>
						<span class="code-repo-name">{{.Name}}</span>
						<span class="code-repo-cmeta">
							{{if .MainLang}}<em>{{.MainLang}}</em>{{end}}
							{{if .Stars}}<span class="code-repo-stars">★ {{.Stars}}</span>{{end}}
							{{if .Commits}}<b>{{.Commits}}c</b>{{end}}
							{{if .Source}}<span class="code-repo-src" style="--sc:{{.SourceColor}}">{{.Source}}</span>{{end}}
						</span>
					</div>
					{{if .Langs}}
					<div class="code-langbar code-langbar--thin">
						{{range .Langs}}<i style="width:{{.Percent}};background:{{.Color}}" title="{{.Name}} {{.Percent}}"></i>{{end}}
					</div>
					{{end}}
				</a>
				{{end}}
			</div>
		</details>
		{{end}}
		{{.GitFeedHTML}}
	</div>
</section>`

	githubURL := ""
	if ghUsername != "" && showGitHub {
		githubURL = "https://github.com/" + ghUsername
	}
	data := struct {
		SectionTitle  string
		GitHub        *GitHubCodeStats
		TotalRepos    int
		GitCommits    string
		HasTopStats   bool
		Languages     []codeLangView
		Waka7d        string
		WakaTotal     string
		WakaLangs     []codeWakaLangView
		WakaLangCount int
		Editors       []codeEditorView
		WeekBar       []codeWeekSeg
		Repos         []codeRepoView
		GitTopHTML    template.HTML
		GitFeedHTML   template.HTML
		GithubURL     string
		OSAll         []codeOSView
		OSLabel       string
	}{
		SectionTitle:  sectionTitle,
		TotalRepos:    totalRepos,
		GitCommits:    gitCommitsStr,
		Languages:     langs,
		Waka7d:        waka7d,
		WakaTotal:     wakaTotal,
		WakaLangs:     wakaLangs,
		WakaLangCount: wakaLangCount,
		Editors:       editors,
		Repos:         repos,
		GitTopHTML:    template.HTML(p.git.RenderTopHTML()),
		GitFeedHTML:   template.HTML(p.git.RenderFeedHTML()),
		GithubURL:     githubURL,
		WeekBar:       weekBar,
		OSAll:         osAll,
		OSLabel:       osLabel,
	}
	if showGitHub {
		data.GitHub = gh
	}
	data.HasTopStats = data.TotalRepos > 0 || data.GitCommits != "" || data.GitHub != nil || data.Waka7d != "" || data.WakaTotal != ""

	t, err := template.New("code").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (p *CodePlugin) RenderText(ctx context.Context) (string, error) {
	p.mu.RLock()
	gh := p.github
	p.mu.RUnlock()
	commits, adds, dels, _, _ := p.git.CurrentStats()
	parts := []string{}
	if gh != nil {
		parts = append(parts, fmt.Sprintf("%d repos, %d stars", gh.PublicRepos, gh.TotalStars))
	}
	if commits > 0 {
		parts = append(parts, fmt.Sprintf("%s commits, +%s/-%s lines",
			gitFormatCount(commits), gitFormatCount(adds), gitFormatCount(dels)))
	}
	if len(parts) == 0 {
		return "Code: No data yet", nil
	}
	return "Code: " + strings.Join(parts, "; "), nil
}

func (p *CodePlugin) GetSettings() map[string]interface{} {
	return p.storage.GetPluginConfig(p.Name()).Settings
}

func (p *CodePlugin) SetSettings(settings map[string]interface{}) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	cfg.Settings = settings
	if err := p.storage.SetPluginConfig(p.Name(), cfg); err != nil {
		return err
	}
	p.mu.Lock()
	p.lastGithub = time.Time{}
	p.lastWaka = time.Time{}
	p.mu.Unlock()
	p.git.ReloadSources()
	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(),
		"action": "settings_changed",
	})
	return nil
}

func (p *CodePlugin) GetMetrics() map[string]interface{} {
	p.mu.RLock()
	gh := p.github
	wk := p.wakatime
	p.mu.RUnlock()
	metrics := map[string]interface{}{
		"github_repos":         0,
		"github_stars":         0,
		"github_followers":     0,
		"github_commits":       int64(0),
		"wakatime_hours_7d":    0.0,
		"wakatime_hours_total": 0.0,
	}
	if gh != nil {
		metrics["github_repos"] = gh.PublicRepos
		metrics["github_stars"] = gh.TotalStars
		metrics["github_followers"] = gh.Followers
	}
	if wk != nil {
		metrics["wakatime_hours_7d"] = wk.Hours7d
		metrics["wakatime_hours_total"] = wk.HoursTotal
	}
	for k, v := range p.git.GetMetrics() {
		metrics[k] = v
	}
	if c, ok := metrics["git_commits_total"].(int64); ok {
		metrics["github_commits"] = c
	}
	return metrics
}

func (p *CodePlugin) getStr(settings map[string]interface{}, key, def string) string {
	keys := strings.Split(key, ".")
	current := settings
	for i, k := range keys {
		if current == nil {
			return def
		}
		if i == len(keys)-1 {
			if v, ok := current[k].(string); ok {
				return v
			}
			return def
		}
		next, ok := current[k].(map[string]interface{})
		if !ok {
			return def
		}
		current = next
	}
	return def
}

func (p *CodePlugin) getBool(settings map[string]interface{}, key string, def bool) bool {
	keys := strings.Split(key, ".")
	current := settings
	for i, k := range keys {
		if current == nil {
			return def
		}
		if i == len(keys)-1 {
			if v, ok := current[k].(bool); ok {
				return v
			}
			return def
		}
		next, ok := current[k].(map[string]interface{})
		if !ok {
			return def
		}
		current = next
	}
	return def
}
