package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Storage struct {
	dataPath string
	data     map[string]interface{}
	mutex    sync.RWMutex

	// Daily backups run in a background goroutine. backupWG lets callers wait
	// for them to finish (see Wait) rather than leaving fire-and-forget
	// goroutines writing into dataPath after the owner considers itself done.
	// Tests using t.TempDir() failed intermittently with "directory not
	// empty" because a backup recreated backups/ while RemoveAll was walking
	// the tree.
	backupWG sync.WaitGroup
}

// Wait blocks until any in-flight background backup has finished.
//
// Call it before deleting dataPath - tests should do
// `st := storage.New(dir); t.Cleanup(st.Wait)`. Cleanups run LIFO, so
// registering this after t.TempDir() makes it run before the directory is
// removed.
func (s *Storage) Wait() {
	s.backupWG.Wait()
}

type PluginConfig struct {
	Enabled  bool                   `json:"enabled"`
	Order    int                    `json:"order"`
	Settings map[string]interface{} `json:"settings"`
}

func New(dataPath string) *Storage {
	return &Storage{
		dataPath: dataPath,
		data:     make(map[string]interface{}),
	}
}

func (s *Storage) Load() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if err := os.MkdirAll(s.dataPath, 0755); err != nil {
		return err
	}

	configFile := filepath.Join(s.dataPath, "config.json")

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		s.data = s.getDefaultConfig()
		s.applyEnvOverrides()
		return s.saveToFileAtomic()
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		fmt.Printf("[Storage] WARNING: config.json is empty, attempting recovery from backup\n")
		if recovered := s.recoverFromBackup(); recovered {
			return nil
		}
		s.data = s.getDefaultConfig()
		s.applyEnvOverrides()
		return s.saveToFileAtomic()
	}

	if err := json.Unmarshal(data, &s.data); err != nil {
		fmt.Printf("[Storage] WARNING: config.json is corrupted: %v, attempting recovery\n", err)
		if recovered := s.recoverFromBackup(); recovered {
			return nil
		}
		s.data = s.getDefaultConfig()
		s.applyEnvOverrides()
		return s.saveToFileAtomic()
	}

	migrated := s.migratePluginKeys()
	if s.backfillDefaultLinks("social", "links") {
		migrated = true
	}
	s.applyEnvOverrides()
	if migrated {
		if err := s.saveToFileAtomic(); err != nil {
			fmt.Printf("[Storage] WARNING: failed to persist plugin migration: %v\n", err)
		}
	}
	return nil
}

func (s *Storage) recoverFromBackup() bool {
	backupDir := filepath.Join(s.dataPath, "backups")

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return false
	}

	var backupFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "config_") && strings.HasSuffix(entry.Name(), ".json") {
			backupFiles = append(backupFiles, entry.Name())
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(backupFiles)))

	for _, backupFile := range backupFiles {
		backupPath := filepath.Join(backupDir, backupFile)
		data, err := os.ReadFile(backupPath)
		if err != nil {
			continue
		}

		if len(data) == 0 {
			continue
		}

		var testData map[string]interface{}
		if err := json.Unmarshal(data, &testData); err != nil {
			continue
		}

		s.data = testData
		fmt.Printf("[Storage] Successfully recovered from backup: %s\n", backupFile)

		if err := s.saveToFileAtomic(); err != nil {
			fmt.Printf("[Storage] WARNING: Failed to save recovered config: %v\n", err)
		}

		return true
	}

	return false
}

func (s *Storage) Save() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.saveToFileAtomic()
}

func (s *Storage) SetPluginConfig(pluginName string, config *PluginConfig) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	plugins, ok := s.data["plugins"].(map[string]interface{})
	if !ok {
		plugins = make(map[string]interface{})
		s.data["plugins"] = plugins
	}

	plugins[pluginName] = map[string]interface{}{
		"enabled":  config.Enabled,
		"order":    config.Order,
		"settings": copySettings(config.Settings),
	}

	if err := s.saveToFileAtomic(); err != nil {
		return fmt.Errorf("failed to save plugin config to file: %w", err)
	}

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err == nil {
		s.backupWG.Add(1)
		go func() {
			defer s.backupWG.Done()
			s.createDailyBackupFromBytes(data)
		}()
	}

	return nil
}

func (s *Storage) createDailyBackupFromBytes(data []byte) {
	backupDir := filepath.Join(s.dataPath, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		fmt.Printf("[Storage] Failed to create backup directory: %v\n", err)
		return
	}

	today := time.Now().Format("2006-01-02")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("config_%s.json", today))

	// Unique temp file per backup. This was a fixed "<backupFile>.tmp", so two
	// concurrent SetPluginConfig calls raced on the same path: whichever
	// renamed first left the other failing with "no such file or directory",
	// silently losing that backup.
	f, err := os.CreateTemp(backupDir, "config_"+today+".*.tmp")
	if err != nil {
		fmt.Printf("[Storage] Failed to create backup temp file: %v\n", err)
		return
	}
	tempFile := f.Name()

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tempFile)
		fmt.Printf("[Storage] Failed to write backup: %v\n", err)
		return
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tempFile)
		fmt.Printf("[Storage] Failed to sync backup: %v\n", err)
		return
	}

	if err := f.Close(); err != nil {
		os.Remove(tempFile)
		fmt.Printf("[Storage] Failed to close backup file: %v\n", err)
		return
	}

	if err := os.Rename(tempFile, backupFile); err != nil {
		os.Remove(tempFile)
		fmt.Printf("[Storage] Failed to finalize backup: %v\n", err)
		return
	}

	s.cleanupOldBackups(backupDir, 7)
}

func (s *Storage) saveToFileAtomic() error {
	configFile := filepath.Join(s.dataPath, "config.json")
	tempFile := configFile + ".tmp"

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	f, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tempFile)
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tempFile)
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tempFile, configFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	dir, err := os.Open(s.dataPath)
	if err == nil {
		dir.Sync()
		dir.Close()
	}

	return nil
}

func (s *Storage) cleanupOldBackups(backupDir string, keepDays int) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	var backupFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "config_") && strings.HasSuffix(entry.Name(), ".json") {
			backupFiles = append(backupFiles, entry.Name())
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(backupFiles)))

	for i, file := range backupFiles {
		if i >= keepDays {
			filePath := filepath.Join(backupDir, file)
			if err := os.Remove(filePath); err != nil {
				fmt.Printf("[Storage] Failed to remove old backup %s: %v\n", file, err)
			} else {
				fmt.Printf("[Storage] Removed old backup: %s\n", file)
			}
		}
	}
}

func (s *Storage) CreateBackup() error {
	s.mutex.RLock()
	data, err := json.MarshalIndent(s.data, "", "  ")
	s.mutex.RUnlock()

	if err != nil {
		return fmt.Errorf("failed to marshal backup data: %w", err)
	}

	backupDir := filepath.Join(s.dataPath, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	today := time.Now().Format("2006-01-02")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("config_%s.json", today))
	tempFile := backupFile + ".tmp"

	f, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tempFile)
		return fmt.Errorf("failed to write backup: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tempFile)
		return fmt.Errorf("failed to sync backup: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to close backup file: %w", err)
	}

	if err := os.Rename(tempFile, backupFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to finalize backup: %w", err)
	}

	s.cleanupOldBackups(backupDir, 7)

	fmt.Printf("[Storage] Backup created: config_%s.json\n", today)
	return nil
}

// copySettings detaches a settings map from whoever else is holding it. Plugins
// persist a setting by reading their config, mutating Settings and writing it
// back, so handing out the map that lives inside s.data put every one of those
// read-modify-write cycles — and every concurrent page render reading the same
// map — on one shared map with no lock between them. That is what killed the
// process with "concurrent map writes" from MusicPlugin.saveStats.
//
// The copy is one level deep, which is the level callers mutate: they replace
// whole top-level keys rather than reaching into a nested value.
func copySettings(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *Storage) GetPluginConfig(pluginName string) *PluginConfig {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	plugins, ok := s.data["plugins"].(map[string]interface{})
	if !ok {
		return &PluginConfig{Enabled: true, Order: 0, Settings: make(map[string]interface{})}
	}

	pluginData, ok := plugins[pluginName].(map[string]interface{})
	if !ok {
		return &PluginConfig{Enabled: true, Order: 0, Settings: make(map[string]interface{})}
	}

	config := &PluginConfig{
		Enabled:  true,
		Order:    0,
		Settings: make(map[string]interface{}),
	}

	if enabled, ok := pluginData["enabled"].(bool); ok {
		config.Enabled = enabled
	}
	if order, ok := pluginData["order"].(float64); ok {
		config.Order = int(order)
	} else if orderInt, ok := pluginData["order"].(int); ok {
		config.Order = orderInt
	}
	if settings, ok := pluginData["settings"].(map[string]interface{}); ok {
		config.Settings = copySettings(settings)
	}

	return config
}

func (s *Storage) getDefaultConfig() map[string]interface{} {
	return map[string]interface{}{
		"plugins": map[string]interface{}{
			"profile": map[string]interface{}{
				"enabled": true,
				"order":   1,
				"settings": map[string]interface{}{
					"name":         "sanspie",
					"title":        "Web FullStack Developer",
					"subtitle":     "DevSecOps",
					"bio":          "I'm a 19-year-old Python/Go developer from Russia, specialized in Django web apps. I'm passionate about web app security and Linux server administration. I've participated in developing monolithic websites using Django, Flask, and FastAPI, as well as microservices with REST and GraphQL. Magnum Opus hackathon team member and SCs ITMO student.",
					"profileImage": "/static/images/sanspie.jpg",
				},
			},
			"social": map[string]interface{}{
				"enabled": true,
				"order":   2,
				"settings": map[string]interface{}{
					"ui": map[string]interface{}{
						"sectionTitle": "Links",
					},
					"links": []interface{}{
						map[string]interface{}{"name": "Telegram", "url": "https://t.me/sanspie", "icon": "telegram"},
						map[string]interface{}{"name": "GitHub", "url": "https://github.com/Alexander-D-Karpov", "icon": "github"},
						map[string]interface{}{"name": "VK", "url": "https://vk.com/al.karpov", "icon": "vk"},
						map[string]interface{}{"name": "X", "url": "https://x.com/s4nspi", "icon": "x"},
						map[string]interface{}{"name": "LinkedIn", "url": "https://www.linkedin.com/in/alexandr-karpov-ba8891218", "icon": "linkedin"},
						map[string]interface{}{"name": "Email", "url": "mailto:sanspie@akarpov.ru", "icon": "email"},
						map[string]interface{}{"name": "Discord", "url": "https://discord.com/users/SansPie#9074", "icon": "discord"},
						map[string]interface{}{"name": "CTFtime", "url": "https://ctftime.org/user/113621", "icon": "ctftime"},
						map[string]interface{}{"name": "CodeWars", "url": "https://www.codewars.com/users/Alexander-D-Karpov", "icon": "codewars"},
						map[string]interface{}{"name": "Last.fm", "url": "https://last.fm/user/sanspie", "icon": "lastfm"},
						map[string]interface{}{"name": "Steam", "url": "https://steamcommunity.com/id/sanspie", "icon": "steam"},
						map[string]interface{}{"name": "BeatLeader", "url": "https://beatleader.com/u/sanspie", "iconPath": "https://akarpov.ru/media/uploads/files/ZKJxiuUied.png"},
					},
				},
			},
			"techstack": map[string]interface{}{
				"enabled": true,
				"order":   3,
				"settings": map[string]interface{}{
					"technologies": []interface{}{
						map[string]interface{}{"name": "Django", "icon": "django"},
						map[string]interface{}{"name": "Python", "icon": "python"},
						map[string]interface{}{"name": "FastAPI", "icon": "fastapi"},
						map[string]interface{}{"name": "Go", "icon": "go"},
						map[string]interface{}{"name": "Flask", "icon": "flask"},
						map[string]interface{}{"name": "Linux", "icon": "linux"},
						map[string]interface{}{"name": "Docker", "icon": "docker"},
						map[string]interface{}{"name": "PostgreSQL", "icon": "postgresql"},
						map[string]interface{}{"name": "Java", "icon": "java"},
						map[string]interface{}{"name": "JavaScript", "icon": "javascript"},
						map[string]interface{}{"name": "HTML", "icon": "html"},
						map[string]interface{}{"name": "CSS", "icon": "css"},
						map[string]interface{}{"name": "Nginx", "icon": "nginx"},
						map[string]interface{}{"name": "Git", "icon": "git"},
						map[string]interface{}{"name": "React", "icon": "react"},
						map[string]interface{}{"name": "Bash", "icon": "bash"},
						map[string]interface{}{"name": "Redis", "icon": "redis"},
						map[string]interface{}{"name": "C++", "icon": "cpp"},
						map[string]interface{}{"name": "GraphQL", "icon": "graphql"},
						map[string]interface{}{"name": "PHP", "icon": "php"},
						map[string]interface{}{"name": "C#", "icon": "csharp"},
					},
				},
			},
			"webring": map[string]interface{}{
				"enabled": true,
				"order":   4,
				"settings": map[string]interface{}{
					"webring_url": "https://webring.otomir23.me",
					"username":    "sanspie",
				},
			},
			"neofetch": map[string]interface{}{
				"enabled": true,
				"order":   5,
				"settings": map[string]interface{}{
					"machines": []interface{}{
						map[string]interface{}{
							"name":   "Desktop PC",
							"output": "❯ neofetch\n                   -`                    sanspie@TacOS\n                  .o+`                   -------------\n                 `ooo/                   OS: Arch Linux x86_64\n                `+oooo:                  Host: 21CBCTO1WW ThinkPad X1 Carbon Gen 10\n               `+oooooo:                 Kernel: 6.16.8-arch2-1\n               -+oooooo+:                Uptime: 5 days, 5 hours, 2 mins\n             `/:-:++oooo+:               Packages: 1830 (pacman), 5 (flatpak)\n            `/++++/+++++++:              Shell: zsh 5.9\n           `/++++++++++++++:             Resolution: 2560x1600\n          `/+++ooooooooooooo/`           WM: i3\n         ./ooosssso++osssssso+`          Theme: Adwaita [GTK2], Arc-Dark [GTK3]\n        .oossssso-````/ossssss+`         Icons: Adwaita [GTK2], Papirus [GTK3]\n       -osssssso.      :ssssssso.        Terminal: alacritty\n      :osssssss/        osssso+++.       CPU: 12th Gen Intel i7-1260P (16) @ 4.700GHz\n     /ossssssss/        +ssssooo/-       GPU: Intel Alder Lake-P GT2 [Iris Xe Graphics]\n   `/ossssso+/:-        -:/+osssso+-     Memory: 14448MiB / 31795MiB\n  `+sso+:-`                 `.-/+oso:    Disk (/): 780G / 953G (83%)\n `++:.                           `-/+/   Battery0: 99% [Not charging]\n .`                                 `/   Font: Cantarell 11 [GTK2], Sans 12 [GTK3]\n",
						},
						map[string]interface{}{
							"name":   "Laptop",
							"output": "❯ neofetch\n                   -`                    sanspie@TacOS\n                  .o+`                   -------------\n                 `ooo/                   OS: Arch Linux x86_64\n                `+oooo:                  Host: 21CBCTO1WW ThinkPad X1 Carbon Gen 10\n               `+oooooo:                 Kernel: 6.16.8-arch2-1\n               -+oooooo+:                Uptime: 5 days, 5 hours, 2 mins\n             `/:-:++oooo+:               Packages: 1830 (pacman), 5 (flatpak)\n            `/++++/+++++++:              Shell: zsh 5.9\n           `/++++++++++++++:             Resolution: 2560x1600\n          `/+++ooooooooooooo/`           WM: i3\n         ./ooosssso++osssssso+`          Theme: Adwaita [GTK2], Arc-Dark [GTK3]\n        .oossssso-````/ossssss+`         Icons: Adwaita [GTK2], Papirus [GTK3]\n       -osssssso.      :ssssssso.        Terminal: alacritty\n      :osssssss/        osssso+++.       CPU: 12th Gen Intel i7-1260P (16) @ 4.700GHz\n     /ossssssss/        +ssssooo/-       GPU: Intel Alder Lake-P GT2 [Iris Xe Graphics]\n   `/ossssso+/:-        -:/+osssso+-     Memory: 14448MiB / 31795MiB\n  `+sso+:-`                 `.-/+oso:    Disk (/): 780G / 953G (83%)\n `++:.                           `-/+/   Battery0: 99% [Not charging]\n .`                                 `/   Font: Cantarell 11 [GTK2], Sans 12 [GTK3]\n",
						},
					},
				},
			},
			"music": map[string]interface{}{
				"enabled": true,
				"order":   7,
				"settings": map[string]interface{}{
					"username": "sanspie",
					"ui": map[string]interface{}{
						"sectionTitle":     "Music",
						"showRecentTracks": true,
						"showStats":        true,
					},
				},
			},
			"beatleader": map[string]interface{}{
				"enabled": true,
				"order":   8,
				"settings": map[string]interface{}{
					"username": "sanspie",
					"ui": map[string]interface{}{
						"sectionTitle":   "BeatLeader Stats",
						"showPepeGif":    true,
						"showRecentMaps": true,
						"showMainStats":  true,
						"loadingText":    "Loading BeatLeader data...",
					},
				},
			},
			"steam": map[string]interface{}{
				"enabled": true,
				"order":   9,
				"settings": map[string]interface{}{
					"steamid": "76561198828323122",
				},
			},
			"visitors": map[string]interface{}{
				"enabled": true,
				"order":   10,
				"settings": map[string]interface{}{
					"ui": map[string]interface{}{
						"sectionTitle": "Visitors",
						"showTotal":    true,
						"showToday":    true,
						"showVisitors": true,
					},
				},
			},
			"code": map[string]interface{}{
				"enabled": true,
				"order":   11,
				"settings": map[string]interface{}{
					"ui": map[string]interface{}{
						"sectionTitle":    "Coding Stats",
						"showGitHub":      true,
						"showWakatime":    true,
						"showLanguages":   true,
						"showCommitGraph": true,
					},
					"github": map[string]interface{}{
						"username": "Alexander-D-Karpov",
					},
					"wakatime": map[string]interface{}{
						"api_key": "",
					},
					"git": map[string]interface{}{
						"ui": map[string]interface{}{
							"showHeatmap":   true,
							"showActivity":  true,
							"activityLimit": 6,
						},
						"sources": []interface{}{},
					},
				},
			},
			"services": map[string]interface{}{
				"enabled": true,
				"order":   12,
				"settings": map[string]interface{}{
					"ui": map[string]interface{}{
						"sectionTitle":     "Local Services",
						"showStatus":       true,
						"showResponseTime": true,
					},
					"services": []interface{}{},
				},
			},
			"personal": map[string]interface{}{
				"enabled": true,
				"order":   13,
				"settings": map[string]interface{}{
					"ui": map[string]interface{}{
						"sectionTitle":   "Personal Info",
						"showImages":     true,
						"showCategories": true,
						"layout":         "grid",
					},
					"info": []interface{}{},
				},
			},
			"meme": map[string]interface{}{
				"enabled": true,
				"order":   14,
				"settings": map[string]interface{}{
					"ui": map[string]interface{}{
						"sectionTitle":    "Random Meme",
						"showMeme":        true,
						"autoRefresh":     false,
						"refreshInterval": 300,
					},
					"memes": []interface{}{
						map[string]interface{}{
							"type":     "image",
							"image":    "/static/memes/test.webp",
							"text":     "really cool",
							"category": "test",
						},
						map[string]interface{}{
							"type":     "image",
							"image":    "/static/memes/test2.jpg",
							"text":     "that says a lot about our society",
							"category": "test",
						},
					},
				},
			},
			"photos": map[string]interface{}{
				"enabled": true,
				"order":   15,
				"settings": map[string]interface{}{
					"apiUrl": "https://photos.akarpov.ru",
					"ui": map[string]interface{}{
						"sectionTitle": "Photos",
						"maxFolders":   6,
					},
					"hiddenFolders":     []interface{}{},
					"hiddenFolderNames": []interface{}{},
				},
			},
			"projects": map[string]interface{}{
				"enabled": true,
				"order":   98,
				"settings": map[string]interface{}{
					"projects": []interface{}{
						map[string]interface{}{
							"name":         "AKarpov",
							"description":  "Personal website",
							"github":       "https://github.com/Alexander-D-Karpov/akarpov",
							"live":         "https://akarpov.ru",
							"image":        "https://akarpov.ru/media/uploads/images/E7BE3B27-5184-4C34-969D-D05E25DA69BE.jpeg",
							"technologies": []interface{}{"Django", "Python", "PostgreSQL", "Docker", "Nginx"},
						},
						map[string]interface{}{
							"name":         "Personal About Page",
							"description":  "This page you're currently viewing",
							"github":       "https://github.com/Alexander-D-Karpov/about",
							"live":         "https://about.akarpov.ru",
							"technologies": []interface{}{"Go", "WebSockets", "JavaScript", "HTML5", "CSS3"},
						},
					},
				},
			},
			"places": map[string]interface{}{
				"enabled": true,
				"order":   6,
				"settings": map[string]interface{}{
					"ui": map[string]interface{}{
						"sectionTitle":     "Visited Places",
						"defaultZoom":      2,
						"defaultLat":       25.0,
						"defaultLng":       0.0,
						"heatmapIntensity": 0.8,
						"heatmapRadius":    30,
						"markerRadius":     8,
					},
					"places": []interface{}{},
				},
			},
			"info": map[string]interface{}{
				"enabled": true,
				"order":   99,
				"settings": map[string]interface{}{
					"ui": map[string]interface{}{
						"sectionTitle":   "Page Info",
						"showServerInfo": true,
						"showBuildInfo":  false,
						"showSourceCode": true,
						"showSystemInfo": false,
					},
					"sourceCodeURL": "https://github.com/Alexander-D-Karpov/about",
				},
			},
		},
	}
}

func (s *Storage) applyEnvOverrides() {
	s.setPluginString("profile", "name", os.Getenv("PROFILE_NAME"))
	s.setPluginString("profile", "title", os.Getenv("PROFILE_TITLE"))
	s.setPluginString("profile", "subtitle", os.Getenv("PROFILE_SUBTITLE"))
	s.setPluginString("profile", "bio", os.Getenv("PROFILE_BIO"))
	s.setPluginString("profile", "profileImage", os.Getenv("PROFILE_IMAGE"))

	if j := os.Getenv("SOCIAL_LINKS_JSON"); j != "" {
		var arr []map[string]interface{}
		if json.Unmarshal([]byte(j), &arr) == nil {
			s.setPluginValue("social", "links", toInterfaceSliceMap(arr))
		}
	}

	if j := os.Getenv("TECHSTACK_JSON"); j != "" {
		var arr []map[string]interface{}
		if json.Unmarshal([]byte(j), &arr) == nil {
			s.setPluginValue("techstack", "technologies", toInterfaceSliceMap(arr))
		}
	}

	s.setPluginNestedString("code", []string{"github", "username"}, os.Getenv("GITHUB_USERNAME"))
	s.setPluginNestedString("code", []string{"github", "token"}, os.Getenv("GITHUB_TOKEN"))
	s.setPluginNestedString("code", []string{"wakatime", "api_key"}, os.Getenv("WAKATIME_API_KEY"))

	if j := os.Getenv("GIT_SOURCES_JSON"); j != "" {
		var arr []map[string]interface{}
		if json.Unmarshal([]byte(j), &arr) == nil {
			s.setPluginNestedValue("code", []string{"git", "sources"}, toInterfaceSliceMap(arr))
		}
	}

	s.setPluginString("music", "username", os.Getenv("LASTFM_USERNAME"))

	s.setPluginNestedString("steam", []string{"steamid"}, os.Getenv("STEAM_ID"))

	if u := os.Getenv("WEBRING_URL"); u != "" {
		s.setPluginString("webring", "webring_url", u)
	}
	if u := os.Getenv("WEBRING_USER"); u != "" {
		s.setPluginString("webring", "username", u)
	}
}

func (s *Storage) setPluginNestedValue(plugin string, path []string, v interface{}) {
	if v == nil {
		return
	}
	s.ensurePlugin(plugin)
	plugins := s.data["plugins"].(map[string]interface{})
	settings := plugins[plugin].(map[string]interface{})["settings"].(map[string]interface{})
	cur := settings
	for i, k := range path {
		if i == len(path)-1 {
			cur[k] = v
			return
		}
		if next, ok := cur[k].(map[string]interface{}); ok {
			cur = next
		} else {
			n := map[string]interface{}{}
			cur[k] = n
			cur = n
		}
	}
}

func (s *Storage) ensurePlugin(name string) {
	if s.data["plugins"] == nil {
		s.data["plugins"] = map[string]interface{}{}
	}
	plugins := s.data["plugins"].(map[string]interface{})
	if plugins[name] == nil {
		plugins[name] = map[string]interface{}{
			"enabled":  true,
			"order":    99,
			"settings": map[string]interface{}{},
		}
	}
	plug := plugins[name].(map[string]interface{})
	if plug["settings"] == nil {
		plug["settings"] = map[string]interface{}{}
	}
}

func (s *Storage) setPluginString(plugin, key, val string) {
	if val == "" {
		return
	}
	s.ensurePlugin(plugin)
	plugins := s.data["plugins"].(map[string]interface{})
	settings := plugins[plugin].(map[string]interface{})["settings"].(map[string]interface{})
	settings[key] = val
}

func (s *Storage) setPluginNestedString(plugin string, path []string, val string) {
	if val == "" {
		return
	}
	s.ensurePlugin(plugin)
	plugins := s.data["plugins"].(map[string]interface{})
	settings := plugins[plugin].(map[string]interface{})["settings"].(map[string]interface{})
	cur := settings
	for i, k := range path {
		if i == len(path)-1 {
			cur[k] = val
			return
		}
		if next, ok := cur[k].(map[string]interface{}); ok {
			cur = next
		} else {
			n := map[string]interface{}{}
			cur[k] = n
			cur = n
		}
	}
}

func (s *Storage) setPluginValue(plugin, key string, v interface{}) {
	s.ensurePlugin(plugin)
	plugins := s.data["plugins"].(map[string]interface{})
	settings := plugins[plugin].(map[string]interface{})["settings"].(map[string]interface{})
	settings[key] = v
}

func toInterfaceSliceMap(in []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	return out
}

func (s *Storage) migratePluginKeys() bool {
	plugins, ok := s.data["plugins"].(map[string]interface{})
	if !ok {
		return false
	}
	if _, hasMusic := plugins["music"]; hasMusic {
		return false
	}
	old, hasOld := plugins["lastfm"]
	if !hasOld {
		return false
	}
	plugins["music"] = old
	delete(plugins, "lastfm")

	if m, ok := old.(map[string]interface{}); ok {
		settings, _ := m["settings"].(map[string]interface{})
		if settings == nil {
			settings = map[string]interface{}{}
			m["settings"] = settings
		}
		ui, _ := settings["ui"].(map[string]interface{})
		if ui == nil {
			ui = map[string]interface{}{}
			settings["ui"] = ui
		}
		if _, ok := ui["showStats"]; !ok {
			ui["showStats"] = true
		}
		if _, ok := ui["showRecentTracks"]; !ok {
			ui["showRecentTracks"] = true
		}
	}
	fmt.Printf("[Storage] Migrated plugin config: lastfm -> music\n")
	return true
}

// seededDefaultsKey holds, per list, the entries of that list in
// getDefaultConfig that an existing config has already been offered. It lives
// at the top level rather than inside the plugin settings because saving a
// plugin from the admin UI replaces its settings map wholesale.
const seededDefaultsKey = "seeded_defaults"

// backfillDefaultLinks adds entries that were introduced in the built-in
// defaults after this config file was written. The defaults are only applied
// on a first run, so an existing install would never see a newly added link
// otherwise.
//
// Every default is offered at most once: the URLs already handled are recorded
// under seededDefaultsKey, so a link removed through the admin UI stays removed
// instead of coming back on the next boot. New entries keep their position
// relative to the neighbouring defaults the config already has.
func (s *Storage) backfillDefaultLinks(plugin, listKey string) bool {
	links, ok := pluginSettings(s.data, plugin)[listKey].([]interface{})
	if !ok {
		return false
	}
	defaults, ok := pluginSettings(s.getDefaultConfig(), plugin)[listKey].([]interface{})
	if !ok {
		return false
	}

	ledger := plugin + "." + listKey
	seeded := s.seededDefaults(ledger)
	seededBefore := len(seeded)

	var added []string
	for i, def := range defaults {
		url := linkField(def, "url")
		if url == "" {
			continue
		}
		if indexOfLink(links, url) < 0 && !seeded[url] {
			links = insertAt(links, def, defaultLinkPosition(links, defaults, i))
			added = append(added, linkField(def, "name"))
		}
		seeded[url] = true
	}

	if len(added) == 0 && len(seeded) == seededBefore {
		return false
	}

	pluginSettings(s.data, plugin)[listKey] = links
	s.setSeededDefaults(ledger, seeded)
	if len(added) > 0 {
		fmt.Printf("[Storage] %s.%s: added from defaults: %s\n", plugin, listKey, strings.Join(added, ", "))
	}
	return true
}

// defaultLinkPosition returns the index at which defaults[i] should be inserted
// so that it lands after the closest preceding default the config still has -
// keeping a backfilled link next to the entries it ships next to.
func defaultLinkPosition(links, defaults []interface{}, i int) int {
	for j := i - 1; j >= 0; j-- {
		if at := indexOfLink(links, linkField(defaults[j], "url")); at >= 0 {
			return at + 1
		}
	}
	return 0
}

func indexOfLink(links []interface{}, url string) int {
	if url == "" {
		return -1
	}
	for i, link := range links {
		if linkField(link, "url") == url {
			return i
		}
	}
	return -1
}

func insertAt(links []interface{}, entry interface{}, i int) []interface{} {
	out := make([]interface{}, 0, len(links)+1)
	out = append(out, links[:i]...)
	out = append(out, entry)
	return append(out, links[i:]...)
}

// linkField reads a string field off a link entry, tolerating the untyped
// shapes that come back out of config.json.
func linkField(entry interface{}, field string) string {
	m, ok := entry.(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := m[field].(string)
	return strings.TrimSpace(v)
}

// pluginSettings returns a plugin's settings map, or nil when the plugin (or
// the config around it) is missing. Reading a nil map is safe, so callers can
// index the result directly.
func pluginSettings(data map[string]interface{}, plugin string) map[string]interface{} {
	plugins, _ := data["plugins"].(map[string]interface{})
	p, _ := plugins[plugin].(map[string]interface{})
	settings, _ := p["settings"].(map[string]interface{})
	return settings
}

func (s *Storage) seededDefaults(ledger string) map[string]bool {
	all, _ := s.data[seededDefaultsKey].(map[string]interface{})
	seen, _ := all[ledger].([]interface{})
	out := make(map[string]bool, len(seen))
	for _, v := range seen {
		if url, ok := v.(string); ok {
			out[url] = true
		}
	}
	return out
}

func (s *Storage) setSeededDefaults(ledger string, seeded map[string]bool) {
	all, _ := s.data[seededDefaultsKey].(map[string]interface{})
	if all == nil {
		all = map[string]interface{}{}
		s.data[seededDefaultsKey] = all
	}
	urls := make([]string, 0, len(seeded))
	for url := range seeded {
		urls = append(urls, url)
	}
	sort.Strings(urls)
	all[ledger] = urls
}
