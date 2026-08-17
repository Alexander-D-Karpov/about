package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	errGitStatsPending = errors.New("stats not ready yet")
	errGitEmptyRepo    = errors.New("repository is empty")
)

const (
	gitMaxRateWait      = 5 * time.Minute
	gitRepoStatsBudget  = 80
	gitStatsPendingTry  = 2
	gitStatsPendingWait = 3 * time.Second
)

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

type gitRateLimitError struct {
	Host  string
	Until time.Time
}

func (e *gitRateLimitError) Error() string {
	return fmt.Sprintf("%s rate limited, resets in %s", e.Host, e.Wait().Round(time.Second))
}

func (e *gitRateLimitError) Wait() time.Duration {
	d := time.Until(e.Until)
	if d < 0 {
		return 0
	}
	return d
}

func gitIsRateLimited(err error) bool {
	var rl *gitRateLimitError
	return errors.As(err, &rl)
}

var gitGate = struct {
	mu    sync.Mutex
	until map[string]time.Time
}{until: make(map[string]time.Time)}

func gitHostOf(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return endpoint
	}
	return u.Host
}

func gitGateBlocked(host string) (time.Time, bool) {
	gitGate.mu.Lock()
	defer gitGate.mu.Unlock()
	until, ok := gitGate.until[host]
	if !ok {
		return time.Time{}, false
	}
	if !time.Now().Before(until) {
		delete(gitGate.until, host)
		return time.Time{}, false
	}
	return until, true
}

func gitGateSet(host string, until time.Time) {
	gitGate.mu.Lock()
	defer gitGate.mu.Unlock()
	if cur, ok := gitGate.until[host]; !ok || until.After(cur) {
		gitGate.until[host] = until
		log.Printf("[Git] %s rate limited, pausing all requests for %s", host, time.Until(until).Round(time.Second))
	}
}

func gitSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// gitSecondaryLimitCooldown is how long to treat a host as blocked after a secondary rate limit.
// GitHub asks for at least a minute; the extra margin keeps background prefetching well clear.
const gitSecondaryLimitCooldown = 5 * time.Minute

func gitBodyIsSecondaryLimit(body []byte) bool {
	msg := strings.ToLower(string(body))
	return strings.Contains(msg, "secondary rate limit") ||
		strings.Contains(msg, "abuse detection") ||
		strings.Contains(msg, "exceeded a secondary")
}

func gitRateLimitReset(resp *http.Response) (time.Time, bool) {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return time.Time{}, false
	}

	if ra := strings.TrimSpace(resp.Header.Get("Retry-After")); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs >= 0 {
			return time.Now().Add(time.Duration(secs) * time.Second), true
		}
	}

	remaining := resp.Header.Get("X-RateLimit-Remaining")
	if remaining == "" {
		remaining = resp.Header.Get("RateLimit-Remaining")
	}
	reset := resp.Header.Get("X-RateLimit-Reset")
	if reset == "" {
		reset = resp.Header.Get("RateLimit-Reset")
	}
	if remaining == "0" && reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			return time.Unix(ts, 0), true
		}
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return time.Now().Add(time.Minute), true
	}
	return time.Time{}, false
}

func gitDoJSON(ctx context.Context, client *http.Client, method, endpoint string, headers map[string]string, body io.Reader, out interface{}) error {
	host := gitHostOf(endpoint)
	if until, blocked := gitGateBlocked(host); blocked {
		return &gitRateLimitError{Host: host, Until: until}
	}

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

	if until, limited := gitRateLimitReset(resp); limited {
		io.Copy(io.Discard, resp.Body)
		gitGateSet(host, until)
		return &gitRateLimitError{Host: host, Until: until}
	}

	switch resp.StatusCode {
	case http.StatusAccepted:
		io.Copy(io.Discard, resp.Body)
		return errGitStatsPending
	case http.StatusNoContent, http.StatusResetContent:
		io.Copy(io.Discard, resp.Body)
		return nil
	case http.StatusConflict:
		io.Copy(io.Discard, resp.Body)
		return errGitEmptyRepo
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

		// GitHub reports secondary rate limits as a 403 whose reason is only in the body: there is
		// no Retry-After and the primary quota still has room, so the header checks above miss it.
		// Without this the caller keeps retrying a request that cannot succeed.
		if (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) &&
			gitBodyIsSecondaryLimit(snippet) {
			until := time.Now().Add(gitSecondaryLimitCooldown)
			gitGateSet(host, until)
			return &gitRateLimitError{Host: host, Until: until}
		}

		return fmt.Errorf("%s %s: status %d: %s", method, endpoint, resp.StatusCode, string(snippet))
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func gitFetchJSON(ctx context.Context, client *http.Client, method, endpoint string, headers map[string]string, payload []byte, out interface{}) error {
	return gitFetchRetry(ctx, client, method, endpoint, headers, payload, out, 1, 0)
}

func gitFetchStatsJSON(ctx context.Context, client *http.Client, method, endpoint string, headers map[string]string, payload []byte, out interface{}) error {
	return gitFetchRetry(ctx, client, method, endpoint, headers, payload, out, gitStatsPendingTry, gitStatsPendingWait)
}

func gitFetchRetry(ctx context.Context, client *http.Client, method, endpoint string, headers map[string]string, payload []byte, out interface{}, pendingAttempts int, pendingWait time.Duration) error {
	if pendingAttempts < 1 {
		pendingAttempts = 1
	}

	attempt := 1
	rateRetries := 0

	for {
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}

		err := gitDoJSON(ctx, client, method, endpoint, headers, body, out)

		var rl *gitRateLimitError
		if errors.As(err, &rl) {
			wait := rl.Wait()
			if wait <= 0 || wait > gitMaxRateWait || rateRetries >= 2 {
				return err
			}
			rateRetries++
			if serr := gitSleep(ctx, wait+2*time.Second); serr != nil {
				return serr
			}
			continue
		}

		if errors.Is(err, errGitStatsPending) && attempt < pendingAttempts {
			attempt++
			if serr := gitSleep(ctx, pendingWait); serr != nil {
				return serr
			}
			continue
		}

		return err
	}
}

type gitRepoRef struct {
	Name     string
	PushedAt int64
}

type repoStatsCollector struct {
	store  *CodeStatsStore
	key    string
	cached map[string]RepoStats
	fresh  map[string]RepoStats
}

func newRepoStatsCollector(store *CodeStatsStore, key string) *repoStatsCollector {
	c := &repoStatsCollector{
		store: store,
		key:   key,
		fresh: make(map[string]RepoStats),
	}
	if store != nil {
		c.cached = store.RepoStatsFor(key)
	}
	if c.cached == nil {
		c.cached = make(map[string]RepoStats)
	}
	return c
}

func (c *repoStatsCollector) Cached(repo string) (RepoStats, bool) {
	if st, ok := c.fresh[repo]; ok {
		return st, true
	}
	st, ok := c.cached[repo]
	return st, ok
}

func (c *repoStatsCollector) Put(repo string, st RepoStats) {
	st.UpdatedAt = time.Now().Unix()
	c.fresh[repo] = st
}

func (c *repoStatsCollector) Flush() {
	if c.store == nil || len(c.fresh) == 0 {
		return
	}
	c.store.SetRepoStatsBatch(c.key, c.fresh)
	for k, v := range c.fresh {
		c.cached[k] = v
	}
	c.fresh = make(map[string]RepoStats)
}

func (c *repoStatsCollector) Sum(repos []string) (RepoStats, int) {
	var out RepoStats
	unmeasured := 0
	for _, name := range repos {
		st, ok := c.Cached(name)
		if !ok {
			unmeasured++
			continue
		}
		out.Commits += st.Commits
		out.Additions += st.Additions
		out.Deletions += st.Deletions
	}
	return out, unmeasured
}

func gitPendingRepos(col *repoStatsCollector, repos []gitRepoRef) []gitRepoRef {
	type cand struct {
		ref gitRepoRef
		pri int
	}

	var out []cand
	for _, r := range repos {
		st, ok := col.Cached(r.Name)
		switch {
		case !ok:
			out = append(out, cand{r, 0})
		case st.PushedAt > 0 && r.PushedAt > st.PushedAt:
			out = append(out, cand{r, 1})
		case st.PushedAt == 0:
			out = append(out, cand{r, 2})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].pri != out[j].pri {
			return out[i].pri < out[j].pri
		}
		return out[i].ref.PushedAt > out[j].ref.PushedAt
	})

	res := make([]gitRepoRef, 0, len(out))
	for _, c := range out {
		res = append(res, c.ref)
	}
	return res
}

func gitFirstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
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
