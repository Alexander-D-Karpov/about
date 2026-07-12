package plugins

import (
	"context"
	"encoding/base64"
	"log"
	"net/http"
	"strings"
	"time"
)

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

func fetchWakatimeOSRange(ctx context.Context, client *http.Client, headers map[string]string, rng string) ([]WakaOSStat, string, bool, error) {
	endpoint := "https://wakatime.com/api/v1/users/current/stats/" + rng

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

func fetchWakatimeStats(ctx context.Context, client *http.Client, apiKey string) (*WakatimeStats, error) {
	headers := map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(apiKey)),
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
		"https://wakatime.com/api/v1/users/current/stats/last_7_days", headers, nil, &weekly); err != nil {
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
		"https://wakatime.com/api/v1/users/current/all_time_since_today", headers, nil, &allTime); err == nil {
		stats.TextTotal = allTime.Data.Text
		stats.HoursTotal = allTime.Data.TotalSeconds / 3600
	}

	var pendingOS []WakaOSStat
	var pendingRange string

	for _, rng := range []string{"all_time", "last_year", "last_6_months"} {
		osList, gotRange, upToDate, err := fetchWakatimeOSRange(ctx, client, headers, rng)
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
