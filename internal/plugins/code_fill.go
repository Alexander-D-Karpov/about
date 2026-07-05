package plugins

import (
	"context"
	"fmt"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *CodePlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	gh := p.githubData
	wk := p.wakatimeData
	p.mutex.RLock()
	langs := p.allRepoLanguages
	p.mutex.RUnlock()

	code := view.CodeVM{ContribTitle: "Contributions", HeatEmpty: "rgba(255,255,255,.05)"}

	if gh != nil {
		code.Totals = append(code.Totals,
			view.CodeTotalVM{Value: fmt.Sprintf("%d", gh.PublicRepos), Label: "Repos", Accent: "#4d9fff", Chip: "rgba(77,159,255,.14)", Icon: "code"},
			view.CodeTotalVM{Value: fmt.Sprintf("%d", gh.TotalStars), Label: "Stars", Accent: "#f0c040", Chip: "rgba(240,192,64,.14)", Icon: "star"},
			view.CodeTotalVM{Value: fmt.Sprintf("%d", gh.TotalCommits), Label: "Commits", Accent: "#10d060", Chip: "rgba(16,208,96,.14)", Icon: "commit"},
		)
	}
	if wk != nil {
		code.Totals = append(code.Totals, view.CodeTotalVM{Value: wk.LastWeek.Text, Label: "This week", Accent: "#b055ff", Chip: "rgba(176,85,255,.14)", Icon: "clock"})
		code.WeekTotal = wk.LastWeek.Text
	}

	cells := make([]view.CellVM, 371)
	for i := range cells {
		cells[i] = view.CellVM{Color: code.HeatEmpty}
	}
	code.ContribCells = cells

	if gh != nil {
		code.ContribStats = []view.StatVM{
			{Value: fmt.Sprintf("%d", gh.TotalCommits), Unit: "commits", Label: "last year", Color: "#10d060"},
			{Value: fmt.Sprintf("%d", gh.PublicRepos), Unit: "repos", Label: "public", Color: "#4d9fff"},
			{Value: fmt.Sprintf("%d", gh.Followers), Label: "followers", Color: "#f0c040"},
		}
	}

	for _, l := range langs {
		pct := fmt.Sprintf("%.1f%%", l.Percentage)
		code.Langs = append(code.Langs, view.LangVM{Name: l.Name, Pct: pct, Color: l.Color, BarPct: pct})
	}

	if gh != nil {
		for _, repo := range gh.RecentRepos {
			meta := repo.UpdatedAt.Format("Jan 2")
			if repo.Language != "" {
				meta = repo.Language + " · " + meta
			}
			code.Activity = append(code.Activity, view.ActivityVM{Icon: "commit", Color: "#4d9fff", Text: repo.Name, Meta: meta})
		}
	}

	if wk != nil {
		max := 0.0
		for _, l := range wk.Languages {
			if l.Percent > max {
				max = l.Percent
			}
		}
		if max == 0 {
			max = 1
		}
		shown := 0
		for _, l := range wk.Languages {
			if l.Percent < 1 {
				continue
			}
			code.ThisWeek = append(code.ThisWeek, view.WeekVM{Name: l.Name, Time: l.Text, Width: fmt.Sprintf("%.1f%%", l.Percent/max*100), Color: l.Color})
			if shown++; shown >= 6 {
				break
			}
		}
	}

	vm.Code = code
	return nil
}
