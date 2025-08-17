package storage

import (
	"encoding/json"
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

	// Overlay env on every boot
	s.applyEnvOverrides()
	return nil
}

func (s *Storage) Save() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.saveToFile()
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

	return s.saveToFile()
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
					"bio":          "I'm a 19-year-old Python/Go developer from Russia, specialized in Django web apps. I'm passionate about web app security and Linux server administration.",
					"profileImage": "/static/images/sanspie.jpg",
				},
			},
			"social": map[string]interface{}{
				"enabled": true,
				"order":   2,
				"settings": map[string]interface{}{
					"links": []interface{}{
						map[string]interface{}{"name": "GitHub", "url": "https://github.com/Alexander-D-Karpov", "icon": "github"},
						map[string]interface{}{"name": "Telegram", "url": "https://t.me/sanspie", "icon": "telegram"},
						map[string]interface{}{"name": "Email", "url": "mailto:sanspie@akarpov.ru", "icon": "email"},
					},
				},
			},
			"techstack": map[string]interface{}{
				"enabled": true,
				"order":   3,
				"settings": map[string]interface{}{
					"technologies": []interface{}{
						map[string]interface{}{"name": "Python", "icon": "python"},
						map[string]interface{}{"name": "Go", "icon": "go"},
						map[string]interface{}{"name": "Django", "icon": "django"},
						map[string]interface{}{"name": "PostgreSQL", "icon": "postgresql"},
						map[string]interface{}{"name": "Docker", "icon": "docker"},
						map[string]interface{}{"name": "Linux", "icon": "linux"},
					},
				},
			},
			"projects": map[string]interface{}{
				"enabled": true,
				"order":   4,
				"settings": map[string]interface{}{
					"projects": []interface{}{
						map[string]interface{}{
							"name":         "AKarpov",
							"description":  "Personal website and portfolio built with Django",
							"github":       "https://github.com/Alexander-D-Karpov/akarpov",
							"live":         "https://akarpov.ru",
							"technologies": []interface{}{"Django", "Python", "PostgreSQL"},
						},
					},
				},
			},
			"lastfm": map[string]interface{}{
				"enabled": true,
				"order":   5,
				"settings": map[string]interface{}{
					"username": "sanspie",
					"ui": map[string]interface{}{
						"sectionTitle":   "LastFM",
						"showScrobbles":  true,
						"showPlayButton": true,
					},
				},
			},
			"beatleader": map[string]interface{}{
				"enabled": true,
				"order":   6,
				"settings": map[string]interface{}{
					"username": "sanspie",
				},
			},
			"steam": map[string]interface{}{
				"enabled": true,
				"order":   7,
				"settings": map[string]interface{}{
					"steamid": "76561198828323122",
				},
			},
			"neofetch": map[string]interface{}{
				"enabled": true,
				"order":   8,
				"settings": map[string]interface{}{
					// Two machines, based on your earlier config/screenshot
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
								"CPU":        "AMD Ryzen 7 3700X",
								"GPU":        "NVIDIA RTX 3070",
								"Memory":     "32GB",
								"Goroutines": "10",
							},
							"colors": []interface{}{"#1793d1", "#ff3e3e", "#33d17a", "#f6d32d", "#9141ac", "#2ec27e", "#c0bfbc"},
						},
					},
				},
			},
			"webring": map[string]interface{}{
				"enabled": true,
				"order":   9,
				"settings": map[string]interface{}{
					"webring_url": "https://webring.otomir23.me",
					"username":    "sanspie",
				},
			},
			"visitors": map[string]interface{}{
				"enabled": true,
				"order":   10,
				"settings": map[string]interface{}{
					"showTotal": true,
					"showToday": true,
				},
			},
		},
	}
}

// ---------------- ENV OVERLAYS ----------------

func (s *Storage) applyEnvOverrides() {
	// Profile
	s.setPluginString("profile", "name", os.Getenv("PROFILE_NAME"))
	s.setPluginString("profile", "title", os.Getenv("PROFILE_TITLE"))
	s.setPluginString("profile", "subtitle", os.Getenv("PROFILE_SUBTITLE"))
	s.setPluginString("profile", "bio", os.Getenv("PROFILE_BIO"))
	s.setPluginString("profile", "profileImage", os.Getenv("PROFILE_IMAGE"))

	// Social links: JSON array of {name,url,icon,iconPath?}
	if j := os.Getenv("SOCIAL_LINKS_JSON"); j != "" {
		var arr []map[string]interface{}
		if json.Unmarshal([]byte(j), &arr) == nil {
			s.setPluginValue("social", "links", toInterfaceSliceMap(arr))
		}
	}

	// Tech stack: JSON array of {name,icon,iconPath?}
	if j := os.Getenv("TECHSTACK_JSON"); j != "" {
		var arr []map[string]interface{}
		if json.Unmarshal([]byte(j), &arr) == nil {
			s.setPluginValue("techstack", "technologies", toInterfaceSliceMap(arr))
		}
	}

	// Code plugin: GitHub & Wakatime
	s.setPluginNestedString("code", []string{"github", "username"}, os.Getenv("GITHUB_USERNAME"))
	s.setPluginNestedString("code", []string{"wakatime", "api_key"}, os.Getenv("WAKATIME_API_KEY"))

	// LastFM
	s.setPluginString("lastfm", "username", os.Getenv("LASTFM_USERNAME"))

	// Steam
	s.setPluginNestedString("steam", []string{"steamid"}, os.Getenv("STEAM_ID"))

	// Webring
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
