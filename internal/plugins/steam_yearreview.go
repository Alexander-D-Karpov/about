package plugins

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"time"
)

const (
	// A summary for the current year is final: that year has ended and its figures never change,
	// so there is nothing to re-check often.
	steamYearReviewTTL = 30 * 24 * time.Hour
	// While the newest summary we hold is for an earlier year, a newer one may be published at
	// any time, so re-check daily and roll over on its own.
	steamYearReviewStaleTTL = 24 * time.Hour
)

// steamYearReviewCandidates lists the years worth asking for, newest first. Steam publishes a
// year's summary only after it ends, so the current year is usually empty and the previous one is
// the newest available. Deriving this from the clock is what makes the page roll over on its own.
func steamYearReviewCandidates(now int) []int {
	return []int{now, now - 1}
}

// steamYearReviewFresh reports whether a cached summary can be reused.
func steamYearReviewFresh(cached steamYearReview, now int, at time.Time) bool {
	if cached.Year <= 0 || len(cached.Playtime) == 0 {
		return false
	}
	age := time.Since(time.Unix(cached.FetchedAt, 0))
	if cached.Year >= now {
		return age < steamYearReviewTTL
	}
	// An older year: keep looking for a newer one.
	return age < steamYearReviewStaleTTL
}

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
				} `json:"games"`
				// Every game played that year, as a share of the year's total playtime.
				GameSummary []struct {
					AppID                     int   `json:"appid"`
					TotalPlaytimePercentageX1 int64 `json:"total_playtime_percentagex100"`
				} `json:"game_summary"`
			} `json:"playtime_stats"`
		} `json:"stats"`
	} `json:"response"`
}

// YearInReview returns per-game playtime in minutes for one year, or ok=false when Steam has not
// published that year yet. This is the only public source of per-year playtime: GetOwnedGames only
// exposes all-time and last-two-weeks totals.
//
// It needs no access token, but it does need a public profile.
func (a *steamAPI) YearInReview(ctx context.Context, steamID string, year int) (map[int]int, bool, error) {
	var resp steamYearReviewResponse
	q := url.Values{
		"steamid": {steamID},
		"year":    {fmt.Sprintf("%d", year)},
	}
	if err := a.get(ctx, "/ISaleFeatureService/GetUserYearInReview/v1/", q, &resp); err != nil {
		return nil, false, err
	}

	stats := resp.Response.Stats.PlaytimeStats
	total := stats.TotalStats.TotalPlaytimeSeconds
	if total <= 0 {
		return nil, false, nil // year not published yet, or nothing played
	}

	out := make(map[int]int, len(stats.GameSummary))

	// Percentages cover the whole year; they are approximate but complete.
	for _, g := range stats.GameSummary {
		if g.AppID == 0 || g.TotalPlaytimePercentageX1 <= 0 {
			continue
		}
		seconds := float64(total) * (float64(g.TotalPlaytimePercentageX1) / 10000.0)
		if mins := int(seconds / 60); mins > 0 {
			out[g.AppID] = mins
		}
	}

	// Exact figures win wherever Steam gives them.
	for _, g := range stats.Games {
		if g.AppID == 0 || g.Stats.TotalPlaytimeSeconds <= 0 {
			continue
		}
		out[g.AppID] = int(g.Stats.TotalPlaytimeSeconds / 60)
	}

	return out, len(out) > 0, nil
}

// refreshYearReview fetches the most recent year Steam has data for. The current year is only
// published after it ends, so this falls back to the previous one and labels it accordingly.
func (p *SteamPlugin) refreshYearReview(ctx context.Context, steamID string) {
	now := time.Now().Year()
	cached := p.store.YearReview()
	if steamYearReviewFresh(cached, now, time.Now()) {
		return
	}

	for _, year := range steamYearReviewCandidates(now) {
		playtime, ok, err := p.api.YearInReview(ctx, steamID, year)
		if err != nil {
			log.Printf("[Steam] year in review %d failed: %v", year, err)
			continue
		}
		if !ok {
			continue // Steam has not published this year yet
		}

		p.store.SetYearReview(steamYearReview{
			Year:      year,
			Playtime:  playtime,
			FetchedAt: time.Now().Unix(),
		})
		log.Printf("[Steam] year in review %d: %d games with playtime", year, len(playtime))
		return
	}

	if cached.Year > 0 {
		// Keep serving what we already have rather than blanking the page.
		log.Printf("[Steam] no newer year in review published, keeping %d", cached.Year)
		return
	}
	log.Printf("[Steam] no published year in review available yet")
}
