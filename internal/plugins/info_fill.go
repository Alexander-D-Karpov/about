package plugins

import (
	"context"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *InfoPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	msk := time.FixedZone("MSK", 3*60*60)
	now := time.Now().In(msk)

	vm.Top.Status.Online = true
	vm.Top.Status.ClockInit = now.Format("15:04:05")
	vm.Top.Status.UptimeSeconds = int64(time.Since(p.startTime).Seconds())
	return nil
}
