package handlers

import (
	"html/template"
	"log"
	"net/http"
)

// SteamPageHandler renders the standalone all-games page. The page shell is static; every value is
// filled in client-side from /api/steam/games so the first byte is never blocked on Steam.
type SteamPageHandler struct {
	template *template.Template
}

func NewSteamPageHandler(tmpl *template.Template) *SteamPageHandler {
	return &SteamPageHandler{template: tmpl}
}

func (h *SteamPageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "Steam Library",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	if err := h.template.Execute(w, data); err != nil {
		log.Printf("Steam page template error: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
	}
}
