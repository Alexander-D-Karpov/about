package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

var lfmPalette = [][2]string{
	{"#3a7bd5", "#fff"}, {"#9046d8", "#fff"}, {"#1a9e54", "#fff"},
	{"#cf8420", "#fff"}, {"#cf4a6e", "#fff"}, {"#1d9e92", "#fff"},
}

var coverGradients = []string{
	"linear-gradient(135deg,#3a7bd5,#2a5298)",
	"linear-gradient(135deg,#9046d8,#6a2cb0)",
	"linear-gradient(135deg,#1a9e54,#0f7a3f)",
	"linear-gradient(135deg,#cf8420,#a86510)",
	"linear-gradient(135deg,#cf4a6e,#a02f50)",
	"linear-gradient(135deg,#1d9e92,#127870)",
}

var totalGradients = []template.CSS{
	"linear-gradient(135deg,#34d399,#10b981)",
	"linear-gradient(135deg,#60a5fa,#3b82f6)",
	"linear-gradient(135deg,#c084fc,#a855f7)",
	"linear-gradient(135deg,#fbbf24,#f59e0b)",
}

var tagPalette = []string{"#4d9fff", "#10d060", "#b055ff", "#f0a040", "#e8557a", "#2fd4c4", "#f0c040", "#7db5ff"}

func paletteAt(i int) (string, string) {
	n := len(lfmPalette)
	e := lfmPalette[((i%n)+n)%n]
	return e[0], e[1]
}

func trackInitial(title string) string {
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "♪"
}

func fmtSec(s int) string {
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func formatDelta(pct float64) string {
	if pct >= 0.5 {
		return fmt.Sprintf("↑%.0f%%", pct)
	}
	if pct <= -0.5 {
		return fmt.Sprintf("↓%.0f%%", -pct)
	}
	return ""
}

func shortCount(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

func sameTrack(sp *SpotifyNowPlaying, t *LastFMTrack) bool {
	if sp == nil || t == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(sp.Track), strings.TrimSpace(t.Name)) &&
		strings.EqualFold(strings.TrimSpace(sp.Artist), strings.TrimSpace(t.Artist.Text))
}

func (p *LastFMPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	p.trackMutex.RLock()
	cur := p.currentTrack
	recent := append([]LastFMTrack(nil), p.recentTracks...)
	start := p.trackStartTime
	length := p.currentTrackLength
	p.trackMutex.RUnlock()

	p.statsMutex.RLock()
	var stats lfmStats
	if p.stats != nil {
		stats = *p.stats
	}
	p.statsMutex.RUnlock()

	music := view.MusicVM{Totals: buildTotals(&stats)}

	p.lovedMu.RLock()
	lovedCount := p.lovedCount
	p.lovedMu.RUnlock()
	p.spSavedMu.RLock()
	spLiked := p.spLikedCount
	p.spSavedMu.RUnlock()

	music.SpotifyConnected = p.spotify != nil && p.spotify.UserEnabled()
	if lovedCount > 0 {
		music.LovedCount = formatNumberWithCommas(int64(lovedCount))
	}
	if music.SpotifyConnected {
		music.SpotifyLikedCount = formatNumberWithCommas(int64(spLiked))
	}

	var curKey string
	if cur != nil {
		curKey = strings.ToLower(cur.Artist.Text + "|" + cur.Name)
		playing := p.isNowPlaying(cur)

		duration := length
		started := int64(0)
		if !start.IsZero() {
			started = start.Unix()
		}

		if sp := p.spotifyNowCached(); sp != nil && sp.IsPlaying && sameTrack(sp, cur) {
			if sp.DurationMs > 0 {
				duration = sp.DurationMs / 1000
			}
			started = time.Now().Add(-time.Duration(sp.ProgressMs) * time.Millisecond).Unix()
			playing = true
		}

		elapsed := 0
		if playing && started > 0 {
			elapsed = int(time.Now().Unix() - started)
		}
		if duration > 0 && elapsed > duration {
			elapsed = duration
		}
		if elapsed < 0 {
			elapsed = 0
		}

		progress := "0%"
		if duration > 0 {
			progress = fmt.Sprintf("%.1f%%", float64(elapsed)/float64(duration)*100)
		}

		music.Now = view.NowVM{
			Playing:      playing,
			Title:        cur.Name,
			Artist:       cur.Artist.Text,
			Album:        cur.Album.Text,
			Art:          p.imageForVM(cur),
			ProgressPct:  progress,
			Elapsed:      fmtSec(elapsed),
			Duration:     fmtSec(duration),
			ElapsedSec:   elapsed,
			DurationSec:  duration,
			LastfmURL:    cur.URL,
			StartedAt:    started,
			LovedLastfm:  p.isLovedTrack(cur.Artist.Text, cur.Name),
			LikedSpotify: p.spotifySavedCached(cur.Artist.Text, cur.Name),
		}
	}

	seen := map[string]bool{}
	for i := range recent {
		t := &recent[i]
		k := strings.ToLower(t.Artist.Text + "|" + t.Name)
		if cur != nil && k == curKey {
			continue
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		bg, fg := paletteAt(len(music.Recent))
		music.Recent = append(music.Recent, view.RecentTrackVM{
			Title:        t.Name,
			Artist:       t.Artist.Text,
			Ago:          p.getRelativeTimeForTrack(t),
			Initial:      trackInitial(t.Name),
			Bg:           bg,
			Color:        fg,
			Image:        p.imageForVM(t),
			Loved:        p.isLovedTrack(t.Artist.Text, t.Name),
			LikedSpotify: p.spotifySavedCached(t.Artist.Text, t.Name),
		})
		if len(music.Recent) >= 12 {
			break
		}
	}

	music.TopArtists = buildCovers(stats.TopArtists)
	music.TopAlbums = buildCovers(stats.TopAlbums)

	wj, _ := json.Marshal(stats.Weekly)
	music.WeeklyJSON = template.JS(wj)
	music.WeeklyPeak = stats.WeeklyPeak
	music.Tags = buildTags(stats.Tags)

	vm.Music = music
	return nil
}

func buildTotals(s *lfmStats) []view.TotalVM {
	return []view.TotalVM{
		{Label: "Scrobbles", Value: formatNumberWithCommas(s.Scrobbles), Color: totalGradients[0], Delta: formatDelta(s.ScrobblesDelta), Spark: sparkFor(s.Weekly, 0, 13)},
		{Label: "Tracks", Value: formatNumberWithCommas(s.Tracks), Color: totalGradients[1], Delta: formatDelta(s.TracksDelta), Spark: sparkFor(s.Weekly, 1, 13)},
		{Label: "Artists", Value: formatNumberWithCommas(s.Artists), Color: totalGradients[2], Delta: formatDelta(s.ArtistsDelta), Spark: sparkFor(s.Weekly, 2, 13)},
		{Label: "Albums", Value: formatNumberWithCommas(s.Albums), Color: totalGradients[3], Delta: formatDelta(s.AlbumsDelta), Spark: sparkFor(s.Weekly, 3, 13)},
	}
}

func sparkFor(weekly [7]int, seed, n int) []int {
	base := make([]float64, 7)
	maxB := 1.0
	for i, v := range weekly {
		base[i] = float64(v)
		if base[i] > maxB {
			maxB = base[i]
		}
	}
	out := make([]int, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1) * 6.0
		lo := int(t)
		frac := t - float64(lo)
		v := base[6]
		if lo+1 < 7 {
			v = base[lo]*(1-frac) + base[lo+1]*frac
		}
		norm := v / maxB
		wob := 0.5 + 0.5*math.Sin(float64(i)*0.85+float64(seed)*1.9)
		mixed := norm*0.6 + wob*0.4
		if mixed < 0 {
			mixed = 0
		}
		out[i] = int(mixed*22) + 2
	}
	return out
}

func buildCovers(items []lfmTopItem) []view.CoverVM {
	out := make([]view.CoverVM, 0, len(items))
	for i, it := range items {
		out = append(out, view.CoverVM{
			Name:  it.Name,
			Plays: formatNumberWithCommas(it.Plays),
			Image: it.Image,
			Bg:    template.CSS(coverGradients[i%len(coverGradients)]),
		})
	}
	return out
}

func buildTags(tags []lfmTag) []view.TagVM {
	if len(tags) == 0 {
		return nil
	}
	max := tags[0].Count
	for _, t := range tags {
		if t.Count > max {
			max = t.Count
		}
	}
	if max <= 0 {
		max = 1
	}
	out := make([]view.TagVM, 0, len(tags))
	for i, t := range tags {
		ratio := float64(t.Count) / float64(max)
		out = append(out, view.TagVM{
			Name: t.Name, Plays: shortCount(t.Count),
			SizePx: 13 + int(ratio*18), Color: tagPalette[i%len(tagPalette)],
		})
	}
	return out
}
