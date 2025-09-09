package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Alexander-D-Karpov/about/internal/storage"
	"github.com/Alexander-D-Karpov/about/internal/stream"
)

type BeatLeaderPlugin struct {
	storage      *storage.Storage
	hub          *stream.Hub
	playerData   *BeatLeaderPlayer
	recentScores []BeatLeaderScore
	lastUpdate   time.Time
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

func NewBeatLeaderPlugin(storage *storage.Storage, hub *stream.Hub) *BeatLeaderPlugin {
	return &BeatLeaderPlugin{
		storage: storage,
		hub:     hub,
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
	config := p.storage.GetPluginConfig(p.Name())
	settings := config.Settings

	if p.playerData == nil {
		return p.renderLoading(settings), nil
	}

	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "BeatLeader Stats")
	showPepeGif := p.getConfigBool(settings, "ui.showPepeGif", true)
	showRecentMaps := p.getConfigBool(settings, "ui.showRecentMaps", true)
	showMainStats := p.getConfigBool(settings, "ui.showMainStats", true)

	tmpl := `
	<div class="beatleader-section section">
		<h3>{{.SectionTitle}}</h3>

		{{if .ShowMainStats}}
		<div class="stats-grid">
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
			<div class="stat-item">
				<div class="stat-label">Average Accuracy</div>
				<div class="stat-value">{{printf "%.1f" (mul .PlayerData.ScoreStats.AverageRankedAccuracy 100)}}%</div>
			</div>
		</div>
		{{end}}

		{{if and .ShowRecentMaps .RecentScores}}
		<h4>Recent Maps {{if .ShowPepeGif}} <img src="/static/images/pepe-dance.gif" alt="" class="pepe-gif" loading="lazy" style="width: 20px; height: 20px">{{end}}</h4>
		<div class="maps-list">
			{{range .RecentScores}}
			<div class="map-item" {{if .ReplayURL}}onclick="window.open('{{.ReplayURL}}', '_blank')" style="cursor: pointer;"{{end}}>
				{{if .Leaderboard.Song.CoverImage}}
				<img src="{{.Leaderboard.Song.CoverImage}}" alt="{{.Leaderboard.Song.Name}}" class="map-cover" loading="lazy">
				{{end}}
				<div class="map-info">
					<div class="map-name">{{.Leaderboard.Song.Name}}</div>
					<div class="map-stats">
						<span>{{.Leaderboard.Difficulty.DifficultyName}}</span>
						<span>{{printf "%.1f" .Leaderboard.Difficulty.Stars}}⭐</span>
						<span>{{printf "%.1f" (mul .Accuracy 100)}}%</span>
						{{if .PP}}<span>{{printf "%.0f" .PP}}pp</span>{{end}}
						{{if .FullCombo}}<span>FC</span>{{end}}
						{{if .ReplayURL}}<span class="replay-indicator">🎬</span>{{end}}
					</div>
				</div>
			</div>
			{{end}}
		</div>
		{{end}}
	</div>`

	var processedScores []map[string]interface{}
	for i, score := range p.recentScores {
		if i >= 5 {
			break
		}

		replayURL := ""
		if scoreID := p.extractScoreID(score.Replay); scoreID != "" {
			replayURL = fmt.Sprintf("https://replay.beatleader.com/?scoreId=%s", scoreID)
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
	}

	data := struct {
		SectionTitle   string
		ShowPepeGif    bool
		ShowRecentMaps bool
		ShowMainStats  bool
		PlayerData     *BeatLeaderPlayer
		RecentScores   []map[string]interface{}
	}{
		SectionTitle:   sectionTitle,
		ShowPepeGif:    showPepeGif,
		ShowRecentMaps: showRecentMaps,
		ShowMainStats:  showMainStats,
		PlayerData:     p.playerData,
		RecentScores:   processedScores,
	}

	t, err := template.New("beatleader").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	err = t.Execute(&buf, data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (p *BeatLeaderPlugin) renderLoading(settings map[string]interface{}) string {
	sectionTitle := p.getConfigValue(settings, "ui.sectionTitle", "BeatLeader Stats")
	loadingText := p.getConfigValue(settings, "ui.loadingText", "Loading BeatLeader data...")

	return fmt.Sprintf(`<div class="beatleader-section section">
		<h3>%s</h3>
		<div class="loading-indicator">
			<div class="loading"></div>
			<p class="text-muted">%s</p>
		</div>
	</div>`, sectionTitle, loadingText)
}

func (p *BeatLeaderPlugin) UpdateData(ctx context.Context) error {
	if time.Since(p.lastUpdate) < 30*time.Minute {
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
		return fmt.Errorf("BeatLeader API returned status %d", resp.StatusCode)
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
		return fmt.Errorf("BeatLeader API returned only whitespace")
	}

	if bodyBytes[start] != '{' {
		return fmt.Errorf("BeatLeader API returned non-JSON content: %s", string(bodyBytes[:min(100, len(bodyBytes))]))
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
	url := fmt.Sprintf("https://api.beatleader.com/player/%s/scores?leaderboardContext=general&page=1&sortBy=date&order=desc&count=5&includeIO=true", username)

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
		return fmt.Errorf("BeatLeader scores API returned status %d", resp.StatusCode)
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
		return fmt.Errorf("BeatLeader scores API returned only whitespace")
	}

	if bodyBytes[start] != '{' {
		return fmt.Errorf("BeatLeader scores API returned non-JSON content: %s", string(bodyBytes[:min(100, len(bodyBytes))]))
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
		} else {
			if next, ok := current[k].(map[string]interface{}); ok {
				current = next
			} else {
				return defaultValue
			}
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
		} else {
			if next, ok := current[k].(map[string]interface{}); ok {
				current = next
			} else {
				return defaultValue
			}
		}
	}

	return defaultValue
}
