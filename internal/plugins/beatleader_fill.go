package plugins

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func blInitial(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "♪"
}

func (p *BeatLeaderPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	if p.playerData == nil {
		return nil
	}
	pd := p.playerData

	p.cubesMutex.RLock()
	cubes := p.cachedCubesSliced
	p.cubesMutex.RUnlock()

	bl := view.BeatLeaderVM{
		PP:          fmt.Sprintf("%.0f", pd.PP),
		RankGlobal:  fmt.Sprintf("#%d", pd.Rank),
		RankRU:      fmt.Sprintf("#%d", pd.CountryRank),
		AvgAcc:      fmt.Sprintf("%.2f%%", pd.ScoreStats.AverageRankedAccuracy*100),
		RankedPlays: fmt.Sprintf("%d", pd.ScoreStats.RankedPlayCount),
		Cubes:       formatNumberWithCommas(cubes),
	}

	for i, s := range p.recentScores {
		if i >= 6 {
			break
		}
		diffColor := "#3fb950"
		if s.Leaderboard.Difficulty.Stars > 7 {
			diffColor = "#e8557a"
		} else if s.Leaderboard.Difficulty.Stars > 4 {
			diffColor = "#f0a040"
		}
		diff := s.Leaderboard.Difficulty.DifficultyName
		if s.Leaderboard.Difficulty.Stars > 0 {
			diff = fmt.Sprintf("%s · %.1f★", diff, s.Leaderboard.Difficulty.Stars)
		}
		pp := ""
		if s.PP > 0 {
			pp = fmt.Sprintf("%.0fpp", s.PP)
		}
		replay := ""
		if s.ID != 0 {
			replay = fmt.Sprintf("https://replay.beatleader.com/?scoreId=%d", s.ID)
		}
		bl.Maps = append(bl.Maps, view.BLMapVM{
			Initial:   blInitial(s.Leaderboard.Song.Name),
			Name:      s.Leaderboard.Song.Name,
			Diff:      diff,
			DiffColor: diffColor,
			Acc:       fmt.Sprintf("%.2f%%", s.Accuracy*100),
			PP:        pp,
			Image:     s.Leaderboard.Song.CoverImage,
			Replay:    replay,
		})
	}

	vm.Games.BL = bl
	return nil
}
