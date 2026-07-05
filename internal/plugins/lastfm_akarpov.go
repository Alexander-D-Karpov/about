package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type akarpovAuthor struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	ImageCropped string `json:"image_cropped"`
}

type akarpovAlbum struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	ImageCropped string `json:"image_cropped"`
}

type akarpovAuthorsResp struct {
	Results []akarpovAuthor `json:"results"`
}

type akarpovAlbumsResp struct {
	Results []akarpovAlbum `json:"results"`
}

func akarpovNormalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (p *LastFMPlugin) getAkarpovJSON(ctx context.Context, endpoint string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("akarpov status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(target)
}

func (p *LastFMPlugin) akarpovAuthorImage(ctx context.Context, name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	endpoint := "https://new.akarpov.ru/api/v1/music/authors/?search=" + url.QueryEscape(name)
	var resp akarpovAuthorsResp
	if err := p.getAkarpovJSON(ctx, endpoint, &resp); err != nil {
		return ""
	}
	want := akarpovNormalize(name)
	for _, a := range resp.Results {
		if akarpovNormalize(a.Name) == want && a.ImageCropped != "" {
			return absAkarpov(a.ImageCropped)
		}
	}
	for _, a := range resp.Results {
		if a.ImageCropped != "" {
			return absAkarpov(a.ImageCropped)
		}
	}
	return ""
}

func (p *LastFMPlugin) akarpovAlbumImage(ctx context.Context, artist, album string) string {
	if strings.TrimSpace(album) == "" {
		return ""
	}
	endpoint := "https://new.akarpov.ru/api/v1/music/albums/?search=" + url.QueryEscape(album)
	var resp akarpovAlbumsResp
	if err := p.getAkarpovJSON(ctx, endpoint, &resp); err != nil {
		return ""
	}
	want := akarpovNormalize(album)
	for _, a := range resp.Results {
		if akarpovNormalize(a.Name) == want && a.ImageCropped != "" {
			return absAkarpov(a.ImageCropped)
		}
	}
	for _, a := range resp.Results {
		if a.ImageCropped != "" {
			return absAkarpov(a.ImageCropped)
		}
	}
	return ""
}
