package handlers

import (
	"context"
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

	// Test main functionality
	testCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mainStatus := "healthy"
	var pluginTestCount int
	var pluginTestErrors []string

	pluginTestResult := make(chan struct {
		count  int
		errors []string
	}, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				pluginTestResult <- struct {
					count  int
					errors []string
				}{0, []string{"main handler panic"}}
			}
		}()

		plugins := h.pluginManager.GetRenderedPlugins(testCtx)
		var errors []string

		// Check if we got expected plugins
		if len(plugins) == 0 && enabledPlugins > 0 {
			errors = append(errors, "no plugins rendered despite having enabled plugins")
		}

		pluginTestResult <- struct {
			count  int
			errors []string
		}{len(plugins), errors}
	}()

	select {
	case result := <-pluginTestResult:
		pluginTestCount = result.count
		pluginTestErrors = result.errors
		if len(result.errors) > 0 {
			mainStatus = "degraded"
		}
	case <-testCtx.Done():
		mainStatus = "unhealthy"
		pluginTestErrors = []string{"main functionality test timeout"}
	}

	status := StatusResponse{
		Status:        mainStatus,
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

	// Add diagnostic details if there are issues
	if len(pluginTestErrors) > 0 || mainStatus != "healthy" {
		status.Details = map[string]string{
			"main_functional":    fmt.Sprintf("%t", len(pluginTestErrors) == 0),
			"plugins_rendered":   fmt.Sprintf("%d", pluginTestCount),
			"plugin_test_errors": strings.Join(pluginTestErrors, "; "),
		}
	}

	if isCurl {
		h.renderTextStatus(w, status, mainStatus != "healthy")
	} else {
		h.renderJSONStatus(w, status, mainStatus != "healthy")
	}
}

func (h *StatusHandler) renderTextStatus(w http.ResponseWriter, status StatusResponse, hasIssues bool) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if hasIssues {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

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

	if hasIssues {
		fmt.Fprintf(w, "│ ⚠️  Main functionality issues detected!        │\n")
		if status.Details != nil {
			if errors, ok := status.Details["plugin_test_errors"]; ok && errors != "" {
				fmt.Fprintf(w, "│ ⚠️  Plugin errors: %-29s │\n", errors[:min(29, len(errors))])
			}
		}
	} else {
		if status.Goroutines > 500 {
			fmt.Fprintf(w, "│ ⚠️  High goroutine count detected!             │\n")
		} else if parseBytes(status.Memory.Allocated) > 100*1024*1024 {
			fmt.Fprintf(w, "│ ⚠️  High memory usage detected!                │\n")
		} else if status.Clients > 50 {
			fmt.Fprintf(w, "│ ⚠️  High client count detected!                │\n")
		} else {
			fmt.Fprintf(w, "│ ✅ All systems operational                     │\n")
		}
	}

	fmt.Fprintf(w, "├─────────────────────────────────────────────────┤\n")
	fmt.Fprintf(w, "│ Endpoints:                                      │\n")
	fmt.Fprintf(w, "│   /              - Main site                    │\n")
	fmt.Fprintf(w, "│   /health        - Health check                 │\n")
	fmt.Fprintf(w, "│   /status        - This status page             │\n")
	fmt.Fprintf(w, "│   /ws            - WebSocket endpoint           │\n")
	fmt.Fprintf(w, "└─────────────────────────────────────────────────┘\n")
}

func (h *StatusHandler) renderJSONStatus(w http.ResponseWriter, status StatusResponse, hasIssues bool) {
	w.Header().Set("Content-Type", "application/json")

	if hasIssues {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

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
