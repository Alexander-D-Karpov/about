package plugins

import (
	"context"
	"fmt"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *PhotosPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	apiUrl := strings.TrimRight(p.getConfigString(cfg.Settings, "apiUrl", "https://photos.akarpov.ru"), "/")
	maxFolders := p.getConfigInt(cfg.Settings, "ui.maxFolders", 6)
	hidden := p.getHiddenFolders(cfg.Settings)

	p.mutex.RLock()
	data := p.data
	p.mutex.RUnlock()
	if data == nil {
		return nil
	}

	albums := make([]view.AlbumVM, 0, maxFolders)
	for _, f := range data.Folders {
		if hidden[f.ID] || f.PhotoCount == 0 {
			continue
		}
		cover := ""
		if len(f.Photos) > 0 {
			cover = apiUrl + f.Photos[0].Thumbnails.Small
		}
		albums = append(albums, view.AlbumVM{Name: f.Name, Count: fmt.Sprintf("%d", f.PhotoCount), Cover: cover})
		if len(albums) >= maxFolders {
			break
		}
	}

	vm.Travel.Albums = albums
	return nil
}
