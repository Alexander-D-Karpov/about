package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port              string
	DataPath          string
	StaticPath        string
	MediaPath         string
	AdminUser         string
	AdminPass         string
	LastFMKey         string
	HCGatewayURL      string
	HCGatewayUser     string
	HCGatewayPassword string
	SteamKey          string
	Debug             bool
	// Prometheus Push (Sending data)
	PrometheusPushGateway  string
	PrometheusUser         string
	PrometheusPassword     string
	PrometheusJobName      string
	PrometheusPushInterval int
	// Prometheus Query (Reading data for stats page)
	PrometheusQueryURL string
}

func Load() *Config {
	return &Config{
		Port:              getEnv("PORT", "8080"),
		DataPath:          getEnv("DATA_PATH", "./data"),
		StaticPath:        getEnv("STATIC_PATH", "./static"),
		MediaPath:         getEnv("MEDIA_PATH", "./media"),
		AdminUser:         getEnv("ADMIN_USER", "admin"),
		AdminPass:         getEnv("ADMIN_PASS", "password"),
		LastFMKey:         getEnv("LASTFM_API_KEY", ""),
		SteamKey:          getEnv("STEAM_API_KEY", ""),
		HCGatewayURL:      getEnv("HCGATEWAY_URL", "https://api.hcgateway.shuchir.dev"),
		HCGatewayUser:     getEnv("HCGATEWAY_USER", ""),
		HCGatewayPassword: getEnv("HCGATEWAY_PASSWORD", ""),
		Debug:             getEnvBool("DEBUG", false),

		PrometheusPushGateway:  strings.TrimRight(getEnv("PROMETHEUS_PUSHGATEWAY_URL", ""), "/"),
		PrometheusUser:         getEnv("PROMETHEUS_USER", ""),
		PrometheusPassword:     getEnv("PROMETHEUS_PASSWORD", ""),
		PrometheusJobName:      getEnv("PROMETHEUS_JOB_NAME", "about_page"),
		PrometheusPushInterval: getEnvInt("PROMETHEUS_PUSH_INTERVAL", 15),

		PrometheusQueryURL: strings.TrimRight(getEnv("PROMETHEUS_QUERY_URL", ""), "/"),
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

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}
