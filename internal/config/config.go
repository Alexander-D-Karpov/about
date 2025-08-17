package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port       string
	DataPath   string
	StaticPath string
	MediaPath  string
	AdminUser  string
	AdminPass  string
	LastFMKey  string
	SteamKey   string
	Debug      bool
}

func Load() *Config {
	return &Config{
		Port:       getEnv("PORT", "8080"),
		DataPath:   getEnv("DATA_PATH", "./data"),
		StaticPath: getEnv("STATIC_PATH", "./static"),
		MediaPath:  getEnv("MEDIA_PATH", "./media"),
		AdminUser:  getEnv("ADMIN_USER", "admin"),
		AdminPass:  getEnv("ADMIN_PASS", "password"),
		LastFMKey:  getEnv("LASTFM_API_KEY", ""),
		SteamKey:   getEnv("STEAM_API_KEY", ""),
		Debug:      getEnvBool("DEBUG", false),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
