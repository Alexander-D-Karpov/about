package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/plugins"
)

type LastFMTopHandler struct {
	pm *plugins.Manager
}

func NewLastFMTopHandler(pm *plugins.Manager) *LastFMTopHandler {
	return &LastFMTopHandler{pm: pm}
}

type lfmTopDTO struct {
	Name  string `json:"name"`
	Plays string `json:"plays"`
	Image string `json:"image"`
}

func (h *LastFMTopHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("type")
	if kind != "albums" {
		kind = "artists"
	}
	period := r.URL.Query().Get("period")

	pl, ok := h.pm.GetPlugin("lastfm")
	if !ok {
		http.Error(w, "lastfm unavailable", http.StatusServiceUnavailable)
		return
	}
	lfm, ok := pl.(*plugins.LastFMPlugin)
	if !ok {
		http.Error(w, "lastfm unavailable", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	items, err := lfm.TopItems(ctx, kind, period)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	out := make([]lfmTopDTO, 0, len(items))
	for _, it := range items {
		out = append(out, lfmTopDTO{Name: it.Name, Plays: commaInt(it.Plays), Image: it.Image})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"type":   kind,
		"period": period,
		"items":  out,
	})
}

func commaInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}
