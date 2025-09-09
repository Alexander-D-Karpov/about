package main

import (
	"embed"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"

	"github.com/Alexander-D-Karpov/about/internal/admin"
	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/handlers"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

//go:embed static/*
var staticFiles embed.FS

//go:embed templates/*
var templateFiles embed.FS

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found")
	}

	cfg := config.Load()

	store := storage.New(cfg.DataPath)
	if err := store.Load(); err != nil {
		log.Fatal("Failed to load storage:", err)
	}

	hub := stream.New()
	go hub.Run()

	pluginManager := plugins.NewManager(store, hub, cfg)
	if err := pluginManager.LoadAll(); err != nil {
		log.Fatal("Failed to load plugins:", err)
	}

	log.Println("Preloading plugin data...")
	if err := pluginManager.PreloadData(); err != nil {
		log.Printf("Warning: Failed to preload some plugin data: %v", err)
	}
	log.Println("Plugin data preloaded successfully")

	go startBackgroundTasks(store, pluginManager)

	r := mux.NewRouter()

	staticHandler := http.FileServer(http.FS(staticFiles))
	r.PathPrefix("/static/").Handler(addCacheHeaders(staticHandler))

	r.HandleFunc("/upload", handlers.NewUploadHandler(cfg).ServeHTTP).Methods("POST")

	mainHandler := handlers.NewMainHandler(pluginManager, cfg, templateFiles)
	r.HandleFunc("/", mainHandler.ServeHTTP).Methods("GET")

	wsHandler := handlers.NewWebSocketHandler(hub)
	r.HandleFunc("/ws", wsHandler.ServeHTTP)

	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}).Methods("GET")

	r.HandleFunc("/favicon.ico", faviconHandler("favicon.ico")).Methods("GET")
	r.HandleFunc("/favicon.png", faviconHandler("favicon.png")).Methods("GET")

	adminHandler := admin.NewHandler(store, pluginManager, cfg, templateFiles, staticFiles)
	r.PathPrefix("/admin").Handler(adminHandler)

	log.Printf("Server starting on port %s", cfg.Port)
	log.Printf("Admin panel available at /admin (user: %s)", cfg.AdminUser)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Fatal(server.ListenAndServe())
}

func addCacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Disable caching for main page and dynamic content to ensure visitor counts update
		if strings.Contains(r.URL.Path, ".css") || strings.Contains(r.URL.Path, ".js") || strings.Contains(r.URL.Path, ".png") || strings.Contains(r.URL.Path, ".jpg") || strings.Contains(r.URL.Path, ".svg") {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		} else {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}
		next.ServeHTTP(w, r)
	})
}

func startBackgroundTasks(store *storage.Storage, pm *plugins.Manager) {
	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 3, 0, 0, 0, now.Location())
			time.Sleep(time.Until(next))

			if err := store.CreateBackup(); err != nil {
				log.Printf("Backup failed: %v", err)
			} else {
				log.Println("Daily backup completed successfully")
			}
		}
	}()

	go func() {
		lastFMTicker := time.NewTicker(10 * time.Minute)
		generalTicker := time.NewTicker(1 * time.Hour)
		systemTicker := time.NewTicker(30 * time.Second)
		defer lastFMTicker.Stop()
		defer generalTicker.Stop()
		defer systemTicker.Stop()

		for {
			select {
			case <-lastFMTicker.C:
				pm.UpdatePlugin("lastfm")
			case <-generalTicker.C:
				pm.UpdateExternalData()
			case <-systemTicker.C:
				if infoPlugin, exists := pm.GetPlugin("info"); exists {
					infoPlugin.UpdateData(nil)
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			if visitors, exists := pm.GetPlugin("visitors"); exists {
				visitors.UpdateData(nil)
			}
		}
	}()
}

func faviconHandler(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFiles.ReadFile("static/favicon/" + path)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		switch {
		case strings.HasSuffix(path, ".ico"):
			w.Header().Set("Content-Type", "image/x-icon")
		case strings.HasSuffix(path, ".png"):
			w.Header().Set("Content-Type", "image/png")
		default:
			w.Header().Set("Content-Type", "application/octet-stream")
		}

		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}
}
