package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/config"
	"github.com/Alexander-D-Karpov/about/internal/plugins"
)

type StatsHandler struct {
	config        *config.Config
	pluginManager *plugins.Manager
	template      *template.Template
	httpClient    *http.Client
}

func NewStatsHandler(cfg *config.Config, pm *plugins.Manager, tmpl *template.Template) *StatsHandler {
	return &StatsHandler{
		config:        cfg,
		pluginManager: pm,
		template:      tmpl,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *StatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/stats":
		h.serveUI(w, r)
	case "/api/stats/data":
		h.serveData(w, r)
	case "/api/stats/health":
		h.serveHealth(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *StatsHandler) serveUI(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "System Statistics",
	}
	if err := h.template.Execute(w, data); err != nil {
		log.Printf("Stats template error: %v", err)
		http.Error(w, "Internal Error", 500)
	}
}

func (h *StatsHandler) serveHealth(w http.ResponseWriter, r *http.Request) {
	promClient := h.pluginManager.GetPrometheusClient()

	health := map[string]interface{}{
		"status":           "ok",
		"prometheus_push":  promClient.IsEnabled(),
		"prometheus_query": h.config.PrometheusQueryURL != "",
		"last_push":        promClient.GetLastPushTime().Unix(),
		"providers":        promClient.GetProviderNames(),
		"provider_count":   promClient.GetProviderCount(),
	}

	if err := promClient.GetLastError(); err != nil {
		health["last_error"] = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (h *StatsHandler) serveData(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	if h.config.PrometheusQueryURL != "" {
		data, err := h.fetchPrometheusData()
		if err != nil {
			log.Printf("[Stats] Prometheus query failed, falling back to in-memory: %v", err)
			h.serveInMemoryData(w)
			return
		}
		json.NewEncoder(w).Encode(data)
		return
	}

	h.serveInMemoryData(w)
}

func (h *StatsHandler) fetchPrometheusData() (map[string]interface{}, error) {
	now := time.Now()
	start := now.Add(-24 * time.Hour)
	step := 30 * time.Minute

	targets := []string{
		"about_visitors_total_visits",
		"about_visitors_today_visits",
		"about_health_heart_rate_current",
		"about_health_heart_rate_avg",
		"about_health_steps_today",
		"about_health_calories_today",
		"about_health_sleep_hours",
		"about_health_active_minutes",
		"about_services_online_count",
		"about_services_offline_count",
		"about_services_total_count",
		"about_services_avg_response_time_ms",
		"about_code_github_repos",
		"about_code_github_stars",
		"about_code_github_commits",
		"about_code_github_followers",
		"about_code_wakatime_hours_7d",
		"about_code_wakatime_hours_total",
		"about_steam_is_online",
		"about_steam_is_playing",
		"about_steam_recent_games_count",
		"about_steam_total_playtime_hours",
		"about_music_nowplaying",
		"about_music_scrobbles",
		"about_music_loved",
		"about_music_spotifyliked",
		"about_webring_status_ok",
		"about_webring_sites_count",
		"about_neofetch_machines_count",
		"about_photos_folders_total",
		"about_photos_images_total",
		"about_places_total_places",
		"about_places_countries_count",
		"about_places_cities_count",
		"about_beatleader_rank",
		"about_beatleader_pp",
		"about_beatleader_play_count",
		"about_meme_total_memes",
		"about_social_total_links",
		"about_techstack_total_technologies",
		"about_projects_total_projects",
		"about_personal_info_items_count",
	}

	result := make(map[string]interface{})
	successCount := 0

	for _, metric := range targets {
		data, err := h.queryPrometheusRange(metric, start, now, step)
		if err == nil && data != nil {
			result[metric] = data
			successCount++
		}
	}

	if successCount == 0 {
		return nil, fmt.Errorf("no metrics retrieved from prometheus")
	}

	log.Printf("[Stats] Retrieved %d/%d metrics from Prometheus", successCount, len(targets))
	return result, nil
}

func (h *StatsHandler) serveInMemoryData(w http.ResponseWriter) {
	timestamp := float64(time.Now().Unix())
	result := make(map[string]interface{})

	promClient := h.pluginManager.GetPrometheusClient()
	cachedMetrics := promClient.GetCachedMetrics()

	if len(cachedMetrics) > 0 {
		for metricName, value := range cachedMetrics {
			result[metricName] = [][]interface{}{{timestamp, fmt.Sprintf("%v", value)}}
		}
		log.Printf("[Stats] Serving %d cached metrics", len(result))
		json.NewEncoder(w).Encode(result)
		return
	}

	pluginMetrics := []string{
		"visitors", "health", "services", "code", "steam",
		"music", "webring", "neofetch", "photos", "places",
		"beatleader", "meme", "social", "techstack", "projects", "personal",
	}

	for _, pluginName := range pluginMetrics {
		if p, ok := h.pluginManager.GetPlugin(pluginName); ok {
			if m, ok := p.(interface{ GetMetrics() map[string]interface{} }); ok {
				metrics := m.GetMetrics()
				for key, value := range metrics {
					metricName := fmt.Sprintf("about_%s_%s", pluginName, key)
					result[metricName] = [][]interface{}{{timestamp, fmt.Sprintf("%v", value)}}
				}
			}
		}
	}

	log.Printf("[Stats] Serving %d in-memory metrics", len(result))
	json.NewEncoder(w).Encode(result)
}

type promQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func (h *StatsHandler) queryPrometheusRange(query string, start, end time.Time, step time.Duration) (interface{}, error) {
	u := fmt.Sprintf("%s/api/v1/query_range", h.config.PrometheusQueryURL)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("query", query)
	q.Add("start", fmt.Sprintf("%d", start.Unix()))
	q.Add("end", fmt.Sprintf("%d", end.Unix()))
	q.Add("step", fmt.Sprintf("%d", int(step.Seconds())))
	req.URL.RawQuery = q.Encode()

	if h.config.PrometheusUser != "" {
		req.SetBasicAuth(h.config.PrometheusUser, h.config.PrometheusPassword)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("prometheus api status: %d", resp.StatusCode)
	}

	var apiResp promQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	if apiResp.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s", apiResp.Status)
	}

	if len(apiResp.Data.Result) > 0 {
		return apiResp.Data.Result[0].Values, nil
	}
	return nil, nil
}
