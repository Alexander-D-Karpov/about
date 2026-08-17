package admin

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
)

// steamPlugin resolves the concrete Steam plugin, or nil when it is not loaded.
func (h *Handler) steamPlugin() *plugins.SteamPlugin {
	p, ok := h.pluginManager.GetPlugin("steam")
	if !ok {
		return nil
	}
	sp, ok := p.(*plugins.SteamPlugin)
	if !ok {
		return nil
	}
	return sp
}

func (h *Handler) steamAdmin(w http.ResponseWriter, r *http.Request) {
	sp := h.steamPlugin()

	data := struct {
		Config   *config.Config
		Status   map[string]interface{}
		HasToken bool
		Missing  bool
	}{
		Config: h.config,
	}

	if sp == nil {
		data.Missing = true
	} else {
		data.Status = sp.AdminStatus()
		data.HasToken = sp.HasAccessToken()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if err := h.templates["admin_steam"].Execute(w, data); err != nil {
		fmt.Printf("Template error: %v\n", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func (h *Handler) getSteamGamesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	sp := h.steamPlugin()
	if sp == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"games":  []interface{}{},
			"status": map[string]interface{}{},
			"error":  "steam plugin not loaded",
		})
		return
	}

	games := sp.AdminGames(r.URL.Query().Get("q"))
	if games == nil {
		games = []plugins.SteamAdminGame{}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"games":  games,
		"status": sp.AdminStatus(),
	})
}

type steamSaveRequest struct {
	// AccessToken is only applied when non-empty; blank means "keep the existing token".
	AccessToken *string `json:"accessToken"`
	ClearToken  bool    `json:"clearToken"`
	HiddenGames *[]int  `json:"hiddenGames"`
}

func (h *Handler) saveSteamAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	sp := h.steamPlugin()
	if sp == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "steam plugin not loaded"})
		return
	}

	var req steamSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	switch {
	case req.ClearToken:
		sp.SetAccessToken("")
	case req.AccessToken != nil && *req.AccessToken != "":
		sp.SetAccessToken(*req.AccessToken)
	}

	if req.HiddenGames != nil {
		sp.SetHiddenGames(*req.HiddenGames)
	}

	h.pluginManager.InvalidatePluginCache("steam")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  sp.AdminStatus(),
	})
}

func (h *Handler) syncSteamAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	sp := h.steamPlugin()
	if sp == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "steam plugin not loaded"})
		return
	}

	sp.ForceFullSync()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Full library sync queued",
		"status":  sp.AdminStatus(),
	})
}
