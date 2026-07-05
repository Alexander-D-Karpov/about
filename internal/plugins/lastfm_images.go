package plugins

import (
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (p *LastFMPlugin) imageForVM(track *LastFMTrack) string {
	u := p.getTrackImageFromCache(track)
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "/media/") {
		return u
	}
	p.imgLocalMu.RLock()
	local, ok := p.imgLocal[u]
	p.imgLocalMu.RUnlock()
	if ok {
		return local
	}
	go p.localizeImage(u)
	return u
}

func (p *LastFMPlugin) localizeImage(remoteURL string) string {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" || p.isPlaceholderImage(remoteURL) {
		return ""
	}
	if strings.HasPrefix(remoteURL, "/media/") {
		return remoteURL
	}
	if !strings.HasPrefix(remoteURL, "http") {
		return remoteURL
	}

	p.imgLocalMu.RLock()
	local, ok := p.imgLocal[remoteURL]
	p.imgLocalMu.RUnlock()
	if ok && p.mediaPath != "" {
		full := filepath.Join(p.mediaPath, strings.TrimPrefix(local, "/media/"))
		if _, err := os.Stat(full); err == nil {
			return local
		}
	} else if ok {
		return local
	}

	local, err := p.downloadImage(remoteURL)
	if err != nil {
		return remoteURL
	}

	p.imgLocalMu.Lock()
	if p.imgLocal == nil {
		p.imgLocal = map[string]string{}
	}
	p.imgLocal[remoteURL] = local
	p.imgLocalMu.Unlock()
	go p.saveImgLocalCache()

	return local
}

func (p *LastFMPlugin) downloadImage(remoteURL string) (string, error) {
	if p.mediaPath == "" {
		return "", fmt.Errorf("no media path")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "AboutPage/1.0 (about.akarpov.ru)")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	ext := imageExt(remoteURL, resp.Header.Get("Content-Type"))
	name := fmt.Sprintf("%x%s", sha1.Sum([]byte(remoteURL)), ext)

	dir := filepath.Join(p.mediaPath, "lastfm")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	fullPath := filepath.Join(dir, name)
	rel := "/media/lastfm/" + name

	if _, err := os.Stat(fullPath); err == nil {
		return rel, nil
	}

	tmp := fullPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 8*1024*1024)); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	f.Close()
	if err := os.Rename(tmp, fullPath); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return rel, nil
}

func imageExt(u, contentType string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "gif"):
		return ".gif"
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		return ".jpg"
	}
	low := strings.ToLower(u)
	for _, e := range []string{".png", ".webp", ".gif", ".jpeg", ".jpg"} {
		if strings.Contains(low, e) {
			if e == ".jpeg" {
				return ".jpg"
			}
			return e
		}
	}
	return ".jpg"
}

func (p *LastFMPlugin) saveImgLocalCache() {
	p.imgLocalMu.RLock()
	cp := make(map[string]interface{}, len(p.imgLocal))
	for k, v := range p.imgLocal {
		cp[k] = v
	}
	p.imgLocalMu.RUnlock()

	cfg := p.storage.GetPluginConfig(p.Name())
	if cfg.Settings == nil {
		cfg.Settings = map[string]interface{}{}
	}
	cfg.Settings["imgLocalCache"] = cp
	_ = p.storage.SetPluginConfig(p.Name(), cfg)
}

func (p *LastFMPlugin) loadImgLocalCache() {
	cfg := p.storage.GetPluginConfig(p.Name())
	raw, ok := cfg.Settings["imgLocalCache"].(map[string]interface{})
	if !ok {
		return
	}
	p.imgLocalMu.Lock()
	if p.imgLocal == nil {
		p.imgLocal = map[string]string{}
	}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			p.imgLocal[k] = s
		}
	}
	p.imgLocalMu.Unlock()
}
