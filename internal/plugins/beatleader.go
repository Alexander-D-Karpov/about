package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type BeatLeaderPlugin struct {
	storage             *storage.Storage
	hub                 *stream.Hub
	cacheInvalidator    func()
	playerData          *BeatLeaderPlayer
	recentScores        []BeatLeaderScore
	lastUpdate          time.Time
	cachedCubesSliced   int64
	lastCubesCalculated time.Time
	cubesCalculating    bool
	cubesMutex          sync.RWMutex
	mediaPath           string
}

func (p *BeatLeaderPlugin) SetCacheInvalidator(fn func()) {
	p.cacheInvalidator = fn
}

type PluginManagerInterface interface {
	UpdatePlugin(pluginName string) error
}

type BeatLeaderPlayer struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Avatar      string  `json:"avatar"`
	Country     string  `json:"country"`
	PP          float64 `json:"pp"`
	Rank        int     `json:"rank"`
	CountryRank int     `json:"countryRank"`
	ScoreStats  struct {
		AverageRankedAccuracy float64 `json:"averageRankedAccuracy"`
		TotalPlayCount        int     `json:"totalPlayCount"`
		RankedPlayCount       int     `json:"rankedPlayCount"`
		TotalScore            int64   `json:"totalScore"`
		TotalUnrankedScore    int64   `json:"totalUnrankedScore"`
		TopPP                 float64 `json:"topPp"`
		PeakRank              int     `json:"peakRank"`
		MaxStreak             int     `json:"maxStreak"`
		AverageAccuracy       float64 `json:"averageAccuracy"`
		MedianRankedAccuracy  float64 `json:"medianRankedAccuracy"`
		MedianAccuracy        float64 `json:"medianAccuracy"`
		TopAccuracy           float64 `json:"topAccuracy"`
		TopPlatform           string  `json:"topPlatform"`
		TopHMD                int     `json:"topHMD"`
		CountryTopPercentage  float64 `json:"countryTopPercentage"`
		LastWeekPP            float64 `json:"lastWeekPp"`
		LastWeekRank          int     `json:"lastWeekRank"`
		LastWeekCountryRank   int     `json:"lastWeekCountryRank"`
	} `json:"scoreStats"`
	Clans []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Tag  string `json:"tag"`
	} `json:"clans"`
	ProfileSettings struct {
		Bio               string `json:"bio"`
		Message           string `json:"message"`
		EffectName        string `json:"effectName"`
		ProfileAppearance string `json:"profileAppearance"`
	} `json:"profileSettings"`
}

type BeatLeaderScoresResponse struct {
	Metadata struct {
		ItemsPerPage int `json:"itemsPerPage"`
		Page         int `json:"page"`
		Total        int `json:"total"`
	} `json:"metadata"`
	Data []BeatLeaderScore `json:"data"`
}

type BeatLeaderScore struct {
	ID            int     `json:"id"`
	BaseScore     int     `json:"baseScore"`
	ModifiedScore int     `json:"modifiedScore"`
	Accuracy      float64 `json:"accuracy"`
	PP            float64 `json:"pp"`
	FcPP          float64 `json:"fcPp"`
	BonusPP       float64 `json:"bonusPp"`
	Rank          int     `json:"rank"`
	Replay        string  `json:"replay"`
	Modifiers     string  `json:"modifiers"`
	BadCuts       int     `json:"badCuts"`
	MissedNotes   int     `json:"missedNotes"`
	BombCuts      int     `json:"bombCuts"`
	WallsHit      int     `json:"wallsHit"`
	Pauses        int     `json:"pauses"`
	FullCombo     bool    `json:"fullCombo"`
	Platform      string  `json:"platform"`
	MaxCombo      int     `json:"maxCombo"`
	MaxStreak     int     `json:"maxStreak"`
	Hmd           int     `json:"hmd"`
	Controller    int     `json:"controller"`
	LeaderboardId string  `json:"leaderboardId"`
	Timeset       string  `json:"timeset"`
	Timepost      int64   `json:"timepost"`
	PlayCount     int     `json:"playCount"`
	Priority      int     `json:"priority"`
	Player        *struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Avatar      string  `json:"avatar"`
		Country     string  `json:"country"`
		Rank        int     `json:"rank"`
		CountryRank int     `json:"countryRank"`
		PP          float64 `json:"pp"`
	} `json:"player"`
	ScoreImprovement *struct {
		ID       int     `json:"id"`
		Timeset  string  `json:"timeset"`
		Score    int     `json:"score"`
		Accuracy float64 `json:"accuracy"`
		PP       float64 `json:"pp"`
		BonusPp  float64 `json:"bonusPp"`
		Rank     int     `json:"rank"`
		AccRight float64 `json:"accRight"`
		AccLeft  float64 `json:"accLeft"`
	} `json:"scoreImprovement"`
	Leaderboard struct {
		ID   string `json:"id"`
		Song struct {
			ID             string  `json:"id"`
			Hash           string  `json:"hash"`
			Name           string  `json:"name"`
			SubName        string  `json:"subName"`
			Author         string  `json:"author"`
			Mapper         string  `json:"mapper"`
			MapperId       int     `json:"mapperId"`
			CoverImage     string  `json:"coverImage"`
			FullCoverImage string  `json:"fullCoverImage"`
			Bpm            float64 `json:"bpm"`
			Duration       float64 `json:"duration"`
			Tags           string  `json:"tags"`
		} `json:"song"`
		Difficulty struct {
			ID             int     `json:"id"`
			Value          int     `json:"value"`
			Mode           int     `json:"mode"`
			DifficultyName string  `json:"difficultyName"`
			ModeName       string  `json:"modeName"`
			Status         int     `json:"status"`
			NominatedTime  int64   `json:"nominatedTime"`
			QualifiedTime  int64   `json:"qualifiedTime"`
			RankedTime     int64   `json:"rankedTime"`
			Stars          float64 `json:"stars"`
			PredictedAcc   float64 `json:"predictedAcc"`
			PassRating     float64 `json:"passRating"`
			AccRating      float64 `json:"accRating"`
			TechRating     float64 `json:"techRating"`
			Type           int     `json:"type"`
			Njs            float64 `json:"njs"`
			Nps            float64 `json:"nps"`
			Notes          int     `json:"notes"`
			Bombs          int     `json:"bombs"`
			Walls          int     `json:"walls"`
			MaxScore       int     `json:"maxScore"`
			Duration       float64 `json:"duration"`
			Requirements   int     `json:"requirements"`
		} `json:"difficulty"`
		Plays             int           `json:"plays"`
		PositiveModifiers []string      `json:"positiveModifiers"`
		Changes           []interface{} `json:"changes"`
		MaxScore          int           `json:"maxScore"`
		CreatedDate       string        `json:"createdDate"`
	} `json:"leaderboard"`
	Weight       float64     `json:"weight"`
	AccLeft      float64     `json:"accLeft"`
	AccRight     float64     `json:"accRight"`
	FcAccuracy   float64     `json:"fcAccuracy"`
	OffsetValues interface{} `json:"offsets"`
}

func NewBeatLeaderPlugin(storage *storage.Storage, hub *stream.Hub, mediaPath string) *BeatLeaderPlugin {
	plugin := &BeatLeaderPlugin{
		storage:   storage,
		hub:       hub,
		mediaPath: mediaPath,
	}

	plugin.loadCachedCubes()
	go plugin.calculateCubesSlicedBackground()

	return plugin
}

func formatBeatLeaderNumber(n int64) string {
	return fmt.Sprintf("%d", n)
	//if n < 1000 {
	//	return fmt.Sprintf("%d", n)
	//} else if n < 1000000 {
	//	return fmt.Sprintf("%.1fK", float64(n)/1000)
	//} else {
	//	return fmt.Sprintf("%.2fM", float64(n)/1000000)
	//}
}

func (p *BeatLeaderPlugin) loadCachedCubes() {
	config := p.storage.GetPluginConfig(p.Name())

	if cubes, ok := config.Settings["cachedCubes"].(float64); ok {
		p.cubesMutex.Lock()
		p.cachedCubesSliced = int64(cubes)
		p.cubesMutex.Unlock()
		log.Printf("BeatLeader: Loaded cached cubes: %d", p.cachedCubesSliced)
	} else {
		log.Printf("BeatLeader: No cached cubes found, will calculate from scratch")
	}

	if timestamp, ok := config.Settings["lastCubesCalculated"].(string); ok {
		if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
			p.cubesMutex.Lock()
			p.lastCubesCalculated = t
			p.cubesMutex.Unlock()
			log.Printf("BeatLeader: Last calculation was at %s", t.Format(time.RFC3339))
		}
	}
}

func (p *BeatLeaderPlugin) saveCachedCubes() {
	p.cubesMutex.RLock()
	cubes := p.cachedCubesSliced
	calculatedAt := p.lastCubesCalculated
	p.cubesMutex.RUnlock()

	config := p.storage.GetPluginConfig(p.Name())
	if config.Settings == nil {
		config.Settings = make(map[string]interface{})
	}

	config.Settings["cachedCubes"] = float64(cubes)
	config.Settings["lastCubesCalculated"] = calculatedAt.Format(time.RFC3339)

	if err := p.storage.SetPluginConfig(p.Name(), config); err != nil {
		log.Printf("BeatLeader: Failed to save cached cubes: %v", err)
	} else {
		log.Printf("BeatLeader: Saved cached cubes to storage: %d", cubes)
	}

	if err := p.storage.Save(); err != nil {
		log.Printf("BeatLeader: Failed to persist storage to disk: %v", err)
	} else {
		log.Printf("BeatLeader: Successfully persisted cubes cache to disk")
	}
}

func (p *BeatLeaderPlugin) calculateCubesSlicedBackground() {
	time.Sleep(10 * time.Second)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	shouldRecalculate := false

	p.cubesMutex.RLock()
	lastCalc := p.lastCubesCalculated
	cachedValue := p.cachedCubesSliced
	p.cubesMutex.RUnlock()

	if cachedValue == 0 {
		shouldRecalculate = true
		log.Printf("BeatLeader: No cached cubes, will calculate now")
	} else if !lastCalc.IsZero() && time.Since(lastCalc) > 24*time.Hour {
		shouldRecalculate = true
		log.Printf("BeatLeader: Cache is stale (%s old), will recalculate", time.Since(lastCalc).Round(time.Hour))
	} else {
		log.Printf("BeatLeader: Using cached cubes: %d (calculated %s ago)", cachedValue, time.Since(lastCalc).Round(time.Minute))
	}

	if shouldRecalculate {
		p.performCubesCalculation()
	}

	for range ticker.C {
		p.performCubesCalculation()
	}
}

func (p *BeatLeaderPlugin) Name() string {
	return "beatleader"
}

func (p *BeatLeaderPlugin) extractScoreID(replayURL string) string {
	if replayURL == "" {
		return ""
	}

	re := regexp.MustCompile(`/(\d+)-`)
	matches := re.FindStringSubmatch(replayURL)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

func (p *BeatLeaderPlugin) Render(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	if p.playerData == nil {
		return p.renderLoading(settings), nil
	}

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "BeatLeader Stats")
	showPepeGif := p.getConfigBool(settings, "ui.showPepeGif", true)
	showRecentMaps := p.getConfigBool(settings, "ui.showRecentMaps", true)
	showMainStats := p.getConfigBool(settings, "ui.showMainStats", true)

	p.cubesMutex.RLock()
	cubesSliced := p.cachedCubesSliced
	p.cubesMutex.RUnlock()

	cubesFormatted := formatBeatLeaderNumber(cubesSliced)
	cubesExact := formatNumberWithCommas(cubesSliced)

	tmpl := `
<section class="beatleader-section section plugin" data-w="2">
	<header class="plugin-header">
		<h3 class="plugin-title">{{.SectionTitle}}</h3>
	</header>

	<div class="plugin__inner">
		{{if .ShowMainStats}}
		<div class="stats-grid">
			{{if .ProfileURL}}
			<a class="stat-item" href="{{.ProfileURL}}" target="_blank" rel="noopener noreferrer">
				<div class="stat-label">Global Rank</div>
				<div class="stat-value">#{{.PlayerData.Rank}}</div>
			</a>
			<a class="stat-item" href="{{.ProfileURL}}" target="_blank" rel="noopener noreferrer">
				<div class="stat-label">{{.PlayerData.Country}} Rank</div>
				<div class="stat-value">#{{.PlayerData.CountryRank}}</div>
			</a>
			<a class="stat-item" href="{{.ProfileURL}}" target="_blank" rel="noopener noreferrer">
				<div class="stat-label">Performance Points</div>
				<div class="stat-value">{{printf "%.0f" .PlayerData.PP}}pp</div>
			</a>
			<a class="stat-item" href="{{.ProfileURL}}" target="_blank" rel="noopener noreferrer" data-tooltip="Exact: {{.CubesExact}}">
				<div class="stat-label">Cubes Sliced</div>
				<div class="stat-value" data-cubes="{{.CubesSliced}}">{{.CubesFormatted}}</div>
			</a>
			{{else}}
			<div class="stat-item">
				<div class="stat-label">Global Rank</div>
				<div class="stat-value">#{{.PlayerData.Rank}}</div>
			</div>
			<div class="stat-item">
				<div class="stat-label">{{.PlayerData.Country}} Rank</div>
				<div class="stat-value">#{{.PlayerData.CountryRank}}</div>
			</div>
			<div class="stat-item">
				<div class="stat-label">Performance Points</div>
				<div class="stat-value">{{printf "%.0f" .PlayerData.PP}}pp</div>
			</div>
			<div class="stat-item" data-tooltip="Exact: {{.CubesExact}}">
				<div class="stat-label">Cubes Sliced</div>
				<div class="stat-value" data-cubes="{{.CubesSliced}}">{{.CubesFormatted}}</div>
			</div>
			{{end}}
		</div>
		{{end}}
		{{if and .ShowRecentMaps .RecentScores}}
		<h4>Recent Maps {{if .ShowPepeGif}} <img src="/static/images/pepe-dance.gif" alt="" class="pepe-gif" loading="lazy" style="width: 20px; height: 20px">{{end}}</h4>
		<div class="maps-list">
			{{range .RecentScores}}
				{{if .ReplayURL}}
				<a class="map-item" href="{{.ReplayURL}}" target="_blank" rel="noopener noreferrer">
					{{if .Leaderboard.Song.CoverImage}}
					<img src="{{.Leaderboard.Song.CoverImage}}" alt="{{.Leaderboard.Song.Name}}" class="map-cover" loading="lazy">
					{{end}}
					<div class="map-info">
						<div class="map-name">{{.Leaderboard.Song.Name}}</div>
						<div class="map-stats">
							<span>{{.Leaderboard.Difficulty.DifficultyName}}</span>
							{{if gt .Leaderboard.Difficulty.Stars 0.0}}
							<span>{{printf "%.1f" .Leaderboard.Difficulty.Stars}}⭐</span>
							{{end}}
							<span>{{printf "%.1f" (mul .Accuracy 100)}}%</span>
							{{if .PP}}<span>{{printf "%.0f" .PP}}pp</span>{{end}}
							{{if .FullCombo}}<span>FC</span>{{end}}
							{{if .ReplayURL}}<span class="replay-indicator">🎬</span>{{end}}
						</div>
					</div>
				</a>
				{{else}}
				<div class="map-item">
					{{if .Leaderboard.Song.CoverImage}}
					<img src="{{.Leaderboard.Song.CoverImage}}" alt="{{.Leaderboard.Song.Name}}" class="map-cover" loading="lazy">
					{{end}}
					<div class="map-info">
						<div class="map-name">{{.Leaderboard.Song.Name}}</div>
						<div class="map-stats">
							<span>{{.Leaderboard.Difficulty.DifficultyName}}</span>
							{{if gt .Leaderboard.Difficulty.Stars 0.0}}
							<span>{{printf "%.1f" .Leaderboard.Difficulty.Stars}}⭐</span>
							{{end}}
							<span>{{printf "%.1f" (mul .Accuracy 100)}}%</span>
							{{if .PP}}<span>{{printf "%.0f" .PP}}pp</span>{{end}}
							{{if .FullCombo}}<span>FC</span>{{end}}
						</div>
					</div>
				</div>
				{{end}}
			{{end}}
		</div>
		{{end}}
	</div>
</section>`
	var processedScores []map[string]interface{}
	for i, score := range p.recentScores {
		if i >= 10 {
			break
		}

		replayURL := ""
		if score.ID != 0 {
			replayURL = fmt.Sprintf("https://replay.beatleader.com/?scoreId=%d", score.ID)
		}

		processedScore := map[string]interface{}{
			"Leaderboard": score.Leaderboard,
			"Accuracy":    score.Accuracy,
			"PP":          score.PP,
			"Rank":        score.Rank,
			"FullCombo":   score.FullCombo,
			"Modifiers":   score.Modifiers,
			"ReplayURL":   replayURL,
		}
		processedScores = append(processedScores, processedScore)
	}

	funcMap := template.FuncMap{
		"mul":    func(a, b float64) float64 { return a * b },
		"printf": fmt.Sprintf,
		"gt":     func(a, b float64) bool { return a > b },
	}

	profileURL := ""
	if p.playerData.ID != "" {
		profileURL = fmt.Sprintf("https://beatleader.com/u/%s", p.playerData.ID)
	}

	data := struct {
		SectionTitle   string
		ShowPepeGif    bool
		ShowRecentMaps bool
		ShowMainStats  bool
		PlayerData     *BeatLeaderPlayer
		RecentScores   []map[string]interface{}
		CubesSliced    int64
		CubesFormatted string
		CubesExact     string
		ProfileURL     string
	}{
		SectionTitle:   sectionTitle,
		ShowPepeGif:    showPepeGif,
		ShowRecentMaps: showRecentMaps,
		ShowMainStats:  showMainStats,
		PlayerData:     p.playerData,
		RecentScores:   processedScores,
		CubesSliced:    cubesSliced,
		CubesFormatted: cubesFormatted,
		CubesExact:     cubesExact,
		ProfileURL:     profileURL,
	}

	t, err := template.New("beatleader").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	err = t.Execute(&buf, data)
	return buf.String(), err
}

func formatNumberWithCommas(n int64) string {
	str := fmt.Sprintf("%d", n)
	var result []rune
	for i, r := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, r)
	}
	return string(result)
}

func (p *BeatLeaderPlugin) renderLoading(settings map[string]interface{}) string {
	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "BeatLeader Stats")
	loadingText := p.getConfigValue(settings, "ui.loadingText", "Loading BeatLeader data...")

	return fmt.Sprintf(`<section class="beatleader-section section plugin" data-w="2">
		<header class="plugin-header">
			<h3 class="plugin-title">%s</h3>
		</header>
		<div class="plugin__inner">
			<div class="loading-indicator">
				<div class="loading"></div>
				<p class="text-muted">%s</p>
			</div>
		</div>
	</section>`, sectionTitle, loadingText)
}

func (p *BeatLeaderPlugin) UpdateData(ctx context.Context) error {
	err := p.generateCubesImage()
	if err != nil {
		log.Printf("BeatLeader: Failed to generate cubes image: %v", err)
	}
	if time.Since(p.lastUpdate) < 5*time.Minute {
		return nil
	}

	config := p.storage.GetPluginConfig(p.Name())
	username, ok := config.Settings["username"].(string)
	if !ok {
		return fmt.Errorf("username not configured")
	}

	if err := p.updatePlayerData(username); err != nil {
		return fmt.Errorf("failed to update player data: %v", err)
	}

	if err := p.updateRecentScores(username); err != nil {
		fmt.Printf("Warning: Failed to update recent scores: %v\n", err)
	}

	p.lastUpdate = time.Now()
	return nil
}

func (p *BeatLeaderPlugin) updatePlayerData(username string) error {
	url := fmt.Sprintf("https://api.beatleader.com/player/%s?stats=true&keepOriginalId=false&leaderboardContext=none", username)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept-Encoding", "identity")
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch player data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("beatLeader API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	start := 0
	for start < len(bodyBytes) && bodyBytes[start] <= 32 {
		start++
	}

	if start >= len(bodyBytes) {
		return fmt.Errorf("beatLeader API returned only whitespace")
	}

	if bodyBytes[start] != '{' {
		return fmt.Errorf("beatLeader API returned non-JSON content: %s", string(bodyBytes[:min(100, len(bodyBytes))]))
	}

	var newData BeatLeaderPlayer
	if err := json.Unmarshal(bodyBytes[start:], &newData); err != nil {
		return fmt.Errorf("failed to decode player data: %v", err)
	}

	if p.playerData == nil ||
		p.playerData.Rank != newData.Rank ||
		p.playerData.PP != newData.PP {
		p.playerData = &newData

		p.hub.Broadcast("beatleader_update", map[string]interface{}{
			"rank":        newData.Rank,
			"countryRank": newData.CountryRank,
			"pp":          newData.PP,
			"accuracy":    newData.ScoreStats.AverageRankedAccuracy * 100,
			"playCount":   newData.ScoreStats.TotalPlayCount,
			"peakRank":    newData.ScoreStats.PeakRank,
		})
	}

	return nil
}

func (p *BeatLeaderPlugin) updateRecentScores(username string) error {
	url := fmt.Sprintf("https://api.beatleader.com/player/%s/scores?leaderboardContext=general&page=1&sortBy=date&order=desc&count=10&includeIO=true", username)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch recent scores: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("beatLeader scores API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	start := 0
	for start < len(bodyBytes) && bodyBytes[start] <= 32 {
		start++
	}

	if start >= len(bodyBytes) {
		return fmt.Errorf("beatLeader scores API returned only whitespace")
	}

	if bodyBytes[start] != '{' {
		return fmt.Errorf("beatLeader scores API returned non-JSON content: %s", string(bodyBytes[:min(100, len(bodyBytes))]))
	}

	var response BeatLeaderScoresResponse
	if err := json.Unmarshal(bodyBytes[start:], &response); err != nil {
		return fmt.Errorf("failed to decode scores data: %v", err)
	}

	p.recentScores = response.Data

	p.hub.Broadcast("beatleader_maps_update", map[string]interface{}{
		"count": len(response.Data),
		"maps":  response.Data,
	})

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (p *BeatLeaderPlugin) GetSettings() map[string]interface{} {
	config := p.storage.GetPluginConfig(p.Name())
	return config.Settings
}

func (p *BeatLeaderPlugin) SetSettings(settings map[string]interface{}) error {
	config := p.storage.GetPluginConfig(p.Name())
	config.Settings = settings

	err := p.storage.SetPluginConfig(p.Name(), config)
	if err != nil {
		return err
	}

	p.lastUpdate = time.Time{}

	p.hub.Broadcast("plugin_update", map[string]interface{}{
		"plugin": p.Name(),
		"action": "settings_changed",
	})

	return nil
}

func (p *BeatLeaderPlugin) getConfigValue(settings map[string]interface{}, key string, defaultValue string) string {
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

func (p *BeatLeaderPlugin) getConfigBool(settings map[string]interface{}, key string, defaultValue bool) bool {
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

func (p *BeatLeaderPlugin) calculateCubesSliced() int64 {
	p.cubesMutex.RLock()
	defer p.cubesMutex.RUnlock()

	return p.cachedCubesSliced
}

func (p *BeatLeaderPlugin) RenderText(ctx context.Context) (string, error) {
	if p.playerData == nil {
		return "BeatSaber: No player data available", nil
	}

	cubesSliced := p.calculateCubesSliced()
	cubesFormatted := formatNumber(cubesSliced)

	return fmt.Sprintf("BeatSaber: Rank #%d (%s #%d), %.0fpp, %s cubes sliced",
		p.playerData.Rank,
		p.playerData.Country,
		p.playerData.CountryRank,
		p.playerData.PP,
		cubesFormatted), nil
}

func (p *BeatLeaderPlugin) performCubesCalculation() {
	p.cubesMutex.Lock()
	if p.cubesCalculating {
		p.cubesMutex.Unlock()
		return
	}
	p.cubesCalculating = true
	p.cubesMutex.Unlock()

	defer func() {
		p.cubesMutex.Lock()
		p.cubesCalculating = false
		p.cubesMutex.Unlock()
	}()

	config := p.storage.GetPluginConfig(p.Name())
	username, ok := config.Settings["username"].(string)
	if !ok {
		log.Printf("BeatLeader: username not configured for cubes calculation")
		return
	}

	log.Printf("BeatLeader: Starting cubes calculation for %s", username)

	var totalCubes int64
	page := 1
	const scoresPerPage = 100
	var totalScores int

	for {
		scores, metadata, err := p.fetchScoresPage(username, page, scoresPerPage)
		if err != nil {
			log.Printf("BeatLeader: Error fetching scores page %d: %v", page, err)
			break
		}

		if page == 1 {
			totalScores = metadata.Total
			log.Printf("BeatLeader: Total scores to process: %d (across %d pages)", totalScores, (totalScores+scoresPerPage-1)/scoresPerPage)
		}

		for _, score := range scores {
			cubesInAttempt := float64(score.BaseScore) / (115.0 * 8.0)
			totalCubes += int64(cubesInAttempt)
		}

		log.Printf("BeatLeader: Processed page %d/%d, total cubes so far: %d", page, (totalScores+scoresPerPage-1)/scoresPerPage, totalCubes)

		if len(scores) == 0 || page*scoresPerPage >= totalScores {
			break
		}

		page++
		time.Sleep(500 * time.Millisecond)
	}

	p.cubesMutex.Lock()
	p.cachedCubesSliced = totalCubes
	p.lastCubesCalculated = time.Now()
	p.cubesMutex.Unlock()

	log.Printf("BeatLeader: Cubes calculation complete: %d total cubes sliced", totalCubes)

	p.saveCachedCubes()

	if err := p.generateCubesImage(); err != nil {
		log.Printf("BeatLeader: Failed to generate cubes image: %v", err)
	}

	p.hub.Broadcast("beatleader_cubes_update", map[string]interface{}{
		"cubes":     totalCubes,
		"formatted": formatBeatLeaderNumber(totalCubes),
		"timestamp": time.Now().Unix(),
	})

	if p.cacheInvalidator != nil {
		p.cacheInvalidator()
	}
}

func (p *BeatLeaderPlugin) fetchScoresPage(username string, page int, count int) ([]BeatLeaderScore, struct {
	Total        int
	Page         int
	ItemsPerPage int
}, error) {
	url := fmt.Sprintf("https://api.beatleader.com/player/%s/scoresstats?page=%d&sortBy=date&order=desc&count=%d",
		username, page, count)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, struct {
			Total        int
			Page         int
			ItemsPerPage int
		}{}, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(req)
	if err != nil {
		return nil, struct {
			Total        int
			Page         int
			ItemsPerPage int
		}{}, fmt.Errorf("failed to fetch scores: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, struct {
			Total        int
			Page         int
			ItemsPerPage int
		}{}, fmt.Errorf("api returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, struct {
			Total        int
			Page         int
			ItemsPerPage int
		}{}, fmt.Errorf("failed to read response: %v", err)
	}

	start := 0
	for start < len(bodyBytes) && bodyBytes[start] <= 32 {
		start++
	}

	if start >= len(bodyBytes) || bodyBytes[start] != '{' {
		return nil, struct {
			Total        int
			Page         int
			ItemsPerPage int
		}{}, fmt.Errorf("invalid API response")
	}

	var response BeatLeaderScoresResponse
	if err := json.Unmarshal(bodyBytes[start:], &response); err != nil {
		return nil, struct {
			Total        int
			Page         int
			ItemsPerPage int
		}{}, fmt.Errorf("failed to decode scores: %v", err)
	}

	metadata := struct {
		Total        int
		Page         int
		ItemsPerPage int
	}{
		Total:        response.Metadata.Total,
		Page:         response.Metadata.Page,
		ItemsPerPage: response.Metadata.ItemsPerPage,
	}

	return response.Data, metadata, nil
}

func (p *BeatLeaderPlugin) generateCubesImage() error {
	p.cubesMutex.RLock()
	cubes := p.cachedCubesSliced
	lastCalc := p.lastCubesCalculated
	p.cubesMutex.RUnlock()

	cubesText := formatNumberWithCommas(cubes)

	line1 := "cubes sliced total:"
	line2 := cubesText

	var actualAt string
	if lastCalc.IsZero() {
		actualAt = "calculating..."
	} else {
		actualAt = fmt.Sprintf("accurate as of %s", lastCalc.Format("Jan 2, 2006 15:04 UTC"))
	}

	imgWidth := 1400
	imgHeight := 500

	img := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	black := color.RGBA{0, 0, 0, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{black}, image.Point{}, draw.Src)

	white := color.RGBA{255, 255, 255, 255}
	gray := color.RGBA{128, 128, 128, 255}

	fontSize := 64.0

	face, err := loadMinecraftFont(fontSize)
	if err != nil {
		log.Printf("BeatLeader: Failed to load Minecraft font: %v, using fallback", err)
		return p.generateCubesImageFallback()
	}
	defer face.Close()

	smallFace, err := loadMinecraftFont(24.0)
	if err != nil {
		log.Printf("BeatLeader: Failed to load small font: %v", err)
		smallFace = face
	} else {
		defer smallFace.Close()
	}

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(white),
		Face: face,
	}

	line1Bounds, _ := d.BoundString(line1)
	line1Width := (line1Bounds.Max.X - line1Bounds.Min.X).Ceil()
	line1X := (imgWidth - line1Width) / 2

	line2Bounds, _ := d.BoundString(line2)
	line2Width := (line2Bounds.Max.X - line2Bounds.Min.X).Ceil()
	line2X := (imgWidth - line2Width) / 2

	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()
	lineHeight := ascent + descent
	lineSpacing := 30

	totalHeight := lineHeight*2 + lineSpacing
	startY := (imgHeight-totalHeight)/2 + ascent - 20

	d.Dot = fixed.Point26_6{
		X: fixed.I(line1X),
		Y: fixed.I(startY),
	}
	d.DrawString(line1)

	d.Dot = fixed.Point26_6{
		X: fixed.I(line2X),
		Y: fixed.I(startY + lineHeight + lineSpacing),
	}
	d.DrawString(line2)

	dSmall := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(gray),
		Face: smallFace,
	}

	smallBounds, _ := dSmall.BoundString(actualAt)
	smallWidth := (smallBounds.Max.X - smallBounds.Min.X).Ceil()
	smallX := (imgWidth - smallWidth) / 2
	smallY := imgHeight - 30

	dSmall.Dot = fixed.Point26_6{
		X: fixed.I(smallX),
		Y: fixed.I(smallY),
	}
	dSmall.DrawString(actualAt)

	imagePath := filepath.Join(p.mediaPath, "bs.jpg")

	if err := os.MkdirAll(filepath.Dir(imagePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(imagePath)
	if err != nil {
		return fmt.Errorf("failed to create image file: %w", err)
	}
	defer file.Close()

	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 95}); err != nil {
		return fmt.Errorf("failed to encode JPEG: %w", err)
	}

	log.Printf("BeatLeader: Generated cubes image with Monocraft font at %s with %d cubes (size: %dx%d)",
		imagePath, cubes, imgWidth, imgHeight)
	return nil
}

func loadMinecraftFont(size float64) (font.Face, error) {
	paths := []string{
		"./static/fonts/Monocraft.ttf",
		"./fonts/Monocraft.ttf",
		"./static/fonts/Minecraft.ttf",
		"./fonts/Minecraft.ttf",
		"./static/fonts/Monocraft.ttc",
		"./fonts/Monocraft.ttc",
		"/usr/share/fonts/truetype/minecraft/Minecraft.ttf",
	}

	var b []byte
	var p string
	for _, path := range paths {
		if bb, err := os.ReadFile(path); err == nil {
			b, p = bb, path
			log.Printf("BeatLeader: Loaded font bytes from %s", p)
			break
		}
	}
	if b == nil {
		return nil, fmt.Errorf("no Minecraft/Monocraft font found")
	}

	if strings.HasSuffix(strings.ToLower(p), ".ttc") {
		coll, err := opentype.ParseCollection(b)
		if err != nil {
			return nil, fmt.Errorf("parse ttc: %w", err)
		}
		for i := 0; i < coll.NumFonts(); i++ {
			fnt, err := coll.Font(i)
			if err != nil {
				continue
			}
			face, err := opentype.NewFace(fnt, &opentype.FaceOptions{
				Size: size, DPI: 96, Hinting: font.HintingFull,
			})
			if err != nil {
				continue
			}
			d := font.Drawer{Face: face}
			if d.MeasureString("000000") == d.MeasureString("111111") { // monospaced
				log.Printf("BeatLeader: Using TTC face #%d from %s", i, p)
				return face, nil
			}
			face.Close()
		}
		return nil, fmt.Errorf("no suitable face in %s", p)
	}

	tt, err := opentype.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("parse ttf: %w", err)
	}
	return opentype.NewFace(tt, &opentype.FaceOptions{Size: size, DPI: 96, Hinting: font.HintingFull})
}

func (p *BeatLeaderPlugin) generateCubesImageFallback() error {
	p.cubesMutex.RLock()
	cubes := p.cachedCubesSliced
	p.cubesMutex.RUnlock()

	cubesText := formatNumberWithCommas(cubes)

	line1 := "cubes sliced total:"
	line2 := cubesText

	imgWidth := 1200
	imgHeight := 400

	img := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	black := color.RGBA{0, 0, 0, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{black}, image.Point{}, draw.Src)

	white := color.RGBA{255, 255, 255, 255}

	scale := 5
	charWidth := 7
	charHeight := 13
	lineSpacing := 40

	scaledCharWidth := charWidth * scale
	scaledCharHeight := charHeight * scale

	line1Width := len(line1) * scaledCharWidth
	line1X := (imgWidth - line1Width) / 2
	line1Y := (imgHeight-(scaledCharHeight*2+lineSpacing))/2 + scaledCharHeight

	drawLargeTextFallback(img, line1, line1X, line1Y, scale, white)

	line2Width := len(line2) * scaledCharWidth
	line2X := (imgWidth - line2Width) / 2
	line2Y := line1Y + scaledCharHeight + lineSpacing

	drawLargeTextFallback(img, line2, line2X, line2Y, scale, white)

	imagePath := filepath.Join(p.mediaPath, "bs.jpg")

	if err := os.MkdirAll(filepath.Dir(imagePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(imagePath)
	if err != nil {
		return fmt.Errorf("failed to create image file: %w", err)
	}
	defer file.Close()

	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 95}); err != nil {
		return fmt.Errorf("failed to encode JPEG: %w", err)
	}

	log.Printf("BeatLeader: Generated cubes image (fallback) at %s with %d cubes", imagePath, cubes)
	return nil
}

func drawLargeTextFallback(img *image.RGBA, text string, startX, startY, scale int, col color.RGBA) {
	face := basicfont.Face7x13

	baseWidth := len(text) * 7
	baseHeight := 13

	tempImg := image.NewRGBA(image.Rect(0, 0, baseWidth, baseHeight))
	draw.Draw(tempImg, tempImg.Bounds(), &image.Uniform{color.RGBA{0, 0, 0, 0}}, image.Point{}, draw.Src)

	d := &font.Drawer{
		Dst:  tempImg,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(0, 10),
	}
	d.DrawString(text)

	for y := 0; y < baseHeight; y++ {
		for x := 0; x < baseWidth; x++ {
			pixel := tempImg.At(x, y)
			r, g, b, a := pixel.RGBA()

			if a > 0 {
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						targetX := startX + x*scale + sx
						targetY := startY + y*scale + sy - (10 * scale)

						if targetX >= 0 && targetX < img.Bounds().Dx() &&
							targetY >= 0 && targetY < img.Bounds().Dy() {
							img.Set(targetX, targetY, color.RGBA{
								uint8(r >> 8),
								uint8(g >> 8),
								uint8(b >> 8),
								255,
							})
						}
					}
				}
			}
		}
	}
}

func (p *BeatLeaderPlugin) GetMetrics() map[string]interface{} {
	p.cubesMutex.RLock()
	cubes := p.cachedCubesSliced
	p.cubesMutex.RUnlock()

	metrics := map[string]interface{}{
		"cubes_sliced": cubes,
		"rank":         0,
		"pp":           0.0,
		"play_count":   0,
	}

	if p.playerData != nil {
		metrics["rank"] = p.playerData.Rank
		metrics["pp"] = p.playerData.PP
		metrics["play_count"] = p.playerData.ScoreStats.TotalPlayCount
		metrics["average_accuracy"] = p.playerData.ScoreStats.AverageRankedAccuracy * 100
	}

	return metrics
}
