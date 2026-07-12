package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type SpotifyClient struct {
	clientID     string
	clientSecret string
	refreshToken string

	httpClient *http.Client

	appToken     string
	appTokenExp  time.Time
	userToken    string
	userTokenExp time.Time
	tokenMu      sync.Mutex
}

type SpotifySavedTrack struct {
	ID      string
	Name    string
	Artists []string
	Album   string
	Image   string
}

type SpotifyNowPlaying struct {
	IsPlaying  bool
	TrackID    string
	Name       string
	Artists    []string
	Album      string
	Image      string
	DurationMs int
	ProgressMs int
}

func NewSpotifyClient(clientID, clientSecret, refreshToken string) *SpotifyClient {
	return &SpotifyClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		httpClient:   NewHTTPClientWithTimeout(15 * time.Second),
	}
}

func (c *SpotifyClient) Enabled() bool {
	return c.clientID != "" && c.clientSecret != ""
}

func (c *SpotifyClient) UserEnabled() bool {
	return c.Enabled() && c.refreshToken != ""
}

func (c *SpotifyClient) appAccessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.appToken != "" && time.Now().Before(c.appTokenExp) {
		return c.appToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	token, expiresIn, err := c.requestToken(ctx, form)
	if err != nil {
		return "", err
	}

	c.appToken = token
	c.appTokenExp = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	return token, nil
}

func (c *SpotifyClient) userAccessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if c.userToken != "" && time.Now().Before(c.userTokenExp) {
		return c.userToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", c.refreshToken)

	token, expiresIn, err := c.requestToken(ctx, form)
	if err != nil {
		return "", err
	}

	c.userToken = token
	c.userTokenExp = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	return token, nil
}

func (c *SpotifyClient) requestToken(ctx context.Context, form url.Values) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://accounts.spotify.com/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.SetBasicAuth(c.clientID, c.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("spotify token status %d", resp.StatusCode)
	}

	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", 0, err
	}
	if body.ExpiresIn <= 0 {
		body.ExpiresIn = 3600
	}
	return body.AccessToken, body.ExpiresIn, nil
}

func (c *SpotifyClient) apiGet(ctx context.Context, token, endpoint string, target interface{}) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("spotify api status %d", resp.StatusCode)
	}
	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func (c *SpotifyClient) SavedTracks(ctx context.Context) ([]SpotifySavedTrack, int, error) {
	if !c.UserEnabled() {
		return nil, 0, fmt.Errorf("spotify user auth not configured")
	}
	token, err := c.userAccessToken(ctx)
	if err != nil {
		return nil, 0, err
	}

	var tracks []SpotifySavedTrack
	total := 0
	endpoint := "https://api.spotify.com/v1/me/tracks?limit=50"

	for endpoint != "" {
		var page struct {
			Total int    `json:"total"`
			Next  string `json:"next"`
			Items []struct {
				Track struct {
					ID      string `json:"id"`
					Name    string `json:"name"`
					Artists []struct {
						Name string `json:"name"`
					} `json:"artists"`
					Album struct {
						Name   string `json:"name"`
						Images []struct {
							URL string `json:"url"`
						} `json:"images"`
					} `json:"album"`
				} `json:"track"`
			} `json:"items"`
		}

		if _, err := c.apiGet(ctx, token, endpoint, &page); err != nil {
			return tracks, total, err
		}

		total = page.Total
		for _, item := range page.Items {
			t := item.Track
			artists := make([]string, 0, len(t.Artists))
			for _, a := range t.Artists {
				artists = append(artists, a.Name)
			}
			image := ""
			if len(t.Album.Images) > 0 {
				image = t.Album.Images[0].URL
			}
			tracks = append(tracks, SpotifySavedTrack{
				ID:      t.ID,
				Name:    t.Name,
				Artists: artists,
				Album:   t.Album.Name,
				Image:   image,
			})
		}

		endpoint = page.Next
	}

	return tracks, total, nil
}

func (c *SpotifyClient) SavedTracksCount(ctx context.Context) (int, error) {
	if !c.UserEnabled() {
		return 0, fmt.Errorf("spotify user auth not configured")
	}
	token, err := c.userAccessToken(ctx)
	if err != nil {
		return 0, err
	}
	var page struct {
		Total int `json:"total"`
	}
	if _, err := c.apiGet(ctx, token, "https://api.spotify.com/v1/me/tracks?limit=1", &page); err != nil {
		return 0, err
	}
	return page.Total, nil
}

func (c *SpotifyClient) trackID(ctx context.Context, artist, track string) (string, error) {
	token, err := c.appAccessToken(ctx)
	if err != nil {
		return "", err
	}
	query := fmt.Sprintf("track:%s artist:%s", track, artist)
	endpoint := "https://api.spotify.com/v1/search?type=track&limit=1&q=" + url.QueryEscape(query)

	var res struct {
		Tracks struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		} `json:"tracks"`
	}
	if _, err := c.apiGet(ctx, token, endpoint, &res); err != nil {
		return "", err
	}
	if len(res.Tracks.Items) == 0 {
		return "", nil
	}
	return res.Tracks.Items[0].ID, nil
}

func (c *SpotifyClient) IsTrackSaved(ctx context.Context, artist, track string) (bool, error) {
	if !c.UserEnabled() {
		return false, nil
	}
	id, err := c.trackID(ctx, artist, track)
	if err != nil || id == "" {
		return false, err
	}
	token, err := c.userAccessToken(ctx)
	if err != nil {
		return false, err
	}
	endpoint := "https://api.spotify.com/v1/me/tracks/contains?ids=" + id
	var res []bool
	if _, err := c.apiGet(ctx, token, endpoint, &res); err != nil {
		return false, err
	}
	return len(res) > 0 && res[0], nil
}

func (c *SpotifyClient) ArtistImage(ctx context.Context, artist string) (string, error) {
	token, err := c.appAccessToken(ctx)
	if err != nil {
		return "", err
	}
	endpoint := "https://api.spotify.com/v1/search?type=artist&limit=1&q=" + url.QueryEscape(artist)
	var res struct {
		Artists struct {
			Items []struct {
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			} `json:"items"`
		} `json:"artists"`
	}
	if _, err := c.apiGet(ctx, token, endpoint, &res); err != nil {
		return "", err
	}
	if len(res.Artists.Items) == 0 || len(res.Artists.Items[0].Images) == 0 {
		return "", nil
	}
	return res.Artists.Items[0].Images[0].URL, nil
}

func (c *SpotifyClient) AlbumImage(ctx context.Context, artist, album string) (string, error) {
	token, err := c.appAccessToken(ctx)
	if err != nil {
		return "", err
	}
	query := fmt.Sprintf("album:%s artist:%s", album, artist)
	endpoint := "https://api.spotify.com/v1/search?type=album&limit=1&q=" + url.QueryEscape(query)
	var res struct {
		Albums struct {
			Items []struct {
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			} `json:"items"`
		} `json:"albums"`
	}
	if _, err := c.apiGet(ctx, token, endpoint, &res); err != nil {
		return "", err
	}
	if len(res.Albums.Items) == 0 || len(res.Albums.Items[0].Images) == 0 {
		return "", nil
	}
	return res.Albums.Items[0].Images[0].URL, nil
}

func (c *SpotifyClient) CurrentlyPlaying(ctx context.Context) (*SpotifyNowPlaying, error) {
	if !c.UserEnabled() {
		return nil, fmt.Errorf("spotify user auth not configured")
	}
	token, err := c.userAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	var res struct {
		IsPlaying  bool `json:"is_playing"`
		ProgressMs int  `json:"progress_ms"`
		Item       *struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			DurationMs int    `json:"duration_ms"`
			Artists    []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Album struct {
				Name   string `json:"name"`
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			} `json:"album"`
		} `json:"item"`
	}

	status, err := c.apiGet(ctx, token, "https://api.spotify.com/v1/me/player/currently-playing", &res)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNoContent || res.Item == nil {
		return nil, nil
	}

	artists := make([]string, 0, len(res.Item.Artists))
	for _, a := range res.Item.Artists {
		artists = append(artists, a.Name)
	}
	image := ""
	if len(res.Item.Album.Images) > 0 {
		image = res.Item.Album.Images[0].URL
	}

	return &SpotifyNowPlaying{
		IsPlaying:  res.IsPlaying,
		TrackID:    res.Item.ID,
		Name:       res.Item.Name,
		Artists:    artists,
		Album:      res.Item.Album.Name,
		Image:      image,
		DurationMs: res.Item.DurationMs,
		ProgressMs: res.ProgressMs,
	}, nil
}
