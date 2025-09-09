package handlers

import (
	"context"
	"embed"
	"html/template"
	"log"
	"net/http"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
)

type MainHandler struct {
	pluginManager *plugins.Manager
	config        *config.Config
	template      *template.Template
}

type TemplateData struct {
	Title   string
	Plugins []template.HTML
}

func NewMainHandler(pluginManager *plugins.Manager, config *config.Config, templateFiles embed.FS) *MainHandler {
	tmpl, err := template.ParseFS(templateFiles, "templates/main.html")
	if err != nil {
		log.Fatalf("Error loading template: %v", err)
	}

	return &MainHandler{
		pluginManager: pluginManager,
		config:        config,
		template:      tmpl,
	}
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

	if visitorsPlugin, exists := h.pluginManager.GetPlugin("visitors"); exists {
		if visitors, ok := visitorsPlugin.(*plugins.VisitorsPlugin); ok {
			visitors.RecordVisit(r.UserAgent(), getClientIP(r))
		}
	}

	ctx := context.Background()
	renderedPlugins := h.pluginManager.GetRenderedPluginsFresh(ctx)

	data := TemplateData{
		Title:   "sanspie - Web Developer & DevSecOps",
		Plugins: renderedPlugins,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if err := h.template.Execute(w, data); err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func getClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return forwarded
	}

	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	return r.RemoteAddr
}
