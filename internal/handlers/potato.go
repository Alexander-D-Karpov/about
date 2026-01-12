package handlers

import (
	"encoding/json"
	"net/http"
	"time"
)

type PotatoHandler struct{}

type PotatoResponse struct {
	Enabled bool `json:"enabled"`
	Success bool `json:"success,omitempty"`
}

func NewPotatoHandler() *PotatoHandler {
	return &PotatoHandler{}
}

func (h *PotatoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		enabled := IsPotatoMode(r)
		json.NewEncoder(w).Encode(PotatoResponse{Enabled: enabled})

	case http.MethodPost:
		enable := r.URL.Query().Get("enable") == "1"

		cookie := &http.Cookie{
			Name:     "potato_mode",
			Path:     "/",
			MaxAge:   365 * 24 * 60 * 60,
			HttpOnly: false,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
			Expires:  time.Now().AddDate(1, 0, 0),
		}

		if enable {
			cookie.Value = "1"
		} else {
			cookie.Value = "0"
		}

		http.SetCookie(w, cookie)
		json.NewEncoder(w).Encode(PotatoResponse{Success: true, Enabled: enable})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
	}
}

func IsPotatoMode(r *http.Request) bool {
	if _, hasParam := r.URL.Query()["potato"]; hasParam {
		val := r.URL.Query().Get("potato")
		return val != "0" && val != "false"
	}

	cookie, err := r.Cookie("potato_mode")
	return err == nil && cookie.Value == "1"
}
