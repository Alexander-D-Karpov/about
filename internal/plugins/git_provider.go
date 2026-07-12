package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var errGitStatsPending = errors.New("stats not ready yet")

type GitActivityItem struct {
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Repo      string    `json:"repo"`
	Ref       string    `json:"ref"`
	URL       string    `json:"url"`
	Time      time.Time `json:"time"`
	Additions int       `json:"additions"`
	Deletions int       `json:"deletions"`
	Commits   int       `json:"commits"`
	Source    string    `json:"source"`
	Color     string    `json:"color"`
	Private   bool      `json:"private"`
}

type GitSourceStats struct {
	Commits   int64 `json:"commits"`
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	Repos     int   `json:"repos"`
	Partial   bool  `json:"partial"`
}

type GitProvider interface {
	Key() string
	Label() string
	Color() string
	Private() bool
	FetchCalendar(ctx context.Context, from, to time.Time) (map[string]int, error)
	FetchActivities(ctx context.Context, since time.Time, limit int) ([]GitActivityItem, error)
	FetchStats(ctx context.Context) (*GitSourceStats, error)
}

type gitSourceConfig struct {
	Type     string
	Name     string
	BaseURL  string
	Username string
	Token    string
	Color    string
	Private  bool
}

func parseGitSourceConfig(raw map[string]interface{}) gitSourceConfig {
	get := func(k string) string {
		if v, ok := raw[k].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}

	cfg := gitSourceConfig{
		Type:     strings.ToLower(get("type")),
		Name:     get("name"),
		BaseURL:  strings.TrimRight(get("base_url"), "/"),
		Username: get("username"),
		Token:    get("token"),
		Color:    get("color"),
	}
	if cfg.BaseURL != "" && !strings.Contains(cfg.BaseURL, "://") {
		cfg.BaseURL = "https://" + cfg.BaseURL
	}
	if v, ok := raw["private"].(bool); ok {
		cfg.Private = v
	}
	if cfg.Name == "" {
		cfg.Name = cfg.Type
	}
	if cfg.Color == "" {
		switch cfg.Type {
		case "github":
			cfg.Color = "#7aa2ff"
		case "gitlab":
			cfg.Color = "#fc6d26"
		default:
			cfg.Color = "#609926"
		}
	}
	return cfg
}

func gitFirstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func gitProviderFromConfig(cfg gitSourceConfig, client *http.Client) GitProvider {
	if cfg.Username == "" {
		return nil
	}
	switch cfg.Type {
	case "github":
		return NewGitHubProvider(cfg, client)
	case "gitlab":
		return NewGitLabProvider(cfg, client)
	case "gitea", "forgejo":
		return NewGiteaProvider(cfg, client)
	}
	return nil
}

func gitDoJSON(ctx context.Context, client *http.Client, method, endpoint string, headers map[string]string, body io.Reader, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted {
		io.Copy(io.Discard, resp.Body)
		return errGitStatsPending
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("%s %s: status %d: %s", method, endpoint, resp.StatusCode, string(snippet))
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func gitParseHexColor(s string) (int, int, int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseInt(s, 16, 64)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v>>16) & 0xff, int(v>>8) & 0xff, int(v) & 0xff, true
}

func gitBlendColors(counts map[string]int, colors map[string]string) string {
	var r, g, b float64
	total := 0
	for key, c := range counts {
		if c <= 0 {
			continue
		}
		cr, cg, cb, ok := gitParseHexColor(colors[key])
		if !ok {
			cr, cg, cb = 0x4d, 0x9f, 0xff
		}
		r += float64(cr * c)
		g += float64(cg * c)
		b += float64(cb * c)
		total += c
	}
	if total == 0 {
		return "#4d9fff"
	}
	return fmt.Sprintf("#%02x%02x%02x", int(r/float64(total)), int(g/float64(total)), int(b/float64(total)))
}

func gitShortRepo(full string) string {
	full = strings.TrimSuffix(full, "/")
	if idx := strings.LastIndex(full, "/"); idx >= 0 {
		return full[idx+1:]
	}
	return full
}

func gitShortRef(ref string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/tags/")
	return ref
}

func gitRelTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

func gitFormatCount(n int64) string {
	if n < 1000 {
		return strconv.FormatInt(n, 10)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.2fM", float64(n)/1000000)
}

func gitPlural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
