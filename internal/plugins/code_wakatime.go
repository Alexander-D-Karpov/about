package plugins

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// wakatimeAPI is the default backend; any other wakatime.api_url (WAKATIME_API_URL) is treated as a
// self-hosted Hakatime server, which speaks WakaTime for heartbeats but has its own stats API.
const wakatimeAPI = "https://wakatime.com/api/v1"

type WakatimeStats struct {
	Text7d     string           `json:"text_7d"`
	Hours7d    float64          `json:"hours_7d"`
	TextTotal  string           `json:"text_total"`
	HoursTotal float64          `json:"hours_total"`
	LangCount  int              `json:"lang_count"`
	Languages  []WakaLangStat   `json:"languages"`
	Editors    []WakaEditorStat `json:"editors"`
	OSAllTime  []WakaOSStat     `json:"os_all_time"`
	OSRange    string           `json:"os_range"`
	OSUpToDate bool             `json:"os_up_to_date"`
	// all-time hours per language (Hakatime backend only); when present, Top Languages shows hours
	LangsAllTime []WakaLangStat `json:"langs_all_time,omitempty"`
}

type WakaOSStat struct {
	Name    string  `json:"name"`
	Percent float64 `json:"percent"`
	Text    string  `json:"text"`
}

type WakaLangStat struct {
	Name    string  `json:"name"`
	Percent float64 `json:"percent"`
	Text    string  `json:"text"`
	Seconds float64 `json:"seconds"`
}

type WakaEditorStat struct {
	Name    string  `json:"name"`
	Percent float64 `json:"percent"`
}

type wakaRawOS struct {
	Name         string  `json:"name"`
	Percent      float64 `json:"percent"`
	Text         string  `json:"text"`
	TotalSeconds float64 `json:"total_seconds"`
}

func wakaOSStats(raw []wakaRawOS) []WakaOSStat {
	var total float64
	for _, o := range raw {
		total += o.TotalSeconds
	}
	var out []WakaOSStat
	for _, o := range raw {
		pct := o.Percent
		if pct == 0 && total > 0 {
			pct = o.TotalSeconds / total * 100
		}
		if pct < 0.5 {
			continue
		}
		out = append(out, WakaOSStat{Name: o.Name, Percent: pct, Text: o.Text})
	}
	return out
}

func wakaRangeLabel(rng string) string {
	switch rng {
	case "all_time":
		return "all time"
	case "last_year":
		return "last year"
	case "last_6_months":
		return "last 6 months"
	case "last_30_days":
		return "last 30 days"
	case "last_7_days":
		return "last 7 days"
	default:
		return strings.ReplaceAll(rng, "_", " ")
	}
}

func fetchWakatimeOSRange(ctx context.Context, client *http.Client, base string, headers map[string]string, rng string) ([]WakaOSStat, string, bool, error) {
	endpoint := base + "/users/current/stats/" + rng

	var resp struct {
		Data struct {
			Range             string      `json:"range"`
			Status            string      `json:"status"`
			IsUpToDate        bool        `json:"is_up_to_date"`
			PercentCalculated int         `json:"percent_calculated"`
			OperatingSystems  []wakaRawOS `json:"operating_systems"`
		} `json:"data"`
	}

	var err error
	gotData := false

	for attempt := 0; attempt < 12; attempt++ {
		err = gitDoJSON(ctx, client, "GET", endpoint, headers, nil, &resp)
		if err == nil {
			gotData = true
			if resp.Data.IsUpToDate {
				break
			}
			log.Printf("[Code] wakatime %s stats still computing (%d%%, status=%s), polling...",
				rng, resp.Data.PercentCalculated, resp.Data.Status)
		} else if err != errGitStatsPending {
			return nil, "", false, err
		}

		if attempt == 11 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, "", false, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}

	if !gotData {
		return nil, "", false, err
	}

	gotRange := resp.Data.Range
	if gotRange == "" {
		gotRange = rng
	}
	return wakaOSStats(resp.Data.OperatingSystems), gotRange, resp.Data.IsUpToDate, nil
}

func fetchWakatimeStats(ctx context.Context, client *http.Client, apiKey, apiURL string) (*WakatimeStats, error) {
	headers := map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(apiKey)),
	}
	base := strings.TrimRight(apiURL, "/")
	if base == "" {
		base = wakatimeAPI
	}
	if !strings.Contains(base, "wakatime.com") {
		return fetchHakatimeStats(ctx, client, base, headers)
	}
	var weekly struct {
		Data struct {
			TotalSeconds       float64 `json:"total_seconds"`
			HumanReadableTotal string  `json:"human_readable_total"`
			Languages          []struct {
				Name         string  `json:"name"`
				Percent      float64 `json:"percent"`
				Text         string  `json:"text"`
				TotalSeconds float64 `json:"total_seconds"`
			} `json:"languages"`
			Editors []struct {
				Name    string  `json:"name"`
				Percent float64 `json:"percent"`
			} `json:"editors"`
			OperatingSystems []wakaRawOS `json:"operating_systems"`
		} `json:"data"`
	}
	if err := gitDoJSON(ctx, client, "GET",
		base+"/users/current/stats/last_7_days", headers, nil, &weekly); err != nil {
		return nil, err
	}
	stats := &WakatimeStats{
		Text7d:    weekly.Data.HumanReadableTotal,
		Hours7d:   weekly.Data.TotalSeconds / 3600,
		LangCount: len(weekly.Data.Languages),
	}
	for _, l := range weekly.Data.Languages {
		stats.Languages = append(stats.Languages, WakaLangStat{
			Name: l.Name, Percent: l.Percent, Text: l.Text, Seconds: l.TotalSeconds,
		})
	}
	for _, e := range weekly.Data.Editors {
		if e.Percent < 1 {
			continue
		}
		stats.Editors = append(stats.Editors, WakaEditorStat{Name: e.Name, Percent: e.Percent})
	}
	if len(stats.Editors) > 5 {
		stats.Editors = stats.Editors[:5]
	}
	var allTime struct {
		Data struct {
			TotalSeconds float64 `json:"total_seconds"`
			Text         string  `json:"text"`
		} `json:"data"`
	}
	if err := gitDoJSON(ctx, client, "GET",
		base+"/users/current/all_time_since_today", headers, nil, &allTime); err == nil {
		stats.TextTotal = allTime.Data.Text
		stats.HoursTotal = allTime.Data.TotalSeconds / 3600
	}

	var pendingOS []WakaOSStat
	var pendingRange string

	for _, rng := range []string{"all_time", "last_year", "last_6_months"} {
		osList, gotRange, upToDate, err := fetchWakatimeOSRange(ctx, client, base, headers, rng)
		if err != nil {
			log.Printf("[Code] wakatime %s os fetch failed: %v", rng, err)
			continue
		}
		if len(osList) == 0 {
			log.Printf("[Code] wakatime %s os returned no data, trying next range", rng)
			continue
		}
		if upToDate {
			stats.OSAllTime = osList
			stats.OSRange = wakaRangeLabel(gotRange)
			stats.OSUpToDate = true
			if gotRange != rng {
				log.Printf("[Code] wakatime requested %s os but api returned range %s", rng, gotRange)
			}
			break
		}
		if pendingOS == nil {
			pendingOS = osList
			pendingRange = gotRange
		}
		log.Printf("[Code] wakatime %s os not up to date yet, keeping as fallback and trying next range", rng)
	}

	if len(stats.OSAllTime) == 0 && pendingOS != nil {
		stats.OSAllTime = pendingOS
		stats.OSRange = wakaRangeLabel(pendingRange)
		stats.OSUpToDate = false
		log.Printf("[Code] wakatime os using partial %s data (still computing server-side)", pendingRange)
	}

	if len(stats.OSAllTime) == 0 && len(weekly.Data.OperatingSystems) > 0 {
		stats.OSAllTime = wakaOSStats(weekly.Data.OperatingSystems)
		stats.OSRange = "last 7 days"
		log.Printf("[Code] wakatime os falling back to last_7_days data")
	}
	return stats, nil
}

// ---------------------------------------------------------------- Hakatime

type hakaItem struct {
	Name         string    `json:"name"`
	TotalSeconds float64   `json:"totalSeconds"`
	TotalDaily   []float64 `json:"totalDaily"`
}

func (h hakaItem) seconds() float64 {
	if h.TotalSeconds > 0 {
		return h.TotalSeconds
	}
	var t float64
	for _, d := range h.TotalDaily {
		t += d
	}
	return t
}

type hakaStats struct {
	TotalSeconds float64    `json:"totalSeconds"`
	Languages    []hakaItem `json:"languages"`
	Editors      []hakaItem `json:"editors"`
	Platforms    []hakaItem `json:"platforms"`
}

// wakaHuman renders seconds the way WakaTime does ("12 hrs 5 mins"), so compactWakaText works on both backends.
func wakaHuman(sec float64) string {
	h, m := int(sec)/3600, int(sec)/60%60
	unit := func(n int, one, many string) string {
		if n == 1 {
			return fmt.Sprintf("%d %s", n, one)
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	switch {
	case h > 0 && m > 0:
		return unit(h, "hr", "hrs") + " " + unit(m, "min", "mins")
	case h > 0:
		return unit(h, "hr", "hrs")
	default:
		return unit(m, "min", "mins")
	}
}

// hakaPlatform turns Hakatime's raw platform strings ("(linux-7.2.3-arch1-2-unknown)") into OS names.
func hakaPlatform(raw string) string {
	r := strings.ToLower(raw)
	switch {
	case strings.Contains(r, "android"):
		return "Android"
	case strings.Contains(r, "linux"):
		return "Linux"
	case strings.Contains(r, "darwin"), strings.Contains(r, "mac"):
		return "Mac"
	case strings.Contains(r, "windows"), strings.Contains(r, "win32"):
		return "Windows"
	}
	return strings.Trim(raw, "()")
}

func hakaFetch(ctx context.Context, client *http.Client, base string, headers map[string]string, from, to time.Time) (*hakaStats, error) {
	q := url.Values{}
	q.Set("start", from.UTC().Format("2006-01-02")+"T00:00:00Z")
	q.Set("end", to.UTC().Format("2006-01-02")+"T23:59:59Z")
	var out hakaStats
	if err := gitDoJSON(ctx, client, "GET", base+"/users/current/stats?"+q.Encode(), headers, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// fold sums items whose names map to the same key and returns them by time, largest first.
func hakaFold(items []hakaItem, key func(string) string) ([]string, map[string]float64, float64) {
	sum := map[string]float64{}
	var total float64
	for _, it := range items {
		s := it.seconds()
		if s <= 0 {
			continue
		}
		sum[key(it.Name)] += s
		total += s
	}
	names := make([]string, 0, len(sum))
	for n := range sum {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return sum[names[i]] > sum[names[j]] })
	return names, sum, total
}

func fetchHakatimeStats(ctx context.Context, client *http.Client, base string, headers map[string]string) (*WakatimeStats, error) {
	now := time.Now()
	week, err := hakaFetch(ctx, client, base, headers, now.AddDate(0, 0, -6), now)
	if err != nil {
		return nil, err
	}
	stats := &WakatimeStats{Text7d: wakaHuman(week.TotalSeconds), Hours7d: week.TotalSeconds / 3600}

	names, sec, total := hakaFold(week.Languages, func(s string) string { return s })
	stats.LangCount = len(names)
	for _, n := range names {
		stats.Languages = append(stats.Languages, WakaLangStat{
			Name: n, Percent: sec[n] / total * 100, Text: wakaHuman(sec[n]), Seconds: sec[n],
		})
	}
	// editors come as "name/version": one row per editor
	names, sec, total = hakaFold(week.Editors, func(s string) string { return strings.SplitN(s, "/", 2)[0] })
	for _, n := range names {
		if pct := sec[n] / total * 100; pct >= 1 && len(stats.Editors) < 5 {
			stats.Editors = append(stats.Editors, WakaEditorStat{Name: n, Percent: pct})
		}
	}

	// all time: Hakatime clamps any range to the last 365 days, so walk back a year at a time and add up
	// (stops after two empty years in a row); the OS split comes from the same windows
	var allSec float64
	var platforms, allLangs []hakaItem
	empty := 0
	for end, i := now, 0; i < 15 && empty < 2; i++ {
		start := end.AddDate(0, 0, -364)
		w, err := hakaFetch(ctx, client, base, headers, start, end)
		if err != nil {
			log.Printf("[Code] hakatime all-time window %s failed: %v", start.Format("2006-01-02"), err)
			break
		}
		if w.TotalSeconds > 0 {
			empty = 0
		} else {
			empty++
		}
		allSec += w.TotalSeconds
		platforms = append(platforms, w.Platforms...)
		allLangs = append(allLangs, w.Languages...)
		end = start.AddDate(0, 0, -1)
	}
	if allSec > 0 {
		stats.TextTotal = wakaHuman(allSec)
		stats.HoursTotal = allSec / 3600
		names, sec, total = hakaFold(platforms, hakaPlatform)
		for _, n := range names {
			if pct := sec[n] / total * 100; pct >= 0.5 {
				stats.OSAllTime = append(stats.OSAllTime, WakaOSStat{Name: n, Percent: pct, Text: wakaHuman(sec[n])})
			}
		}
		stats.OSRange, stats.OSUpToDate = "all time", len(stats.OSAllTime) > 0
		names, sec, total = hakaFold(allLangs, func(s string) string { return s })
		for _, n := range names {
			stats.LangsAllTime = append(stats.LangsAllTime, WakaLangStat{
				Name: n, Percent: sec[n] / total * 100, Text: wakaHuman(sec[n]), Seconds: sec[n],
			})
		}
	}
	return stats, nil
}
