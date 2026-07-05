package plugins

import (
	"context"
	"fmt"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *WebringPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	base := strings.TrimRight(getString(cfg.Settings, "webring_url", "https://webring.otomir23.me"), "/")
	user := getString(cfg.Settings, "username", "sanspie")

	wr := view.WebringVM{BaseURL: base}

	p.mutex.RLock()
	data := p.webringData
	p.mutex.RUnlock()

	if data != nil {
		wr.Prev = view.WebringPeer{Name: data.Prev.Name, URL: data.Prev.URL}
		wr.Next = view.WebringPeer{Name: data.Next.Name, URL: data.Next.URL}
		if data.Prev.Favicon != "" {
			wr.Prev.Favicon = fmt.Sprintf("%s/media/%s", base, data.Prev.Favicon)
		}
		if data.Next.Favicon != "" {
			wr.Next.Favicon = fmt.Sprintf("%s/media/%s", base, data.Next.Favicon)
		}
	} else {
		wr.Prev = view.WebringPeer{Name: "…", URL: fmt.Sprintf("%s/prev/%s", base, user)}
		wr.Next = view.WebringPeer{Name: "…", URL: fmt.Sprintf("%s/next/%s", base, user)}
	}

	vm.Top.Webring = wr
	return nil
}
