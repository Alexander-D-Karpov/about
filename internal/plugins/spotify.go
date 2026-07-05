package plugins

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	httpClient   *http.Client

	mu         sync.Mutex
	appToken   string
	appExpiry  time.Time
	userToken  string
	userExpiry time.Time
}

type SpotifyNowPlaying struct {
	Artist     string
	Track      string
	ProgressMs int
	DurationMs int
	IsPlaying  bool
}

type spImage struct {
	URL string `json:"url"`
}

type spTokenResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type spSearchResp struct {
	Artists struct {
		Items []struct {
			Images []spImage `json:"images"`
		} `json:"items"`
	} `json:"artists"`
	Albums struct {
		Items []struct {
			Images []spImage `json:"images"`
		} `json:"items"`
	} `json:"albums"`
	Tracks struct {
		Items []struct {
			ID    string `json:"id"`
			Album struct {
				Images []spImage `json:"images"`
			} `json:"album"`
		} `json:"items"`
	} `json:"tracks"`
}

type SpotifySavedTrack struct {
	Artist string
	Track  string
}

func NewSpotifyClient(clientID, clientSecret, refreshToken string) *SpotifyClient {
	return &SpotifyClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		httpClient:   &http.Client{Timeout: 12 * time.Second},
	}
}

func (s *SpotifyClient) Enabled() bool {
	return s != nil && s.clientID != "" && s.clientSecret != ""
}

func (s *SpotifyClient) UserEnabled() bool {
	return s.Enabled() && s.refreshToken != ""
}

func (s *SpotifyClient) basicAuth() string {
	return base64.StdEncoding.EncodeToString([]byte(s.clientID + ":" + s.clientSecret))
}

func (s *SpotifyClient) appAccessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.appToken != "" && time.Now().Before(s.appExpiry) {
		t := s.appToken
		s.mu.Unlock()
		return t, nil
	}
	s.mu.Unlock()

	form := url.Values{"grant_type": {"client_credentials"}}
	tok, err := s.requestToken(ctx, form)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.appToken = tok.AccessToken
	s.appExpiry = time.Now().Add(time.Duration(tok.ExpiresIn-60) * time.Second)
	s.mu.Unlock()
	return tok.AccessToken, nil
}

func (s *SpotifyClient) userAccessToken(ctx context.Context) (string, error) {
	if !s.UserEnabled() {
		return "", fmt.Errorf("spotify user auth not configured")
	}
	s.mu.Lock()
	if s.userToken != "" && time.Now().Before(s.userExpiry) {
		t := s.userToken
		s.mu.Unlock()
		return t, nil
	}
	s.mu.Unlock()

	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {s.refreshToken}}
	tok, err := s.requestToken(ctx, form)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.userToken = tok.AccessToken
	s.userExpiry = time.Now().Add(time.Duration(tok.ExpiresIn-60) * time.Second)
	s.mu.Unlock()
	return tok.AccessToken, nil
}

func (s *SpotifyClient) SavedTracks(ctx context.Context) ([]SpotifySavedTrack, int, error) {
	if !s.UserEnabled() {
		return nil, 0, nil
	}
	tok, err := s.userAccessToken(ctx)
	if err != nil {
		return nil, 0, err
	}

	var out []SpotifySavedTrack
	total := 0
	limit := 50
	for offset := 0; offset < 10000; offset += limit {
		var page struct {
			Total int `json:"total"`
			Items []struct {
				Track struct {
					Name    string `json:"name"`
					Artists []struct {
						Name string `json:"name"`
					} `json:"artists"`
				} `json:"track"`
			} `json:"items"`
		}
		endpoint := fmt.Sprintf("me/tracks?limit=%d&offset=%d", limit, offset)
		if err := s.apiGet(ctx, endpoint, tok, &page); err != nil {
			if err == io.EOF {
				break
			}
			return out, total, err
		}
		if page.Total > 0 {
			total = page.Total
		}
		if len(page.Items) == 0 {
			break
		}
		for _, it := range page.Items {
			artist := ""
			if len(it.Track.Artists) > 0 {
				artist = it.Track.Artists[0].Name
			}
			out = append(out, SpotifySavedTrack{Artist: artist, Track: it.Track.Name})
		}
		if offset+limit >= total {
			break
		}
		select {
		case <-ctx.Done():
			return out, total, ctx.Err()
		default:
		}
	}
	return out, total, nil
}

func (s *SpotifyClient) requestToken(ctx context.Context, form url.Values) (*spTokenResp, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", "https://accounts.spotify.com/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Basic "+s.basicAuth())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify token status %d", resp.StatusCode)
	}

	var tok spTokenResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func (s *SpotifyClient) apiGet(ctx context.Context, endpoint, token string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.spotify.com/v1/"+endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return io.EOF
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("spotify status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(target)
}

func (s *SpotifyClient) search(ctx context.Context, query, typ string) (*spSearchResp, error) {
	tok, err := s.appAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("search?q=%s&type=%s&limit=1", url.QueryEscape(query), typ)
	var out spSearchResp
	if err := s.apiGet(ctx, endpoint, tok, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *SpotifyClient) ArtistImage(ctx context.Context, name string) string {
	if !s.Enabled() || name == "" {
		return ""
	}
	res, err := s.search(ctx, name, "artist")
	if err != nil || len(res.Artists.Items) == 0 || len(res.Artists.Items[0].Images) == 0 {
		return ""
	}
	return res.Artists.Items[0].Images[0].URL
}

func (s *SpotifyClient) AlbumImage(ctx context.Context, artist, album string) string {
	if !s.Enabled() || album == "" {
		return ""
	}
	res, err := s.search(ctx, fmt.Sprintf("album:%s artist:%s", album, artist), "album")
	if err != nil || len(res.Albums.Items) == 0 || len(res.Albums.Items[0].Images) == 0 {
		return ""
	}
	return res.Albums.Items[0].Images[0].URL
}

func (s *SpotifyClient) trackID(ctx context.Context, artist, track string) (string, string) {
	res, err := s.search(ctx, fmt.Sprintf("track:%s artist:%s", track, artist), "track")
	if err != nil || len(res.Tracks.Items) == 0 {
		return "", ""
	}
	img := ""
	if len(res.Tracks.Items[0].Album.Images) > 0 {
		img = res.Tracks.Items[0].Album.Images[0].URL
	}
	return res.Tracks.Items[0].ID, img
}

func (s *SpotifyClient) IsTrackSaved(ctx context.Context, artist, track string) bool {
	if !s.UserEnabled() {
		return false
	}
	id, _ := s.trackID(ctx, artist, track)
	if id == "" {
		return false
	}
	tok, err := s.userAccessToken(ctx)
	if err != nil {
		return false
	}
	var out []bool
	if err := s.apiGet(ctx, "me/tracks/contains?ids="+id, tok, &out); err != nil {
		return false
	}
	return len(out) > 0 && out[0]
}

func (s *SpotifyClient) SavedTracksCount(ctx context.Context) int {
	if !s.UserEnabled() {
		return 0
	}
	tok, err := s.userAccessToken(ctx)
	if err != nil {
		return 0
	}
	var out struct {
		Total int `json:"total"`
	}
	if err := s.apiGet(ctx, "me/tracks?limit=1", tok, &out); err != nil {
		return 0
	}
	return out.Total
}

func (s *SpotifyClient) CurrentlyPlaying(ctx context.Context) (*SpotifyNowPlaying, error) {
	if !s.UserEnabled() {
		return nil, nil
	}
	tok, err := s.userAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		IsPlaying  bool `json:"is_playing"`
		ProgressMs int  `json:"progress_ms"`
		Item       struct {
			Name       string `json:"name"`
			DurationMs int    `json:"duration_ms"`
			Artists    []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"item"`
	}
	if err := s.apiGet(ctx, "me/player/currently-playing", tok, &out); err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, err
	}
	np := &SpotifyNowPlaying{
		Track:      out.Item.Name,
		ProgressMs: out.ProgressMs,
		DurationMs: out.Item.DurationMs,
		IsPlaying:  out.IsPlaying,
	}
	if len(out.Item.Artists) > 0 {
		np.Artist = out.Item.Artists[0].Name
	}
	return np, nil
}
