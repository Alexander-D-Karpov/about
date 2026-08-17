package plugins

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"time"
)

const (
	// A year summary never changes once published, so the history is refreshed rarely. The window
	// is short enough that a newly published year is still picked up within a day.
	steamYearHistoryTTL = 24 * time.Hour
	// How far back to look for published summaries, and how many consecutive empty years to accept
	// before concluding there is nothing older.
	steamYearHistoryMaxYears = 8
	steamYearHistoryMaxGaps  = 2
)

type steamYearReviewResponse struct {
	Response struct {
		Stats struct {
			Year          int `json:"year"`
			PlaytimeStats struct {
				TotalStats struct {
					TotalPlaytimeSeconds int64 `json:"total_playtime_seconds"`
				} `json:"total_stats"`
				// Exact figures, but only for the top handful of games.
				Games []struct {
					AppID int `json:"appid"`
					Stats struct {
						TotalPlaytimeSeconds int64 `json:"total_playtime_seconds"`
					} `json:"stats"`
					RtimeFirstPlayed int64 `json:"rtime_first_played"`
				} `json:"games"`
				// Every game played that year, as a share of the year's total playtime.
				GameSummary []struct {
					AppID                     int   `json:"appid"`
					TotalPlaytimePercentageX1 int64 `json:"total_playtime_percentagex100"`
					RtimeFirstPlayedLifetime  int64 `json:"rtime_first_played_lifetime"`
				} `json:"game_summary"`
			} `json:"playtime_stats"`
		} `json:"stats"`
	} `json:"response"`
}

// YearInReview returns per-game playtime in minutes for one year plus each game's first-played
// time, or ok=false when Steam has not published that year. This is the only public source of
// per-year playtime: GetOwnedGames exposes all-time and last-two-weeks totals only.
//
// It needs no access token, but it does need a public profile.
func (a *steamAPI) YearInReview(ctx context.Context, steamID string, year int) (minutes map[int]int, firstPlayed map[int]int64, ok bool, err error) {
	var resp steamYearReviewResponse
	q := url.Values{
		"steamid": {steamID},
		"year":    {fmt.Sprintf("%d", year)},
	}
	if err := a.get(ctx, "/ISaleFeatureService/GetUserYearInReview/v1/", q, &resp); err != nil {
		return nil, nil, false, err
	}

	stats := resp.Response.Stats.PlaytimeStats
	total := stats.TotalStats.TotalPlaytimeSeconds
	if total <= 0 {
		return nil, nil, false, nil // year not published yet, or nothing played
	}

	minutes = make(map[int]int, len(stats.GameSummary))
	firstPlayed = make(map[int]int64, len(stats.GameSummary))

	// Percentages cover the whole year; approximate but complete.
	for _, g := range stats.GameSummary {
		if g.AppID == 0 {
			continue
		}
		if g.TotalPlaytimePercentageX1 > 0 {
			seconds := float64(total) * (float64(g.TotalPlaytimePercentageX1) / 10000.0)
			if mins := int(seconds / 60); mins > 0 {
				minutes[g.AppID] = mins
			}
		}
		if g.RtimeFirstPlayedLifetime > 0 {
			firstPlayed[g.AppID] = g.RtimeFirstPlayedLifetime
		}
	}

	// Exact figures win wherever Steam gives them.
	for _, g := range stats.Games {
		if g.AppID == 0 {
			continue
		}
		if g.Stats.TotalPlaytimeSeconds > 0 {
			minutes[g.AppID] = int(g.Stats.TotalPlaytimeSeconds / 60)
		}
		if g.RtimeFirstPlayed > 0 {
			if cur, seen := firstPlayed[g.AppID]; !seen || g.RtimeFirstPlayed < cur {
				firstPlayed[g.AppID] = g.RtimeFirstPlayed
			}
		}
	}

	return minutes, firstPlayed, len(minutes) > 0, nil
}

// steamYearHistoryFresh reports whether the cached history can be reused.
func steamYearHistoryFresh(h steamYearHistory) bool {
	return len(h.Years) > 0 && time.Since(time.Unix(h.FetchedAt, 0)) < steamYearHistoryTTL
}

// refreshYearHistory collects every published year summary, walking back from the current year.
// Steam publishes a year only once it has ended, and older years eventually stop being served, so
// the range is discovered rather than assumed.
func (p *SteamPlugin) refreshYearHistory(ctx context.Context, steamID string) {
	if steamYearHistoryFresh(p.store.YearHistory()) {
		return
	}

	now := time.Now().Year()
	years := make(map[int]map[int]int)
	firstPlayed := make(map[int]int64)
	gaps := 0

	for i := 0; i < steamYearHistoryMaxYears; i++ {
		year := now - i

		minutes, first, ok, err := p.api.YearInReview(ctx, steamID, year)
		if err != nil {
			log.Printf("[Steam] year in review %d failed: %v", year, err)
			gaps++
		} else if !ok {
			gaps++
		} else {
			years[year] = minutes
			for appID, ts := range first {
				if cur, seen := firstPlayed[appID]; !seen || ts < cur {
					firstPlayed[appID] = ts
				}
			}
			gaps = 0
		}

		// Two misses in a row means we have walked off the end of what Steam serves. The current
		// year is expected to be missing, so it never counts towards that.
		if gaps >= steamYearHistoryMaxGaps && year < now {
			break
		}
	}

	if len(years) == 0 {
		log.Printf("[Steam] no published year in review available")
		return
	}

	p.store.SetYearHistory(steamYearHistory{
		Years:       years,
		FirstPlayed: firstPlayed,
		FetchedAt:   time.Now().Unix(),
	})

	earliest, latest := 0, 0
	for y := range years {
		if earliest == 0 || y < earliest {
			earliest = y
		}
		if y > latest {
			latest = y
		}
	}
	log.Printf("[Steam] year history: %d..%d (%d years, %d games with a first-played date)",
		earliest, latest, len(years), len(firstPlayed))
}
