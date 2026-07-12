package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type LastFMClient struct {
	apiKey     string
	httpClient *http.Client
}

type lastfmImage struct {
	Text string `json:"#text"`
	Size string `json:"size"`
}

type musicRecentResponse struct {
	RecentTracks struct {
		Track []MusicTrack `json:"track"`
		Attr  struct {
			Total      string `json:"total"`
			TotalPages string `json:"totalPages"`
			Page       string `json:"page"`
		} `json:"@attr"`
	} `json:"recenttracks"`
}

type musicUserResponse struct {
	User MusicUser `json:"user"`
}

type windowedTrack struct {
	artist, name, album string
	uts                 int64
}

type LastFMLovedTrack struct {
	Artist string
	Name   string
}

func NewLastFMClient(apiKey string) *LastFMClient {
	return &LastFMClient{
		apiKey:     apiKey,
		httpClient: NewHTTPClientWithTimeout(15 * time.Second),
	}
}

func (c *LastFMClient) Enabled() bool { return c.apiKey != "" }

func (c *LastFMClient) get(ctx context.Context, urlStr string, target interface{}) error {
	backoff := 500 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, urlStr, nil)
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")
		req.Header.Set("Accept", "application/json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			cancel()
			lastErr = err
		} else {
			func() {
				defer resp.Body.Close()
				defer cancel()
				if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
					lastErr = fmt.Errorf("status %d", resp.StatusCode)
					return
				}
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
				if readErr != nil {
					lastErr = readErr
					return
				}
				var probe struct {
					Error   int    `json:"error"`
					Message string `json:"message"`
				}
				if json.Unmarshal(body, &probe) == nil && probe.Error != 0 {
					lastErr = fmt.Errorf("lastfm error %d: %s", probe.Error, probe.Message)
					return
				}
				if err := json.Unmarshal(body, target); err != nil {
					lastErr = err
					return
				}
				lastErr = nil
			}()
		}
		if lastErr == nil {
			return nil
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
				if backoff > 5*time.Second {
					backoff = 5 * time.Second
				}
			}
		}
	}
	return lastErr
}

func (c *LastFMClient) RecentTracks(ctx context.Context, username string, limit int) ([]MusicTrack, error) {
	if limit <= 0 {
		limit = 10
	}
	urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getrecenttracks&user=%s&api_key=%s&format=json&limit=%d",
		url.QueryEscape(username), url.QueryEscape(c.apiKey), limit)
	var resp musicRecentResponse
	if err := c.get(ctx, urlStr, &resp); err != nil {
		return nil, err
	}
	return resp.RecentTracks.Track, nil
}

func (c *LastFMClient) RecentTracksFrom(ctx context.Context, username string, fromUnix int64, maxPages int) ([]windowedTrack, error) {
	if maxPages <= 0 {
		maxPages = 6
	}
	var out []windowedTrack
	page := 1
	totalPages := 1
	for page <= totalPages && page <= maxPages {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getrecenttracks&user=%s&api_key=%s&format=json&limit=200&from=%d&page=%d",
			url.QueryEscape(username), url.QueryEscape(c.apiKey), fromUnix, page)
		var resp musicRecentResponse
		if err := c.get(ctx, urlStr, &resp); err != nil {
			return out, err
		}
		for i := range resp.RecentTracks.Track {
			t := &resp.RecentTracks.Track[i]
			if t.Attr.NowPlaying == "true" || strings.TrimSpace(t.Date.Uts) == "" {
				continue
			}
			sec, err := strconv.ParseInt(t.Date.Uts, 10, 64)
			if err != nil {
				continue
			}
			out = append(out, windowedTrack{artist: t.Artist.Text, name: t.Name, album: t.Album.Text, uts: sec})
		}
		if tp, err := strconv.Atoi(resp.RecentTracks.Attr.TotalPages); err == nil && tp > 0 {
			totalPages = tp
		}
		page++
	}
	return out, nil
}

func (c *LastFMClient) UserInfo(ctx context.Context, username string) (*MusicUser, error) {
	urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getinfo&user=%s&api_key=%s&format=json",
		url.QueryEscape(username), url.QueryEscape(c.apiKey))
	var resp musicUserResponse
	if err := c.get(ctx, urlStr, &resp); err != nil {
		return nil, err
	}
	u := resp.User
	return &u, nil
}

func (c *LastFMClient) LovedTracks(ctx context.Context, username string, maxPages int) ([]LastFMLovedTrack, error) {
	if maxPages <= 0 {
		maxPages = 20
	}
	var out []LastFMLovedTrack
	page := 1
	totalPages := 1
	for page <= totalPages && page <= maxPages {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getlovedtracks&user=%s&api_key=%s&format=json&limit=200&page=%d",
			url.QueryEscape(username), url.QueryEscape(c.apiKey), page)
		var resp struct {
			LovedTracks struct {
				Track []struct {
					Name   string `json:"name"`
					Artist struct {
						Name string `json:"name"`
					} `json:"artist"`
				} `json:"track"`
				Attr struct {
					TotalPages string `json:"totalPages"`
				} `json:"@attr"`
			} `json:"lovedtracks"`
		}
		if err := c.get(ctx, urlStr, &resp); err != nil {
			return out, err
		}
		for _, tr := range resp.LovedTracks.Track {
			out = append(out, LastFMLovedTrack{Artist: tr.Artist.Name, Name: tr.Name})
		}
		if tp, err := strconv.Atoi(resp.LovedTracks.Attr.TotalPages); err == nil && tp > 0 {
			totalPages = tp
		}
		page++
	}
	return out, nil
}

func (c *LastFMClient) TopArtists(ctx context.Context, username, period string, limit int) ([]topItem, error) {
	urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.gettopartists&user=%s&api_key=%s&format=json&period=%s&limit=%d",
		url.QueryEscape(username), url.QueryEscape(c.apiKey), lastfmPeriod(period), limit)
	var resp struct {
		TopArtists struct {
			Artist []struct {
				Name      string        `json:"name"`
				PlayCount string        `json:"playcount"`
				Image     []lastfmImage `json:"image"`
			} `json:"artist"`
		} `json:"topartists"`
	}
	if err := c.get(ctx, urlStr, &resp); err != nil {
		return nil, err
	}
	out := make([]topItem, 0, len(resp.TopArtists.Artist))
	for _, a := range resp.TopArtists.Artist {
		plays, _ := strconv.Atoi(a.PlayCount)
		out = append(out, topItem{Name: a.Name, Image: pickSizedImage(a.Image), Plays: plays})
	}
	return out, nil
}

func (c *LastFMClient) TopArtistsAll(ctx context.Context, username, period string, maxPages, perPage int) ([]topItem, error) {
	if perPage <= 0 || perPage > 1000 {
		perPage = 1000
	}
	if maxPages <= 0 {
		maxPages = 3
	}
	var out []topItem
	page := 1
	totalPages := 1
	for page <= totalPages && page <= maxPages {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.gettopartists&user=%s&api_key=%s&format=json&period=%s&limit=%d&page=%d",
			url.QueryEscape(username), url.QueryEscape(c.apiKey), lastfmPeriod(period), perPage, page)
		var resp struct {
			TopArtists struct {
				Artist []struct {
					Name      string        `json:"name"`
					PlayCount string        `json:"playcount"`
					Image     []lastfmImage `json:"image"`
				} `json:"artist"`
				Attr struct {
					TotalPages string `json:"totalPages"`
				} `json:"@attr"`
			} `json:"topartists"`
		}
		if err := c.get(ctx, urlStr, &resp); err != nil {
			return out, err
		}
		for _, a := range resp.TopArtists.Artist {
			plays, _ := strconv.Atoi(a.PlayCount)
			out = append(out, topItem{Name: a.Name, Image: pickSizedImage(a.Image), Plays: plays})
		}
		if tp, err := strconv.Atoi(resp.TopArtists.Attr.TotalPages); err == nil && tp > 0 {
			totalPages = tp
		}
		page++
	}
	return out, nil
}

func (c *LastFMClient) TopAlbums(ctx context.Context, username, period string, limit int) ([]topItem, error) {
	urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.gettopalbums&user=%s&api_key=%s&format=json&period=%s&limit=%d",
		url.QueryEscape(username), url.QueryEscape(c.apiKey), lastfmPeriod(period), limit)
	var resp struct {
		TopAlbums struct {
			Album []struct {
				Name      string `json:"name"`
				PlayCount string `json:"playcount"`
				Artist    struct {
					Name string `json:"name"`
				} `json:"artist"`
				Image []lastfmImage `json:"image"`
			} `json:"album"`
		} `json:"topalbums"`
	}
	if err := c.get(ctx, urlStr, &resp); err != nil {
		return nil, err
	}
	out := make([]topItem, 0, len(resp.TopAlbums.Album))
	for _, a := range resp.TopAlbums.Album {
		plays, _ := strconv.Atoi(a.PlayCount)
		out = append(out, topItem{Name: a.Name, Artist: a.Artist.Name, Image: pickSizedImage(a.Image), Plays: plays})
	}
	return out, nil
}

func (c *LastFMClient) TopTags(ctx context.Context, artists []topItem, maxArtists int, cache *ArtistTagCache) []tagItem {
	if c.apiKey == "" || len(artists) == 0 {
		return nil
	}
	if maxArtists <= 0 || maxArtists > len(artists) {
		maxArtists = len(artists)
	}
	weights := map[string]int{}
	var mu sync.Mutex
	applyTags := func(tags []artistTag, plays int) {
		mu.Lock()
		for _, t := range tags {
			weights[t.Name] += (t.Count * plays) / 100
		}
		mu.Unlock()
	}
	type fetchTarget struct {
		name  string
		plays int
	}
	var toFetch []fetchTarget
	cached := 0
	for i := 0; i < maxArtists; i++ {
		name := artists[i].Name
		plays := artists[i].Plays
		if plays < 1 {
			plays = 1
		}
		if cache != nil {
			if tags, ok := cache.Get(name); ok {
				applyTags(tags, plays)
				cached++
				continue
			}
		}
		toFetch = append(toFetch, fetchTarget{name: name, plays: plays})
	}
	if cached > 0 {
		fmt.Fprintf(os.Stderr, "[Music] genres: %d/%d artists from cache, fetching %d\n", cached, maxArtists, len(toFetch))
	}
	if len(toFetch) > 0 {
		var processed int64
		total := int64(len(toFetch))
		progressDone := make(chan struct{})
		go func() {
			ticker := time.NewTicker(400 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-progressDone:
					return
				case <-ticker.C:
					n := atomic.LoadInt64(&processed)
					fmt.Fprintf(os.Stderr, "\r[Music] Collecting genres: %d/%d artists processed", n, total)
				}
			}
		}()
		var wg sync.WaitGroup
		sem := make(chan struct{}, 4)
		for _, target := range toFetch {
			if ctx.Err() != nil {
				break
			}
			sem <- struct{}{}
			if ctx.Err() != nil {
				<-sem
				break
			}
			wg.Add(1)
			go func(name string, plays int) {
				defer wg.Done()
				defer func() { <-sem }()
				defer atomic.AddInt64(&processed, 1)
				urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=artist.gettoptags&artist=%s&api_key=%s&format=json",
					url.QueryEscape(name), url.QueryEscape(c.apiKey))
				var resp struct {
					TopTags struct {
						Tag []struct {
							Name  string `json:"name"`
							Count int    `json:"count"`
						} `json:"tag"`
					} `json:"toptags"`
				}
				tagCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
				defer cancel()
				if err := c.get(tagCtx, urlStr, &resp); err != nil {
					return
				}
				tags := make([]artistTag, 0, 6)
				for _, t := range resp.TopTags.Tag {
					tn := strings.ToLower(strings.TrimSpace(t.Name))
					if tn == "" || t.Count < 10 {
						continue
					}
					tags = append(tags, artistTag{Name: tn, Count: t.Count})
					if len(tags) >= 6 {
						break
					}
				}
				if cache != nil {
					cache.Set(name, tags)
				}
				applyTags(tags, plays)
			}(target.name, target.plays)
		}
		wg.Wait()
		close(progressDone)
		fmt.Fprintf(os.Stderr, "\r[Music] Collecting genres: %d/%d artists processed\n", atomic.LoadInt64(&processed), total)
		if cache != nil {
			cache.Flush()
		}
	}
	tags := make([]tagItem, 0, len(weights))
	for name, w := range weights {
		tags = append(tags, tagItem{Name: name, Weight: w})
	}
	sortTagsDesc(tags)
	if len(tags) > 28 {
		tags = tags[:28]
	}
	return tags
}

func lastfmPeriod(period string) string {
	switch period {
	case "7day", "1month", "3month", "6month", "12month", "overall":
		return period
	default:
		return "overall"
	}
}

func pickSizedImage(images []lastfmImage) string {
	order := map[string]int{"small": 1, "medium": 2, "large": 3, "extralarge": 4, "mega": 5}
	best, bestRank := "", 0
	for _, img := range images {
		u := strings.TrimSpace(img.Text)
		if u == "" || strings.Contains(u, "2a96cbd8b46e442fc41c2b86b821562f") {
			continue
		}
		if r := order[img.Size]; r >= bestRank {
			best, bestRank = u, r
		}
	}
	return best
}

func sortTagsDesc(tags []tagItem) {
	for i := 1; i < len(tags); i++ {
		for j := i; j > 0 && tags[j].Weight > tags[j-1].Weight; j-- {
			tags[j], tags[j-1] = tags[j-1], tags[j]
		}
	}
}

func (c *LastFMClient) TopCounts(ctx context.Context, username, period string) (scrobbles, tracks, albums, artists int) {
	count := func(method, key string) int {
		urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=%s&user=%s&api_key=%s&format=json&period=%s&limit=1",
			method, url.QueryEscape(username), url.QueryEscape(c.apiKey), lastfmPeriod(period))
		var resp map[string]json.RawMessage
		if err := c.get(ctx, urlStr, &resp); err != nil {
			return 0
		}
		raw, ok := resp[key]
		if !ok {
			return 0
		}
		var wrap struct {
			Attr struct {
				Total string `json:"total"`
			} `json:"@attr"`
		}
		if err := json.Unmarshal(raw, &wrap); err != nil {
			return 0
		}
		n, _ := strconv.Atoi(wrap.Attr.Total)
		return n
	}
	tracks = count("user.gettoptracks", "toptracks")
	albums = count("user.gettopalbums", "topalbums")
	artists = count("user.gettopartists", "topartists")
	scrobbles = c.scrobbleSum(ctx, username, period)
	return
}

func (c *LastFMClient) scrobbleSum(ctx context.Context, username, period string) int {
	total := 0
	page := 1
	totalPages := 1
	const maxPages = 10
	for page <= totalPages && page <= maxPages {
		if ctx.Err() != nil {
			return total
		}
		urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.gettoptracks&user=%s&api_key=%s&format=json&period=%s&limit=1000&page=%d",
			url.QueryEscape(username), url.QueryEscape(c.apiKey), lastfmPeriod(period), page)
		var resp struct {
			TopTracks struct {
				Track []struct {
					PlayCount string `json:"playcount"`
				} `json:"track"`
				Attr struct {
					TotalPages string `json:"totalPages"`
				} `json:"@attr"`
			} `json:"toptracks"`
		}
		if err := c.get(ctx, urlStr, &resp); err != nil {
			return total
		}
		for _, t := range resp.TopTracks.Track {
			n, _ := strconv.Atoi(t.PlayCount)
			total += n
		}
		if tp, err := strconv.Atoi(resp.TopTracks.Attr.TotalPages); err == nil && tp > 0 {
			totalPages = tp
		}
		page++
	}
	return total
}

func (c *LastFMClient) RecentTracksWindow(ctx context.Context, username string, fromUnix, toUnix int64, maxPages int) ([]windowedTrack, error) {
	if maxPages <= 0 {
		maxPages = 20
	}
	var out []windowedTrack
	page := 1
	totalPages := 1
	for page <= totalPages && page <= maxPages {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getrecenttracks&user=%s&api_key=%s&format=json&limit=200&from=%d&to=%d&page=%d",
			url.QueryEscape(username), url.QueryEscape(c.apiKey), fromUnix, toUnix, page)
		var resp musicRecentResponse
		if err := c.get(ctx, urlStr, &resp); err != nil {
			return out, err
		}
		for i := range resp.RecentTracks.Track {
			t := &resp.RecentTracks.Track[i]
			if t.Attr.NowPlaying == "true" || strings.TrimSpace(t.Date.Uts) == "" {
				continue
			}
			sec, err := strconv.ParseInt(t.Date.Uts, 10, 64)
			if err != nil {
				continue
			}
			out = append(out, windowedTrack{artist: t.Artist.Text, name: t.Name, album: t.Album.Text, uts: sec})
		}
		if tp, err := strconv.Atoi(resp.RecentTracks.Attr.TotalPages); err == nil && tp > 0 {
			totalPages = tp
		}
		page++
	}
	return out, nil
}

func (c *LastFMClient) ScrobbleCountWindow(ctx context.Context, username string, fromUnix, toUnix int64) (int, error) {
	urlStr := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=user.getrecenttracks&user=%s&api_key=%s&format=json&limit=1&from=%d&to=%d",
		url.QueryEscape(username), url.QueryEscape(c.apiKey), fromUnix, toUnix)
	var resp musicRecentResponse
	if err := c.get(ctx, urlStr, &resp); err != nil {
		return 0, err
	}
	n, _ := strconv.Atoi(resp.RecentTracks.Attr.Total)
	return n, nil
}
