package plugins

import (
	"context"
	"fmt"

	"github.com/Alexander-D-Karpov/about/internal/view"
)

func (p *ServicesPlugin) Fill(ctx context.Context, vm *view.PageVM) error {
	cfg := p.storage.GetPluginConfig(p.Name())
	raw, ok := cfg.Settings["services"].([]interface{})
	if !ok {
		return nil
	}

	p.mutex.RLock()
	statuses := make(map[string]ServiceStatus, len(p.serviceStatuses))
	for k, v := range p.serviceStatuses {
		statuses[k] = v
	}
	p.mutex.RUnlock()

	online := 0
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := p.getStringFromMap(m, "name", "Service")
		st := statuses[name]
		status := "loading"
		switch st.Status {
		case "online":
			status = "online"
			online++
		case "offline":
			status = "offline"
		}
		ping := ""
		if st.ResponseTime > 0 {
			ping = fmt.Sprintf("%dms", st.ResponseTime)
		}
		vm.Hosting.Services = append(vm.Hosting.Services, view.ServiceVM{
			Name:   name,
			Desc:   p.getStringFromMap(m, "description", ""),
			Ping:   ping,
			Icon:   p.getIconString(m),
			Status: status,
			URL:    p.getStringFromMap(m, "url", ""),
		})
	}

	vm.Hosting.Online = online
	vm.Hosting.Total = len(vm.Hosting.Services)
	return nil
}
