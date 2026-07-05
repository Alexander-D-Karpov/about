package plugins

import (
	"context"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *MemePlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	p.mutex.RLock()
	meme := p.currentMeme
	p.mutex.RUnlock()
	if meme == nil {
		return nil
	}

	mv := view.MemeVM{Type: meme.Type, Image: meme.Image, Text: meme.Text, Source: meme.Source}
	vm.Machines.Meme = mv
	vm.Meme = mv
	return nil
}
