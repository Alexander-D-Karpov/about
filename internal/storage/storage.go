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

	s.applyEnvOverrides()
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
		"settings": config.Settings,
	}

	if err := s.saveToFileAtomic(); err != nil {
		return fmt.Errorf("failed to save plugin config to file: %w", err)
	}

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err == nil {
		go s.createDailyBackupFromBytes(data)
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
	tempFile := backupFile + ".tmp"

	f, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Printf("[Storage] Failed to create backup temp file: %v\n", err)
		return
	}

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

func (s *Storage) createDailyBackup() {
	s.mutex.RLock()
	data, err := json.MarshalIndent(s.data, "", "  ")
	s.mutex.RUnlock()

	if err != nil {
		fmt.Printf("[Storage] Failed to marshal backup data: %v\n", err)
		return
	}

	backupDir := filepath.Join(s.dataPath, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		fmt.Printf("[Storage] Failed to create backup directory: %v\n", err)
		return
	}

	today := time.Now().Format("2006-01-02")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("config_%s.json", today))
	tempFile := backupFile + ".tmp"

	f, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Printf("[Storage] Failed to create backup temp file: %v\n", err)
		return
	}

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
		config.Settings = settings
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
			"lastfm": map[string]interface{}{
				"enabled": true,
				"order":   7,
				"settings": map[string]interface{}{
					"username": "sanspie",
					"ui": map[string]interface{}{
						"sectionTitle":     "Music",
						"showScrobbles":    true,
						"showPlayButton":   true,
						"showRecentTracks": true,
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

	s.setPluginString("lastfm", "username", os.Getenv("LASTFM_USERNAME"))

	s.setPluginNestedString("steam", []string{"steamid"}, os.Getenv("STEAM_ID"))

	if u := os.Getenv("WEBRING_URL"); u != "" {
		s.setPluginString("webring", "webring_url", u)
	}
	if u := os.Getenv("WEBRING_USER"); u != "" {
		s.setPluginString("webring", "username", u)
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
