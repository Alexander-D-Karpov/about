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
	"sort"
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
	Name         string                 `json:"name"`
	Enabled      bool                   `json:"enabled"`
	Order        int                    `json:"order"`
	Settings     map[string]interface{} `json:"settings"`
	SettingsJSON string                 `json:"settingsJSON"`
	Description  string                 `json:"description"`
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

		// Use settings directly from storage without any processing
		settings := config.Settings
		if settings == nil {
			settings = make(map[string]interface{})
		}

		pluginList = append(pluginList, PluginData{
			Name:        name,
			Enabled:     config.Enabled,
			Order:       config.Order,
			Settings:    settings,
			Description: description,
		})
	}

	sort.Slice(pluginList, func(i, j int) bool {
		if pluginList[i].Order == pluginList[j].Order {
			return pluginList[i].Name < pluginList[j].Name
		}
		return pluginList[i].Order < pluginList[j].Order
	})

	data := struct {
		Plugins []PluginData
		Config  *config.Config
	}{
		Plugins: pluginList,
		Config:  h.config,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := h.template.Execute(w, data); err != nil {
		fmt.Printf("Template error: %v\n", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func NewHandler(storage *storage.Storage, pluginManager *plugins.Manager, config *config.Config, templates embed.FS, static embed.FS) *Handler {
	funcMap := template.FuncMap{
		"json": func(v interface{}) string {
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				return "{}"
			}
			return string(jsonBytes)
		},
	}

	adminTmpl, err := template.New("admin.html").Funcs(funcMap).ParseFS(templates, "templates/admin.html")
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

func (h *Handler) mergeSettings(defaults, current map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for key, defaultValue := range defaults {
		if currentValue, exists := current[key]; exists {
			if defaultMap, ok := defaultValue.(map[string]interface{}); ok {
				if currentMap, ok := currentValue.(map[string]interface{}); ok {
					result[key] = h.mergeSettings(defaultMap, currentMap)
				} else {
					result[key] = currentValue
				}
			} else {
				result[key] = currentValue
			}
		} else {
			result[key] = defaultValue
		}
	}

	for key, value := range current {
		if _, exists := result[key]; !exists {
			result[key] = value
		}
	}

	return result
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

func (h *Handler) getPluginsAPI(w http.ResponseWriter, r *http.Request) {
	allPlugins := h.pluginManager.GetAllPlugins()
	var pluginList []PluginData

	for name := range allPlugins {
		config := h.storage.GetPluginConfig(name)

		// Always use actual settings from storage, preserve all existing data
		settings := config.Settings
		if settings == nil {
			settings = make(map[string]interface{})
		}

		// Deep copy to avoid modifying original
		settingsCopy := h.deepCopySettings(settings)

		pluginList = append(pluginList, PluginData{
			Name:     name,
			Enabled:  config.Enabled,
			Order:    config.Order,
			Settings: settingsCopy,
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

	updatedPlugins := make([]string, 0, len(plugins))

	for _, pluginData := range plugins {
		config := &storage.PluginConfig{
			Enabled:  pluginData.Enabled,
			Order:    pluginData.Order,
			Settings: pluginData.Settings,
		}

		// Save to storage
		if err := h.storage.SetPluginConfig(pluginData.Name, config); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save %s: %v", pluginData.Name, err), http.StatusInternalServerError)
			return
		}

		// Update plugin settings in memory
		if plugin, exists := h.pluginManager.GetPlugin(pluginData.Name); exists {
			if err := plugin.SetSettings(pluginData.Settings); err != nil {
				fmt.Printf("Warning: Failed to update plugin %s settings in memory: %v\n", pluginData.Name, err)
			}
		}

		updatedPlugins = append(updatedPlugins, pluginData.Name)
	}

	// Force storage to save to disk immediately
	if err := h.storage.Save(); err != nil {
		fmt.Printf("Warning: Failed to persist storage to disk: %v\n", err)
	}

	// Clear any cached data
	h.pluginManager.InvalidateCache()

	// Broadcast update to all connected clients
	h.pluginManager.BroadcastUpdate("plugins_updated", map[string]interface{}{
		"action":          "bulk_update_complete",
		"updated_plugins": updatedPlugins,
		"count":           len(updatedPlugins),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"message":         fmt.Sprintf("Successfully updated %d plugins", len(plugins)),
		"updated_plugins": updatedPlugins,
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
		// Parse JSON directly without additional escaping
		if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
			http.Error(w, "Invalid settings JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Process file uploads for this plugin
	if err := h.processFileUploads(r, pluginName, settings); err != nil {
		http.Error(w, fmt.Sprintf("File upload error: %v", err), http.StatusInternalServerError)
		return
	}

	// Clean and validate settings to ensure no double-escaping
	cleanedSettings := h.cleanAndValidateSettings(settings)

	config := &storage.PluginConfig{
		Enabled:  enabled,
		Order:    order,
		Settings: cleanedSettings,
	}

	// Save to storage first
	if err := h.storage.SetPluginConfig(pluginName, config); err != nil {
		http.Error(w, "Failed to save configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Force storage to save to disk immediately
	if err := h.storage.Save(); err != nil {
		fmt.Printf("Warning: Failed to persist storage to disk: %v\n", err)
	}

	// Update plugin settings in memory
	if plugin, exists := h.pluginManager.GetPlugin(pluginName); exists {
		if err := plugin.SetSettings(cleanedSettings); err != nil {
			fmt.Printf("Warning: Failed to update plugin settings in memory: %v\n", err)
		}
	}

	// Clear any cached data and force rerender
	h.pluginManager.InvalidateCache()

	// Broadcast update to all connected clients
	h.pluginManager.BroadcastUpdate("plugin_update", map[string]interface{}{
		"plugin":  pluginName,
		"action":  "settings_changed",
		"order":   order,
		"enabled": enabled,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Plugin %s updated successfully", pluginName),
		"plugin":  pluginName,
		"order":   order,
		"enabled": enabled,
	})
}

func (h *Handler) cleanAndValidateSettings(settings map[string]interface{}) map[string]interface{} {
	cleaned := make(map[string]interface{})

	for key, value := range settings {
		cleaned[key] = h.cleanValue(value)
	}

	return cleaned
}

func (h *Handler) cleanValue(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		// Don't escape or modify string values - use them as-is
		// Remove any potential double-quotes that might have been added
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			// Try to parse as JSON string to remove outer quotes
			var unquoted string
			if err := json.Unmarshal([]byte(v), &unquoted); err == nil {
				return unquoted
			}
		}
		return v
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
	case bool, int, int64, float64:
		return v
	case nil:
		return nil
	default:
		// Convert unknown types to string without escaping
		return fmt.Sprintf("%v", v)
	}
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

func (h *Handler) deepCopySettings(settings map[string]interface{}) map[string]interface{} {
	if settings == nil {
		return make(map[string]interface{})
	}

	result := make(map[string]interface{})
	for key, value := range settings {
		result[key] = h.deepCopyValue(value)
	}
	return result
}

func (h *Handler) deepCopyValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case map[string]interface{}:
		copied := make(map[string]interface{})
		for key, val := range v {
			copied[key] = h.deepCopyValue(val)
		}
		return copied
	case []interface{}:
		copied := make([]interface{}, len(v))
		for i, val := range v {
			copied[i] = h.deepCopyValue(val)
		}
		return copied
	case string, bool, float64, int, int64:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}
