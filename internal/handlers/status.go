package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type StatusHandler struct {
	pluginManager *plugins.Manager
	config        *config.Config
	hub           *stream.Hub
	startTime     time.Time
}

type StatusResponse struct {
	Status        string            `json:"status"`
	Version       string            `json:"version"`
	Uptime        string            `json:"uptime"`
	UptimeSeconds int64             `json:"uptime_seconds"`
	Memory        MemoryStats       `json:"memory"`
	Clients       int               `json:"websocket_clients"`
	Plugins       int               `json:"enabled_plugins"`
	Goroutines    int               `json:"goroutines"`
	ServerTime    string            `json:"server_time"`
	Details       map[string]string `json:"details,omitempty"`
}

type MemoryStats struct {
	Allocated string `json:"allocated"`
	System    string `json:"system"`
	GCCount   uint32 `json:"gc_count"`
}

func NewStatusHandler(pm *plugins.Manager, cfg *config.Config, hub *stream.Hub, startTime time.Time) *StatusHandler {
	return &StatusHandler{
		pluginManager: pm,
		config:        cfg,
		hub:           hub,
		startTime:     startTime,
	}
}

func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userAgent := r.Header.Get("User-Agent")
	isCurl := strings.Contains(strings.ToLower(userAgent), "curl")

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	uptime := time.Since(h.startTime)
	enabledPlugins := len(h.pluginManager.GetEnabledPlugins())
	clientCount := h.hub.GetClientCount()

	status := StatusResponse{
		Status:        "healthy",
		Version:       "1.0.0",
		Uptime:        formatDuration(uptime),
		UptimeSeconds: int64(uptime.Seconds()),
		Memory: MemoryStats{
			Allocated: formatBytes(m.Alloc),
			System:    formatBytes(m.Sys),
			GCCount:   m.NumGC,
		},
		Clients:    clientCount,
		Plugins:    enabledPlugins,
		Goroutines: runtime.NumGoroutine(),
		ServerTime: time.Now().Format("2006-01-02 15:04:05 MST"),
	}

	if isCurl {
		h.renderTextStatus(w, status)
	} else {
		h.renderJSONStatus(w, status)
	}
}

func (h *StatusHandler) renderTextStatus(w http.ResponseWriter, status StatusResponse) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "┌─────────────────────────────────────────────────┐\n")
	fmt.Fprintf(w, "│                 sanspie - About                 │\n")
	fmt.Fprintf(w, "├─────────────────────────────────────────────────┤\n")
	fmt.Fprintf(w, "│ Status: %-39s │\n", status.Status)
	fmt.Fprintf(w, "│ Version: %-38s │\n", status.Version)
	fmt.Fprintf(w, "│ Uptime: %-39s │\n", status.Uptime)
	fmt.Fprintf(w, "│ Memory: %-39s │\n", status.Memory.Allocated)
	fmt.Fprintf(w, "│ WebSocket Clients: %-27d │\n", status.Clients)
	fmt.Fprintf(w, "│ Active Plugins: %-30d │\n", status.Plugins)
	fmt.Fprintf(w, "│ Goroutines: %-34d │\n", status.Goroutines)
	fmt.Fprintf(w, "│ Server Time: %-33s │\n", status.ServerTime)
	fmt.Fprintf(w, "├─────────────────────────────────────────────────┤\n")

	if status.Goroutines > 500 {
		fmt.Fprintf(w, "│ ⚠️  High goroutine count detected!             │\n")
	}
	if parseBytes(status.Memory.Allocated) > 100*1024*1024 {
		fmt.Fprintf(w, "│ ⚠️  High memory usage detected!                │\n")
	}
	if status.Clients > 50 {
		fmt.Fprintf(w, "│ ⚠️  High client count detected!                │\n")
	} else {
		fmt.Fprintf(w, "│ ✅ All systems operational                     │\n")
	}

	fmt.Fprintf(w, "├─────────────────────────────────────────────────┤\n")
	fmt.Fprintf(w, "│ Endpoints:                                      │\n")
	fmt.Fprintf(w, "│   /              - Main site                    │\n")
	fmt.Fprintf(w, "│   /health        - Health check                 │\n")
	fmt.Fprintf(w, "│   /status        - This status page             │\n")
	fmt.Fprintf(w, "│   /ws            - WebSocket endpoint           │\n")
	fmt.Fprintf(w, "└─────────────────────────────────────────────────┘\n")
}

func (h *StatusHandler) renderJSONStatus(w http.ResponseWriter, status StatusResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func parseBytes(s string) uint64 {
	if strings.Contains(s, "MB") {
		var val float64
		fmt.Sscanf(s, "%f", &val)
		return uint64(val * 1024 * 1024)
	}
	if strings.Contains(s, "KB") {
		var val float64
		fmt.Sscanf(s, "%f", &val)
		return uint64(val * 1024)
	}
	if strings.Contains(s, "GB") {
		var val float64
		fmt.Sscanf(s, "%f", &val)
		return uint64(val * 1024 * 1024 * 1024)
	}
	return 0
}
