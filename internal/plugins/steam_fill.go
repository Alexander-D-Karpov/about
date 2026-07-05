package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"unicode"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

var gamePalette = [][2]string{
	{"#2a3b5c", "#9fc2ff"}, {"#3a2a5c", "#c9a0ff"}, {"#2a5c3b", "#8ff0b0"},
	{"#5c4a2a", "#f0c98a"}, {"#5c2a3b", "#f0a0bc"}, {"#2a5c5c", "#8ff0e8"},
}

func gameInitial(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "?"
}

var steamPlatformMeta = []struct {
	Name  string
	Color string
}{
	{"Windows", "#4d9fff"},
	{"macOS", "#c5ccd4"},
	{"Linux", "#f0a040"},
	{"Steam Deck", "#9b6dff"},
}

func (p *SteamPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	ps := p.playerSummary

	if ps != nil && ps.GameExtraInfo != "" {
		vm.Games.InGame = true
		cover := ps.LocalGameCover
		if cover == "" && ps.GameID != "" {
			cover = steamHeaderURLStr(ps.GameID)
		}
		store := ""
		if ps.GameID != "" {
			store = fmt.Sprintf("https://store.steampowered.com/app/%s/", ps.GameID)
		}
		vm.Games.Current = view.CurrentGameVM{Cover: cover, Name: ps.GameExtraInfo, SteamURL: store}
	}

	for i, g := range p.recentGames {
		if i >= 3 {
			break
		}
		pal := gamePalette[i%len(gamePalette)]
		vm.Games.RecentGames = append(vm.Games.RecentGames, view.GameSmallVM{
			Name:    g.Name,
			Weeks:   fmt.Sprintf("%.1fh last 2 weeks", float64(g.Playtime2w)/60.0),
			Total:   fmt.Sprintf("%.1fh total", float64(g.PlaytimeAll)/60.0),
			Image:   g.LocalIcon,
			Initial: gameInitial(g.Name), Bg: pal[0], Color: pal[1],
		})
	}

	for i, g := range p.topGames {
		if i >= 8 {
			break
		}
		pal := gamePalette[i%len(gamePalette)]
		vm.Games.TopGames = append(vm.Games.TopGames, view.TopGameVM{
			Rank:    fmt.Sprintf("%d", i+1),
			Name:    g.Name,
			Hours:   fmt.Sprintf("%.0fh", float64(g.PlaytimeAll)/60.0),
			Image:   g.LocalIcon,
			Initial: gameInitial(g.Name), Bg: pal[0], Color: pal[1],
		})
	}

	p.statMu.RLock()
	platforms := p.platformTotals
	genreT := p.genreTotals
	p.statMu.RUnlock()

	var donut [][2]interface{}
	for _, meta := range steamPlatformMeta {
		mins := platforms[meta.Name]
		if mins <= 0 {
			continue
		}
		hours := float64(mins) / 60.0
		vm.Games.Platforms = append(vm.Games.Platforms, view.PlatformVM{Name: meta.Name, Color: meta.Color, Value: hours})
		donut = append(donut, [2]interface{}{hours, meta.Color})
	}
	if donut == nil {
		donut = [][2]interface{}{}
	}
	pj, _ := json.Marshal(donut)
	vm.Games.PlatformJSON = template.JS(pj)

	type genreEntry struct {
		name string
		mins int
	}
	entries := make([]genreEntry, 0, len(genreT))
	for name, mins := range genreT {
		entries = append(entries, genreEntry{name, mins})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].mins == entries[j].mins {
			return entries[i].name < entries[j].name
		}
		return entries[i].mins > entries[j].mins
	})
	if len(entries) > 6 {
		entries = entries[:6]
	}
	shownTotal := 0
	for _, e := range entries {
		shownTotal += e.mins
	}
	if shownTotal == 0 {
		shownTotal = 1
	}
	for i, e := range entries {
		vm.Games.Genres = append(vm.Games.Genres, view.GenreVM{
			Name:  e.name,
			Hours: fmt.Sprintf("%.0fh", float64(e.mins)/60.0),
			Share: fmt.Sprintf("%.1f%%", float64(e.mins)/float64(shownTotal)*100),
			Color: tagPalette[i%len(tagPalette)],
		})
	}

	return nil
}
