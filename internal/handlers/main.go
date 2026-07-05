package handlers

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Alexander-D-Karpov/about/internal/assets"
	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
)

type MainHandler struct {
	pluginManager  *plugins.Manager
	config         *config.Config
	template       *template.Template
	potatoTemplate *template.Template
	bundler        *assets.Bundler
}

type TemplateData struct {
	Title         string
	Description   string
	Canonical     string
	OGTitle       string
	OGDescription string
	OGImage       string
	PotatoMode    bool
	CSSHash       string
	JSHash        string
	Plugins       []template.HTML
	PrebakeCSS    template.CSS
}

func NewMainHandler(pluginManager *plugins.Manager, cfg *config.Config, templateFiles embed.FS, bundler *assets.Bundler) *MainHandler {
	funcs := template.FuncMap{
		"default": defaultFunc,
	}

	tmpl, err := template.New("main.html").
		Funcs(funcs).
		ParseFS(templateFiles, "templates/main.html")
	if err != nil {
		log.Fatalf("Error loading main template: %v", err)
	}

	potatoTmpl, err := template.New("potato.html").
		Funcs(funcs).
		ParseFS(templateFiles, "templates/potato.html")
	if err != nil {
		log.Printf("Warning: potato template not found, using main template: %v", err)
		potatoTmpl = tmpl
	}

	return &MainHandler{
		pluginManager:  pluginManager,
		config:         cfg,
		template:       tmpl,
		potatoTemplate: potatoTmpl,
		bundler:        bundler,
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

	if visitorsPlugin, exists := h.pluginManager.GetPlugin("visitors"); exists {
		if visitors, ok := visitorsPlugin.(*plugins.VisitorsPlugin); ok {
			visitors.RecordVisit(r.UserAgent(), getClientIP(r))
		}
	}

	if strings.Contains(strings.ToLower(r.Header.Get("User-Agent")), "curl") {
		h.renderTextResponse(w, r)
		return
	}

	theme := "dark"
	if c, err := r.Cookie("theme"); err == nil && (c.Value == "light" || c.Value == "dark") {
		theme = c.Value
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	html, err := h.pluginManager.BuildPage(ctx, theme)
	if err != nil {
		log.Printf("BuildPage error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, string(html))
}

func (h *MainHandler) serveMinimalPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusServiceUnavailable)

	minimalHTML := `<!DOCTYPE html>
<html><head><title>Loading...</title>
<style>body{font-family:system-ui;background:#0a0a0a;color:#e0e0e0;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
.c{text-align:center;padding:40px}.s{width:40px;height:40px;border:3px solid #333;border-top-color:#6a9fff;border-radius:50%;animation:s 1s linear infinite;margin:0 auto 20px}
@keyframes s{to{transform:rotate(360deg)}}</style></head>
<body><div class="c"><div class="s"></div><h1>Loading...</h1><p>Please wait or refresh in a moment.</p></div>
<script>setTimeout(()=>location.reload(),3000)</script></body></html>`

	w.Write([]byte(minimalHTML))
}

func (h *MainHandler) renderTextResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if visitorsPlugin, exists := h.pluginManager.GetPlugin("visitors"); exists {
		if visitors, ok := visitorsPlugin.(*plugins.VisitorsPlugin); ok {
			visitors.RecordVisit(r.UserAgent(), getClientIP(r))
		}
	}

	textPlugins := h.pluginManager.GetTextRenderedPlugins(ctx)
	systemSummary := h.pluginManager.GetSystemTextSummary()

	width := 63
	innerWidth := width - 4
	headerText := "sanspie - About Page"
	centeredHeader := centerText(headerText, innerWidth)

	fmt.Fprintf(w, "┌%s┐\n", strings.Repeat("─", width-2))
	fmt.Fprintf(w, "│ %s │\n", centeredHeader)
	fmt.Fprintf(w, "├%s┤\n", strings.Repeat("─", width-2))

	for _, pluginText := range textPlugins {
		if pluginText != "" {
			lines := wrapText(pluginText, innerWidth)
			for _, line := range lines {
				paddedLine := padTextToWidth(line, innerWidth)
				fmt.Fprintf(w, "│ %s │\n", paddedLine)
			}
		}
	}

	if systemSummary != "" {
		fmt.Fprintf(w, "├%s┤\n", strings.Repeat("─", width-2))
		paddedSummary := padTextToWidth(systemSummary, innerWidth)
		fmt.Fprintf(w, "│ %s │\n", paddedSummary)
	}

	fmt.Fprintf(w, "├%s┤\n", strings.Repeat("─", width-2))
	fmt.Fprintf(w, "│ %s │\n", padTextToWidth("Access:", innerWidth))
	fmt.Fprintf(w, "│ %s │\n", padTextToWidth("  Web: https://about.akarpov.ru", innerWidth))
	fmt.Fprintf(w, "│ %s │\n", padTextToWidth("  API: curl /health, /status", innerWidth))
	fmt.Fprintf(w, "└%s┘\n", strings.Repeat("─", width-2))
}

func centerText(text string, width int) string {
	textLen := utf8.RuneCountInString(text)
	if textLen >= width {
		runes := []rune(text)
		return string(runes[:width])
	}

	padding := width - textLen
	leftPad := padding / 2
	rightPad := padding - leftPad

	return strings.Repeat(" ", leftPad) + text + strings.Repeat(" ", rightPad)
}

func padTextToWidth(text string, width int) string {
	textLen := utf8.RuneCountInString(text)
	if textLen >= width {
		runes := []rune(text)
		return string(runes[:width])
	}
	return text + strings.Repeat(" ", width-textLen)
}

func wrapText(text string, width int) []string {
	textLen := utf8.RuneCountInString(text)
	if textLen <= width {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	var currentLine string

	for _, word := range words {
		testLine := currentLine
		if testLine != "" {
			testLine += " "
		}
		testLine += word

		if utf8.RuneCountInString(testLine) <= width {
			currentLine = testLine
		} else {
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			currentLine = word
			if utf8.RuneCountInString(currentLine) > width {
				runes := []rune(currentLine)
				lines = append(lines, string(runes[:width-3])+"...")
				currentLine = ""
			}
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

func getClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	return r.RemoteAddr
}
