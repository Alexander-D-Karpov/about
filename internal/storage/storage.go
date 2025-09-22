package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
		return s.saveToFile()
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &s.data); err != nil {
		s.data = s.getDefaultConfig()
		s.applyEnvOverrides()
		return s.saveToFile()
	}

	s.applyEnvOverrides()
	return nil
}

func (s *Storage) Save() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.saveToFile()
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

	// Immediately persist to file
	if err := s.saveToFile(); err != nil {
		return fmt.Errorf("failed to save plugin config to file: %w", err)
	}

	return nil
}

func (s *Storage) saveToFile() error {
	configFile := filepath.Join(s.dataPath, "config.json")
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0644)
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

func (s *Storage) CreateBackup() error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	backupDir := filepath.Join(s.dataPath, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	oldBackup := filepath.Join(backupDir, "config.json.bak")
	_ = os.Remove(oldBackup)

	configFile := filepath.Join(s.dataPath, "config.json")
	backupFile := filepath.Join(backupDir, "config.json.bak")

	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	return os.WriteFile(backupFile, data, 0644)
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
							"name":     "Desktop PC",
							"username": "sanspie",
							"hostname": "TanOS",
							"ascii": []interface{}{
								"                   -`                    ",
								"                  .o+`                   ",
								"                 `ooo/                   ",
								"                `+oooo:                  ",
								"               `+oooooo:                 ",
								"               -+oooooo+:                ",
								"             `/:-:++oooo+:               ",
								"            `/++++/+++++++:              ",
								"           `/++++++++++++++:             ",
								"          `/+++ooooooooooooo/`           ",
								"         ./ooosssso++osssssso+`          ",
								"        .oossssso-````/ossssss+`         ",
								"       -osssssso.      :ssssssso.        ",
								"      :osssssss/        osssso+++.       ",
								"     /ossssssss/        +ssssooo/-       ",
								"   `/ossssso+/:-        -:/+osssso+-     ",
								"  `+sso+:-`                 `.-/+oso:    ",
								" `++:.                           `-/+/   ",
								" .`                                 `/   ",
							},
							"info": map[string]interface{}{
								"OS":         "Arch Linux x86_64",
								"Kernel":     "6.8.4-arch1-1",
								"Uptime":     "20 hours, 26 mins",
								"Packages":   "2285 (pacman)",
								"Shell":      "zsh 5.9",
								"Resolution": "1920x1080, 3440x1440",
								"DE":         "Plasma 6.0.3",
								"WM":         "KWin",
								"WM Theme":   "Sweet-Mars-transparent",
								"Theme":      "Breeze-Dark [GTK2/3]",
								"Icons":      "candy-icons [Plasma]",
								"Terminal":   "alacritty",
								"CPU":        "AMD Ryzen 7 3700X",
								"GPU":        "NVIDIA RTX 5070",
								"Memory":     "17958MiB / 32022MiB",
								"Disk (/)":   "411G / 466G (89%)",
							},
						},
						map[string]interface{}{
							"name":     "Laptop",
							"username": "sanspie",
							"hostname": "TacOS",
							"ascii": []interface{}{
								"                   -`                    ",
								"                  .o+`                   ",
								"                 `ooo/                   ",
								"                `+oooo:                  ",
								"               `+oooooo:                 ",
								"               -+oooooo+:                ",
								"             `/:-:++oooo+:               ",
								"            `/++++/+++++++:              ",
								"           `/++++++++++++++:             ",
								"          `/+++ooooooooooooo/`           ",
								"         ./ooosssso++osssssso+`          ",
								"        .oossssso-````/ossssss+`         ",
								"       -osssssso.      :ssssssso.        ",
								"      :osssssss/        osssso+++.       ",
								"     /ossssssss/        +ssssooo/-       ",
								"   `/ossssso+/:-        -:/+osssso+-     ",
								"  `+sso+:-`                 `.-/+oso:    ",
								" `++:.                           `-/+/   ",
								" .`                                 `/   ",
							},
							"info": map[string]interface{}{
								"OS":         "Arch Linux x86_64",
								"Host":       "ThinkPad X1 Carbon Gen 10",
								"Kernel":     "6.12.1-arch1-1",
								"Uptime":     "6 hours, 39 mins",
								"Packages":   "1358 (pacman)",
								"Shell":      "zsh 5.9",
								"Resolution": "3840x2400",
								"WM":         "i3",
								"Theme":      "Arc-Dark [GTK3]",
								"Icons":      "Papirus [GTK3]",
								"Terminal":   "alacritty",
								"CPU":        "Intel i7-1260P",
								"GPU":        "Intel Iris Xe Graphics",
								"Memory":     "12197MiB / 31800MiB",
								"Disk (/)":   "107G / 953G (12%)",
								"Battery0":   "59% [Charging]",
							},
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
