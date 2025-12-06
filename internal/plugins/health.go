package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type HealthData struct {
	StepsToday          int64        `json:"steps_today"`
	StepsWeek           int64        `json:"steps_week"`
	CaloriesToday       float64      `json:"calories_today"`
	CaloriesWeek        float64      `json:"calories_week"`
	WorkoutMinutesWeek  int64        `json:"workout_minutes_week"`
	WorkoutCountWeek    int          `json:"workout_count_week"`
	AvgHeartRate        int          `json:"avg_heart_rate"`
	RestingHeartRate    int          `json:"resting_heart_rate"`
	SleepHoursLastNight float64      `json:"sleep_hours_last_night"`
	SleepAvgWeek        float64      `json:"sleep_avg_week"`
	DistanceToday       float64      `json:"distance_today"`
	DistanceWeek        float64      `json:"distance_week"`
	HydrationToday      float64      `json:"hydration_today"`
	LastWorkout         *WorkoutInfo `json:"last_workout"`
	WeeklyTrend         *WeeklyTrend `json:"weekly_trend"`
	LastUpdated         time.Time    `json:"last_updated"`
}

type WorkoutInfo struct {
	Type     string    `json:"type"`
	Duration int64     `json:"duration"`
	Calories float64   `json:"calories"`
	Date     time.Time `json:"date"`
}

type WeeklyTrend struct {
	StepsTrend    string `json:"steps_trend"`
	CaloriesTrend string `json:"calories_trend"`
	SleepTrend    string `json:"sleep_trend"`
}

type FlexibleTime struct {
	time.Time
}

func (ft *FlexibleTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		ft.Time = time.Time{}
		return nil
	}

	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.000000",
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	var parseErr error
	for _, format := range formats {
		t, err := time.Parse(format, s)
		if err == nil {
			ft.Time = t
			return nil
		}
		parseErr = err
	}

	return fmt.Errorf("unable to parse time %q: %w", s, parseErr)
}

type HCGatewayLoginResponse struct {
	Token   string       `json:"token"`
	Refresh string       `json:"refresh"`
	Expiry  FlexibleTime `json:"expiry"`
}

type HCGatewayRecord struct {
	ID    string                 `json:"_id"`
	Data  map[string]interface{} `json:"data"`
	Start FlexibleTime           `json:"start"`
	End   *FlexibleTime          `json:"end"`
	App   string                 `json:"app"`
}

type HealthPlugin struct {
	storage      *storage.Storage
	hub          *stream.Hub
	httpClient   *http.Client
	baseURL      string
	username     string
	password     string
	token        string
	refreshToken string
	tokenExpiry  time.Time
	healthData   *HealthData
	lastUpdate   time.Time
	mutex        sync.RWMutex
}

func NewHealthPlugin(storage *storage.Storage, hub *stream.Hub, baseURL, username, password string) *HealthPlugin {
	return &HealthPlugin{
		storage:    storage,
		hub:        hub,
		httpClient: NewHTTPClientWithTimeout(20 * time.Second),
		baseURL:    baseURL,
		username:   username,
		password:   password,
		healthData: &HealthData{},
	}
}

func (p *HealthPlugin) Name() string { return "health" }

func (p *HealthPlugin) Render(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Me Irl")
	showSteps := p.getConfigBool(settings, "ui.showSteps", true)
	showCalories := p.getConfigBool(settings, "ui.showCalories", true)
	showWorkouts := p.getConfigBool(settings, "ui.showWorkouts", true)
	showSleep := p.getConfigBool(settings, "ui.showSleep", true)
	showHeartRate := p.getConfigBool(settings, "ui.showHeartRate", true)
	showHydration := p.getConfigBool(settings, "ui.showHydration", true)

	p.mutex.RLock()
	data := p.healthData
	lastUpdated := p.lastUpdate
	p.mutex.RUnlock()

	if data == nil {
		data = &HealthData{}
	}

	apiConfigured := p.baseURL != "" && p.username != ""

	if !apiConfigured {
		return p.renderNoConfig(sectionTitle), nil
	}

	lastUpdatedText := "never"
	if !lastUpdated.IsZero() {
		lastUpdatedText = p.formatTimeAgo(lastUpdated)
	}

	tmpl := `
<section class="health-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
		<div class="health-updated">
			<span class="update-time">{{.LastUpdatedText}}</span>
		</div>
	</header>
	
	<div class="plugin__inner">
		<div class="health-grid">
			{{if .ShowSteps}}
			<div class="health-card health-card--steps">
				<div class="health-card-icon">
					<svg viewBox="0 0 24 24" fill="currentColor">
						<path d="M13.5 5.5c1.1 0 2-.9 2-2s-.9-2-2-2-2 .9-2 2 .9 2 2 2zM9.8 8.9L7 23h2.1l1.8-8 2.1 2v6h2v-7.5l-2.1-2 .6-3C14.8 12 16.8 13 19 13v-2c-1.9 0-3.5-1-4.3-2.4l-1-1.6c-.4-.6-1-1-1.7-1-.3 0-.5.1-.8.1L6 8.3V13h2V9.6l1.8-.7"/>
					</svg>
				</div>
				<div class="health-card-content">
					<div class="health-card-value" data-metric="steps-today">{{.StepsToday}}</div>
					<div class="health-card-label">Steps Today</div>
					<div class="health-card-sub">{{.StepsWeek}} this week</div>
				</div>
			</div>
			{{end}}
			
			{{if .ShowCalories}}
			<div class="health-card health-card--calories">
				<div class="health-card-icon">
					<svg viewBox="0 0 24 24" fill="currentColor">
						<path d="M13.5.67s.74 2.65.74 4.8c0 2.06-1.35 3.73-3.41 3.73-2.07 0-3.63-1.67-3.63-3.73l.03-.36C5.21 7.51 4 10.62 4 14c0 4.42 3.58 8 8 8s8-3.58 8-8C20 8.61 17.41 3.8 13.5.67zM11.71 19c-1.78 0-3.22-1.4-3.22-3.14 0-1.62 1.05-2.76 2.81-3.12 1.77-.36 3.6-1.21 4.62-2.58.39 1.29.59 2.65.59 4.04 0 2.65-2.15 4.8-4.8 4.8z"/>
					</svg>
				</div>
				<div class="health-card-content">
					<div class="health-card-value" data-metric="calories-today">{{.CaloriesToday}}</div>
					<div class="health-card-label">Calories</div>
					<div class="health-card-sub">{{.CaloriesWeek}} this week</div>
				</div>
			</div>
			{{end}}
			
			{{if .ShowWorkouts}}
			<div class="health-card health-card--workouts">
				<div class="health-card-icon">
					<svg viewBox="0 0 24 24" fill="currentColor">
						<path d="M20.57 14.86L22 13.43 20.57 12 17 15.57 8.43 7 12 3.43 10.57 2 9.14 3.43 7.71 2 5.57 4.14 4.14 2.71 2.71 4.14l1.43 1.43L2 7.71l1.43 1.43L2 10.57 3.43 12 7 8.43 15.57 17 12 20.57 13.43 22l1.43-1.43L16.29 22l2.14-2.14 1.43 1.43 1.43-1.43-1.43-1.43L22 16.29z"/>
					</svg>
				</div>
				<div class="health-card-content">
					<div class="health-card-value" data-metric="workout-minutes">{{.WorkoutMinutes}}</div>
					<div class="health-card-label">Workout</div>
					<div class="health-card-sub">{{.WorkoutCount}} sessions</div>
				</div>
			</div>
			{{end}}
			
			{{if .ShowSleep}}
			<div class="health-card health-card--sleep">
				<div class="health-card-icon">
					<svg viewBox="0 0 24 24" fill="currentColor">
						<path d="M9 2c-1.05 0-2.05.16-3 .46 4.06 1.27 7 5.06 7 9.54 0 4.48-2.94 8.27-7 9.54.95.3 1.95.46 3 .46 5.52 0 10-4.48 10-10S14.52 2 9 2z"/>
					</svg>
				</div>
				<div class="health-card-content">
					<div class="health-card-value" data-metric="sleep-hours">{{.SleepLastNight}}</div>
					<div class="health-card-label">Sleep</div>
					<div class="health-card-sub">{{.SleepAvg}} avg</div>
				</div>
			</div>
			{{end}}
			
			{{if .ShowHeartRate}}
			<div class="health-card health-card--heart">
				<div class="health-card-icon">
					<svg viewBox="0 0 24 24" fill="currentColor">
						<path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"/>
					</svg>
				</div>
				<div class="health-card-content">
					<div class="health-card-value" data-metric="heart-rate">{{.AvgHeartRate}}</div>
					<div class="health-card-label">Heart Rate</div>
					<div class="health-card-sub">{{.RestingHeartRate}} resting</div>
				</div>
			</div>
			{{end}}
			
			{{if .ShowHydration}}
			<div class="health-card health-card--hydration">
				<div class="health-card-icon">
					<svg viewBox="0 0 24 24" fill="currentColor">
						<path d="M12 2c-5.33 4.55-8 8.48-8 11.8 0 4.98 3.8 8.2 8 8.2s8-3.22 8-8.2c0-3.32-2.67-7.25-8-11.8zm0 18c-3.35 0-6-2.57-6-6.2 0-2.34 1.95-5.44 6-9.14 4.05 3.7 6 6.79 6 9.14 0 3.63-2.65 6.2-6 6.2z"/>
					</svg>
				</div>
				<div class="health-card-content">
					<div class="health-card-value" data-metric="hydration">{{.Hydration}}</div>
					<div class="health-card-label">Hydration</div>
					<div class="health-card-sub">ml today</div>
				</div>
			</div>
			{{end}}
		</div>
		
		{{if and .ShowWorkouts .LastWorkout}}
		<div class="health-last-workout">
			<div class="last-workout-header">
				<svg class="last-workout-icon" viewBox="0 0 24 24" fill="currentColor">
					<path d="M13.5 5.5c1.1 0 2-.9 2-2s-.9-2-2-2-2 .9-2 2 .9 2 2 2zM9.8 8.9L7 23h2.1l1.8-8 2.1 2v6h2v-7.5l-2.1-2 .6-3C14.8 12 16.8 13 19 13v-2c-1.9 0-3.5-1-4.3-2.4l-1-1.6c-.4-.6-1-1-1.7-1-.3 0-.5.1-.8.1L6 8.3V13h2V9.6l1.8-.7"/>
				</svg>
				<span class="last-workout-title">Last Workout</span>
			</div>
			<div class="last-workout-info">
				<span class="last-workout-type">{{.LastWorkout.Type}}</span>
				<span class="last-workout-duration">{{.LastWorkout.Duration}} min</span>
				<span class="last-workout-calories">{{.LastWorkout.Calories}} cal</span>
				<span class="last-workout-date">{{.LastWorkout.DateStr}}</span>
			</div>
		</div>
		{{end}}
	</div>
</section>`

	stepsToday := formatNumberCommas(data.StepsToday)
	stepsWeek := formatNumberCommas(data.StepsWeek)
	caloriesToday := fmt.Sprintf("%.0f", data.CaloriesToday)
	caloriesWeek := fmt.Sprintf("%.0f", data.CaloriesWeek)
	workoutMinutes := fmt.Sprintf("%d min", data.WorkoutMinutesWeek)
	sleepLastNight := fmt.Sprintf("%.1fh", data.SleepHoursLastNight)
	sleepAvg := fmt.Sprintf("%.1fh", data.SleepAvgWeek)
	avgHeartRate := fmt.Sprintf("%d bpm", data.AvgHeartRate)
	restingHeartRate := fmt.Sprintf("%d bpm", data.RestingHeartRate)
	hydration := fmt.Sprintf("%.0f", data.HydrationToday)

	var lastWorkoutData map[string]interface{}
	if data.LastWorkout != nil {
		lastWorkoutData = map[string]interface{}{
			"Type":     data.LastWorkout.Type,
			"Duration": data.LastWorkout.Duration,
			"Calories": fmt.Sprintf("%.0f", data.LastWorkout.Calories),
			"DateStr":  data.LastWorkout.Date.Format("Mon, Jan 2"),
		}
	}

	templateData := struct {
		SectionTitle     string
		ShowSteps        bool
		ShowCalories     bool
		ShowWorkouts     bool
		ShowSleep        bool
		ShowHeartRate    bool
		ShowHydration    bool
		StepsToday       string
		StepsWeek        string
		CaloriesToday    string
		CaloriesWeek     string
		WorkoutMinutes   string
		WorkoutCount     int
		SleepLastNight   string
		SleepAvg         string
		AvgHeartRate     string
		RestingHeartRate string
		Hydration        string
		LastWorkout      map[string]interface{}
		LastUpdatedText  string
	}{
		SectionTitle:     sectionTitle,
		ShowSteps:        showSteps,
		ShowCalories:     showCalories,
		ShowWorkouts:     showWorkouts,
		ShowSleep:        showSleep,
		ShowHeartRate:    showHeartRate,
		ShowHydration:    showHydration,
		StepsToday:       stepsToday,
		StepsWeek:        stepsWeek,
		CaloriesToday:    caloriesToday,
		CaloriesWeek:     caloriesWeek,
		WorkoutMinutes:   workoutMinutes,
		WorkoutCount:     data.WorkoutCountWeek,
		SleepLastNight:   sleepLastNight,
		SleepAvg:         sleepAvg,
		AvgHeartRate:     avgHeartRate,
		RestingHeartRate: restingHeartRate,
		Hydration:        hydration,
		LastWorkout:      lastWorkoutData,
		LastUpdatedText:  lastUpdatedText,
	}

	t, err := template.New("health").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := t.Execute(&buf, templateData); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (p *HealthPlugin) renderNoConfig(sectionTitle string) string {
	return fmt.Sprintf(`<section class="health-section section plugin" data-w="2">
		<header class="plugin-header">
			<h3 class="plugin-title">%s</h3>
		</header>
		<div class="plugin__inner">
			<div class="health-no-config">
				<svg class="no-config-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
					<path d="M20.84 4.61a5.5 5.5 0 00-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 00-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 000-7.78z"/>
					<path d="M12 8v4M12 16h.01"/>
				</svg>
				<p class="no-config-text">Health Connect not configured</p>
				<p class="no-config-hint">Set up HCGateway credentials in admin panel</p>
			</div>
		</div>
	</section>`, sectionTitle)
}

func (p *HealthPlugin) UpdateData(ctx context.Context) error {
	if time.Since(p.lastUpdate) < 5*time.Minute {
		return nil
	}

	if p.baseURL == "" || p.username == "" {
		return nil
	}

	if err := p.ensureAuthenticated(ctx); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	newData := &HealthData{LastUpdated: time.Now()}

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfWeek := startOfDay.AddDate(0, 0, -int(now.Weekday()))

	if steps, err := p.fetchSteps(ctx, startOfDay, now); err == nil {
		newData.StepsToday = steps
	}
	if stepsWeek, err := p.fetchSteps(ctx, startOfWeek, now); err == nil {
		newData.StepsWeek = stepsWeek
	}

	if calories, err := p.fetchCalories(ctx, startOfDay, now); err == nil {
		newData.CaloriesToday = calories
	}
	if caloriesWeek, err := p.fetchCalories(ctx, startOfWeek, now); err == nil {
		newData.CaloriesWeek = caloriesWeek
	}

	if minutes, count, lastWorkout, err := p.fetchWorkouts(ctx, startOfWeek, now); err == nil {
		newData.WorkoutMinutesWeek = minutes
		newData.WorkoutCountWeek = count
		newData.LastWorkout = lastWorkout
	}

	yesterday := startOfDay.AddDate(0, 0, -1)
	if sleepHours, err := p.fetchSleep(ctx, yesterday, startOfDay); err == nil {
		newData.SleepHoursLastNight = sleepHours
	}
	if sleepAvg, err := p.fetchSleepAverage(ctx, startOfWeek, now); err == nil {
		newData.SleepAvgWeek = sleepAvg
	}

	if avgHR, restingHR, err := p.fetchHeartRate(ctx, startOfDay, now); err == nil {
		newData.AvgHeartRate = avgHR
		newData.RestingHeartRate = restingHR
	}

	if hydration, err := p.fetchHydration(ctx, startOfDay, now); err == nil {
		newData.HydrationToday = hydration
	}

	if distance, err := p.fetchDistance(ctx, startOfDay, now); err == nil {
		newData.DistanceToday = distance
	}
	if distanceWeek, err := p.fetchDistance(ctx, startOfWeek, now); err == nil {
		newData.DistanceWeek = distanceWeek
	}

	p.mutex.Lock()
	p.healthData = newData
	p.lastUpdate = time.Now()
	p.mutex.Unlock()

	p.broadcastUpdate(newData)

	return nil
}

func (p *HealthPlugin) ensureAuthenticated(ctx context.Context) error {
	p.mutex.RLock()
	tokenValid := p.token != "" && time.Now().Before(p.tokenExpiry.Add(-5*time.Minute))
	p.mutex.RUnlock()

	if tokenValid {
		return nil
	}

	p.mutex.RLock()
	hasRefreshToken := p.refreshToken != ""
	p.mutex.RUnlock()

	if hasRefreshToken {
		if err := p.refreshAuth(ctx); err == nil {
			return nil
		}
	}

	return p.login(ctx)
}

func (p *HealthPlugin) login(ctx context.Context) error {
	loginURL := fmt.Sprintf("%s/api/v2/login", strings.TrimRight(p.baseURL, "/"))

	payload := map[string]string{
		"username": p.username,
		"password": p.password,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", loginURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var loginResp HCGatewayLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return fmt.Errorf("failed to decode login response: %w", err)
	}

	p.mutex.Lock()
	p.token = loginResp.Token
	p.refreshToken = loginResp.Refresh
	p.tokenExpiry = loginResp.Expiry.Time
	p.mutex.Unlock()

	return nil
}

func (p *HealthPlugin) refreshAuth(ctx context.Context) error {
	refreshURL := fmt.Sprintf("%s/api/v2/refresh", strings.TrimRight(p.baseURL, "/"))

	p.mutex.RLock()
	refreshToken := p.refreshToken
	p.mutex.RUnlock()

	payload := map[string]string{"refresh": refreshToken}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", refreshURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("refresh failed with status %d", resp.StatusCode)
	}

	var refreshResp HCGatewayLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&refreshResp); err != nil {
		return err
	}

	p.mutex.Lock()
	p.token = refreshResp.Token
	p.refreshToken = refreshResp.Refresh
	p.tokenExpiry = refreshResp.Expiry.Time
	p.mutex.Unlock()

	return nil
}

func (p *HealthPlugin) fetchData(ctx context.Context, method string, startTime, endTime time.Time) ([]HCGatewayRecord, error) {
	fetchURL := fmt.Sprintf("%s/api/v2/fetch/%s", strings.TrimRight(p.baseURL, "/"), method)

	query := map[string]interface{}{
		"queries": map[string]interface{}{
			"start": map[string]interface{}{
				"$gte": startTime.Format(time.RFC3339),
				"$lte": endTime.Format(time.RFC3339),
			},
		},
	}
	body, _ := json.Marshal(query)

	req, err := http.NewRequestWithContext(ctx, "POST", fetchURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	p.mutex.RLock()
	token := p.token
	p.mutex.RUnlock()

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch %s failed: %d - %s", method, resp.StatusCode, string(bodyBytes))
	}

	var records []HCGatewayRecord
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return nil, err
	}

	return records, nil
}

func (p *HealthPlugin) fetchSteps(ctx context.Context, start, end time.Time) (int64, error) {
	records, err := p.fetchData(ctx, "steps", start, end)
	if err != nil {
		return 0, err
	}

	var total int64
	for _, r := range records {
		if count, ok := r.Data["count"].(float64); ok {
			total += int64(count)
		}
	}
	return total, nil
}

func (p *HealthPlugin) fetchCalories(ctx context.Context, start, end time.Time) (float64, error) {
	records, err := p.fetchData(ctx, "activeCaloriesBurned", start, end)
	if err != nil {
		return 0, err
	}

	var total float64
	for _, r := range records {
		if energy, ok := r.Data["energy"].(map[string]interface{}); ok {
			if kcal, ok := energy["inKilocalories"].(float64); ok {
				total += kcal
			}
		}
	}
	return total, nil
}

func (p *HealthPlugin) fetchWorkouts(ctx context.Context, start, end time.Time) (int64, int, *WorkoutInfo, error) {
	records, err := p.fetchData(ctx, "exerciseSession", start, end)
	if err != nil {
		return 0, 0, nil, err
	}

	var totalMinutes int64
	var lastWorkout *WorkoutInfo

	for _, r := range records {
		if r.End != nil {
			duration := r.End.Time.Sub(r.Start.Time)
			totalMinutes += int64(duration.Minutes())
		}

		exerciseType := "Workout"
		if t, ok := r.Data["exerciseType"].(string); ok {
			exerciseType = formatExerciseType(t)
		}

		if lastWorkout == nil || r.Start.Time.After(lastWorkout.Date) {
			var calories float64
			if energy, ok := r.Data["energy"].(map[string]interface{}); ok {
				if kcal, ok := energy["inKilocalories"].(float64); ok {
					calories = kcal
				}
			}

			duration := int64(0)
			if r.End != nil {
				duration = int64(r.End.Time.Sub(r.Start.Time).Minutes())
			}

			lastWorkout = &WorkoutInfo{
				Type:     exerciseType,
				Duration: duration,
				Calories: calories,
				Date:     r.Start.Time,
			}
		}
	}

	return totalMinutes, len(records), lastWorkout, nil
}

func (p *HealthPlugin) fetchSleep(ctx context.Context, start, end time.Time) (float64, error) {
	records, err := p.fetchData(ctx, "sleepSession", start, end)
	if err != nil {
		return 0, err
	}

	var totalHours float64
	for _, r := range records {
		if r.End != nil {
			duration := r.End.Time.Sub(r.Start.Time)
			totalHours += duration.Hours()
		}
	}
	return totalHours, nil
}

func (p *HealthPlugin) fetchSleepAverage(ctx context.Context, start, end time.Time) (float64, error) {
	records, err := p.fetchData(ctx, "sleepSession", start, end)
	if err != nil {
		return 0, err
	}

	if len(records) == 0 {
		return 0, nil
	}

	var totalHours float64
	for _, r := range records {
		if r.End != nil {
			duration := r.End.Time.Sub(r.Start.Time)
			totalHours += duration.Hours()
		}
	}
	return totalHours / float64(len(records)), nil
}

func (p *HealthPlugin) fetchHeartRate(ctx context.Context, start, end time.Time) (int, int, error) {
	records, err := p.fetchData(ctx, "heartRate", start, end)
	if err != nil {
		return 0, 0, err
	}

	if len(records) == 0 {
		return 0, 0, nil
	}

	var total, count int
	var minHR int = 999
	for _, r := range records {
		if bpm, ok := r.Data["beatsPerMinute"].(float64); ok {
			total += int(bpm)
			count++
			if int(bpm) < minHR && int(bpm) > 40 {
				minHR = int(bpm)
			}
		}
	}

	avgHR := 0
	if count > 0 {
		avgHR = total / count
	}

	restingRecords, err := p.fetchData(ctx, "restingHeartRate", start, end)
	restingHR := minHR
	if err == nil && len(restingRecords) > 0 {
		for _, r := range restingRecords {
			if bpm, ok := r.Data["beatsPerMinute"].(float64); ok {
				restingHR = int(bpm)
				break
			}
		}
	}

	if restingHR == 999 {
		restingHR = 0
	}

	return avgHR, restingHR, nil
}

func (p *HealthPlugin) fetchHydration(ctx context.Context, start, end time.Time) (float64, error) {
	records, err := p.fetchData(ctx, "hydration", start, end)
	if err != nil {
		return 0, err
	}

	var total float64
	for _, r := range records {
		if volume, ok := r.Data["volume"].(map[string]interface{}); ok {
			if ml, ok := volume["inMilliliters"].(float64); ok {
				total += ml
			}
		}
	}
	return total, nil
}

func (p *HealthPlugin) fetchDistance(ctx context.Context, start, end time.Time) (float64, error) {
	records, err := p.fetchData(ctx, "distance", start, end)
	if err != nil {
		return 0, err
	}

	var total float64
	for _, r := range records {
		if distance, ok := r.Data["distance"].(map[string]interface{}); ok {
			if meters, ok := distance["inMeters"].(float64); ok {
				total += meters
			}
		}
	}
	return total / 1000, nil
}

func (p *HealthPlugin) broadcastUpdate(data *HealthData) {
	p.hub.Broadcast("health_update", map[string]interface{}{
		"steps_today":        data.StepsToday,
		"steps_week":         data.StepsWeek,
		"calories_today":     data.CaloriesToday,
		"calories_week":      data.CaloriesWeek,
		"workout_minutes":    data.WorkoutMinutesWeek,
		"workout_count":      data.WorkoutCountWeek,
		"avg_heart_rate":     data.AvgHeartRate,
		"resting_heart_rate": data.RestingHeartRate,
		"sleep_last_night":   data.SleepHoursLastNight,
		"sleep_avg":          data.SleepAvgWeek,
		"hydration_today":    data.HydrationToday,
		"distance_today":     data.DistanceToday,
		"distance_week":      data.DistanceWeek,
		"timestamp":          time.Now().Unix(),
	})
}

func (p *HealthPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *HealthPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings

	if err := p.storage.SetPluginConfig(p.Name(), config); err != nil {
		return err
	}

	p.mutex.Lock()
	p.token = ""
	p.refreshToken = ""
	p.tokenExpiry = time.Time{}
	p.mutex.Unlock()

	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(),
		"action": "settings_changed",
	})

	return nil
}

func (p *HealthPlugin) RenderText(ctx context.Context) (string, error) {
	p.mutex.RLock()
	data := p.healthData
	p.mutex.RUnlock()

	if data == nil || data.StepsToday == 0 {
		return "Health: No data available", nil
	}

	return fmt.Sprintf("Health: %s steps, %.0f cal burned, %.1fh sleep",
		formatNumberCommas(data.StepsToday),
		data.CaloriesToday,
		data.SleepHoursLastNight), nil
}

func (p *HealthPlugin) formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func (p *HealthPlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
	keys := strings.Split(key, ".")
	current := settings
	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(string); ok {
				return value
			}
			return defaultValue
		}
		if next, ok := current[k].(map[string]interface{}); ok {
			current = next
		} else {
			return defaultValue
		}
	}
	return defaultValue
}

func (p *HealthPlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
	keys := strings.Split(key, ".")
	current := settings
	for i, k := range keys {
		if i == len(keys)-1 {
			if value, ok := current[k].(bool); ok {
				return value
			}
			return defaultValue
		}
		if next, ok := current[k].(map[string]interface{}); ok {
			current = next
		} else {
			return defaultValue
		}
	}
	return defaultValue
}

func formatNumberCommas(n int64) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}
	var result []rune
	for i, r := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, r)
	}
	return string(result)
}

func formatExerciseType(t string) string {
	types := map[string]string{
		"EXERCISE_TYPE_RUNNING":           "Running",
		"EXERCISE_TYPE_WALKING":           "Walking",
		"EXERCISE_TYPE_BIKING":            "Cycling",
		"EXERCISE_TYPE_SWIMMING":          "Swimming",
		"EXERCISE_TYPE_STRENGTH_TRAINING": "Strength",
		"EXERCISE_TYPE_YOGA":              "Yoga",
		"EXERCISE_TYPE_HIKING":            "Hiking",
		"EXERCISE_TYPE_OTHER_WORKOUT":     "Workout",
	}
	if formatted, ok := types[t]; ok {
		return formatted
	}
	t = strings.TrimPrefix(t, "EXERCISE_TYPE_")
	t = strings.ReplaceAll(t, "_", " ")
	return strings.Title(strings.ToLower(t))
}
