package admin

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
	"github.com/Alexander-D-Karpov/about/internal/storage"
)

type Handler struct {
	storage       *storage.Storage
	pluginManager *plugins.Manager
	config        *config.Config
	template      *template.Template
}

type PluginData struct {
	Name        string                 `json:"name"`
	Enabled     bool                   `json:"enabled"`
	Order       int                    `json:"order"`
	Settings    map[string]interface{} `json:"settings"`
	Description string                 `json:"description"`
}

func NewHandler(storage *storage.Storage, pluginManager *plugins.Manager, config *config.Config, templates embed.FS, static embed.FS) *Handler {
	adminTmpl, err := template.ParseFS(templates, "templates/admin.html")
	if err != nil {
		panic(fmt.Sprintf("Failed to parse admin template: %v", err))
	}

	return &Handler{
		storage:       storage,
		pluginManager: pluginManager,
		config:        config,
		template:      adminTmpl,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authenticate(w, r) {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/admin")
	switch {
	case path == "" || path == "/":
		h.dashboard(w, r)
	case path == "/api/plugins" && r.Method == "GET":
		h.getPluginsAPI(w, r)
	case path == "/api/plugins" && r.Method == "POST":
		h.updatePluginsAPI(w, r)
	case path == "/api/plugin" && r.Method == "POST":
		h.updatePluginAPI(w, r)
	case path == "/api/upload" && r.Method == "POST":
		h.uploadFileAPI(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="Admin"`)
		w.WriteHeader(401)
		w.Write([]byte("Unauthorized"))
		return false
	}

	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(h.config.AdminUser))
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(h.config.AdminPass))

	if userMatch != 1 || passMatch != 1 {
		w.Header().Set("WWW-Authenticate", `Basic realm="Admin"`)
		w.WriteHeader(401)
		w.Write([]byte("Unauthorized"))
		return false
	}

	return true
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	allPlugins := h.pluginManager.GetAllPlugins()

	var pluginList []PluginData
	descriptions := map[string]string{
		"profile":    "User profile information with bio, name, and image",
		"social":     "Social media links and contact information",
		"techstack":  "Technical skills and technologies used",
		"projects":   "Portfolio projects with descriptions and links",
		"lastfm":     "Last.fm music integration showing current/recent tracks",
		"beatleader": "BeatSaber stats from BeatLeader API",
		"steam":      "Steam gaming activity and recent games",
		"neofetch":   "System information display for multiple machines",
		"webring":    "Webring navigation for connected websites",
		"visitors":   "Website visitor counter with analytics",
		"services":   "Local services and applications list",
		"code":       "GitHub and coding statistics",
		"info":       "Page information and server status",
		"personal":   "Personal information with markdown support",
		"meme":       "Random meme display for entertainment",
	}

	for name := range allPlugins {
		config := h.storage.GetPluginConfig(name)
		description := descriptions[name]
		if description == "" {
			description = "Plugin configuration"
		}

		// Clean and prepare settings for JSON serialization
		cleanSettings := h.cleanSettings(config.Settings)

		pluginList = append(pluginList, PluginData{
			Name:        name,
			Enabled:     config.Enabled,
			Order:       config.Order,
			Settings:    cleanSettings,
			Description: description,
		})
	}

	data := struct {
		Plugins []PluginData
		Config  *config.Config
	}{
		Plugins: pluginList,
		Config:  h.config,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := h.template.Execute(w, data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func (h *Handler) cleanSettings(settings map[string]interface{}) map[string]interface{} {
	if settings == nil {
		return make(map[string]interface{})
	}

	cleaned := make(map[string]interface{})
	for key, value := range settings {
		cleaned[key] = h.cleanValue(value)
	}
	return cleaned
}

func (h *Handler) cleanValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		cleaned := make(map[string]interface{})
		for key, val := range v {
			cleaned[key] = h.cleanValue(val)
		}
		return cleaned
	case []interface{}:
		cleaned := make([]interface{}, len(v))
		for i, val := range v {
			cleaned[i] = h.cleanValue(val)
		}
		return cleaned
	case string:
		return v
	case bool:
		return v
	case float64:
		return v
	case int:
		return float64(v) // Convert ints to float64 for JSON consistency
	case int64:
		return float64(v)
	default:
		// For any other type, convert to string
		return fmt.Sprintf("%v", v)
	}
}

func (h *Handler) getPluginsAPI(w http.ResponseWriter, r *http.Request) {
	allPlugins := h.pluginManager.GetAllPlugins()
	var pluginList []PluginData

	for name := range allPlugins {
		config := h.storage.GetPluginConfig(name)
		cleanSettings := h.cleanSettings(config.Settings)

		pluginList = append(pluginList, PluginData{
			Name:     name,
			Enabled:  config.Enabled,
			Order:    config.Order,
			Settings: cleanSettings,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pluginList)
}

func (h *Handler) updatePluginsAPI(w http.ResponseWriter, r *http.Request) {
	var plugins []PluginData
	if err := json.NewDecoder(r.Body).Decode(&plugins); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	for _, pluginData := range plugins {
		config := &storage.PluginConfig{
			Enabled:  pluginData.Enabled,
			Order:    pluginData.Order,
			Settings: pluginData.Settings,
		}

		if err := h.storage.SetPluginConfig(pluginData.Name, config); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save %s: %v", pluginData.Name, err), http.StatusInternalServerError)
			return
		}

		if plugin, exists := h.pluginManager.GetPlugin(pluginData.Name); exists {
			plugin.SetSettings(pluginData.Settings)
		}
	}

	// Broadcast update to all connected clients
	h.pluginManager.BroadcastUpdate("plugins_updated", map[string]interface{}{
		"action": "reorder_complete",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Plugins updated successfully",
	})
}

func (h *Handler) updatePluginAPI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	pluginName := r.FormValue("plugin")
	if pluginName == "" {
		http.Error(w, "Plugin name required", http.StatusBadRequest)
		return
	}

	enabled := r.FormValue("enabled") == "true"
	order, _ := strconv.Atoi(r.FormValue("order"))

	settings := make(map[string]interface{})
	if settingsJSON := r.FormValue("settings"); settingsJSON != "" {
		if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
			http.Error(w, "Invalid settings JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Process file uploads
	if err := h.processFileUploads(r, pluginName, settings); err != nil {
		http.Error(w, fmt.Sprintf("File upload error: %v", err), http.StatusInternalServerError)
		return
	}

	config := &storage.PluginConfig{
		Enabled:  enabled,
		Order:    order,
		Settings: settings,
	}

	if err := h.storage.SetPluginConfig(pluginName, config); err != nil {
		http.Error(w, "Failed to save configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if plugin, exists := h.pluginManager.GetPlugin(pluginName); exists {
		if err := plugin.SetSettings(settings); err != nil {
			http.Error(w, "Failed to update plugin settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Broadcast individual plugin update
	h.pluginManager.BroadcastUpdate("plugin_update", map[string]interface{}{
		"plugin": pluginName,
		"action": "settings_changed",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Plugin %s updated successfully", pluginName),
	})
}

func (h *Handler) uploadFileAPI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !h.isValidFileType(header.Filename) {
		http.Error(w, "Invalid file type", http.StatusBadRequest)
		return
	}

	if header.Size > 10<<20 {
		http.Error(w, "File too large (max 10MB)", http.StatusBadRequest)
		return
	}

	uploadsDir := filepath.Join(h.config.MediaPath, "uploads")
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		http.Error(w, "Failed to create uploads directory", http.StatusInternalServerError)
		return
	}

	filename := h.sanitizeFilename(header.Filename)
	savePath := filepath.Join(uploadsDir, filename)

	out, err := os.Create(savePath)
	if err != nil {
		http.Error(w, "Failed to create file", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	fileURL := fmt.Sprintf("/media/uploads/%s", filename)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"url":      fileURL,
		"filename": filename,
	})
}

func (h *Handler) processFileUploads(r *http.Request, pluginName string, settings map[string]interface{}) error {
	if r.MultipartForm == nil {
		return nil
	}

	for fieldName, files := range r.MultipartForm.File {
		if len(files) == 0 {
			continue
		}

		file := files[0]
		if err := h.saveUploadedFile(file, pluginName, fieldName, settings); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) saveUploadedFile(fileHeader *multipart.FileHeader, pluginName, fieldName string, settings map[string]interface{}) error {
	file, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer file.Close()

	pluginDir := filepath.Join(h.config.MediaPath, "uploads", pluginName)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return err
	}

	filename := h.sanitizeFilename(fileHeader.Filename)
	savePath := filepath.Join(pluginDir, filename)

	out, err := os.Create(savePath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		return err
	}

	fileURL := fmt.Sprintf("/media/uploads/%s/%s", pluginName, filename)
	settings[fieldName] = fileURL

	return nil
}

func (h *Handler) isValidFileType(filename string) bool {
	allowedExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg"}
	ext := strings.ToLower(filepath.Ext(filename))

	for _, allowed := range allowedExts {
		if ext == allowed {
			return true
		}
	}
	return false
}

func (h *Handler) sanitizeFilename(filename string) string {
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)

	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "..", "")
	name = strings.ReplaceAll(name, "/", "")
	name = strings.ReplaceAll(name, "\\", "")

	if len(name) > 50 {
		name = name[:50]
	}

	return name + ext
}
