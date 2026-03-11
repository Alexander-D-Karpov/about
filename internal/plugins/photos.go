package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type PhotosPlugin struct {
	storage    *storage.Storage
	hub        *stream.Hub
	httpClient *http.Client
	data       *PhotosData
	lastUpdate time.Time
	mutex      sync.RWMutex
}

type PhotosData struct {
	Folders    []PhotosFolder `json:"folders"`
	TotalCount int            `json:"total_count"`
}

type PhotosFolder struct {
	ID             int           `json:"id"`
	Name           string        `json:"name"`
	Path           string        `json:"path"`
	PhotoCount     int           `json:"photo_count"`
	SubfolderCount int           `json:"subfolder_count"`
	TotalSize      int64         `json:"total_size"`
	CreatedAt      string        `json:"created_at"`
	CoverPhotoID   *int          `json:"cover_photo_id"`
	Photos         []PhotosPhoto `json:"photos,omitempty"`
}

type PhotosPhoto struct {
	ID         int              `json:"id"`
	FolderID   int              `json:"folder_id"`
	Filename   string           `json:"filename"`
	Path       string           `json:"path"`
	URL        string           `json:"url"`
	Title      *string          `json:"title"`
	Width      int              `json:"width"`
	Height     int              `json:"height"`
	SizeBytes  int64            `json:"size_bytes"`
	Blurhash   string           `json:"blurhash"`
	CreatedAt  string           `json:"created_at"`
	TakenAt    string           `json:"taken_at"`
	Thumbnails PhotosThumbnails `json:"thumbnails"`
}

type PhotosThumbnails struct {
	Small  string `json:"small"`
	Medium string `json:"medium"`
	Large  string `json:"large"`
}

type PhotosAPIFoldersResponse struct {
	Folders []PhotosFolder `json:"folders"`
}

type PhotosAPIPhotosResponse struct {
	Photos  []PhotosPhoto `json:"photos"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
	Pages   int           `json:"pages"`
	PerPage int           `json:"per_page"`
}

func NewPhotosPlugin(storage *storage.Storage, hub *stream.Hub) *PhotosPlugin {
	return &PhotosPlugin{
		storage: storage,
		hub:     hub,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (p *PhotosPlugin) Name() string { return "photos" }

func (p *PhotosPlugin) Render(ctx context.Context) (string, error) {
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	sectionTitle := p.getConfigString(settings, "ui.sectionTitle", "Photos")
	apiUrl := p.getConfigString(settings, "apiUrl", "https://photos.akarpov.ru")
	apiUrl = strings.TrimRight(apiUrl, "/")
	maxFolders := p.getConfigInt(settings, "ui.maxFolders", 4)

	hiddenFolders := p.getHiddenFolders(settings)

	p.mutex.RLock()
	data := p.data
	p.mutex.RUnlock()

	if data == nil || len(data.Folders) == 0 {
		return "", nil
	}

	var folders []map[string]interface{}
	for _, f := range data.Folders {
		if hiddenFolders[f.ID] || hiddenFolders[0] && p.isFolderNameHidden(f.Name, settings) {
			continue
		}

		if f.PhotoCount == 0 {
			continue
		}

		folderUrl := fmt.Sprintf("%s/p/%s/", apiUrl, url.PathEscape(f.Path))

		var photoThumbs []string
		for i, photo := range f.Photos {
			if i >= 4 {
				break
			}
			thumbUrl := fmt.Sprintf("%s%s", apiUrl, photo.Thumbnails.Small)
			photoThumbs = append(photoThumbs, thumbUrl)
		}

		for len(photoThumbs) < 4 && len(photoThumbs) > 0 {
			photoThumbs = append(photoThumbs, photoThumbs[len(photoThumbs)-1])
		}

		folders = append(folders, map[string]interface{}{
			"ID":         f.ID,
			"Name":       f.Name,
			"Url":        folderUrl,
			"PhotoCount": f.PhotoCount,
			"Photos":     photoThumbs,
			"HasPhotos":  len(photoThumbs) > 0,
		})

		if len(folders) >= maxFolders {
			break
		}
	}

	if len(folders) == 0 {
		return "", nil
	}

	tmpl := `
<section class="photos-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
		<a href="{{.ApiUrl}}" target="_blank" rel="noopener" class="plugin-header-link" title="View Gallery">
			<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
				<path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
				<polyline points="15 3 21 3 21 9"></polyline>
				<line x1="10" y1="14" x2="21" y2="3"></line>
			</svg>
		</a>
	</header>
	<div class="plugin__inner">
		<div class="photos-grid">
			{{range .Folders}}
			<a href="{{.Url}}" target="_blank" rel="noopener" class="photos-card">
				<div class="photos-cover">
					{{if .HasPhotos}}
					<div class="photos-preview-grid">
						{{range .Photos}}
						<div class="photos-preview-item">
							<img src="{{.}}" alt="" loading="lazy">
						</div>
						{{end}}
					</div>
					{{else}}
					<div class="photos-no-preview">
						<svg viewBox="0 0 24 24" width="32" height="32" fill="currentColor">
							<path d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/>
						</svg>
					</div>
					{{end}}
					<div class="photos-badge">{{.PhotoCount}}</div>
				</div>
				<div class="photos-info">
					<span class="photos-name">{{.Name}}</span>
				</div>
			</a>
			{{end}}
		</div>
	</div>
</section>`

	templateData := struct {
		SectionTitle string
		ApiUrl       string
		Folders      []map[string]interface{}
	}{
		SectionTitle: sectionTitle,
		ApiUrl:       apiUrl,
		Folders:      folders,
	}

	t, err := template.New("photos").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := t.Execute(&buf, templateData); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (p *PhotosPlugin) UpdateData(ctx context.Context) error {
	if !p.lastUpdate.IsZero() && time.Since(p.lastUpdate) < 5*time.Minute {
		return nil
	}

	config := p.storage.GetPluginConfig(p.Name())
	apiUrl := p.getConfigString(config.Settings, "apiUrl", "https://photos.akarpov.ru")
	apiUrl = strings.TrimRight(apiUrl, "/")

	foldersReq, err := http.NewRequestWithContext(ctx, "GET", apiUrl+"/api/folders", nil)
	if err != nil {
		return err
	}
	foldersReq.Header.Set("User-Agent", "AboutPage/1.0")

	foldersResp, err := p.httpClient.Do(foldersReq)
	if err != nil {
		return err
	}
	defer foldersResp.Body.Close()

	if foldersResp.StatusCode != http.StatusOK {
		return fmt.Errorf("photos api folders returned status: %d", foldersResp.StatusCode)
	}

	var foldersData PhotosAPIFoldersResponse
	if err := json.NewDecoder(foldersResp.Body).Decode(&foldersData); err != nil {
		return err
	}

	for i := range foldersData.Folders {
		folder := &foldersData.Folders[i]
		if folder.PhotoCount > 0 {
			photos, err := p.fetchFolderPhotos(ctx, apiUrl, folder.ID)
			if err != nil {
				continue
			}
			folder.Photos = photos
		}
	}

	totalPhotos := 0
	for _, f := range foldersData.Folders {
		totalPhotos += f.PhotoCount
	}

	p.mutex.Lock()
	p.data = &PhotosData{
		Folders:    foldersData.Folders,
		TotalCount: totalPhotos,
	}
	p.lastUpdate = time.Now()
	p.mutex.Unlock()

	return nil
}

func (p *PhotosPlugin) fetchFolderPhotos(ctx context.Context, apiUrl string, folderID int) ([]PhotosPhoto, error) {
	photosUrl := fmt.Sprintf("%s/api/photos?folder_id=%d&per_page=4", apiUrl, folderID)

	req, err := http.NewRequestWithContext(ctx, "GET", photosUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AboutPage/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("photos api returned status: %d", resp.StatusCode)
	}

	var photosResp PhotosAPIPhotosResponse
	if err := json.NewDecoder(resp.Body).Decode(&photosResp); err != nil {
		return nil, err
	}

	return photosResp.Photos, nil
}

func (p *PhotosPlugin) RenderText(ctx context.Context) (string, error) {
	p.mutex.RLock()
	data := p.data
	p.mutex.RUnlock()

	if data == nil {
		return "Photos: No data", nil
	}
	return fmt.Sprintf("Photos: %d folders, %d photos total", len(data.Folders), data.TotalCount), nil
}

func (p *PhotosPlugin) GetSettings() map[string]interface{} {
	return p.storage.GetPluginConfig(p.Name()).Settings
}

func (p *PhotosPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings
	p.lastUpdate = time.Time{}
	return p.storage.SetPluginConfig(p.Name(), config)
}

func (p *PhotosPlugin) getConfigString(settings map[string]interface{}, key string, defaultValue string) string {
	if settings == nil {
		return defaultValue
	}

	if strings.Contains(key, ".") {
		parts := strings.Split(key, ".")
		current := settings
		for i, part := range parts {
			if i == len(parts)-1 {
				if v, ok := current[part].(string); ok && v != "" {
					return v
				}
				return defaultValue
			}
			if next, ok := current[part].(map[string]interface{}); ok {
				current = next
			} else {
				return defaultValue
			}
		}
	}

	if v, ok := settings[key].(string); ok && v != "" {
		return v
	}
	return defaultValue
}

func (p *PhotosPlugin) getConfigInt(settings map[string]interface{}, key string, defaultValue int) int {
	if settings == nil {
		return defaultValue
	}

	if strings.Contains(key, ".") {
		parts := strings.Split(key, ".")
		current := settings
		for i, part := range parts {
			if i == len(parts)-1 {
				if v, ok := current[part].(float64); ok {
					return int(v)
				}
				if v, ok := current[part].(int); ok {
					return v
				}
				return defaultValue
			}
			if next, ok := current[part].(map[string]interface{}); ok {
				current = next
			} else {
				return defaultValue
			}
		}
	}

	if v, ok := settings[key].(float64); ok {
		return int(v)
	}
	if v, ok := settings[key].(int); ok {
		return v
	}
	return defaultValue
}

func (p *PhotosPlugin) getHiddenFolders(settings map[string]interface{}) map[int]bool {
	hidden := make(map[int]bool)

	if hiddenList, ok := settings["hiddenFolders"].([]interface{}); ok {
		for _, v := range hiddenList {
			if id, ok := v.(float64); ok {
				hidden[int(id)] = true
			}
			if id, ok := v.(int); ok {
				hidden[id] = true
			}
		}
	}

	return hidden
}

func (p *PhotosPlugin) isFolderNameHidden(name string, settings map[string]interface{}) bool {
	if hiddenNames, ok := settings["hiddenFolderNames"].([]interface{}); ok {
		for _, v := range hiddenNames {
			if hiddenName, ok := v.(string); ok {
				if strings.EqualFold(name, hiddenName) {
					return true
				}
			}
		}
	}
	return false
}

func (p *PhotosPlugin) GetMetrics() map[string]interface{} {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	metrics := map[string]interface{}{
		"photos_folders_total": 0,
		"photos_images_total":  0,
	}

	if p.data != nil {
		metrics["photos_folders_total"] = len(p.data.Folders)
		metrics["photos_images_total"] = p.data.TotalCount
	}

	return metrics
}
