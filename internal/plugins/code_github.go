package plugins

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type GitHubRecentRepo struct {
	Name      string         `json:"name"`
	URL       string         `json:"url"`
	MainLang  string         `json:"main_lang"`
	Commits   int            `json:"commits"`
	Stars     int            `json:"stars"`
	Languages []CodeLangStat `json:"languages"`
}

type GitHubCodeStats struct {
	Followers   int                `json:"followers"`
	PublicRepos int                `json:"public_repos"`
	TotalStars  int                `json:"total_stars"`
	Languages   []CodeLangStat     `json:"languages"`
	RecentRepos []GitHubRecentRepo `json:"recent_repos"`
}

type CodeLangStat struct {
	Name    string  `json:"name"`
	Color   string  `json:"color"`
	Percent float64 `json:"percent"`
}

func fetchGitHubCodeStats(ctx context.Context, client *http.Client, username, token string) (*GitHubCodeStats, error) {
	headers := map[string]string{"X-GitHub-Api-Version": "2022-11-28"}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	var user struct {
		Followers   int `json:"followers"`
		PublicRepos int `json:"public_repos"`
	}
	if err := gitDoJSON(ctx, client, "GET",
		fmt.Sprintf("https://api.github.com/users/%s", username), headers, nil, &user); err != nil {
		return nil, err
	}
	stats := &GitHubCodeStats{
		Followers:   user.Followers,
		PublicRepos: user.PublicRepos,
	}
	langCounts := make(map[string]int)
	totalLangRepos := 0
	for page := 1; page <= 3; page++ {
		var repos []struct {
			StargazersCount int    `json:"stargazers_count"`
			Language        string `json:"language"`
			Fork            bool   `json:"fork"`
		}
		endpoint := fmt.Sprintf("https://api.github.com/users/%s/repos?per_page=100&page=%d&type=owner", username, page)
		if err := gitDoJSON(ctx, client, "GET", endpoint, headers, nil, &repos); err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		if len(repos) == 0 {
			break
		}
		for _, r := range repos {
			stats.TotalStars += r.StargazersCount
			if !r.Fork && r.Language != "" {
				langCounts[r.Language]++
				totalLangRepos++
			}
		}
		if len(repos) < 100 {
			break
		}
	}
	if totalLangRepos > 0 {
		for name, count := range langCounts {
			stats.Languages = append(stats.Languages, CodeLangStat{
				Name:    name,
				Color:   GetLanguageColor(name),
				Percent: float64(count) / float64(totalLangRepos) * 100,
			})
		}
		sort.Slice(stats.Languages, func(i, j int) bool {
			return stats.Languages[i].Percent > stats.Languages[j].Percent
		})
		if len(stats.Languages) > 8 {
			var other float64
			for _, l := range stats.Languages[8:] {
				other += l.Percent
			}
			stats.Languages = stats.Languages[:8]
			stats.Languages = append(stats.Languages, CodeLangStat{
				Name: "Other", Color: GetLanguageColor("Other"), Percent: other,
			})
		}
	}
	return stats, nil
}

func isBotCommit(committerLogin, committerName string) bool {
	l := strings.ToLower(committerLogin)
	n := strings.ToLower(committerName)
	return strings.HasSuffix(l, "[bot]") || strings.Contains(l, "github-actions") ||
		strings.HasSuffix(n, "[bot]") || strings.Contains(n, "github-actions") ||
		strings.Contains(n, "github actions")
}
