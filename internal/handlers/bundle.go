package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/assets"
)

type BundleHandler struct {
	bundler *assets.Bundler
}

func NewBundleHandler(bundler *assets.Bundler) *BundleHandler {
	return &BundleHandler{bundler: bundler}
}

func (h *BundleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	var data []byte
	var hash string
	var contentType string

	potatoMode := IsPotatoMode(r)

	switch {
	case strings.HasSuffix(path, "/bundle.css"):
		if potatoMode {
			data, hash = h.bundler.PotatoCSSBundle()
		} else {
			data, hash = h.bundler.CSSBundle()
		}
		contentType = "text/css; charset=utf-8"

	case strings.HasSuffix(path, "/bundle.js"):
		if potatoMode {
			data, hash = h.bundler.PotatoJSBundle()
		} else {
			data, hash = h.bundler.JSBundle()
		}
		contentType = "application/javascript; charset=utf-8"

	default:
		http.NotFound(w, r)
		return
	}

	if data == nil {
		http.NotFound(w, r)
		return
	}

	etag := `"` + hash + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Expires", time.Now().AddDate(1, 0, 0).Format(http.TimeFormat))
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
