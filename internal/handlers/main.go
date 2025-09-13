package handlers

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
)

type MainHandler struct {
	pluginManager *plugins.Manager
	config        *config.Config
	template      *template.Template
}

type TemplateData struct {
	Title         string
	Description   string
	Canonical     string
	OGTitle       string
	OGDescription string
	OGImage       string
	Plugins       []template.HTML
}

func NewMainHandler(pluginManager *plugins.Manager, config *config.Config, templateFiles embed.FS) *MainHandler {
	funcs := template.FuncMap{
		"default": defaultFunc, // {{ .Field | default "fallback" }}
	}

	tmpl, err := template.New("main.html").
		Funcs(funcs).
		ParseFS(templateFiles, "templates/main.html")
	if err != nil {
		log.Fatalf("Error loading template: %v", err)
	}

	return &MainHandler{
		pluginManager: pluginManager,
		config:        config,
		template:      tmpl,
	}
}

func defaultFunc(v any, def string) string {
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	if v != nil {
		switch x := v.(type) {
		case bool:
			if x {
				return "true"
			}
		case int, int64, float64:
			if fmt.Sprint(x) != "0" {
				return fmt.Sprint(x)
			}
		}
	}
	return def
}

func (h *MainHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	userAgent := r.Header.Get("User-Agent")
	isCurl := strings.Contains(strings.ToLower(userAgent), "curl")

	if visitorsPlugin, exists := h.pluginManager.GetPlugin("visitors"); exists {
		if visitors, ok := visitorsPlugin.(*plugins.VisitorsPlugin); ok {
			visitors.RecordVisit(r.UserAgent(), getClientIP(r))
		}
	}

	if isCurl {
		h.renderTextResponse(w, r)
		return
	}

	ctx := context.Background()
	renderedPlugins := h.pluginManager.GetRenderedPluginsFresh(ctx)

	data := TemplateData{
		Title:         "sanspie",
		Description:   "WebDev & DevSecOps",
		Canonical:     "",
		OGTitle:       "sanspie",
		OGDescription: "",
		OGImage:       "",
		Plugins:       renderedPlugins,
	}

	var buf bytes.Buffer
	if err := h.template.Execute(&buf, data); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}

func (h *MainHandler) renderTextResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintln(w, "┌─────────────────────────────────────────────────┐")
	fmt.Fprintln(w, "│                    sanspie                      │")
	fmt.Fprintln(w, "│                 About Page                      │")
	fmt.Fprintln(w, "├─────────────────────────────────────────────────┤")
	fmt.Fprintln(w, "│ WebDev & DevSecOps                              │")
	fmt.Fprintln(w, "├─────────────────────────────────────────────────┤")
	fmt.Fprintln(w, "│ Available endpoints:                            │")
	fmt.Fprintln(w, "│   /              - Main site (pritty curl soon) │")
	fmt.Fprintln(w, "│   /health        - Health check (JSON)          │")
	fmt.Fprintln(w, "│   /status        - System status                │")
	fmt.Fprintln(w, "├─────────────────────────────────────────────────┤")
	fmt.Fprintln(w, "│ Usage:                                          │")
	fmt.Fprintln(w, "│   View in browser: https://about.akarpov.ru     │")
	fmt.Fprintln(w, "│   Health check:    curl /health                 │")
	fmt.Fprintln(w, "│   Status:          curl /status                 │")
	fmt.Fprintln(w, "└─────────────────────────────────────────────────┘")
}

func getClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return forwarded
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	return r.RemoteAddr
}
