package handlers

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Alexander-D-Karpov/about/internal/assets"
	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/layout"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
)

type MainHandler struct {
	pluginManager  *plugins.Manager
	config         *config.Config
	template       *template.Template
	potatoTemplate *template.Template
	bundler        *assets.Bundler
	heightStore    layout.HeightLookup
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

func NewMainHandler(pluginManager *plugins.Manager, cfg *config.Config, templateFiles embed.FS, bundler *assets.Bundler, store layout.HeightLookup) *MainHandler {
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
		heightStore:    store,
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

	mainCtx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	userAgent := r.Header.Get("User-Agent")
	isCurl := strings.Contains(strings.ToLower(userAgent), "curl")

	isMeasure := r.URL.Query().Get("measure") == "1"
	if !isMeasure {
		if visitorsPlugin, exists := h.pluginManager.GetPlugin("visitors"); exists {
			if visitors, ok := visitorsPlugin.(*plugins.VisitorsPlugin); ok {
				visitors.RecordVisit(r.UserAgent(), getClientIP(r))
			}
		}
	}

	if isCurl {
		h.renderTextResponse(w, r)
		return
	}

	potatoMode := IsPotatoMode(r)

	// Set cookie if potato mode activated via URL param
	if _, hasParam := r.URL.Query()["potato"]; hasParam {
		val := r.URL.Query().Get("potato")
		cookieVal := "1"
		if val == "0" || val == "false" {
			cookieVal = "0"
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "potato_mode",
			Value:    cookieVal,
			Path:     "/",
			MaxAge:   365 * 24 * 60 * 60,
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
		})
	}

	type renderResult struct {
		plugins []template.HTML
		err     error
	}

	done := make(chan renderResult, 1)

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("Plugin rendering panic: %v", rec)
				done <- renderResult{nil, fmt.Errorf("rendering failed: %v", rec)}
			}
		}()

		renderedPlugins := h.pluginManager.GetRenderedPlugins(mainCtx)
		done <- renderResult{renderedPlugins, nil}
	}()

	var renderedPlugins []template.HTML

	select {
	case result := <-done:
		if result.err != nil {
			log.Printf("Plugin rendering error: %v", result.err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		renderedPlugins = result.plugins

	case <-mainCtx.Done():
		log.Printf("Main handler timeout - rendering taking too long")
		h.serveMinimalPage(w)
		return

	case <-r.Context().Done():
		log.Printf("Client disconnected during rendering")
		return
	}

	var cssHash, jsHash string
	if potatoMode {
		_, cssHash = h.bundler.PotatoCSSBundle()
		_, jsHash = h.bundler.PotatoJSBundle()
	} else {
		_, cssHash = h.bundler.CSSBundle()
		_, jsHash = h.bundler.JSBundle()
	}

	prebakedPlugins := layout.Prebake(renderedPlugins, h.heightStore)

	data := TemplateData{
		Title:       "sanspie",
		Description: "WebDev & DevSecOps",
		PotatoMode:  potatoMode,
		CSSHash:     cssHash,
		JSHash:      jsHash,
		Plugins:     prebakedPlugins,
		PrebakeCSS:  layout.GenerateResponsiveCSS(),
	}

	var buf bytes.Buffer
	templateCtx, templateCancel := context.WithTimeout(mainCtx, 1*time.Second)
	defer templateCancel()

	var tmpl *template.Template
	if potatoMode {
		tmpl = h.potatoTemplate
	} else {
		tmpl = h.template
	}

	templateDone := make(chan error, 1)
	go func() {
		templateDone <- tmpl.Execute(&buf, data)
	}()

	select {
	case err := <-templateDone:
		if err != nil {
			log.Printf("Error executing template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	case <-templateCtx.Done():
		log.Printf("Template execution timeout")
		http.Error(w, "Request timeout", http.StatusRequestTimeout)
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
		return strings.TrimSpace(realIP)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
