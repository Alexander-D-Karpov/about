package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/assets"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"

	"github.com/Alexander-D-Karpov/about/internal/admin"
	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/handlers"
	"github.com/Alexander-D-Karpov/about/internal/layout/measure"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
	"github.com/Alexander-D-Karpov/about/internal/ranking"
	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

//go:embed static/*
var staticFiles embed.FS

//go:embed all:templates
var templateFiles embed.FS

var appStartTime time.Time

func main() {
	appStartTime = time.Now()

	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found")
	}

	cfg := config.Load()

	if err := initializeMediaFolders(cfg.MediaPath); err != nil {
		log.Fatal("Failed to initialize media folders:", err)
	}

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

	heightStore := measure.NewHeightStore(cfg.DataPath)
	if err := heightStore.Load(); err != nil {
		log.Printf("height store load failed: %v", err)
	}
	measureWorker := measure.NewWorker(heightStore, "http://127.0.0.1:"+cfg.Port)
	pluginManager.SetLayoutNotifier(measureWorker)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	go measureWorker.Start(workerCtx)

	go startBackgroundTasks(store, pluginManager)
	go pluginManager.StartPrometheusMetrics(context.Background())

	bundler := assets.NewBundler(staticFiles)
	bundler.Build()

	r := mux.NewRouter()

	bundleHandler := handlers.NewBundleHandler(bundler)
	r.Handle("/static/bundle.css", bundleHandler)
	r.Handle("/static/bundle.js", bundleHandler)

	r.PathPrefix("/static/css/stats.css").Handler(http.FileServer(http.FS(staticFiles)))
	r.PathPrefix("/static/js/stats.js").Handler(http.FileServer(http.FS(staticFiles)))

	libsHandler := http.StripPrefix("/static/libs/", http.FileServer(http.Dir("static/libs")))
	r.PathPrefix("/static/libs/").Handler(addCacheHeaders(libsHandler))

	staticHandler := http.FileServer(http.FS(staticFiles))
	r.PathPrefix("/static/").Handler(disableDirectoryListing(addCacheHeaders(staticHandler)))

	if err := os.MkdirAll(cfg.MediaPath, 0755); err != nil {
		log.Fatal("Failed to create media directory:", err)
	}

	mediaHandler := http.StripPrefix("/media/", http.FileServer(http.Dir(cfg.MediaPath)))
	r.PathPrefix("/media/").Handler(disableDirectoryListing(addCacheHeaders(mediaHandler)))

	r.HandleFunc("/upload", handlers.NewUploadHandler(cfg).ServeHTTP).Methods("POST")

	statsTmpl, err := template.ParseFS(templateFiles, "templates/stats.html")
	if err == nil {
		statsHandler := handlers.NewStatsHandler(cfg, pluginManager, statsTmpl)
		r.HandleFunc("/stats", statsHandler.ServeHTTP)
		log.Printf("Serving stats UI at /stats")
		r.HandleFunc("/api/stats/data", statsHandler.ServeHTTP)
	} else {
		log.Printf("Warning: Failed to load stats template: %v", err)
	}

	mainHandler := handlers.NewMainHandler(pluginManager, cfg, templateFiles, bundler, heightStore)
	r.HandleFunc("/", mainHandler.ServeHTTP).Methods("GET")

	wsHandler := handlers.NewWebSocketHandler(hub)
	r.HandleFunc("/ws", wsHandler.ServeHTTP)

	potatoHandler := handlers.NewPotatoHandler()
	r.Handle("/api/potato", potatoHandler)

	r.HandleFunc("/api/meme/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		memePlugin, exists := pluginManager.GetPlugin("meme")
		if !exists {
			http.Error(w, "Meme plugin not found", http.StatusNotFound)
			return
		}
		meme, ok := memePlugin.(*plugins.MemePlugin)
		if !ok {
			http.Error(w, "Invalid plugin type", http.StatusInternalServerError)
			return
		}
		currentMeme := meme.RefreshMeme()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"meme":    currentMeme,
		})
	}).Methods("POST")

	if mp, ok := pluginManager.GetPlugin("music"); ok {
		if music, ok := mp.(*plugins.MusicPlugin); ok {
			r.HandleFunc("/api/music/stats", music.HandleStatsAPI).Methods("GET")
			r.HandleFunc("/api/music/now", music.HandleNowAPI).Methods("GET")
		}
	}

	if cp, ok := pluginManager.GetPlugin("code"); ok {
		if code, ok := cp.(*plugins.CodePlugin); ok {
			r.HandleFunc("/api/git/day", code.Git().HandleDayAPI).Methods("GET")
		}
	}

	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")

		status := "ok"
		statusCode := http.StatusOK

		testCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		pluginTestResult := make(chan bool, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					pluginTestResult <- false
				}
			}()

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

	healthPlugin, healthExists := pluginManager.GetPlugin("health")
	if healthExists {
		if hp, ok := healthPlugin.(*plugins.HealthPlugin); ok {
			r.HandleFunc("/api/health/sync/{type}", hp.HandleSync).Methods("POST")
			r.HandleFunc("/api/health/status", hp.HandleStatus).Methods("GET")
		}
	}

	if vp, ok := pluginManager.GetPlugin("visitors"); ok {
		if visitors, ok := vp.(*plugins.VisitorsPlugin); ok {
			r.HandleFunc("/api/visitors/regions", visitors.HandleRegionsAPI).Methods("GET")
		}
	}

	if pgURL := os.Getenv("POSTGRES_URL"); pgURL != "" {
		rankingStore, err := ranking.NewStore(pgURL, cfg.MediaPath)
		if err != nil {
			log.Printf("Warning: Ranking module failed to initialize: %v", err)
		} else {
			rankingHandler := ranking.NewHandler(rankingStore, cfg, templateFiles)
			r.PathPrefix("/ranking").Handler(rankingHandler)
			log.Println("Ranking module enabled at /ranking")
		}
	} else {
		log.Println("Ranking module disabled (POSTGRES_URL not set)")
	}

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
	workerCancel() // cancel any in-flight measurement so Chromium is torn down

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

func disableDirectoryListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func startBackgroundTasks(store *storage.Storage, pm *plugins.Manager) {
	quit := make(chan struct{})

	// Daily: refresh projects' "last updated" (and missing "created") from GitHub.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Projects updater panic recovered: %v", r)
			}
		}()

		initial := time.NewTimer(45 * time.Second) // first run shortly after boot
		defer initial.Stop()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		run := func() {
			if err := pm.UpdatePlugin("projects"); err != nil {
				log.Printf("[Projects] daily update failed: %v", err)
			} else {
				log.Println("[Projects] refreshed last-updated dates from GitHub")
			}
		}

		for {
			select {
			case <-quit:
				return
			case <-initial.C:
				run()
			case <-ticker.C:
				run()
			}
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Backup task panic recovered: %v", r)
				time.Sleep(30 * time.Second)
				if pm != nil {
					go startBackgroundTasks(store, pm)
				}
			}
		}()

		if err := store.CreateBackup(); err != nil {
			log.Printf("Initial backup failed: %v", err)
		} else {
			log.Println("Initial backup completed successfully")
		}

		ticker := time.NewTicker(1 * time.Hour)
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
						log.Printf("Hourly backup failed: %v", err)
					} else {
						log.Println("Hourly backup completed successfully")
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

		lastFMTicker := time.NewTicker(20 * time.Second)
		generalTicker := time.NewTicker(10 * time.Minute)
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

					_, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()

					if err := pm.UpdatePlugin("music"); err != nil {
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
						}
					}
				}()
			}
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Steam update task panic recovered: %v", r)
				time.Sleep(30 * time.Second)
				if pm != nil {
					go startBackgroundTasks(store, pm)
				}
			}
		}()

		steamTicker := time.NewTicker(2 * time.Minute)
		defer steamTicker.Stop()

		for {
			select {
			case <-quit:
				return
			case <-steamTicker.C:
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Steam update panic recovered: %v", r)
						}
					}()

					if pm != nil && pm.GetClientCount() > 0 {
						if !updateMutex.TryLock() {
							return
						}
						defer updateMutex.Unlock()

						_, cancel := context.WithTimeout(context.Background(), 25*time.Second)
						defer cancel()

						if err := pm.UpdatePlugin("steam"); err != nil {
							log.Printf("Steam update failed (non-fatal): %v", err)
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

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Heartrate broadcast task panic recovered: %v", r)
				time.Sleep(30 * time.Second)
				if pm != nil {
					go startBackgroundTasks(store, pm)
				}
			}
		}()

		hrTicker := time.NewTicker(5 * time.Minute)
		dailyResetTicker := time.NewTicker(1 * time.Hour)
		persistTicker := time.NewTicker(10 * time.Minute)
		defer hrTicker.Stop()
		defer dailyResetTicker.Stop()
		defer persistTicker.Stop()

		var lastResetDay int

		for {
			select {
			case <-quit:
				return
			case <-hrTicker.C:
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Heartrate broadcast panic recovered: %v", r)
						}
					}()

					if healthPlugin, exists := pm.GetPlugin("health"); exists {
						if hp, ok := healthPlugin.(*plugins.HealthPlugin); ok {
							hp.BroadcastHeartRate()
						}
					}
				}()

			case <-dailyResetTicker.C:
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Daily reset panic recovered: %v", r)
						}
					}()

					now := time.Now()
					if now.Day() != lastResetDay && now.Hour() == 0 {
						if healthPlugin, exists := pm.GetPlugin("health"); exists {
							if hp, ok := healthPlugin.(*plugins.HealthPlugin); ok {
								hp.ResetDailyData()
								lastResetDay = now.Day()
								log.Println("Health daily data reset completed")
							}
						}
					}
				}()
			case <-persistTicker.C:
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Health persist panic recovered: %v", r)
						}
					}()

					if healthPlugin, exists := pm.GetPlugin("health"); exists {
						if hp, ok := healthPlugin.(*plugins.HealthPlugin); ok {
							hp.PersistNow()
						}
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

func initializeMediaFolders(mediaPath string) error {
	folders := []string{
		"uploads",
		"icons",
		"memes",
		"projects",
		"profile",
		"uploads/profile",
		"uploads/projects",
		"uploads/social",
		"uploads/personal",
		"uploads/services",
		"uploads/meme",
		"ranking",
	}

	for _, folder := range folders {
		path := filepath.Join(mediaPath, folder)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create folder %s: %w", folder, err)
		}
	}

	return nil
}
