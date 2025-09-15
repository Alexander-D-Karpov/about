package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
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

var appStartTime time.Time

func main() {
	appStartTime = time.Now()

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

	pluginManager := plugins.NewManager(store, hub, cfg, appStartTime)
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
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		response := fmt.Sprintf(`{"status":"ok","timestamp":%d,"version":"1.0.0","uptime":%d}`,
			time.Now().Unix(),
			int64(time.Since(appStartTime).Seconds()))

		w.Write([]byte(response))
	}).Methods("GET")

	statusHandler := handlers.NewStatusHandler(pluginManager, cfg, hub, appStartTime)
	r.HandleFunc("/status", statusHandler.ServeHTTP).Methods("GET")

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

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server listening on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	} else {
		log.Println("Server shutdown completed")
	}
}

func addCacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "about-server/1.0")

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
	quit := make(chan struct{})

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Daily backup task panic recovered: %v", r)
				time.Sleep(30 * time.Second)
				if pm != nil {
					go startBackgroundTasks(store, pm)
				}
			}
		}()

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				if err := store.CreateBackup(); err != nil {
					log.Printf("Backup failed: %v", err)
				} else {
					log.Println("Daily backup completed successfully")
				}
			}
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Plugin update task panic recovered: %v", r)
				time.Sleep(30 * time.Second)
				if pm != nil {
					go startBackgroundTasks(store, pm)
				}
			}
		}()

		lastFMTicker := time.NewTicker(10 * time.Minute)
		generalTicker := time.NewTicker(1 * time.Hour)
		systemTicker := time.NewTicker(30 * time.Second)

		defer func() {
			lastFMTicker.Stop()
			generalTicker.Stop()
			systemTicker.Stop()
		}()

		for {
			select {
			case <-quit:
				return
			case <-lastFMTicker.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("LastFM update panic recovered: %v", r)
						}
					}()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					pm.UpdatePlugin("lastfm")
					<-ctx.Done()
				}()

			case <-generalTicker.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("General plugin update panic recovered: %v", r)
						}
					}()
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
					defer cancel()
					pm.UpdateExternalData()
					<-ctx.Done()
				}()

			case <-systemTicker.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("System info update panic recovered: %v", r)
						}
					}()
					if infoPlugin, exists := pm.GetPlugin("info"); exists {
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						infoPlugin.UpdateData(ctx)
						cancel()
					}
				}()
			}
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Visitors update task panic recovered: %v", r)
				time.Sleep(30 * time.Second)
				if pm != nil {
					go startBackgroundTasks(store, pm)
				}
			}
		}()

		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Visitors plugin update panic recovered: %v", r)
						}
					}()
					if visitors, exists := pm.GetPlugin("visitors"); exists {
						ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						visitors.UpdateData(ctx)
						cancel()
					}
				}()
			}
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Resource monitor panic recovered: %v", r)
				time.Sleep(30 * time.Second)
				if pm != nil {
					go startBackgroundTasks(store, pm)
				}
			}
		}()

		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				numGoroutines := runtime.NumGoroutine()

				if numGoroutines > 1000 {
					log.Printf("WARNING: High goroutine count: %d", numGoroutines)
				}

				if m.Alloc > 100*1024*1024 {
					log.Printf("WARNING: High memory usage: %d MB", m.Alloc/1024/1024)
					runtime.GC()
				}
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
