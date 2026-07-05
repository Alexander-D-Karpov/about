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

func steamIconURL(appID int, hash string) string {
	if appID == 0 || hash == "" {
		return ""
	}
	return fmt.Sprintf("https://media.steampowered.com/steamcommunity/public/images/apps/%d/%s.jpg", appID, hash)
}

func steamHeaderURLStr(gameID string) string {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return ""
	}
	return fmt.Sprintf("https://cdn.cloudflare.steamstatic.com/steam/apps/%s/header.jpg", gameID)
}

func (p *SteamPlugin) localizeIcons(games []SteamGame) {
	for i := range games {
		if games[i].ImgIconURL == "" {
			continue
		}
		u := steamIconURL(games[i].AppID, games[i].ImgIconURL)
		if u == "" {
			continue
		}
		games[i].LocalIcon = p.localizeImage(u)
	}
}

func (p *SteamPlugin) localizeImage(remoteURL string) string {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" || !strings.HasPrefix(remoteURL, "http") {
		return remoteURL
	}

	p.imgMu.RLock()
	local, ok := p.imgLocal[remoteURL]
	p.imgMu.RUnlock()
	if ok {
		if p.mediaPath == "" {
			return local
		}
		full := filepath.Join(p.mediaPath, strings.TrimPrefix(local, "/media/"))
		if _, err := os.Stat(full); err == nil {
			return local
		}
	}

	local, err := p.downloadImage(remoteURL)
	if err != nil {
		return ""
	}

	p.imgMu.Lock()
	if p.imgLocal == nil {
		p.imgLocal = map[string]string{}
	}
	p.imgLocal[remoteURL] = local
	p.imgMu.Unlock()
	go p.saveImgLocalCache()

	return local
}

func (p *SteamPlugin) downloadImage(remoteURL string) (string, error) {
	if p.mediaPath == "" {
		return "", fmt.Errorf("no media path")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
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

	dir := filepath.Join(p.mediaPath, "steam")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	fullPath := filepath.Join(dir, name)
	rel := "/media/steam/" + name

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

func (p *SteamPlugin) saveImgLocalCache() {
	p.imgMu.RLock()
	cp := make(map[string]interface{}, len(p.imgLocal))
	for k, v := range p.imgLocal {
		cp[k] = v
	}
	p.imgMu.RUnlock()

	cfg := p.storage.GetPluginConfig(p.Name())
	if cfg.Settings == nil {
		cfg.Settings = map[string]interface{}{}
	}
	cfg.Settings["imgLocalCache"] = cp
	_ = p.storage.SetPluginConfig(p.Name(), cfg)
}

func (p *SteamPlugin) loadImgLocalCache() {
	cfg := p.storage.GetPluginConfig(p.Name())
	raw, ok := cfg.Settings["imgLocalCache"].(map[string]interface{})
	if !ok {
		return
	}
	p.imgMu.Lock()
	if p.imgLocal == nil {
		p.imgLocal = map[string]string{}
	}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			p.imgLocal[k] = s
		}
	}
	p.imgMu.Unlock()
}
