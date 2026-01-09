package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
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
	WorkoutMinutesToday int64        `json:"workout_minutes_today"`
	WorkoutMinutesWeek  int64        `json:"workout_minutes_week"`
	WorkoutCountToday   int          `json:"workout_count_today"`
	WorkoutCountWeek    int          `json:"workout_count_week"`
	CurrentHeartRate    int          `json:"current_heart_rate"`
	RestingHeartRate    int          `json:"resting_heart_rate"`
	SleepHoursLastNight float64      `json:"sleep_hours_last_night"`
	SleepAvgWeek        float64      `json:"sleep_avg_week"`
	DistanceToday       float64      `json:"distance_today"`
	DistanceWeek        float64      `json:"distance_week"`
	HydrationToday      float64      `json:"hydration_today"`
	LastWorkout         *WorkoutInfo `json:"last_workout"`
	LastUpdated         time.Time    `json:"last_updated"`
	LastHRUpdate        time.Time    `json:"last_hr_update"`
}

type WorkoutInfo struct {
	Type     string    `json:"type"`
	Duration int64     `json:"duration"`
	Calories float64   `json:"calories"`
	Date     time.Time `json:"date"`
}

type DailyAverage struct {
	Date           string  `json:"date"`
	Steps          int64   `json:"steps"`
	Calories       float64 `json:"calories"`
	SleepHours     float64 `json:"sleep_hours"`
	HydrationML    float64 `json:"hydration_ml"`
	WorkoutMinutes int64   `json:"workout_minutes"`
	DistanceKM     float64 `json:"distance_km"`
}

type IncomingHealthRecord struct {
	ID        string                 `json:"id,omitempty"`
	StartTime string                 `json:"startTime,omitempty"`
	EndTime   string                 `json:"endTime,omitempty"`
	Time      string                 `json:"time,omitempty"`
	Data      map[string]interface{} `json:"-"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

func (r *IncomingHealthRecord) UnmarshalJSON(data []byte) error {
	type Alias IncomingHealthRecord
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	var rawMap map[string]interface{}
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}
	r.Data = rawMap
	return nil
}

type SleepRecord struct {
	ID        string
	StartTime time.Time
	EndTime   time.Time
	Hours     float64
}

type HealthPlugin struct {
	storage       *storage.Storage
	hub           *stream.Hub
	username      string
	password      string
	healthData    *HealthData
	dailyAverages map[string]*DailyAverage
	todayRawData  map[string][]map[string]interface{}
	mutex         sync.RWMutex
	lastPersist   time.Time

	processedRecords map[string]map[string]struct{}
	sleepRecords     map[string]*SleepRecord
	lastResetDate    string
}

func NewHealthPlugin(storage *storage.Storage, hub *stream.Hub, _, username, password string) *HealthPlugin {
	p := &HealthPlugin{
		storage:          storage,
		hub:              hub,
		username:         username,
		password:         password,
		healthData:       &HealthData{},
		dailyAverages:    make(map[string]*DailyAverage),
		todayRawData:     make(map[string][]map[string]interface{}),
		processedRecords: make(map[string]map[string]struct{}),
		sleepRecords:     make(map[string]*SleepRecord),
		lastResetDate:    time.Now().Format("2006-01-02"),
	}
	p.loadPersistedData()
	return p
}

func (p *HealthPlugin) Name() string { return "health" }

func (p *HealthPlugin) ValidateAuth(username, password string) bool {
	return p.username != "" && p.password != "" &&
		username == p.username && password == p.password
}

func (p *HealthPlugin) HandleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, password, ok := r.BasicAuth()
	if !ok || !p.ValidateAuth(username, password) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/health/sync/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		http.Error(w, "Missing data type", http.StatusBadRequest)
		return
	}
	dataType := strings.ToLower(pathParts[0])

	var payload struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var records []map[string]interface{}
	if err := json.Unmarshal(payload.Data, &records); err != nil {
		var single map[string]interface{}
		if err2 := json.Unmarshal(payload.Data, &single); err2 != nil {
			http.Error(w, "Invalid data format", http.StatusBadRequest)
			return
		}
		records = []map[string]interface{}{single}
	}

	p.processRecords(dataType, records)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   len(records),
	})
}

func (p *HealthPlugin) HandleStatus(w http.ResponseWriter, r *http.Request) {
	p.mutex.RLock()
	data := p.healthData
	p.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (p *HealthPlugin) getRecordID(dataType string, record map[string]interface{}) string {
	if metadata, ok := record["metadata"].(map[string]interface{}); ok {
		if id, ok := metadata["id"].(string); ok && id != "" {
			return id
		}
	}

	if id, ok := record["id"].(string); ok && id != "" {
		return id
	}

	var startStr, endStr string
	if v, ok := record["startTime"].(string); ok {
		startStr = v
	} else if v, ok := record["time"].(string); ok {
		startStr = v
	}
	if v, ok := record["endTime"].(string); ok {
		endStr = v
	}

	if startStr != "" {
		return fmt.Sprintf("%s|%s|%s", dataType, startStr, endStr)
	}

	return ""
}

func (p *HealthPlugin) isRecordProcessed(dataType string, recordID string) bool {
	if recordID == "" {
		return false
	}
	if p.processedRecords[dataType] == nil {
		return false
	}
	_, exists := p.processedRecords[dataType][recordID]
	return exists
}

func (p *HealthPlugin) markRecordProcessed(dataType string, recordID string) {
	if recordID == "" {
		return
	}
	if p.processedRecords[dataType] == nil {
		p.processedRecords[dataType] = make(map[string]struct{})
	}
	p.processedRecords[dataType][recordID] = struct{}{}
}

func (p *HealthPlugin) checkAndResetDaily() {
	today := time.Now().Format("2006-01-02")
	if p.lastResetDate != today {
		p.ResetDailyData()
		p.lastResetDate = today
	}
}

func (p *HealthPlugin) processRecords(dataType string, records []map[string]interface{}) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.checkAndResetDaily()

	now := time.Now()
	today := now.Format("2006-01-02")
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfWeek := startOfDay.AddDate(0, 0, -int(now.Weekday()))

	for _, record := range records {
		recordID := p.getRecordID(dataType, record)

		if dataType != "heartrate" && p.isRecordProcessed(dataType, recordID) {
			continue
		}

		recordTime := p.parseRecordTime(record)
		if recordTime.IsZero() {
			recordTime = now
		}

		isToday := recordTime.After(startOfDay) || recordTime.Equal(startOfDay)
		isThisWeek := recordTime.After(startOfWeek) || recordTime.Equal(startOfWeek)

		switch dataType {
		case "steps":
			count := p.extractInt(record, "count")
			if count > 0 {
				p.markRecordProcessed(dataType, recordID)
				if isToday {
					p.healthData.StepsToday += count
				}
				if isThisWeek {
					p.healthData.StepsWeek += count
				}
			}

		case "activecaloriesburned", "totalcaloriesburned":
			calories := p.extractCalories(record)
			if calories > 0 {
				p.markRecordProcessed(dataType, recordID)
				if isToday {
					p.healthData.CaloriesToday += calories
				}
				if isThisWeek {
					p.healthData.CaloriesWeek += calories
				}
			}

		case "heartrate":
			bpm := p.extractHeartRate(record)
			if bpm > 0 {
				p.healthData.CurrentHeartRate = bpm
				p.healthData.LastHRUpdate = now
				if bpm < p.healthData.RestingHeartRate || p.healthData.RestingHeartRate == 0 {
					if bpm > 40 {
						p.healthData.RestingHeartRate = bpm
					}
				}
			}

		case "restingheartrate":
			bpm := p.extractHeartRate(record)
			if bpm > 0 {
				p.healthData.RestingHeartRate = bpm
			}

		case "sleepsession":
			hours := p.extractSleepHours(record)
			if hours > 0 {
				canonicalID := p.getCanonicalSleepID(record)
				if canonicalID == "" {
					continue
				}
				if _, exists := p.sleepRecords[canonicalID]; !exists {
					endTime := p.parseRecordEndTime(record)
					if endTime.IsZero() {
						endTime = recordTime.Add(time.Duration(hours * float64(time.Hour)))
					}

					p.sleepRecords[canonicalID] = &SleepRecord{
						ID:        canonicalID,
						StartTime: recordTime,
						EndTime:   endTime,
						Hours:     hours,
					}

					p.recalculateSleep(startOfDay)
				}
			}

		case "exercisesession":
			p.markRecordProcessed(dataType, recordID)
			duration, calories, exerciseType := p.extractWorkout(record)
			if isToday {
				p.healthData.WorkoutMinutesToday += duration
				p.healthData.WorkoutCountToday++
			}
			if isThisWeek {
				p.healthData.WorkoutMinutesWeek += duration
				p.healthData.WorkoutCountWeek++
			}
			if p.healthData.LastWorkout == nil || recordTime.After(p.healthData.LastWorkout.Date) {
				p.healthData.LastWorkout = &WorkoutInfo{
					Type:     exerciseType,
					Duration: duration,
					Calories: calories,
					Date:     recordTime,
				}
			}

		case "hydration":
			ml := p.extractHydration(record)
			if ml > 0 {
				p.markRecordProcessed(dataType, recordID)
				if isToday {
					p.healthData.HydrationToday += ml
				}
			}

		case "distance":
			km := p.extractDistance(record)
			if km > 0 {
				p.markRecordProcessed(dataType, recordID)
				if isToday {
					p.healthData.DistanceToday += km
				}
				if isThisWeek {
					p.healthData.DistanceWeek += km
				}
			}
		}
	}

	p.healthData.LastUpdated = now

	if _, exists := p.dailyAverages[today]; !exists {
		p.dailyAverages[today] = &DailyAverage{Date: today}
	}
	p.dailyAverages[today].Steps = p.healthData.StepsToday
	p.dailyAverages[today].Calories = p.healthData.CaloriesToday
	p.dailyAverages[today].SleepHours = p.healthData.SleepHoursLastNight
	p.dailyAverages[today].HydrationML = p.healthData.HydrationToday
	p.dailyAverages[today].WorkoutMinutes = p.healthData.WorkoutMinutesToday
	p.dailyAverages[today].DistanceKM = p.healthData.DistanceToday

	if time.Since(p.lastPersist) > 5*time.Minute {
		go p.persistData()
	}
}

func (p *HealthPlugin) deduplicateSleepRecords() {
	seen := make(map[string]*SleepRecord)
	for id, sr := range p.sleepRecords {
		canonicalKey := fmt.Sprintf("%s|%s", sr.StartTime.Format(time.RFC3339), sr.EndTime.Format(time.RFC3339))
		if existing, exists := seen[canonicalKey]; exists {
			delete(p.sleepRecords, id)
			if existing.ID != id {
				delete(p.sleepRecords, existing.ID)
			}
			seen[canonicalKey].ID = canonicalKey
			p.sleepRecords[canonicalKey] = seen[canonicalKey]
		} else {
			seen[canonicalKey] = sr
		}
	}
}

func (p *HealthPlugin) recalculateSleep(startOfDay time.Time) {
	now := time.Now()
	todayNoon := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	yesterdayEvening := time.Date(now.Year(), now.Month(), now.Day()-1, 18, 0, 0, 0, now.Location())

	var totalSleepHours float64
	var weekSleepHours float64
	var weekSleepCount int

	startOfWeek := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -int(now.Weekday()))

	for _, sr := range p.sleepRecords {
		endLocal := sr.EndTime.In(now.Location())

		if endLocal.After(yesterdayEvening) && endLocal.Before(todayNoon) {
			totalSleepHours += sr.Hours
		}

		if sr.EndTime.After(startOfWeek) {
			weekSleepHours += sr.Hours
			weekSleepCount++
		}
	}

	p.healthData.SleepHoursLastNight = totalSleepHours

	if weekSleepCount > 0 {
		p.healthData.SleepAvgWeek = weekSleepHours / float64(weekSleepCount)
	}
}

func (p *HealthPlugin) parseRecordTime(record map[string]interface{}) time.Time {
	for _, key := range []string{"startTime", "time", "start"} {
		if v, ok := record[key].(string); ok && v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t
			}
			if t, err := time.Parse("2006-01-02T15:04:05", v); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func (p *HealthPlugin) parseRecordEndTime(record map[string]interface{}) time.Time {
	for _, key := range []string{"endTime", "end"} {
		if v, ok := record[key].(string); ok && v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t
			}
			if t, err := time.Parse("2006-01-02T15:04:05", v); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func (p *HealthPlugin) extractInt(record map[string]interface{}, key string) int64 {
	if v, ok := record[key].(float64); ok {
		return int64(v)
	}
	if v, ok := record[key].(int); ok {
		return int64(v)
	}
	return 0
}

func (p *HealthPlugin) extractCalories(record map[string]interface{}) float64 {
	if energy, ok := record["energy"].(map[string]interface{}); ok {
		if kcal, ok := energy["inKilocalories"].(float64); ok {
			return kcal
		}
	}
	if kcal, ok := record["calories"].(float64); ok {
		return kcal
	}
	return 0
}

func (p *HealthPlugin) extractHeartRate(record map[string]interface{}) int {
	if bpm, ok := record["bpm"].(float64); ok {
		return int(bpm)
	}
	if bpm, ok := record["beatsPerMinute"].(float64); ok {
		return int(bpm)
	}
	if samples, ok := record["samples"].([]interface{}); ok && len(samples) > 0 {
		if sample, ok := samples[0].(map[string]interface{}); ok {
			if bpm, ok := sample["beatsPerMinute"].(float64); ok {
				return int(bpm)
			}
		}
	}
	return 0
}

func (p *HealthPlugin) extractSleepHours(record map[string]interface{}) float64 {
	startStr, _ := record["startTime"].(string)
	endStr, _ := record["endTime"].(string)
	if startStr == "" || endStr == "" {
		return 0
	}
	start, err1 := time.Parse(time.RFC3339, startStr)
	end, err2 := time.Parse(time.RFC3339, endStr)
	if err1 != nil || err2 != nil {
		return 0
	}
	return end.Sub(start).Hours()
}

func (p *HealthPlugin) extractWorkout(record map[string]interface{}) (int64, float64, string) {
	var duration int64
	var calories float64
	exerciseType := "Workout"

	startStr, _ := record["startTime"].(string)
	endStr, _ := record["endTime"].(string)
	if startStr != "" && endStr != "" {
		start, _ := time.Parse(time.RFC3339, startStr)
		end, _ := time.Parse(time.RFC3339, endStr)
		if !start.IsZero() && !end.IsZero() {
			duration = int64(end.Sub(start).Minutes())
		}
	}

	if energy, ok := record["energy"].(map[string]interface{}); ok {
		if kcal, ok := energy["inKilocalories"].(float64); ok {
			calories = kcal
		}
	} else if kcal, ok := record["calories"].(float64); ok {
		calories = kcal
	}

	if t, ok := record["exerciseType"].(string); ok {
		exerciseType = formatExerciseType(t)
	} else if t, ok := record["exerciseType"].(float64); ok {
		exerciseType = fmt.Sprintf("Exercise %d", int(t))
	}

	return duration, calories, exerciseType
}

func (p *HealthPlugin) extractHydration(record map[string]interface{}) float64 {
	if volume, ok := record["volume"].(map[string]interface{}); ok {
		if ml, ok := volume["inMilliliters"].(float64); ok {
			return ml
		}
		if l, ok := volume["inLiters"].(float64); ok {
			return l * 1000
		}
	}
	if ml, ok := record["volume"].(float64); ok {
		return ml
	}
	return 0
}

func (p *HealthPlugin) extractDistance(record map[string]interface{}) float64 {
	if distance, ok := record["distance"].(map[string]interface{}); ok {
		if m, ok := distance["inMeters"].(float64); ok {
			return m / 1000
		}
		if km, ok := distance["inKilometers"].(float64); ok {
			return km
		}
	}
	if m, ok := record["distance"].(float64); ok {
		return m / 1000
	}
	return 0
}

func (p *HealthPlugin) BroadcastHeartRate() {
	p.mutex.RLock()
	hr := p.healthData.CurrentHeartRate
	lastUpdate := p.healthData.LastHRUpdate
	p.mutex.RUnlock()

	if hr > 0 {
		p.hub.Broadcast("heartrate_update", map[string]interface{}{
			"bpm":         hr,
			"last_update": lastUpdate.Unix(),
			"timestamp":   time.Now().Unix(),
		})
	}
}

func (p *HealthPlugin) ResetDailyData() {
	today := time.Now().Format("2006-01-02")

	if avg, exists := p.dailyAverages[today]; !exists || avg.Steps == 0 {
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		if _, exists := p.dailyAverages[yesterday]; !exists {
			p.dailyAverages[yesterday] = &DailyAverage{
				Date:           yesterday,
				Steps:          p.healthData.StepsToday,
				Calories:       p.healthData.CaloriesToday,
				SleepHours:     p.healthData.SleepHoursLastNight,
				HydrationML:    p.healthData.HydrationToday,
				WorkoutMinutes: p.healthData.WorkoutMinutesToday,
				DistanceKM:     p.healthData.DistanceToday,
			}
		}
	}

	p.healthData.StepsToday = 0
	p.healthData.CaloriesToday = 0
	p.healthData.WorkoutMinutesToday = 0
	p.healthData.WorkoutCountToday = 0
	p.healthData.HydrationToday = 0
	p.healthData.DistanceToday = 0
	p.healthData.SleepHoursLastNight = 0

	p.processedRecords = make(map[string]map[string]struct{})

	p.cleanOldSleepRecords()

	p.recalculateWeeklyData()
	p.cleanOldData()
	p.persistData()

	p.lastResetDate = today
}

func (p *HealthPlugin) cleanOldSleepRecords() {
	cutoff := time.Now().AddDate(0, 0, -8)
	for id, sr := range p.sleepRecords {
		if sr.EndTime.Before(cutoff) {
			delete(p.sleepRecords, id)
		}
	}
}

func (p *HealthPlugin) recalculateWeeklyData() {
	now := time.Now()
	startOfWeek := now.AddDate(0, 0, -int(now.Weekday()))

	var stepsWeek int64
	var caloriesWeek float64
	var workoutMinutesWeek int64
	var workoutCountWeek int
	var distanceWeek float64
	var sleepTotal float64
	var sleepCount int

	for dateStr, avg := range p.dailyAverages {
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		if date.After(startOfWeek) || date.Equal(startOfWeek) {
			stepsWeek += avg.Steps
			caloriesWeek += avg.Calories
			workoutMinutesWeek += avg.WorkoutMinutes
			distanceWeek += avg.DistanceKM
			if avg.SleepHours > 0 {
				sleepTotal += avg.SleepHours
				sleepCount++
			}
		}
	}

	p.healthData.StepsWeek = stepsWeek
	p.healthData.CaloriesWeek = caloriesWeek
	p.healthData.WorkoutMinutesWeek = workoutMinutesWeek
	p.healthData.WorkoutCountWeek = workoutCountWeek
	p.healthData.DistanceWeek = distanceWeek
	if sleepCount > 0 {
		p.healthData.SleepAvgWeek = sleepTotal / float64(sleepCount)
	}
}

func (p *HealthPlugin) cleanOldData() {
	cutoff := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	for dateStr := range p.dailyAverages {
		if dateStr < cutoff {
			delete(p.dailyAverages, dateStr)
		}
	}
}

func (p *HealthPlugin) persistData() {
	config := p.storage.GetPluginConfig(p.Name())
	if config.Settings == nil {
		config.Settings = make(map[string]interface{})
	}

	avgData := make(map[string]interface{})
	for date, avg := range p.dailyAverages {
		avgData[date] = map[string]interface{}{
			"steps":           avg.Steps,
			"calories":        avg.Calories,
			"sleep_hours":     avg.SleepHours,
			"hydration_ml":    avg.HydrationML,
			"workout_minutes": avg.WorkoutMinutes,
			"distance_km":     avg.DistanceKM,
		}
	}
	config.Settings["daily_averages"] = avgData

	config.Settings["current_data"] = map[string]interface{}{
		"steps_today":           p.healthData.StepsToday,
		"steps_week":            p.healthData.StepsWeek,
		"calories_today":        p.healthData.CaloriesToday,
		"calories_week":         p.healthData.CaloriesWeek,
		"workout_minutes_today": p.healthData.WorkoutMinutesToday,
		"workout_minutes_week":  p.healthData.WorkoutMinutesWeek,
		"workout_count_today":   p.healthData.WorkoutCountToday,
		"workout_count_week":    p.healthData.WorkoutCountWeek,
		"current_heart_rate":    p.healthData.CurrentHeartRate,
		"resting_heart_rate":    p.healthData.RestingHeartRate,
		"sleep_hours_last":      p.healthData.SleepHoursLastNight,
		"sleep_avg_week":        p.healthData.SleepAvgWeek,
		"distance_today":        p.healthData.DistanceToday,
		"distance_week":         p.healthData.DistanceWeek,
		"hydration_today":       p.healthData.HydrationToday,
		"last_updated":          p.healthData.LastUpdated.Format(time.RFC3339),
		"last_hr_update":        p.healthData.LastHRUpdate.Format(time.RFC3339),
	}

	if p.healthData.LastWorkout != nil {
		config.Settings["last_workout"] = map[string]interface{}{
			"type":     p.healthData.LastWorkout.Type,
			"duration": p.healthData.LastWorkout.Duration,
			"calories": p.healthData.LastWorkout.Calories,
			"date":     p.healthData.LastWorkout.Date.Format(time.RFC3339),
		}
	}

	sleepData := make(map[string]interface{})
	for id, sr := range p.sleepRecords {
		sleepData[id] = map[string]interface{}{
			"start_time": sr.StartTime.Format(time.RFC3339),
			"end_time":   sr.EndTime.Format(time.RFC3339),
			"hours":      sr.Hours,
		}
	}
	config.Settings["sleep_records"] = sleepData

	config.Settings["last_reset_date"] = p.lastResetDate
	config.Settings["last_persist"] = time.Now().Format(time.RFC3339)

	if err := p.storage.SetPluginConfig(p.Name(), config); err != nil {
		log.Printf("Failed to persist health data: %v", err)
	} else {
		log.Printf("Health data persisted successfully")
	}

	p.lastPersist = time.Now()
}

func (p *HealthPlugin) loadPersistedData() {
	config := p.storage.GetPluginConfig(p.Name())
	if config.Settings == nil {
		return
	}

	if avgData, ok := config.Settings["daily_averages"].(map[string]interface{}); ok {
		for date, data := range avgData {
			if dayData, ok := data.(map[string]interface{}); ok {
				p.dailyAverages[date] = &DailyAverage{
					Date:           date,
					Steps:          int64(getFloat(dayData, "steps")),
					Calories:       getFloat(dayData, "calories"),
					SleepHours:     getFloat(dayData, "sleep_hours"),
					HydrationML:    getFloat(dayData, "hydration_ml"),
					WorkoutMinutes: int64(getFloat(dayData, "workout_minutes")),
					DistanceKM:     getFloat(dayData, "distance_km"),
				}
			}
		}
	}

	if sleepData, ok := config.Settings["sleep_records"].(map[string]interface{}); ok {
		for id, data := range sleepData {
			if srData, ok := data.(map[string]interface{}); ok {
				sr := &SleepRecord{
					ID:    id,
					Hours: getFloat(srData, "hours"),
				}
				if startStr, ok := srData["start_time"].(string); ok {
					if t, err := time.Parse(time.RFC3339, startStr); err == nil {
						sr.StartTime = t
					}
				}
				if endStr, ok := srData["end_time"].(string); ok {
					if t, err := time.Parse(time.RFC3339, endStr); err == nil {
						sr.EndTime = t
					}
				}
				p.sleepRecords[id] = sr
			}
		}
	}

	if resetDate, ok := config.Settings["last_reset_date"].(string); ok {
		p.lastResetDate = resetDate
	}

	if currentData, ok := config.Settings["current_data"].(map[string]interface{}); ok {
		p.healthData.StepsToday = int64(getFloat(currentData, "steps_today"))
		p.healthData.StepsWeek = int64(getFloat(currentData, "steps_week"))
		p.healthData.CaloriesToday = getFloat(currentData, "calories_today")
		p.healthData.CaloriesWeek = getFloat(currentData, "calories_week")
		p.healthData.WorkoutMinutesToday = int64(getFloat(currentData, "workout_minutes_today"))
		p.healthData.WorkoutMinutesWeek = int64(getFloat(currentData, "workout_minutes_week"))
		p.healthData.WorkoutCountToday = int(getFloat(currentData, "workout_count_today"))
		p.healthData.WorkoutCountWeek = int(getFloat(currentData, "workout_count_week"))
		p.healthData.CurrentHeartRate = int(getFloat(currentData, "current_heart_rate"))
		p.healthData.RestingHeartRate = int(getFloat(currentData, "resting_heart_rate"))
		p.healthData.SleepHoursLastNight = getFloat(currentData, "sleep_hours_last")
		p.healthData.SleepAvgWeek = getFloat(currentData, "sleep_avg_week")
		p.healthData.DistanceToday = getFloat(currentData, "distance_today")
		p.healthData.DistanceWeek = getFloat(currentData, "distance_week")
		p.healthData.HydrationToday = getFloat(currentData, "hydration_today")

		if lastUpdated, ok := currentData["last_updated"].(string); ok {
			if t, err := time.Parse(time.RFC3339, lastUpdated); err == nil {
				p.healthData.LastUpdated = t
			}
		}
		if lastHR, ok := currentData["last_hr_update"].(string); ok {
			if t, err := time.Parse(time.RFC3339, lastHR); err == nil {
				p.healthData.LastHRUpdate = t
			}
		}
	}

	if workoutData, ok := config.Settings["last_workout"].(map[string]interface{}); ok {
		workout := &WorkoutInfo{
			Type:     getStringC(workoutData, "type"),
			Duration: int64(getFloat(workoutData, "duration")),
			Calories: getFloat(workoutData, "calories"),
		}
		if dateStr, ok := workoutData["date"].(string); ok {
			if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
				workout.Date = t
			}
		}
		p.healthData.LastWorkout = workout
	}

	today := time.Now().Format("2006-01-02")
	if p.lastResetDate != today {
		p.healthData.StepsToday = 0
		p.healthData.CaloriesToday = 0
		p.healthData.WorkoutMinutesToday = 0
		p.healthData.WorkoutCountToday = 0
		p.healthData.HydrationToday = 0
		p.healthData.DistanceToday = 0
		p.healthData.SleepHoursLastNight = 0
		p.lastResetDate = today
	}

	p.deduplicateSleepRecords()
	p.recalculateWeeklyData()

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	p.recalculateSleep(startOfDay)

	log.Printf("Health data loaded: steps=%d, hr=%d, sleep=%.1fh",
		p.healthData.StepsToday, p.healthData.CurrentHeartRate, p.healthData.SleepHoursLastNight)
}

func getStringC(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

func (p *HealthPlugin) Render(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "Health")
	showSteps := p.getConfigBool(settings, "ui.showSteps", true)
	showCalories := p.getConfigBool(settings, "ui.showCalories", true)
	showWorkouts := p.getConfigBool(settings, "ui.showWorkouts", true)
	showSleep := p.getConfigBool(settings, "ui.showSleep", true)
	showHeartRate := p.getConfigBool(settings, "ui.showHeartRate", true)
	showHydration := p.getConfigBool(settings, "ui.showHydration", true)

	p.mutex.RLock()
	data := p.healthData
	p.mutex.RUnlock()

	if data == nil {
		data = &HealthData{}
	}

	if p.username == "" {
		return p.renderNoConfig(sectionTitle), nil
	}

	lastUpdatedText := "never"
	if !data.LastUpdated.IsZero() {
		lastUpdatedText = p.formatTimeAgo(data.LastUpdated)
	}

	hrUpdateText := "no data"
	if !data.LastHRUpdate.IsZero() {
		hrUpdateText = p.formatTimeAgo(data.LastHRUpdate)
	}

	tmpl := `
<section class="health-section section plugin" data-w="1" data-plugin="health">
	<header class="plugin-header">
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
		<div class="health-updated">
			<span class="update-time" data-health-updated>{{.LastUpdatedText}}</span>
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
					<svg class="health-heartbeat" data-heartbeat viewBox="0 0 24 24" fill="currentColor" data-bpm="{{.CurrentBPM}}">
						<path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"/>
					</svg>
				</div>
				<div class="health-card-content">
					<div class="health-card-value" data-metric="heart-rate">{{.CurrentHeartRate}}</div>
					<div class="health-card-label">Current HR</div>
					<div class="health-card-sub" data-metric="hr-update">{{.HRUpdateText}}</div>
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
	</div>
</section>`

	stepsToday := formatNumberCommas(data.StepsToday)
	stepsWeek := formatNumberCommas(data.StepsWeek)
	caloriesToday := fmt.Sprintf("%.0f", data.CaloriesToday)
	caloriesWeek := fmt.Sprintf("%.0f", data.CaloriesWeek)
	workoutMinutes := fmt.Sprintf("%d min", data.WorkoutMinutesWeek)
	sleepLastNight := fmt.Sprintf("%.1fh", data.SleepHoursLastNight)
	sleepAvg := fmt.Sprintf("%.1fh", data.SleepAvgWeek)
	currentHeartRate := fmt.Sprintf("%d bpm", data.CurrentHeartRate)
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
		CurrentHeartRate string
		CurrentBPM       int
		HRUpdateText     string
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
		CurrentHeartRate: currentHeartRate,
		CurrentBPM:       data.CurrentHeartRate,
		HRUpdateText:     hrUpdateText,
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
	return fmt.Sprintf(`<section class="health-section section plugin" data-w="1">
		<header class="plugin-header">
			<h3 class="plugin-title">%s</h3>
		</header>
		<div class="plugin__inner">
			<div class="health-no-config">
				<svg class="no-config-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
					<path d="M20.84 4.61a5.5 5.5 0 00-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 00-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 000-7.78z"/>
					<path d="M12 8v4M12 16h.01"/>
				</svg>
				<p class="no-config-text">Health sync not configured</p>
				<p class="no-config-hint">Set credentials in config to receive health data</p>
			</div>
		</div>
	</section>`, sectionTitle)
}

func (p *HealthPlugin) UpdateData(ctx context.Context) error {
	return nil
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

	return fmt.Sprintf("Health: %s steps, %.0f cal, %.1fh sleep, %d bpm",
		formatNumberCommas(data.StepsToday),
		data.CaloriesToday,
		data.SleepHoursLastNight,
		data.CurrentHeartRate), nil
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

func (p *HealthPlugin) getCanonicalSleepID(record map[string]interface{}) string {
	startStr, _ := record["startTime"].(string)
	endStr, _ := record["endTime"].(string)
	if startStr == "" || endStr == "" {
		return ""
	}
	return fmt.Sprintf("sleep|%s|%s", startStr, endStr)
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

func (p *HealthPlugin) GetCurrentHeartRate() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.healthData.CurrentHeartRate
}

func (p *HealthPlugin) GetHealthData() *HealthData {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	dataCopy := *p.healthData
	return &dataCopy
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
	words := strings.Fields(strings.ToLower(t))
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + word[1:]
		}
	}
	return strings.Join(words, " ")
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

func (p *HealthPlugin) PersistNow() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.persistData()
}
