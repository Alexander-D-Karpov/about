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
	"time"
	"unicode/utf8"

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

	mainCtx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

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

	type renderResult struct {
		plugins []template.HTML
		err     error
	}

	done := make(chan renderResult, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Plugin rendering panic: %v", r)
				done <- renderResult{nil, fmt.Errorf("rendering failed: %v", r)}
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

		// Try to serve a minimal page
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusServiceUnavailable)

		minimalHTML := `<!DOCTYPE html>
<html><head><title>Loading...</title></head>
<body>
<div style="text-align:center;padding:50px;">
<h1>Site Loading</h1>
<p>The page is currently loading. Please refresh in a moment.</p>
<script>setTimeout(() => location.reload(), 3000);</script>
</div>
</body></html>`

		w.Write([]byte(minimalHTML))
		return

	case <-r.Context().Done():
		log.Printf("Client disconnected during rendering")
		return
	}

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
	templateCtx, templateCancel := context.WithTimeout(mainCtx, 1*time.Second)
	defer templateCancel()

	templateDone := make(chan error, 1)
	go func() {
		templateDone <- h.template.Execute(&buf, data)
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
	innerWidth := width - 4 // Account for "│ " and " │"
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
			// Handle case where single word is longer than width
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
		return forwarded
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	return r.RemoteAddr
}
