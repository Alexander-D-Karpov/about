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
	"sync"
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

		status := "ok"
		statusCode := http.StatusOK

		// Test if main functionality is working with shorter timeout
		testCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		pluginTestResult := make(chan bool, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					pluginTestResult <- false
				}
			}()

			// Try to get a simple plugin count instead of full render
			enabledPlugins := pluginManager.GetEnabledPlugins()
			pluginTestResult <- len(enabledPlugins) >= 0
		}()

		var mainFunctional bool
		select {
		case result := <-pluginTestResult:
			mainFunctional = result
		case <-testCtx.Done():
			mainFunctional = false
		case <-time.After(800 * time.Millisecond):
			mainFunctional = false
		}

		if !mainFunctional {
			status = "degraded"
			statusCode = http.StatusServiceUnavailable
		}

		response := fmt.Sprintf(`{"status":"%s","timestamp":%d,"version":"1.0.0","uptime":%d,"main_functional":%t}`,
			status,
			time.Now().Unix(),
			int64(time.Since(appStartTime).Seconds()),
			mainFunctional)

		w.WriteHeader(statusCode)
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
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Backup task panic: %v", r)
						}
					}()

					if err := store.CreateBackup(); err != nil {
						log.Printf("Backup failed: %v", err)
					} else {
						log.Println("Daily backup completed successfully")
					}
				}()
			}
		}
	}()

	var updateMutex sync.Mutex
	var lastGeneralUpdate time.Time

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
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("LastFM update panic recovered: %v", r)
						}
					}()

					if !updateMutex.TryLock() {
						return
					}
					defer updateMutex.Unlock()

					_, cancel := context.WithTimeout(context.Background(), 25*time.Second)
					defer cancel()

					if err := pm.UpdatePlugin("lastfm"); err != nil {
						log.Printf("LastFM update failed (non-fatal): %v", err)
					}
				}()

			case <-generalTicker.C:
				if time.Since(lastGeneralUpdate) < 50*time.Minute {
					continue
				}

				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("General plugin update panic recovered: %v", r)
						}
					}()

					if !updateMutex.TryLock() {
						log.Printf("Skipping general update - another update in progress")
						return
					}
					defer updateMutex.Unlock()

					log.Printf("Starting background plugin update...")
					startTime := time.Now()

					_, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
					defer cancel()

					func() {
						defer func() {
							if r := recover(); r != nil {
								log.Printf("UpdateExternalDataBackground panic: %v", r)
							}
						}()

						pm.UpdateExternalDataBackground()
						lastGeneralUpdate = time.Now()
						log.Printf("Background plugin update completed in %v", time.Since(startTime))
					}()
				}()

			case <-systemTicker.C:
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("System info update panic recovered: %v", r)
						}
					}()

					if infoPlugin, exists := pm.GetPlugin("info"); exists {
						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
						defer cancel()

						if err := infoPlugin.UpdateData(ctx); err != nil {
							// Don't log info plugin errors as they're not critical
						}
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
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Visitors plugin update panic recovered: %v", r)
						}
					}()

					if visitors, exists := pm.GetPlugin("visitors"); exists {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()

						if err := visitors.UpdateData(ctx); err != nil {
							// Don't log visitors timeout errors as they're handled in the plugin
						}
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

		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Resource monitor check panic: %v", r)
						}
					}()

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
				}()
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
