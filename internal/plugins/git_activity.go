package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

const (
	gitCalendarInterval     = 30 * time.Minute
	gitFeedInterval         = 10 * time.Minute
	gitStatsInterval        = 24 * time.Hour
	gitStatsPartialInterval = time.Hour
	gitStatsRunTimeout      = 20 * time.Minute
	gitFeedWindow           = 365 * 24 * time.Hour
	gitFeedFetchLimit       = 200
	gitFeedKeep             = 600
	gitFeedRenderMax        = 60
	gitCalendarWeeks        = 79
	gitPrefetchDaysPerRun   = 60
	gitPrefetchDelay        = 4 * time.Second
)

type GitActivity struct {
	storage    *storage.Storage
	hub        *stream.Hub
	pluginName string
	client     *http.Client
	store      *CodeStatsStore

	mu        sync.RWMutex
	providers []GitProvider
	sourcesFP string

	calendar map[string]map[string]int
	feed     []GitActivityItem

	lastCalendar time.Time
	lastFeed     time.Time

	statsMu    sync.Mutex
	dayStore   *GitDayStore
	prefetchMu sync.Mutex
	dayClient  *http.Client
}

type GitDayDetailer interface {
	FetchDayDetails(ctx context.Context, date string) ([]GitActivityItem, error)
}

func NewGitActivity(st *storage.Storage, hub *stream.Hub, pluginName string, store *CodeStatsStore) *GitActivity {
	g := &GitActivity{
		storage:    st,
		hub:        hub,
		pluginName: pluginName,
		client:     NewHTTPClientWithTimeout(20 * time.Second),
		store:      store,
		calendar:   make(map[string]map[string]int),
		dayStore:   NewGitDayStore(store.Dir()),
		dayClient:  NewHTTPClientWithTimeout(60 * time.Second),
	}
	g.reloadSources(false)
	g.loadFeedCache()
	return g
}

func (g *GitActivity) gitSettings() map[string]interface{} {
	cfg := g.storage.GetPluginConfig(g.pluginName)
	if cfg.Settings == nil {
		return nil
	}
	if m, ok := cfg.Settings["git"].(map[string]interface{}); ok {
		return m
	}
	return nil
}

func (g *GitActivity) implicitGitHubConfig() *gitSourceConfig {
	cfg := g.storage.GetPluginConfig(g.pluginName)
	if cfg.Settings == nil {
		return nil
	}
	gh, ok := cfg.Settings["github"].(map[string]interface{})
	if !ok {
		return nil
	}
	username, _ := gh["username"].(string)
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	token, _ := gh["token"].(string)
	color, _ := gh["color"].(string)

	c := parseGitSourceConfig(map[string]interface{}{
		"type":     "github",
		"name":     "GitHub",
		"username": username,
		"token":    strings.TrimSpace(token),
		"color":    strings.TrimSpace(color),
	})
	return &c
}

func (g *GitActivity) reloadSources(force bool) {
	gs := g.gitSettings()
	var raw []interface{}
	if gs != nil {
		raw, _ = gs["sources"].([]interface{})
	}

	implicit := g.implicitGitHubConfig()

	fpBytes, _ := json.Marshal(raw)
	fp := string(fpBytes)
	if implicit != nil {
		fp += "|gh:" + implicit.Username + ":" + implicit.Token + ":" + implicit.Color
	}

	g.mu.Lock()
	if !force && fp == g.sourcesFP {
		g.mu.Unlock()
		return
	}

	providers := make([]GitProvider, 0, len(raw)+1)
	seenGithub := make(map[string]bool)

	for _, r := range raw {
		m, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		cfg := parseGitSourceConfig(m)
		if p := gitProviderFromConfig(cfg, g.client); p != nil {
			providers = append(providers, p)
			if cfg.Type == "github" {
				seenGithub[strings.ToLower(cfg.Username)] = true
			}
		}
	}

	implicitAdded := false
	if implicit != nil && !seenGithub[strings.ToLower(implicit.Username)] {
		providers = append([]GitProvider{NewGitHubProvider(*implicit, g.client)}, providers...)
		implicitAdded = true
	}

	for _, p := range providers {
		if sa, ok := p.(interface{ SetStatsStore(*CodeStatsStore) }); ok {
			sa.SetStatsStore(g.store)
		}
	}

	g.providers = providers
	g.sourcesFP = fp
	g.lastCalendar = time.Time{}
	g.lastFeed = time.Time{}
	g.mu.Unlock()

	if len(providers) > 0 {
		names := make([]string, len(providers))
		for i, p := range providers {
			names[i] = p.Key()
		}
		suffix := ""
		if implicitAdded {
			suffix = " (github:GitHub is implicit from github.username/token settings)"
		}
		log.Printf("[Git] sources configured: %s%s", strings.Join(names, ", "), suffix)
		if implicitAdded && implicit.Token == "" {
			log.Printf("[Git] implicit GitHub source has no token: heatmap calendar will be unavailable, feed/stats rate-limited")
		}
	}
}

func (g *GitActivity) ReloadSources() {
	g.reloadSources(true)
}

func (g *GitActivity) Providers() []GitProvider {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return append([]GitProvider(nil), g.providers...)
}

func (g *GitActivity) Update(ctx context.Context) (bool, error) {
	g.reloadSources(false)

	g.mu.RLock()
	providers := append([]GitProvider(nil), g.providers...)
	needCal := time.Since(g.lastCalendar) > gitCalendarInterval
	needFeed := time.Since(g.lastFeed) > gitFeedInterval
	g.mu.RUnlock()

	if len(providers) == 0 {
		return false, nil
	}

	changed := false
	if needCal {
		if g.updateCalendars(ctx, providers) {
			changed = true
		}
	}
	if needFeed {
		if g.updateFeed(ctx) {
			changed = true
		}
	}
	return changed, nil
}

func (g *GitActivity) updateCalendars(ctx context.Context, providers []GitProvider) bool {
	start := time.Now()
	log.Printf("[Git] updating calendars for %d source(s)...", len(providers))

	to := time.Now()
	from := to.AddDate(0, 0, -7*gitCalendarWeeks)

	type result struct {
		key string
		cal map[string]int
	}

	results := make(chan result, len(providers))
	var wg sync.WaitGroup

	for _, p := range providers {
		wg.Add(1)
		go func(p GitProvider) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Git] calendar panic for %s: %v", p.Key(), r)
				}
			}()

			pctx, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()

			pstart := time.Now()
			cal, err := p.FetchCalendar(pctx, from, to)
			if err != nil {
				log.Printf("[Git] calendar fetch failed for %s: %v", p.Key(), err)
				return
			}
			log.Printf("[Git] calendar fetched for %s: %d active days in %v", p.Key(), len(cal), time.Since(pstart).Round(time.Millisecond))
			results <- result{key: p.Key(), cal: cal}
		}(p)
	}

	wg.Wait()
	close(results)

	fetched := make(map[string]map[string]int)
	for r := range results {
		fetched[r.key] = r.cal
	}
	if len(fetched) == 0 {
		log.Printf("[Git] calendar update produced no data (%v)", time.Since(start).Round(time.Millisecond))
		return false
	}

	g.mu.Lock()
	for date := range g.calendar {
		for key := range fetched {
			delete(g.calendar[date], key)
		}
		if len(g.calendar[date]) == 0 {
			delete(g.calendar, date)
		}
	}
	for key, cal := range fetched {
		for date, count := range cal {
			if g.calendar[date] == nil {
				g.calendar[date] = make(map[string]int)
			}
			g.calendar[date][key] = count
		}
	}
	g.lastCalendar = time.Now()
	g.mu.Unlock()

	log.Printf("[Git] calendars updated for %d/%d sources in %v", len(fetched), len(providers), time.Since(start).Round(time.Millisecond))
	return true
}

func (g *GitActivity) updateFeed(ctx context.Context) bool {
	start := time.Now()
	g.mu.RLock()
	providers := append([]GitProvider(nil), g.providers...)
	g.mu.RUnlock()

	log.Printf("[Git] updating activity feed for %d source(s)...", len(providers))

	since := time.Now().Add(-gitFeedWindow)
	var (
		mu     sync.Mutex
		merged []GitActivityItem
		wg     sync.WaitGroup
	)

	for _, p := range providers {
		wg.Add(1)
		go func(p GitProvider) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()
			pstart := time.Now()
			items, err := p.FetchActivities(pctx, since, gitFeedFetchLimit)
			if err != nil {
				log.Printf("[Git] feed fetch failed for %s: %v", p.Key(), err)
				return
			}
			log.Printf("[Git] feed fetched for %s: %d items in %v", p.Key(), len(items), time.Since(pstart).Round(time.Millisecond))
			mu.Lock()
			merged = append(merged, items...)
			mu.Unlock()
		}(p)
	}
	wg.Wait()

	sort.Slice(merged, func(i, j int) bool { return merged[i].Time.After(merged[j].Time) })
	if len(merged) > gitFeedKeep {
		merged = merged[:gitFeedKeep]
	}

	g.mu.Lock()
	if len(merged) > 0 || len(g.feed) == 0 {
		g.feed = merged
	}
	g.lastFeed = time.Now()
	g.mu.Unlock()

	if len(merged) > 0 {
		g.saveFeedCache()
	}
	log.Printf("[Git] feed updated: %d items in %v", len(merged), time.Since(start).Round(time.Millisecond))
	return len(merged) > 0
}

func (g *GitActivity) EnsureStats(force bool) bool {
	g.reloadSources(false)
	providers := g.Providers()
	if len(providers) == 0 {
		return false
	}

	var stale []GitProvider
	for _, p := range providers {
		st, at, ok := g.store.GetGitStats(p.Key())
		interval := gitStatsInterval
		if ok && st.Partial {
			interval = gitStatsPartialInterval
		}
		if force || !ok || time.Since(at) > interval {
			stale = append(stale, p)
		}
	}

	if len(stale) == 0 {
		log.Printf("[Git] stats fresh for all %d sources, nothing to collect", len(providers))
		return false
	}

	if !g.statsMu.TryLock() {
		log.Printf("[Git] stats collection already in progress, skipping this round")
		return false
	}
	defer g.statsMu.Unlock()

	changed := false
	for _, p := range stale {
		key := p.Key()
		if prev, at, ok := g.store.GetGitStats(key); ok {
			log.Printf("[Git] recollecting stats for %s (age %s, partial=%t)", key, time.Since(at).Round(time.Minute), prev.Partial)
		} else {
			log.Printf("[Git] stats for %s missing, collecting...", key)
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Git] stats panic for %s: %v", key, r)
				}
			}()

			ctx, cancel := context.WithTimeout(context.Background(), gitStatsRunTimeout)
			defer cancel()

			start := time.Now()
			s, err := p.FetchStats(ctx)
			if err != nil {
				log.Printf("[Git] stats fetch failed for %s: %v", key, err)
				return
			}

			merged := g.store.MergeGitStats(key, *s)
			changed = true

			suffix := ""
			if merged.Partial {
				suffix = " (partial, will retry next hour)"
			}
			log.Printf("[Git] stats stored for %s in %s: %d commits, +%d/-%d lines, %d repos%s",
				key, time.Since(start).Round(time.Second),
				merged.Commits, merged.Additions, merged.Deletions, merged.Repos, suffix)
		}()
	}
	return changed
}

func (g *GitActivity) statsLocked() (commits, adds, dels int64, repos int, partial bool) {
	keys := make(map[string]bool, len(g.providers))
	for _, p := range g.providers {
		keys[p.Key()] = true
	}
	for key, e := range g.store.AllGitStats() {
		if !keys[key] {
			continue
		}
		commits += e.Stats.Commits
		adds += e.Stats.Additions
		dels += e.Stats.Deletions
		repos += e.Stats.Repos
		if e.Stats.Partial {
			partial = true
		}
	}
	return
}

func (g *GitActivity) CurrentStats() (commits, adds, dels int64, repos int, partial bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.statsLocked()
}

func (g *GitActivity) providerMeta() (map[string]string, map[string]string, map[string]bool) {
	colors := make(map[string]string)
	labels := make(map[string]string)
	private := make(map[string]bool)
	for _, p := range g.providers {
		colors[p.Key()] = p.Color()
		labels[p.Key()] = p.Label()
		private[p.Key()] = p.Private()
	}
	return colors, labels, private
}

func (g *GitActivity) RenderTopHTML() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.providers) == 0 {
		return ""
	}

	gs := g.gitSettings()
	showHeatmap := gitCfgBool(gs, "ui", "showHeatmap", true)

	var b strings.Builder
	b.WriteString(`<div class="git-activity git-activity--top">`)
	if showHeatmap {
		g.writeHeatmap(&b)
	}
	g.writeStats(&b)
	b.WriteString(`</div>`)
	return b.String()
}

func (g *GitActivity) RenderFeedHTML() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.providers) == 0 {
		return ""
	}

	gs := g.gitSettings()
	if !gitCfgBool(gs, "ui", "showActivity", true) {
		return ""
	}
	activityLimit := gitCfgInt(gs, "ui", "activityLimit", 6)

	var b strings.Builder
	b.WriteString(`<div class="git-activity git-activity--feed">`)
	g.writeFeed(&b, activityLimit)
	b.WriteString(`</div>`)
	return b.String()
}

func (g *GitActivity) RenderHTML() string {
	return g.RenderTopHTML() + g.RenderFeedHTML()
}

func (g *GitActivity) writeHeatmap(b *strings.Builder) {
	colors, labels, private := g.providerMeta()

	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := end.AddDate(0, 0, -(gitCalendarWeeks*7 - 1))
	for start.Weekday() != time.Sunday {
		start = start.AddDate(0, 0, -1)
	}

	maxCount := 0
	srcTotals := make(map[string]int)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		total := 0
		for key, c := range g.calendar[d.Format("2006-01-02")] {
			total += c
			srcTotals[key] += c
		}
		if total > maxCount {
			maxCount = total
		}
	}

	b.WriteString(`<div class="git-heatmap-block"><div class="git-heatmap-scroll"><div class="git-hm-inner">`)

	type hmSpan struct {
		start int
		text  string
	}
	var months []hmSpan
	var years []hmSpan
	week := 0
	lastMonth := time.Month(0)
	lastYear := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 7) {
		if d.Year() != lastYear {
			lastYear = d.Year()
			years = append(years, hmSpan{start: week, text: d.Format("2006")})
		}
		if d.Month() != lastMonth {
			lastMonth = d.Month()
			months = append(months, hmSpan{start: week, text: d.Format("Jan")})
		}
		week++
	}
	totalWeeks := week

	b.WriteString(`<div class="git-years">`)
	for i, y := range years {
		endW := totalWeeks
		if i+1 < len(years) {
			endW = years[i+1].start
		}
		if endW-y.start < 4 {
			continue
		}
		fmt.Fprintf(b, `<span class="git-year" style="grid-column:%d / %d">%s</span>`, y.start+1, endW+1, y.text)
	}
	b.WriteString(`</div>`)

	b.WriteString(`<div class="git-months">`)
	prevLabelEnd := -1
	for i, m := range months {
		endW := totalWeeks
		if i+1 < len(months) {
			endW = months[i+1].start
		}
		if endW-m.start < 3 || m.start <= prevLabelEnd {
			continue
		}
		fmt.Fprintf(b, `<span class="git-month" style="grid-column-start:%d">%s</span>`, m.start+1, m.text)
		prevLabelEnd = m.start + 2
	}
	b.WriteString(`</div>`)

	b.WriteString(`<div class="git-hm-row">`)
	b.WriteString(`<div class="git-weekdays"><span></span><span>Mon</span><span></span><span>Wed</span><span></span><span>Fri</span><span></span></div>`)

	b.WriteString(`<div class="git-heatmap">`)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		date := d.Format("2006-01-02")
		day := g.calendar[date]
		total := 0
		for _, c := range day {
			total += c
		}

		level := 0
		if total > 0 && maxCount > 0 {
			level = int(math.Ceil(float64(total) / float64(maxCount) * 4))
			if level < 1 {
				level = 1
			}
			if level > 4 {
				level = 4
			}
		}

		if total > 0 {
			srcJSON, _ := json.Marshal(day)
			fmt.Fprintf(b, `<span class="git-day" data-date="%s" data-count="%d" data-level="%d" data-src="%s" style="--gc:%s"></span>`,
				date, total, level, template.HTMLEscapeString(string(srcJSON)), gitBlendColors(day, colors))
		} else {
			fmt.Fprintf(b, `<span class="git-day" data-date="%s" data-count="0" data-level="0"></span>`, date)
		}
	}
	b.WriteString(`</div></div></div></div>`)

	b.WriteString(`<div class="git-heatmap-foot"><div class="git-legend">`)
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		cls := "git-src"
		suffix := ""
		if private[k] {
			cls += " git-src--private"
			suffix = `<span class="git-src-lock" title="private source">` + gitLockSVG + `</span>`
		}
		countStr := ""
		if srcTotals[k] > 0 {
			countStr = fmt.Sprintf(`<em>%s</em>`, gitFormatComma(int64(srcTotals[k])))
		}
		fmt.Fprintf(b, `<span class="%s" data-key="%s" data-label="%s" role="button" tabindex="0" title="hover: filter · click: toggle" style="--gc:%s"><i></i>%s%s%s</span>`,
			cls, template.HTMLEscapeString(k), template.HTMLEscapeString(labels[k]), colors[k],
			template.HTMLEscapeString(labels[k]), countStr, suffix)
	}
	b.WriteString(`</div><div class="git-scale">Less`)
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(b, `<span class="git-day git-day--scale" data-level="%d" style="--gc:#4d9fff"></span>`, i)
	}
	b.WriteString(`More</div></div></div>`)
}

func (g *GitActivity) writeStats(b *strings.Builder) {
	now := time.Now()

	yearTotal := 0
	yearCutoff := now.AddDate(0, 0, -365).Format("2006-01-02")
	for date, day := range g.calendar {
		if date < yearCutoff {
			continue
		}
		for _, c := range day {
			yearTotal += c
		}
	}

	weekTotal := 0
	for i := 0; i < 7; i++ {
		for _, c := range g.calendar[now.AddDate(0, 0, -i).Format("2006-01-02")] {
			weekTotal += c
		}
	}

	startOffset := 0
	if len(g.calendar[now.Format("2006-01-02")]) == 0 {
		startOffset = 1
	}
	streak := 0
	for i := startOffset; i < 400; i++ {
		sum := 0
		for _, c := range g.calendar[now.AddDate(0, 0, -i).Format("2006-01-02")] {
			sum += c
		}
		if sum == 0 {
			break
		}
		streak++
	}

	_, adds, dels, _, partial := g.statsLocked()

	approx := ""
	if partial {
		approx = "~"
	}

	b.WriteString(`<div class="git-stats">`)
	if adds > 0 || dels > 0 {
		fmt.Fprintf(b, `<div class="git-stat git-stat--wide" title="+%s / −%s lines across all git sources"><span class="git-stat-value"><em class="git-add">+%s</em> <em class="git-del">−%s</em></span><span class="git-stat-label">%slines changed</span></div>`,
			gitFormatComma(adds), gitFormatComma(dels), gitFormatCount(adds), gitFormatCount(dels), approx)
	}
	fmt.Fprintf(b, `<div class="git-stat" title="contributions in the last 7 days, all git sources"><span class="git-stat-value">%s</span><span class="git-stat-label">this week</span></div>`,
		gitFormatComma(int64(weekTotal)))
	fmt.Fprintf(b, `<div class="git-stat" title="contributions in the last year, all git sources"><span class="git-stat-value">%s</span><span class="git-stat-label">this year</span></div>`,
		gitFormatComma(int64(yearTotal)))
	if streak > 0 {
		fmt.Fprintf(b, `<div class="git-stat"><span class="git-stat-value">%d</span><span class="git-stat-label">day streak</span></div>`, streak)
	}
	b.WriteString(`</div>`)
}

func (g *GitActivity) writeFeed(b *strings.Builder, limit int) {
	if limit <= 0 {
		limit = 6
	}

	items := make([]GitActivityItem, 0, gitFeedRenderMax)
	for _, it := range g.feed {
		if it.Private {
			continue
		}
		items = append(items, it)
		if len(items) >= gitFeedRenderMax {
			break
		}
	}
	if len(items) == 0 {
		return
	}

	b.WriteString(`<details class="code-block git-feed" data-git-feed>`)
	fmt.Fprintf(b, `<summary><span>Recent Activity</span><span class="code-block-meta">%d</span></summary>`, len(items))
	b.WriteString(`<div class="code-block-body git-feed-body"><div class="git-feed-list">`)
	for i, it := range items {
		cls := "git-item"
		if i >= limit {
			cls += " git-item--hidden"
		}
		fmt.Fprintf(b, `<div class="%s" data-type="%s" data-source="%s">`,
			cls, template.HTMLEscapeString(it.Type), template.HTMLEscapeString(it.Source))
		fmt.Fprintf(b, `<span class="git-item-icon" style="--sc:%s">%s</span>`, it.Color, gitIconSVG(it.Type))
		b.WriteString(`<div class="git-item-body">`)

		title := template.HTMLEscapeString(it.Title)
		if it.URL != "" {
			fmt.Fprintf(b, `<a class="git-item-title" href="%s" target="_blank" rel="noopener">%s</a>`,
				template.HTMLEscapeString(it.URL), title)
		} else {
			fmt.Fprintf(b, `<div class="git-item-title">%s</div>`, title)
		}

		b.WriteString(`<div class="git-item-meta">`)
		if it.Repo != "" {
			fmt.Fprintf(b, `<span class="git-repo">%s</span><span class="git-dot">·</span>`, template.HTMLEscapeString(it.Repo))
		}
		if it.Ref != "" {
			fmt.Fprintf(b, `<span class="git-ref">%s</span><span class="git-dot">·</span>`, template.HTMLEscapeString(it.Ref))
		}
		fmt.Fprintf(b, `<span>%s</span>`, gitRelTime(it.Time))
		if it.Commits > 1 {
			fmt.Fprintf(b, `<span class="git-commits">%d commits</span>`, it.Commits)
		}
		if it.Additions > 0 || it.Deletions > 0 {
			fmt.Fprintf(b, `<span class="git-add">+%s</span><span class="git-del">−%s</span>%s`,
				gitFormatComma(int64(it.Additions)), gitFormatComma(int64(it.Deletions)),
				gitDiffBlocks(it.Additions, it.Deletions))
		}
		fmt.Fprintf(b, `<span class="git-item-src" style="--sc:%s">%s</span>`, it.Color, template.HTMLEscapeString(it.Source))
		b.WriteString(`</div></div></div>`)
	}
	b.WriteString(`</div>`)
	if len(items) > limit {
		fmt.Fprintf(b, `<button type="button" class="git-feed-more" data-git-feed-more>Show more (%d)</button>`, len(items)-limit)
	}
	b.WriteString(`</div></details>`)
}

func gitFormatComma(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := ""
	if strings.HasPrefix(s, "-") {
		neg = "-"
		s = s[1:]
	}
	if len(s) <= 3 {
		return neg + s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteString(",")
		}
		b.WriteString(s[i : i+3])
	}
	return neg + b.String()
}

func gitDiffBlocks(a, d int) string {
	if a == 0 && d == 0 {
		return ""
	}
	total := a + d
	greens := int(math.Round(float64(a) / float64(total) * 5))
	if a > 0 && greens == 0 {
		greens = 1
	}
	if d > 0 && greens == 5 {
		greens = 4
	}

	var b strings.Builder
	b.WriteString(`<span class="git-blocks">`)
	for i := 0; i < 5; i++ {
		if i < greens {
			b.WriteString(`<i class="gb gb-a"></i>`)
		} else {
			b.WriteString(`<i class="gb gb-d"></i>`)
		}
	}
	b.WriteString(`</span>`)
	return b.String()
}

const gitLockSVG = `<svg viewBox="0 0 16 16" width="11" height="11" fill="currentColor"><path d="M4 4a4 4 0 0 1 8 0v2h.25c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0 1 12.25 15h-8.5A1.75 1.75 0 0 1 2 13.25v-5.5C2 6.784 2.784 6 3.75 6H4Zm8.25 3.5h-8.5a.25.25 0 0 0-.25.25v5.5c0 .138.112.25.25.25h8.5a.25.25 0 0 0 .25-.25v-5.5a.25.25 0 0 0-.25-.25ZM10.5 6V4a2.5 2.5 0 1 0-5 0v2Z"/></svg>`

func gitIconSVG(t string) string {
	switch t {
	case "push":
		return `<svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M11.93 8.5a4.002 4.002 0 0 1-7.86 0H.75a.75.75 0 0 1 0-1.5h3.32a4.002 4.002 0 0 1 7.86 0h3.32a.75.75 0 0 1 0 1.5Zm-1.43-.75a2.5 2.5 0 1 0-5 0 2.5 2.5 0 0 0 5 0Z"/></svg>`
	case "pr", "merge":
		return `<svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M5.45 5.154A4.25 4.25 0 0 0 9.25 7.5h1.378a2.251 2.251 0 1 1 0 1.5H9.25A5.734 5.734 0 0 1 5 7.123v3.505a2.25 2.25 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.95-.218ZM4.25 13.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5Zm8.5-4.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5ZM5 3.25a.75.75 0 1 0-1.5 0 .75.75 0 0 0 1.5 0Z"/></svg>`
	case "issue":
		return `<svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M8 9.5a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3Z"/><path d="M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0ZM1.5 8a6.5 6.5 0 1 0 13 0 6.5 6.5 0 0 0-13 0Z"/></svg>`
	case "create":
		return `<svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M2 2.5A2.5 2.5 0 0 1 4.5 0h8.75a.75.75 0 0 1 .75.75v12.5a.75.75 0 0 1-.75.75h-2.5a.75.75 0 0 1 0-1.5h1.75v-2h-8a1 1 0 0 0-.714 1.7.75.75 0 1 1-1.072 1.05A2.495 2.495 0 0 1 2 11.5Zm10.5-1h-8a1 1 0 0 0-1 1v6.708A2.486 2.486 0 0 1 4.5 9h8ZM5 12.25a.25.25 0 0 1 .25-.25h3.5a.25.25 0 0 1 .25.25v3.25a.25.25 0 0 1-.4.2l-1.45-1.087a.249.249 0 0 0-.3 0L5.4 15.7a.25.25 0 0 1-.4-.2Z"/></svg>`
	case "star":
		return `<svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M8 .25a.75.75 0 0 1 .673.418l1.882 3.815 4.21.612a.75.75 0 0 1 .416 1.279l-3.046 2.97.719 4.192a.751.751 0 0 1-1.088.791L8 12.347l-3.766 1.98a.75.75 0 0 1-1.088-.79l.72-4.194L.818 6.374a.75.75 0 0 1 .416-1.28l4.21-.611L7.327.668A.75.75 0 0 1 8 .25Z"/></svg>`
	case "release":
		return `<svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M1 7.775V2.75C1 1.784 1.784 1 2.75 1h5.025c.464 0 .91.184 1.238.513l6.25 6.25a1.75 1.75 0 0 1 0 2.474l-5.026 5.026a1.75 1.75 0 0 1-2.474 0l-6.25-6.25A1.752 1.752 0 0 1 1 7.775ZM6 5a1 1 0 1 0-2 0 1 1 0 0 0 2 0Z"/></svg>`
	default:
		return `<svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><circle cx="8" cy="8" r="3"/></svg>`
	}
}

func (g *GitActivity) GetMetrics() map[string]interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()

	yearTotal := 0
	yearCutoff := time.Now().AddDate(0, 0, -365).Format("2006-01-02")
	for date, day := range g.calendar {
		if date < yearCutoff {
			continue
		}
		for _, c := range day {
			yearTotal += c
		}
	}

	commits, adds, dels, repos, _ := g.statsLocked()

	return map[string]interface{}{
		"git_contributions_year": yearTotal,
		"git_commits_total":      commits,
		"git_additions_total":    adds,
		"git_deletions_total":    dels,
		"git_repos_total":        repos,
		"git_sources_count":      len(g.providers),
	}
}

func gitCfgBool(m map[string]interface{}, section, key string, def bool) bool {
	if m == nil {
		return def
	}
	sub, ok := m[section].(map[string]interface{})
	if !ok {
		return def
	}
	if v, ok := sub[key].(bool); ok {
		return v
	}
	return def
}

func gitCfgInt(m map[string]interface{}, section, key string, def int) int {
	if m == nil {
		return def
	}
	sub, ok := m[section].(map[string]interface{})
	if !ok {
		return def
	}
	switch v := sub[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

func gitDateIsFinal(date string) bool {
	return date < time.Now().AddDate(0, 0, -1).Format("2006-01-02")
}

func (g *GitActivity) fetchDayFromProviders(ctx context.Context, date string, providers []GitProvider) []GitActivityItem {
	var out []GitActivityItem
	for _, p := range providers {
		dd, ok := p.(GitDayDetailer)
		if !ok {
			continue
		}
		fctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		items, err := dd.FetchDayDetails(fctx, date)
		cancel()
		if err != nil {
			log.Printf("[Git] day details fetch failed for %s (%s): %v", p.Key(), date, err)
			continue
		}
		out = append(out, items...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

func (g *GitActivity) ensureDayDetails(ctx context.Context, date string) gitDayEntry {
	if entry, ok := g.dayStore.Get(date); ok {
		if gitDateIsFinal(date) || time.Since(time.Unix(entry.FetchedAt, 0)) < 10*time.Minute {
			return entry
		}
	}
	g.mu.RLock()
	providers := append([]GitProvider(nil), g.providers...)
	g.mu.RUnlock()
	items := g.fetchDayFromProviders(ctx, date, providers)
	private := 0
	public := make([]GitActivityItem, 0, len(items))
	for _, it := range items {
		if it.Private {
			private++
			continue
		}
		public = append(public, it)
	}
	g.dayStore.Set(date, public, private)
	g.dayStore.Flush()
	entry, _ := g.dayStore.Get(date)
	return entry
}

func (g *GitActivity) feedExtrasForDate(date string) []GitActivityItem {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var out []GitActivityItem
	for _, it := range g.feed {
		if it.Private || it.Type == "push" {
			continue
		}
		if it.Time.Format("2006-01-02") == date {
			out = append(out, it)
		}
	}
	return out
}

func (g *GitActivity) PrefetchDayDetails(maxDays int) {
	if maxDays <= 0 {
		maxDays = gitPrefetchDaysPerRun
	}
	if !g.prefetchMu.TryLock() {
		return
	}
	defer g.prefetchMu.Unlock()

	g.mu.RLock()
	dates := make([]string, 0, len(g.calendar))
	for date, day := range g.calendar {
		total := 0
		for _, c := range day {
			total += c
		}
		if total > 0 {
			dates = append(dates, date)
		}
	}
	providers := append([]GitProvider(nil), g.providers...)
	g.mu.RUnlock()
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))

	pending := make([]string, 0, maxDays)
	for _, date := range dates {
		if len(pending) >= maxDays {
			break
		}
		if entry, ok := g.dayStore.Get(date); ok {
			if gitDateIsFinal(date) || time.Since(time.Unix(entry.FetchedAt, 0)) < 10*time.Minute {
				continue
			}
		}
		pending = append(pending, date)
	}
	if len(pending) == 0 {
		return
	}

	start := time.Now()
	log.Printf("[Git] prefetching day details for %d day(s)...", len(pending))

	for i, date := range pending {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		items := g.fetchDayFromProviders(ctx, date, providers)
		cancel()

		private := 0
		public := make([]GitActivityItem, 0, len(items))
		for _, it := range items {
			if it.Private {
				private++
				continue
			}
			public = append(public, it)
		}
		g.dayStore.Set(date, public, private)

		if i%20 == 0 && i > 0 {
			g.dayStore.Flush()
		}
		time.Sleep(gitPrefetchDelay)
	}

	g.dayStore.Flush()
	log.Printf("[Git] day details prefetched for %d days in %v", len(pending), time.Since(start).Round(time.Second))
}

func (g *GitActivity) HandleDayAPI(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}
	type srcOut struct {
		Name    string `json:"name"`
		Color   string `json:"color"`
		Count   int    `json:"count"`
		Private bool   `json:"private"`
	}
	type actOut struct {
		Type      string `json:"type"`
		Title     string `json:"title"`
		Repo      string `json:"repo"`
		Ref       string `json:"ref"`
		URL       string `json:"url"`
		Time      string `json:"time"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Source    string `json:"source"`
		Color     string `json:"color"`
	}
	g.mu.RLock()
	colors, labels, private := g.providerMeta()
	day := g.calendar[date]
	total := 0
	sourcePrivate := 0
	sources := make([]srcOut, 0, len(day))
	for key, count := range day {
		total += count
		if private[key] {
			sourcePrivate += count
		}
		sources = append(sources, srcOut{Name: labels[key], Color: colors[key], Count: count, Private: private[key]})
	}
	g.mu.RUnlock()

	var items []GitActivityItem
	itemPrivate := 0
	if total > sourcePrivate {
		entry := g.ensureDayDetails(r.Context(), date)
		items = entry.Items
		itemPrivate = entry.Private
	}

	extras := g.feedExtrasForDate(date)

	publicCount := 0
	for _, it := range items {
		c := it.Commits
		if c <= 0 {
			c = 1
		}
		publicCount += c
	}
	for _, it := range extras {
		if it.Type == "pr" || it.Type == "issue" {
			publicCount++
		}
	}

	privateTotal := sourcePrivate + itemPrivate
	if accounted := publicCount + privateTotal; total > accounted {
		privateTotal += total - accounted
	}
	if shown := publicCount + privateTotal; shown > total {
		total = shown
	}

	all := make([]GitActivityItem, 0, len(items)+len(extras))
	all = append(all, items...)
	all = append(all, extras...)
	sort.Slice(all, func(i, j int) bool { return all[i].Time.After(all[j].Time) })

	activities := make([]actOut, 0, len(all))
	for _, it := range all {
		activities = append(activities, actOut{
			Type: it.Type, Title: it.Title, Repo: it.Repo, Ref: it.Ref, URL: it.URL,
			Time: it.Time.Format("15:04"), Additions: it.Additions, Deletions: it.Deletions,
			Source: it.Source, Color: it.Color,
		})
		if len(activities) >= 100 {
			break
		}
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Count > sources[j].Count })
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"date":       date,
		"total":      total,
		"private":    privateTotal,
		"sources":    sources,
		"activities": activities,
	})
}

func (g *GitActivity) feedCachePath() string {
	return filepath.Join(g.store.Dir(), "git_feed.json")
}

func (g *GitActivity) loadFeedCache() {
	b, err := os.ReadFile(g.feedCachePath())
	if err != nil {
		return
	}
	var items []GitActivityItem
	if json.Unmarshal(b, &items) != nil {
		return
	}
	g.mu.Lock()
	if len(g.feed) == 0 {
		g.feed = items
	}
	g.mu.Unlock()
	log.Printf("[Git] feed cache loaded: %d items", len(items))
}

func (g *GitActivity) saveFeedCache() {
	g.mu.RLock()
	items := append([]GitActivityItem(nil), g.feed...)
	g.mu.RUnlock()
	b, err := json.Marshal(items)
	if err != nil {
		return
	}
	tmp := g.feedCachePath() + ".tmp"
	if os.WriteFile(tmp, b, 0644) == nil {
		os.Rename(tmp, g.feedCachePath())
	}
}

func (g *GitActivity) RefreshToday() {
	today := time.Now().Format("2006-01-02")
	g.mu.RLock()
	providers := append([]GitProvider(nil), g.providers...)
	g.mu.RUnlock()
	if len(providers) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	items := g.fetchDayFromProviders(ctx, today, providers)
	private := 0
	public := make([]GitActivityItem, 0, len(items))
	for _, it := range items {
		if it.Private {
			private++
			continue
		}
		public = append(public, it)
	}
	g.dayStore.Set(today, public, private)
	g.dayStore.Flush()
	log.Printf("[Git] today (%s) refreshed: %d public, %d private", today, len(public), private)
}
