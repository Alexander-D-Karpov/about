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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
	"github.com/Alexander-D-Karpov/about/internal/storage"
)

type Handler struct {
	storage       *storage.Storage
	pluginManager *plugins.Manager
	config        *config.Config
	templates     map[string]*template.Template
	staticFiles   embed.FS
}

type PluginData struct {
	Name         string                 `json:"name"`
	Enabled      bool                   `json:"enabled"`
	Order        int                    `json:"order"`
	Settings     map[string]interface{} `json:"settings"`
	SettingsJSON string                 `json:"settingsJSON"`
	Description  string                 `json:"description"`
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
		"jsonPretty": func(v interface{}) string {
			jsonBytes, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return "{}"
			}
			return string(jsonBytes)
		},
	}

	tmplMap := make(map[string]*template.Template)
	for _, name := range []string{"admin", "admin_projects"} {
		t, err := template.New(name+".html").Funcs(funcMap).ParseFS(templates, "templates/"+name+".html")
		if err != nil {
			panic(fmt.Sprintf("Failed to parse %s template: %v", name, err))
		}
		tmplMap[name] = t
	}

	return &Handler{
		storage:       storage,
		pluginManager: pluginManager,
		config:        config,
		templates:     tmplMap,
		staticFiles:   static,
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
	case path == "/projects":
		h.projectsAdmin(w, r)
	case path == "/api/projects" && r.Method == "POST":
		h.saveProjectsAPI(w, r)
	case path == "/api/plugins" && r.Method == "POST":
		h.updatePluginsAPI(w, r)
	case path == "/api/plugin" && r.Method == "POST":
		h.updatePluginAPI(w, r)
	case path == "/api/plugin/reload" && r.Method == "GET":
		h.reloadPluginAPI(w, r)
	case path == "/api/upload" && r.Method == "POST":
		h.uploadFileAPI(w, r)
	case path == "/api/refresh" && r.Method == "POST":
		h.refreshConfigAPI(w, r)
	case path == "/api/bike/upload-gpx" && r.Method == "POST":
		h.uploadGPXAPI_Bike(w, r)
	case path == "/api/bike/reprocess-gpx" && r.Method == "POST":
		h.reprocessGPXAPI_Bike(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	// Force reload from storage to ensure fresh data
	h.storage.Load()

	allPlugins := h.pluginManager.GetAllPlugins()

	var pluginList []PluginData

	descriptions := map[string]string{
		"profile":    "User profile information with bio, name, and image",
		"social":     "Social media links and contact information",
		"techstack":  "Technical skills and technologies used",
		"projects":   "Portfolio projects with descriptions and links",
		"music":      "Last.fm music integration showing current/recent tracks",
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
		"places":     "Map of visited places with coordinates and details",
		"health":     "Health Connect stats via HCGateway API (steps, calories, sleep, etc.)",
		"photos":     "Photo gallery with folder previews from photos.akarpov.ru",
		"bike":       "Bike rides with GPX import, route maps and stats",
	}

	for name := range allPlugins {
		config := h.storage.GetPluginConfig(name)
		description := descriptions[name]
		if description == "" {
			description = "Plugin configuration"
		}

		settings := config.Settings
		if settings == nil {
			settings = make(map[string]interface{})
		}

		settings = h.normalizeSettings(settings)
		settings = h.filterInternalSettings(name, settings)

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
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	if err := h.templates["admin"].Execute(w, data); err != nil {
		fmt.Printf("Template error: %v\n", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func (h *Handler) saveBikeGPXData(originalName string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(originalName))
	if ext != ".gpx" {
		ext = ".gpx"
	}

	base := strings.TrimSuffix(filepath.Base(originalName), filepath.Ext(originalName))
	if base == "" {
		base = "ride"
	}

	filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), h.sanitizeFilename(base), ext)

	dir := filepath.Join(h.config.MediaPath, "uploads", "bike", "gpx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	fullPath := filepath.Join(dir, filename)
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", err
	}

	return fmt.Sprintf("/media/uploads/bike/gpx/%s", filename), nil
}

func (h *Handler) readBikeGPXFromURL(fileURL string) ([]byte, error) {
	const prefix = "/media/uploads/bike/gpx/"
	if !strings.HasPrefix(fileURL, prefix) {
		return nil, fmt.Errorf("invalid GPX path")
	}

	name := strings.TrimPrefix(fileURL, prefix)
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		return nil, fmt.Errorf("invalid GPX filename")
	}

	fullPath := filepath.Join(h.config.MediaPath, "uploads", "bike", "gpx", name)
	return os.ReadFile(fullPath)
}

func (h *Handler) getStoredBikeGPXMeta(rideIndex int) (string, string, error) {
	cfg := h.storage.GetPluginConfig("bike")
	if cfg.Settings == nil {
		return "", "", fmt.Errorf("bike plugin has no settings")
	}

	rides, ok := cfg.Settings["rides"].([]interface{})
	if !ok || rideIndex < 0 || rideIndex >= len(rides) {
		return "", "", fmt.Errorf("ride index out of range")
	}

	ride, ok := rides[rideIndex].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("ride entry is malformed")
	}

	gpxFile, _ := ride["gpx_file"].(string)
	gpxOriginalName, _ := ride["gpx_original_name"].(string)

	if gpxFile == "" {
		return "", "", fmt.Errorf("ride has no stored GPX file")
	}

	return gpxFile, gpxOriginalName, nil
}

func (h *Handler) mergeBikeMetadata(submitted map[string]interface{}, existing map[string]interface{}) map[string]interface{} {
	submittedRides, ok1 := submitted["rides"].([]interface{})
	existingRides, ok2 := existing["rides"].([]interface{})
	if !ok1 || !ok2 {
		return submitted
	}

	preserveKeys := []string{
		"gpx_file",
		"gpx_original_name",
	}

	for i := range submittedRides {
		if i >= len(existingRides) {
			continue
		}

		subRide, ok1 := submittedRides[i].(map[string]interface{})
		exRide, ok2 := existingRides[i].(map[string]interface{})
		if !ok1 || !ok2 {
			continue
		}

		for _, key := range preserveKeys {
			cur, exists := subRide[key]
			if !exists || cur == nil || cur == "" {
				if oldVal, ok := exRide[key]; ok && oldVal != nil && oldVal != "" {
					subRide[key] = oldVal
				}
			}
		}
	}

	submitted["rides"] = submittedRides
	return submitted
}

func (h *Handler) uploadGPXAPI_Bike(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to parse form",
		})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No file provided",
		})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to read file",
		})
		return
	}

	ride, err := plugins.ParseGPX(data)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	gpxURL, err := h.saveBikeGPXData(header.Filename, data)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to store GPX file: " + err.Error(),
		})
		return
	}

	rideResponse := map[string]interface{}{
		"name":              ride.Name,
		"date":              ride.Date,
		"distance_km":       ride.DistanceKm,
		"elevation_gain_m":  ride.ElevationGainM,
		"duration_minutes":  ride.DurationMin,
		"coordinates":       ride.Coordinates,
		"gpx_file":          gpxURL,
		"gpx_original_name": header.Filename,
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"ride":        rideResponse,
		"coord_count": len(ride.Coordinates),
	})
}

func (h *Handler) reprocessGPXAPI_Bike(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to parse form",
		})
		return
	}

	rideIndex := -1
	if rideIndexStr := r.FormValue("ride_index"); rideIndexStr != "" {
		parsedIndex, err := strconv.Atoi(rideIndexStr)
		if err != nil || parsedIndex < 0 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Invalid ride_index",
			})
			return
		}
		rideIndex = parsedIndex
	}

	var (
		data            []byte
		gpxFile         = r.FormValue("gpx_file")
		gpxOriginalName = r.FormValue("gpx_original_name")
	)

	file, header, err := r.FormFile("file")
	if err == nil {
		defer file.Close()

		data, err = io.ReadAll(file)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Failed to read uploaded file",
			})
			return
		}

		gpxFile, err = h.saveBikeGPXData(header.Filename, data)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Failed to store GPX file: " + err.Error(),
			})
			return
		}
		gpxOriginalName = header.Filename
	} else {
		if gpxFile == "" && rideIndex >= 0 {
			storedFile, storedName, metaErr := h.getStoredBikeGPXMeta(rideIndex)
			if metaErr == nil {
				gpxFile = storedFile
				if gpxOriginalName == "" {
					gpxOriginalName = storedName
				}
			}
		}

		if gpxFile == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "No stored GPX file for this ride. Upload GPX first.",
			})
			return
		}

		data, err = h.readBikeGPXFromURL(gpxFile)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Failed to read stored GPX: " + err.Error(),
			})
			return
		}
	}

	parsed, err := plugins.ParseGPX(data)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	rideResponse := map[string]interface{}{
		"coordinates":       parsed.Coordinates,
		"distance_km":       parsed.DistanceKm,
		"elevation_gain_m":  parsed.ElevationGainM,
		"duration_minutes":  parsed.DurationMin,
		"gpx_file":          gpxFile,
		"gpx_original_name": gpxOriginalName,
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"ride_index":  rideIndex,
		"coord_count": len(parsed.Coordinates),
		"ride":        rideResponse,
	})
}

func (h *Handler) projectsAdmin(w http.ResponseWriter, r *http.Request) {
	h.storage.Load()
	cfg := h.storage.GetPluginConfig("projects")
	settings := cfg.Settings
	if settings == nil {
		settings = make(map[string]interface{})
	}
	projects, _ := settings["projects"].([]interface{})

	data := struct {
		Projects []interface{}
		Config   *config.Config
	}{
		Projects: projects,
		Config:   h.config,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if err := h.templates["admin_projects"].Execute(w, data); err != nil {
		fmt.Printf("Template error: %v\n", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

func (h *Handler) saveProjectsAPI(w http.ResponseWriter, r *http.Request) {
	var projects []interface{}
	if err := json.NewDecoder(r.Body).Decode(&projects); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	cfg := h.storage.GetPluginConfig("projects")
	if cfg.Settings == nil {
		cfg.Settings = make(map[string]interface{})
	}
	cfg.Settings["projects"] = projects

	if err := h.storage.SetPluginConfig("projects", cfg); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	if plugin, exists := h.pluginManager.GetPlugin("projects"); exists {
		plugin.SetSettings(cfg.Settings)
	}
	h.pluginManager.InvalidateCache()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "count": len(projects)})
}

var hiddenSettingKeys = map[string][]string{
	"beatleader": {"cachedCubes", "lastCubesCalculated"},
	"health": {
		"current_data", "daily_averages", "sleep_records",
		"last_persist", "last_reset_date", "last_workout",
	},
	"code": {"gitStatsCache"},
}

func isHiddenSettingKey(pluginName, key string) bool {
	lk := strings.ToLower(key)
	if strings.Contains(lk, "cache") {
		return true
	}
	if pluginName == "music" {
		if strings.HasPrefix(lk, "loved") || strings.HasPrefix(lk, "spotifysaved") || lk == "runtimecache" {
			return true
		}
	}
	for _, k := range hiddenSettingKeys[pluginName] {
		if key == k {
			return true
		}
	}
	return false
}

func (h *Handler) filterInternalSettings(pluginName string, settings map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range settings {
		if isHiddenSettingKey(pluginName, key) {
			continue
		}
		switch v := value.(type) {
		case map[string]interface{}:
			result[key] = h.filterInternalSettings(pluginName, v)
		default:
			result[key] = value
		}
	}
	return result
}

func (h *Handler) preserveHiddenSettings(pluginName string, settings map[string]interface{}) map[string]interface{} {
	existing := h.storage.GetPluginConfig(pluginName)
	if existing.Settings == nil {
		return settings
	}
	if settings == nil {
		settings = make(map[string]interface{})
	}
	for key, val := range existing.Settings {
		if !isHiddenSettingKey(pluginName, key) {
			continue
		}
		if _, present := settings[key]; !present {
			settings[key] = val
		}
	}
	return settings
}

func (h *Handler) normalizeSettings(settings map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range settings {
		switch v := value.(type) {
		case map[string]interface{}:
			result[key] = h.normalizeSettings(v)
		case []interface{}:
			result[key] = h.normalizeArray(v)
		default:
			result[key] = value
		}
	}

	return result
}

func (h *Handler) normalizeArray(arr []interface{}) []interface{} {
	result := make([]interface{}, len(arr))

	for i, item := range arr {
		switch v := item.(type) {
		case map[string]interface{}:
			result[i] = h.normalizeSettings(v)
		case []interface{}:
			result[i] = h.normalizeArray(v)
		default:
			result[i] = item
		}
	}

	return result
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

func (h *Handler) getPluginsAPI(w http.ResponseWriter, r *http.Request) {
	// Force reload from storage
	h.storage.Load()

	allPlugins := h.pluginManager.GetAllPlugins()
	var pluginList []PluginData

	for name := range allPlugins {
		config := h.storage.GetPluginConfig(name)

		settings := config.Settings
		if settings == nil {
			settings = make(map[string]interface{})
		}

		settings = h.normalizeSettings(settings)
		settings = h.filterInternalSettings(name, settings)

		pluginList = append(pluginList, PluginData{
			Name:     name,
			Enabled:  config.Enabled,
			Order:    config.Order,
			Settings: settings,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(pluginList)
}

func (h *Handler) reloadPluginAPI(w http.ResponseWriter, r *http.Request) {
	pluginName := r.URL.Query().Get("plugin")
	if pluginName == "" {
		http.Error(w, "Plugin name required", http.StatusBadRequest)
		return
	}

	// Force reload from storage
	h.storage.Load()

	config := h.storage.GetPluginConfig(pluginName)

	settings := config.Settings
	if settings == nil {
		settings = make(map[string]interface{})
	}

	settings = h.normalizeSettings(settings)
	settings = h.filterInternalSettings(pluginName, settings)

	response := map[string]interface{}{
		"success":  true,
		"name":     pluginName,
		"enabled":  config.Enabled,
		"order":    config.Order,
		"settings": settings,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) refreshConfigAPI(w http.ResponseWriter, r *http.Request) {
	if err := h.storage.Load(); err != nil {
		http.Error(w, "Failed to reload config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Configuration reloaded from disk",
	})
}

func (h *Handler) updatePluginsAPI(w http.ResponseWriter, r *http.Request) {
	var plugins []PluginData
	if err := json.NewDecoder(r.Body).Decode(&plugins); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	updatedPlugins := make([]string, 0, len(plugins))

	for _, pluginData := range plugins {
		pluginData.Settings = h.preserveHiddenSettings(pluginData.Name, pluginData.Settings)
		if pluginData.Name == "bike" {
			existingCfg := h.storage.GetPluginConfig(pluginData.Name)
			if existingCfg.Settings != nil {
				pluginData.Settings = h.mergeBikeMetadata(pluginData.Settings, existingCfg.Settings)
			}
		}

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
			if err := plugin.SetSettings(pluginData.Settings); err != nil {
				fmt.Printf("Warning: Failed to update plugin %s settings in memory: %v\n", pluginData.Name, err)
			}
		}

		updatedPlugins = append(updatedPlugins, pluginData.Name)
	}

	if err := h.storage.Save(); err != nil {
		fmt.Printf("Warning: Failed to persist storage to disk: %v\n", err)
	}

	h.pluginManager.InvalidateCache()

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
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to parse form: " + err.Error(),
		})
		return
	}

	pluginName := r.FormValue("plugin")
	if pluginName == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Plugin name required",
		})
		return
	}

	enabled := r.FormValue("enabled") == "true"
	order, _ := strconv.Atoi(r.FormValue("order"))

	settings := make(map[string]interface{})
	if settingsJSON := r.FormValue("settings"); settingsJSON != "" {
		if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Invalid settings JSON: " + err.Error(),
			})
			return
		}
	}

	existingCfg := h.storage.GetPluginConfig(pluginName)
	settings = h.preserveHiddenSettings(pluginName, settings)
	if pluginName == "bike" && existingCfg.Settings != nil {
		settings = h.mergeBikeMetadata(settings, existingCfg.Settings)
	}

	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		for fieldName, files := range r.MultipartForm.File {
			if len(files) > 0 {
				file := files[0]
				uploadedURL, err := h.handleFileUpload(file, pluginName)
				if err != nil {
					fmt.Printf("File upload error for %s: %v\n", fieldName, err)
				} else {
					h.setNestedValue(settings, fieldName, uploadedURL)
				}
			}
		}
	}

	config := &storage.PluginConfig{
		Enabled:  enabled,
		Order:    order,
		Settings: settings,
	}

	if err := h.storage.SetPluginConfig(pluginName, config); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to save configuration: " + err.Error(),
		})
		return
	}

	if err := h.storage.Save(); err != nil {
		fmt.Printf("Warning: Failed to persist storage to disk: %v\n", err)
	}

	if plugin, exists := h.pluginManager.GetPlugin(pluginName); exists {
		if err := plugin.SetSettings(settings); err != nil {
			fmt.Printf("Warning: Failed to update plugin settings in memory: %v\n", err)
		}
	}

	h.pluginManager.InvalidateCache()

	h.pluginManager.BroadcastUpdate("plugin_update", map[string]interface{}{
		"plugin":  pluginName,
		"action":  "settings_changed",
		"order":   order,
		"enabled": enabled,
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  fmt.Sprintf("Plugin %s updated successfully", pluginName),
		"plugin":   pluginName,
		"order":    order,
		"enabled":  enabled,
		"settings": settings,
	})
}

func (h *Handler) setNestedValue(obj map[string]interface{}, path string, value interface{}) {
	keys := strings.Split(path, ".")
	current := obj

	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]

		if strings.Contains(key, "[") && strings.Contains(key, "]") {
			// Handle array notation
			arrayKey := key[:strings.Index(key, "[")]
			indexStr := key[strings.Index(key, "[")+1 : strings.Index(key, "]")]
			index, _ := strconv.Atoi(indexStr)

			if current[arrayKey] == nil {
				current[arrayKey] = []interface{}{}
			}

			arr, ok := current[arrayKey].([]interface{})
			if !ok {
				return
			}

			// Ensure array is large enough
			for len(arr) <= index {
				arr = append(arr, make(map[string]interface{}))
			}
			current[arrayKey] = arr

			if index < len(arr) {
				if m, ok := arr[index].(map[string]interface{}); ok {
					current = m
				} else {
					arr[index] = make(map[string]interface{})
					current = arr[index].(map[string]interface{})
				}
			}
		} else {
			// Handle regular object key
			if current[key] == nil {
				current[key] = make(map[string]interface{})
			}
			if next, ok := current[key].(map[string]interface{}); ok {
				current = next
			} else {
				return
			}
		}
	}

	lastKey := keys[len(keys)-1]
	current[lastKey] = value
}

func (h *Handler) handleFileUpload(fileHeader *multipart.FileHeader, pluginName string) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Validate file type
	if !h.isValidFileType(fileHeader.Filename) {
		return "", fmt.Errorf("invalid file type")
	}

	// Create unique filename with timestamp
	ext := filepath.Ext(fileHeader.Filename)
	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("%s_%s_%s%s", pluginName, timestamp, h.sanitizeFilename(fileHeader.Filename[:len(fileHeader.Filename)-len(ext)]), ext)

	uploadsDir := filepath.Join(h.config.MediaPath, "uploads", pluginName)
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		return "", err
	}

	savePath := filepath.Join(uploadsDir, filename)

	out, err := os.Create(savePath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("/media/uploads/%s/%s", pluginName, filename), nil
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

	pluginName := r.FormValue("plugin")
	fieldPath := r.FormValue("field")

	if !h.isValidFileType(header.Filename) {
		http.Error(w, "Invalid file type", http.StatusBadRequest)
		return
	}

	if header.Size > 10<<20 {
		http.Error(w, "File too large (max 10MB)", http.StatusBadRequest)
		return
	}

	ext := filepath.Ext(header.Filename)
	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("%s_%s%s", timestamp, h.sanitizeFilename(header.Filename[:len(header.Filename)-len(ext)]), ext)

	var uploadsDir string
	if pluginName != "" {
		uploadsDir = filepath.Join(h.config.MediaPath, "uploads", pluginName)
	} else {
		uploadsDir = filepath.Join(h.config.MediaPath, "uploads")
	}

	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		http.Error(w, "Failed to create uploads directory", http.StatusInternalServerError)
		return
	}

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

	var fileURL string
	if pluginName != "" {
		fileURL = fmt.Sprintf("/media/uploads/%s/%s", pluginName, filename)
	} else {
		fileURL = fmt.Sprintf("/media/uploads/%s", filename)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"url":      fileURL,
		"filename": filename,
		"field":    fieldPath,
		"plugin":   pluginName,
	})
}

func (h *Handler) isValidFileType(filename string) bool {
	allowedExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".ico"}
	ext := strings.ToLower(filepath.Ext(filename))

	for _, allowed := range allowedExts {
		if ext == allowed {
			return true
		}
	}
	return false
}

func (h *Handler) sanitizeFilename(filename string) string {
	// Remove special characters
	reg := regexp.MustCompile(`[^a-zA-Z0-9\-_]`)
	name := reg.ReplaceAllString(filename, "_")

	// Limit length
	if len(name) > 50 {
		name = name[:50]
	}

	return name
}
