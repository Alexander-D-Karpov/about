package plugins

import (
	"context"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *VisitorsPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	p.mutex.RLock()
	total := p.visitCount
	today := p.todayCount
	p.mutex.RUnlock()

	vm.Top.Status.VisitsTotal = formatNumber(total)
	vm.Top.Status.VisitorsToday = int(today)
	return nil
}
