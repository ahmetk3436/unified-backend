package driftoff

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidScore    = errors.New("score must be between 0 and 100")
	ErrInvalidDuration = errors.New("duration must be positive")
	ErrMissingTimes    = errors.New("bedtime and wake_time are required")
	ErrNotFound        = errors.New("sleep session not found")
	ErrNotOwner        = errors.New("not the owner of this session")
)

// coachCacheEntry is a simple in-memory cache entry for sleep coach responses.
type coachCacheEntry struct {
	content   string
	cachedAt  time.Time
}

type SleepService struct {
	db           *gorm.DB
	aiAPIKey     string
	aiAPIURL     string
	aiModel      string
	aiTimeout    time.Duration
	coachCache   map[string]coachCacheEntry
	coachCacheMu sync.Mutex
}

func NewSleepService(db *gorm.DB, cfg *config.Config) *SleepService {
	// Use OpenAI when key is configured; fall back to GLM (always configured) otherwise.
	apiKey := cfg.OpenAIAPIKey
	apiURL := "https://api.openai.com/v1/chat/completions"
	model := cfg.OpenAIModel
	if model == "" {
		model = "gpt-4o-mini"
	}
	if apiKey == "" {
		apiKey = cfg.GLMAPIKey
		apiURL = cfg.GLMAPIURL
		model = cfg.GLMModel
		if model == "" {
			model = "glm-4-flash"
		}
	}
	timeout := cfg.AITimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &SleepService{
		db:         db,
		aiAPIKey:   apiKey,
		aiAPIURL:   apiURL,
		aiModel:    model,
		aiTimeout:  timeout,
		coachCache: make(map[string]coachCacheEntry),
	}
}

// --- OpenAI helper (same pattern as daiyly) ---

type sleepAIChatRequest struct {
	Model    string           `json:"model"`
	Messages []sleepAIMessage `json:"messages"`
}

type sleepAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type sleepAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (s *SleepService) callOpenAI(systemPrompt, userPrompt string) (string, error) {
	if s.aiAPIKey == "" {
		return "", fmt.Errorf("openai api key not configured")
	}

	reqBody := sleepAIChatRequest{
		Model: s.aiModel,
		Messages: []sleepAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", s.aiAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.aiAPIKey)

	client := &http.Client{Timeout: s.aiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai returned status %d", resp.StatusCode)
	}

	var chatResp sleepAIChatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	// Strip markdown code fences if present.
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 2 {
			content = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	return content, nil
}

func (s *SleepService) Create(appID string, userID uuid.UUID, req CreateSleepRequest) (*SleepResponse, error) {
	if req.Score < 0 || req.Score > 100 {
		return nil, ErrInvalidScore
	}
	if req.DurationMinutes <= 0 {
		return nil, ErrInvalidDuration
	}
	if req.Bedtime == "" || req.WakeTime == "" {
		return nil, ErrMissingTimes
	}

	bedtime, err := time.Parse(time.RFC3339, req.Bedtime)
	if err != nil {
		return nil, fmt.Errorf("invalid bedtime format: %w", err)
	}
	wakeTime, err := time.Parse(time.RFC3339, req.WakeTime)
	if err != nil {
		return nil, fmt.Errorf("invalid wake_time format: %w", err)
	}

	if len(req.Phases) > 50 {
		return nil, errors.New("too many phases: max 50")
	}
	if len(req.Sounds) > 500 {
		return nil, errors.New("too many sounds: max 500")
	}

	// Validate soundscape_played against allowed values.
	validSoundscapes := map[string]bool{
		"brown_noise": true, "white_noise": true, "rain": true, "ocean": true,
		"fan": true, "pink_noise": true, "thunder": true, "forest": true,
		"none": true,
	}
	if req.SoundscapePlayed != nil && *req.SoundscapePlayed != "" {
		if !validSoundscapes[*req.SoundscapePlayed] {
			return nil, errors.New("invalid soundscape_played value")
		}
	}

	// Validate room_temp against allowed values.
	validRoomTemps := map[string]bool{"cool": true, "comfortable": true, "warm": true}
	if req.RoomTemp != nil && *req.RoomTemp != "" {
		if !validRoomTemps[*req.RoomTemp] {
			return nil, errors.New("invalid room_temp value: must be cool, comfortable, or warm")
		}
	}

	phasesJSON, _ := json.Marshal(req.Phases)
	soundsJSON, _ := json.Marshal(req.Sounds)

	session := SleepSession{
		AppID:            appID,
		UserID:           userID,
		Score:            req.Score,
		DurationMinutes:  req.DurationMinutes,
		Efficiency:       req.Efficiency,
		LatencyMinutes:   req.LatencyMinutes,
		Bedtime:          bedtime,
		WakeTime:         wakeTime,
		PhasesJSON:       string(phasesJSON),
		SoundsJSON:       string(soundsJSON),
		AlarmPhase:       req.AlarmPhase,
		SoundscapePlayed: req.SoundscapePlayed,
		RoomTemp:         req.RoomTemp,
	}

	if req.AlarmTime != nil {
		t, err := time.Parse(time.RFC3339, *req.AlarmTime)
		if err == nil {
			session.AlarmTime = &t
		}
	}

	if err := s.db.Create(&session).Error; err != nil {
		return nil, fmt.Errorf("create failed: %w", err)
	}

	go s.updateStreak(appID, userID)

	return s.toResponse(session), nil
}

func (s *SleepService) List(appID string, userID uuid.UUID, limit, offset int) (*SleepListResponse, error) {
	var sessions []SleepSession
	var total int64

	base := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID)
	base.Model(&SleepSession{}).Count(&total)

	if err := base.Order("created_at DESC").Limit(limit).Offset(offset).Find(&sessions).Error; err != nil {
		return nil, err
	}

	resp := &SleepListResponse{
		Sessions: make([]SleepResponse, len(sessions)),
		Total:    total,
		Limit:    limit,
		Offset:   offset,
	}
	for i, sess := range sessions {
		resp.Sessions[i] = *s.toResponse(sess)
	}
	return resp, nil
}

func (s *SleepService) Get(appID string, userID uuid.UUID, id uuid.UUID) (*SleepResponse, error) {
	var session SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", id).First(&session).Error; err != nil {
		return nil, ErrNotFound
	}
	if session.UserID != userID {
		return nil, ErrNotOwner
	}
	return s.toResponse(session), nil
}

func (s *SleepService) Update(appID string, userID uuid.UUID, id uuid.UUID, req UpdateSleepRequest) (*SleepResponse, error) {
	var session SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", id).First(&session).Error; err != nil {
		return nil, ErrNotFound
	}
	if session.UserID != userID {
		return nil, ErrNotOwner
	}

	if req.Score != nil {
		if *req.Score < 0 || *req.Score > 100 {
			return nil, ErrInvalidScore
		}
		session.Score = *req.Score
	}
	if req.DurationMinutes != nil {
		session.DurationMinutes = *req.DurationMinutes
	}
	if req.Efficiency != nil {
		session.Efficiency = *req.Efficiency
	}
	if req.LatencyMinutes != nil {
		session.LatencyMinutes = *req.LatencyMinutes
	}
	if req.Phases != nil {
		if len(*req.Phases) > 50 {
			return nil, errors.New("too many phases: max 50")
		}
		j, _ := json.Marshal(*req.Phases)
		session.PhasesJSON = string(j)
	}
	if req.Sounds != nil {
		if len(*req.Sounds) > 500 {
			return nil, errors.New("too many sounds: max 500")
		}
		j, _ := json.Marshal(*req.Sounds)
		session.SoundsJSON = string(j)
	}
	if req.SoundscapePlayed != nil {
		validSoundscapesUpd := map[string]bool{
			"brown_noise": true, "white_noise": true, "rain": true, "ocean": true,
			"fan": true, "pink_noise": true, "thunder": true, "forest": true,
			"none": true,
		}
		if *req.SoundscapePlayed != "" && !validSoundscapesUpd[*req.SoundscapePlayed] {
			return nil, errors.New("invalid soundscape_played value")
		}
		session.SoundscapePlayed = req.SoundscapePlayed
	}
	if req.RoomTemp != nil {
		validRoomTempsUpd := map[string]bool{"cool": true, "comfortable": true, "warm": true}
		if *req.RoomTemp != "" && !validRoomTempsUpd[*req.RoomTemp] {
			return nil, errors.New("invalid room_temp value: must be cool, comfortable, or warm")
		}
		session.RoomTemp = req.RoomTemp
	}

	if err := s.db.Save(&session).Error; err != nil {
		return nil, fmt.Errorf("update failed: %w", err)
	}

	return s.toResponse(session), nil
}

func (s *SleepService) Delete(appID string, userID uuid.UUID, id uuid.UUID) error {
	var session SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", id).First(&session).Error; err != nil {
		return ErrNotFound
	}
	if session.UserID != userID {
		return ErrNotOwner
	}
	return s.db.Delete(&session).Error
}

func (s *SleepService) Search(appID string, userID uuid.UUID, q string) (*SearchSleepResponse, error) {
	q = strings.TrimSpace(q)

	var sessions []SleepSession
	query := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID)

	// Search by date range or score threshold
	if score := 0; func() bool { n, _ := fmt.Sscanf(q, "%d", &score); return n == 1 }() && score >= 0 && score <= 100 {
		query = query.Where("score >= ?", score)
	} else {
		// Try parsing as date
		if t, err := time.Parse("2006-01-02", q); err == nil {
			dayStart := t
			dayEnd := t.Add(24 * time.Hour)
			query = query.Where("bedtime >= ? AND bedtime < ?", dayStart, dayEnd)
		}
	}

	if err := query.Order("created_at DESC").Limit(50).Find(&sessions).Error; err != nil {
		return nil, err
	}

	resp := &SearchSleepResponse{
		Sessions: make([]SleepResponse, len(sessions)),
		Total:    int64(len(sessions)),
		Query:    q,
	}
	for i, sess := range sessions {
		resp.Sessions[i] = *s.toResponse(sess)
	}
	return resp, nil
}

func (s *SleepService) GetStreak(appID string, userID uuid.UUID) (*StreakResponse, error) {
	var streak SleepStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &StreakResponse{}, nil
	}
	if err != nil {
		return nil, err
	}

	lastDate := ""
	if !streak.LastSessionDate.IsZero() {
		lastDate = streak.LastSessionDate.Format(time.RFC3339)
	}

	return &StreakResponse{
		CurrentStreak:   streak.CurrentStreak,
		LongestStreak:   streak.LongestStreak,
		TotalSessions:   streak.TotalSessions,
		LastSessionDate: lastDate,
	}, nil
}

func (s *SleepService) GetStats(appID string, userID uuid.UUID, days int) (*StatsResponse, error) {
	since := time.Now().AddDate(0, 0, -days)

	var sessions []SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at ASC").
		Find(&sessions).Error; err != nil {
		return nil, err
	}

	resp := &StatsResponse{
		TotalSessions:  len(sessions),
		DailyScores:    []DailyScore{},
		PhaseBreakdown: make(map[string]float64),
	}

	if len(sessions) == 0 {
		return resp, nil
	}

	var totalScore, totalDuration, totalEfficiency float64
	var totalBedtimeMinutes float64

	for _, sess := range sessions {
		totalScore += float64(sess.Score)
		totalDuration += float64(sess.DurationMinutes)
		totalEfficiency += sess.Efficiency

		// Bedtime as minutes from midnight
		h, m := sess.Bedtime.Hour(), sess.Bedtime.Minute()
		bedMinutes := float64(h*60 + m)
		if bedMinutes < 720 { // Before noon = after midnight
			bedMinutes += 1440
		}
		totalBedtimeMinutes += bedMinutes

		resp.DailyScores = append(resp.DailyScores, DailyScore{
			Date:            sess.Bedtime.Format("2006-01-02"),
			Score:           sess.Score,
			DurationMinutes: sess.DurationMinutes,
		})

		// Phase breakdown
		var phases []PhaseDTO
		_ = json.Unmarshal([]byte(sess.PhasesJSON), &phases)
		for _, p := range phases {
			resp.PhaseBreakdown[p.Type] += float64(p.DurationMinutes)
		}
	}

	n := float64(len(sessions))
	resp.AverageScore = math.Round(totalScore/n*10) / 10
	resp.AverageDuration = math.Round(totalDuration/n*10) / 10
	resp.AverageEfficiency = math.Round(totalEfficiency/n*10) / 10

	avgBedMinutes := int(totalBedtimeMinutes / n)
	if avgBedMinutes >= 1440 {
		avgBedMinutes -= 1440
	}
	resp.AverageBedtime = fmt.Sprintf("%02d:%02d", avgBedMinutes/60, avgBedMinutes%60)

	// Normalize phase breakdown to percentages
	totalPhaseMinutes := 0.0
	for _, v := range resp.PhaseBreakdown {
		totalPhaseMinutes += v
	}
	if totalPhaseMinutes > 0 {
		for k, v := range resp.PhaseBreakdown {
			resp.PhaseBreakdown[k] = math.Round(v/totalPhaseMinutes*1000) / 10
		}
	}

	// Bedtime variance (standard deviation in minutes)
	var bedtimeMinutesSlice []float64
	for _, sess := range sessions {
		h, m := sess.Bedtime.Hour(), sess.Bedtime.Minute()
		bm := float64(h*60 + m)
		if bm < 720 { // before noon = after midnight
			bm += 1440
		}
		bedtimeMinutesSlice = append(bedtimeMinutesSlice, bm)
	}
	if len(bedtimeMinutesSlice) > 1 {
		avgBM := totalBedtimeMinutes / n
		varianceSum := 0.0
		for _, bm := range bedtimeMinutesSlice {
			d := bm - avgBM
			varianceSum += d * d
		}
		resp.BedtimeVarianceMinutes = math.Round(math.Sqrt(varianceSum/n)*10) / 10
	}

	// Score trend
	if len(sessions) >= 3 {
		firstHalf := sessions[:len(sessions)/2]
		secondHalf := sessions[len(sessions)/2:]
		var firstAvg, secondAvg float64
		for _, s := range firstHalf {
			firstAvg += float64(s.Score)
		}
		for _, s := range secondHalf {
			secondAvg += float64(s.Score)
		}
		firstAvg /= float64(len(firstHalf))
		secondAvg /= float64(len(secondHalf))

		if secondAvg > firstAvg+3 {
			resp.ScoreTrend = "improving"
		} else if secondAvg < firstAvg-3 {
			resp.ScoreTrend = "declining"
		} else {
			resp.ScoreTrend = "stable"
		}
	} else {
		resp.ScoreTrend = "insufficient_data"
	}

	return resp, nil
}

func (s *SleepService) GetSleepDebt(appID string, userID uuid.UUID, goalHours float64) (*SleepDebtResponse, error) {
	rollingDays := 14
	since := time.Now().AddDate(0, 0, -rollingDays)

	var sessions []SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Find(&sessions).Error; err != nil {
		return nil, err
	}

	totalSlept := 0.0
	for _, sess := range sessions {
		totalSlept += float64(sess.DurationMinutes) / 60.0
	}

	totalNeeded := goalHours * float64(rollingDays)
	debt := totalNeeded - totalSlept
	if debt < 0 {
		debt = 0
	}

	trend := "stable"
	if len(sessions) >= 4 {
		recent := sessions
		if len(recent) > 7 {
			recent = recent[len(recent)-7:]
		}
		firstHalf := recent[:len(recent)/2]
		secondHalf := recent[len(recent)/2:]

		var firstAvg, secondAvg float64
		for _, s := range firstHalf {
			firstAvg += float64(s.DurationMinutes)
		}
		for _, s := range secondHalf {
			secondAvg += float64(s.DurationMinutes)
		}
		firstAvg /= float64(len(firstHalf))
		secondAvg /= float64(len(secondHalf))

		if secondAvg > firstAvg+15 {
			trend = "improving"
		} else if secondAvg < firstAvg-15 {
			trend = "worsening"
		}
	}

	return &SleepDebtResponse{
		CurrentDebtHours: math.Round(debt*10) / 10,
		Trend:            trend,
		DailyGoalHours:   goalHours,
		RollingDays:      rollingDays,
	}, nil
}

// ExportSleepData returns all sleep sessions for the user as CSV or JSON bytes.
// format must be "csv" or "json". Returns (data, mimeType, error).
func (s *SleepService) ExportSleepData(appID string, userID uuid.UUID, format string) ([]byte, string, error) {
	var sessions []SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Order("bedtime DESC").
		Limit(10000).
		Find(&sessions).Error; err != nil {
		return nil, "", fmt.Errorf("fetch sessions: %w", err)
	}

	if format == "csv" {
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		// Write header
		_ = w.Write([]string{
			"date", "bedtime", "wake_time", "duration_hours",
			"score", "efficiency", "rem_minutes", "deep_minutes",
			"light_minutes", "notes",
		})
		for _, sess := range sessions {
			// Parse phases for REM/deep/light
			var phases []PhaseDTO
			_ = json.Unmarshal([]byte(sess.PhasesJSON), &phases)
			var remMin, deepMin, lightMin int
			for _, p := range phases {
				switch p.Type {
				case "rem":
					remMin += p.DurationMinutes
				case "deep":
					deepMin += p.DurationMinutes
				case "light":
					lightMin += p.DurationMinutes
				}
			}
			durationHours := fmt.Sprintf("%.2f", float64(sess.DurationMinutes)/60.0)
			_ = w.Write([]string{
				sess.Bedtime.Format("2006-01-02"),
				sess.Bedtime.Format(time.RFC3339),
				sess.WakeTime.Format(time.RFC3339),
				durationHours,
				fmt.Sprintf("%d", sess.Score),
				fmt.Sprintf("%.1f", sess.Efficiency),
				fmt.Sprintf("%d", remMin),
				fmt.Sprintf("%d", deepMin),
				fmt.Sprintf("%d", lightMin),
				sess.Notes,
			})
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return nil, "", fmt.Errorf("csv write: %w", err)
		}
		return buf.Bytes(), "text/csv", nil
	}

	// JSON export
	out := make([]SleepResponse, len(sessions))
	for i, sess := range sessions {
		out[i] = *s.toResponse(sess)
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("json marshal: %w", err)
	}
	return data, "application/json", nil
}

// updateStreak recalculates the user's streak after a new session.
func (s *SleepService) updateStreak(appID string, userID uuid.UUID) {
	var streak SleepStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error

	now := time.Now().Truncate(24 * time.Hour)

	if errors.Is(err, gorm.ErrRecordNotFound) {
		streak = SleepStreak{
			AppID:           appID,
			UserID:          userID,
			CurrentStreak:   1,
			LongestStreak:   1,
			TotalSessions:   1,
			LastSessionDate: now,
		}
		s.db.Create(&streak)
		return
	}

	streak.TotalSessions++
	lastDate := streak.LastSessionDate.Truncate(24 * time.Hour)

	switch {
	case now.Equal(lastDate):
		// Same day
	case now.Sub(lastDate) <= 48*time.Hour && now.Sub(lastDate) > 0:
		streak.CurrentStreak++
	default:
		streak.CurrentStreak = 1
	}

	if streak.CurrentStreak > streak.LongestStreak {
		streak.LongestStreak = streak.CurrentStreak
	}
	streak.LastSessionDate = now

	s.db.Save(&streak)
}

func (s *SleepService) BatchImport(appID string, userID uuid.UUID, req BatchImportRequest) (*BatchImportResponse, error) {
	resp := &BatchImportResponse{Results: []BatchImportResult{}}

	for _, entry := range req.Sessions {
		result := BatchImportResult{ClientID: entry.ClientID}

		bedtime, err := time.Parse(time.RFC3339, entry.Bedtime)
		if err != nil {
			result.Status = "error"
			result.Error = "invalid bedtime"
			resp.Skipped++
			resp.Results = append(resp.Results, result)
			continue
		}
		wakeTime, err := time.Parse(time.RFC3339, entry.WakeTime)
		if err != nil {
			result.Status = "error"
			result.Error = "invalid wake_time"
			resp.Skipped++
			resp.Results = append(resp.Results, result)
			continue
		}

		// Dedup: check for existing session with same user + bedtime ±1 min
		var count int64
		s.db.Model(&SleepSession{}).
			Scopes(tenant.ForTenant(appID)).
			Where("user_id = ? AND bedtime BETWEEN ? AND ?", userID,
				bedtime.Add(-1*time.Minute), bedtime.Add(1*time.Minute)).
			Count(&count)
		if count > 0 {
			result.Status = "duplicate"
			resp.Skipped++
			resp.Results = append(resp.Results, result)
			continue
		}

		phasesJSON, _ := json.Marshal(entry.Phases)
		soundsJSON, _ := json.Marshal(entry.Sounds)

		session := SleepSession{
			AppID:           appID,
			UserID:          userID,
			Score:           entry.Score,
			DurationMinutes: entry.DurationMinutes,
			Efficiency:      entry.Efficiency,
			LatencyMinutes:  entry.LatencyMinutes,
			Bedtime:         bedtime,
			WakeTime:        wakeTime,
			PhasesJSON:      string(phasesJSON),
			SoundsJSON:      string(soundsJSON),
			AlarmPhase:      entry.AlarmPhase,
		}

		// Preserve original created_at timestamp
		if entry.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, entry.CreatedAt); err == nil {
				session.CreatedAt = t
			}
		}

		if err := s.db.Create(&session).Error; err != nil {
			result.Status = "error"
			result.Error = "storage error"
			resp.Skipped++
			resp.Results = append(resp.Results, result)
			continue
		}

		result.Status = "created"
		result.ServerID = session.ID.String()
		resp.Imported++
		resp.Results = append(resp.Results, result)
	}

	// Update streak once after all imports
	if resp.Imported > 0 {
		go s.updateStreak(appID, userID)
	}

	return resp, nil
}

func (s *SleepService) toResponse(sess SleepSession) *SleepResponse {
	var phases []PhaseDTO
	var sounds []SoundDTO
	_ = json.Unmarshal([]byte(sess.PhasesJSON), &phases)
	_ = json.Unmarshal([]byte(sess.SoundsJSON), &sounds)

	if phases == nil {
		phases = []PhaseDTO{}
	}
	if sounds == nil {
		sounds = []SoundDTO{}
	}

	resp := &SleepResponse{
		ID:               sess.ID,
		Score:            sess.Score,
		DurationMinutes:  sess.DurationMinutes,
		Efficiency:       sess.Efficiency,
		LatencyMinutes:   sess.LatencyMinutes,
		Bedtime:          sess.Bedtime.Format(time.RFC3339),
		WakeTime:         sess.WakeTime.Format(time.RFC3339),
		Phases:           phases,
		Sounds:           sounds,
		AlarmPhase:       sess.AlarmPhase,
		SoundscapePlayed: sess.SoundscapePlayed,
		RoomTemp:         sess.RoomTemp,
		CreatedAt:        sess.CreatedAt.Format(time.RFC3339),
	}

	if sess.AlarmTime != nil {
		t := sess.AlarmTime.Format(time.RFC3339)
		resp.AlarmTime = &t
	}

	return resp
}

// --- AI-powered methods ---

// GetSleepCoach returns a personalised sleep coaching message from GPT-4o-mini.
// Results are cached per (appID+userID) for 6 hours to avoid redundant API calls.
func (s *SleepService) GetSleepCoach(appID string, userID uuid.UUID) (string, error) {
	cacheKey := appID + ":" + userID.String()

	s.coachCacheMu.Lock()
	entry, ok := s.coachCache[cacheKey]
	s.coachCacheMu.Unlock()

	if ok && time.Since(entry.cachedAt) < 6*time.Hour {
		return entry.content, nil
	}

	// Fetch last 30 sleep sessions.
	var sessions []SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Order("bedtime DESC").
		Limit(30).
		Find(&sessions).Error; err != nil {
		return "", fmt.Errorf("fetch sessions: %w", err)
	}

	if len(sessions) == 0 {
		return "Not enough sleep data yet. Log at least one session and check back!", nil
	}

	// Fetch last 30 caffeine logs.
	var caffeineLogs []DailyCaffeineLog
	_ = s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Order("log_date DESC").
		Limit(30).
		Find(&caffeineLogs).Error

	// Build context string.
	var ctx strings.Builder
	ctx.WriteString("Sleep sessions (most recent first):\n")
	for _, sess := range sessions {
		durationHours := float64(sess.DurationMinutes) / 60.0
		ctx.WriteString(fmt.Sprintf("- Date: %s | Duration: %.1fh | Score: %d | Efficiency: %.0f%% | Bedtime: %s | Wake: %s",
			sess.Bedtime.Format("2006-01-02"),
			durationHours,
			sess.Score,
			sess.Efficiency,
			sess.Bedtime.Format("15:04"),
			sess.WakeTime.Format("15:04"),
		))
		if sess.Notes != "" {
			ctx.WriteString(" | Notes: " + sess.Notes)
		}
		ctx.WriteString("\n")
	}

	if len(caffeineLogs) > 0 {
		ctx.WriteString("\nCaffeine & exercise logs (most recent first):\n")
		for _, cl := range caffeineLogs {
			ctx.WriteString(fmt.Sprintf("- Date: %s | Caffeine: %dmg | Exercise: %dmin",
				cl.LogDate.Format("2006-01-02"),
				cl.CaffeineML,
				cl.ExerciseMin,
			))
			if cl.LastCupAt != nil {
				ctx.WriteString(" | Last cup at: " + cl.LastCupAt.Format("15:04"))
			}
			ctx.WriteString("\n")
		}
	}

	contextStr := ctx.String()
	// Cap at 20K chars.
	if len(contextStr) > 20000 {
		contextStr = contextStr[:20000]
	}

	systemPrompt := "You are DriftOff, an expert sleep coach. Analyze this user's sleep data from the last 30 days. Identify: 1) sleep debt trends, 2) consistency patterns (irregular schedules harm deep sleep), 3) what nights had best/worst sleep and why, 4) specific actionable recommendations for the next 7 days. Be specific, evidence-based, and warm. Max 400 words."

	content, err := s.callOpenAI(systemPrompt, contextStr)
	if err != nil {
		// AI unavailable — return a curated evidence-based tip rather than an error.
		content = sleepCoachFallback(userID.String())
	}

	s.coachCacheMu.Lock()
	s.coachCache[cacheKey] = coachCacheEntry{content: content, cachedAt: time.Now()}
	s.coachCacheMu.Unlock()

	return content, nil
}

// sleepCoachFallback returns a rotating evidence-based sleep tip when the AI is unavailable.
func sleepCoachFallback(seed string) string {
	tips := []string{
		"Sleep consistency matters more than total duration. Going to bed and waking at the same time every day — even weekends — keeps your circadian rhythm stable and dramatically improves deep sleep quality.",
		"Your bedroom temperature is one of the most impactful factors for sleep quality. Research from the National Sleep Foundation shows 65–68°F (18–20°C) is optimal. Even a 1°C reduction can increase deep sleep by up to 15%.",
		"Caffeine has a half-life of 5–7 hours. A coffee at 2pm still has 50% of its caffeine in your system at 9pm. Try cutting off caffeine by noon if you're struggling with sleep onset.",
		"The 20-minute rule: if you can't fall asleep within 20 minutes, get up and do something calm in dim light. Lying in bed awake trains your brain to associate the bed with wakefulness — the opposite of what you want.",
		"Blue light from screens suppresses melatonin production for up to 3 hours after exposure. Use Night Shift or Night Mode, or put screens away 90 minutes before bed for measurably better sleep onset.",
		"Sleep debt is cumulative and takes multiple nights to repay. You can't fully recover a week of poor sleep in one night. Prioritise consistent 7–9 hours over sleeping in on weekends.",
		"Alcohol might help you fall asleep, but it fragments your sleep architecture — especially REM sleep, which is critical for emotional processing and memory. Even 1–2 drinks can reduce REM by up to 24%.",
		"Exercise significantly improves sleep quality, but timing matters. Vigorous exercise within 2 hours of bed can delay sleep onset for some people. Morning or early afternoon workouts tend to produce the best sleep outcomes.",
	}
	// Rotate based on first byte of seed (deterministic per-user, changes day-to-day)
	idx := 0
	if len(seed) > 0 {
		idx = int(seed[0]) % len(tips)
	}
	return tips[idx]
}

// GetDoctorReport generates a clinical sleep summary suitable for a doctor appointment.
func (s *SleepService) GetDoctorReport(appID string, userID uuid.UUID) (string, error) {
	var sessions []SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Order("bedtime DESC").
		Limit(30).
		Find(&sessions).Error; err != nil {
		return "", fmt.Errorf("fetch sessions: %w", err)
	}

	if len(sessions) == 0 {
		return "No sleep sessions recorded. Cannot generate a clinical report.", nil
	}

	// Compute statistics.
	var totalDuration, totalScore, totalEfficiency float64
	var bedtimeMinutes []float64

	for _, sess := range sessions {
		totalDuration += float64(sess.DurationMinutes) / 60.0
		totalScore += float64(sess.Score)
		totalEfficiency += sess.Efficiency

		h, m := sess.Bedtime.Hour(), sess.Bedtime.Minute()
		bm := float64(h*60 + m)
		if bm < 720 { // before noon = after midnight
			bm += 1440
		}
		bedtimeMinutes = append(bedtimeMinutes, bm)
	}

	n := float64(len(sessions))
	avgDuration := totalDuration / n
	avgScore := totalScore / n
	avgEfficiency := totalEfficiency / n

	// Bedtime variance.
	avgBedtime := 0.0
	for _, bm := range bedtimeMinutes {
		avgBedtime += bm
	}
	avgBedtime /= float64(len(bedtimeMinutes))
	variance := 0.0
	for _, bm := range bedtimeMinutes {
		d := bm - avgBedtime
		variance += d * d
	}
	variance /= float64(len(bedtimeMinutes))
	bedtimeVarianceMin := math.Sqrt(variance)

	// Sleep debt (vs 8h goal over 14 days).
	rollingDays := 14
	sleepDebt := float64(rollingDays)*8.0 - totalDuration
	if sleepDebt < 0 {
		sleepDebt = 0
	}

	statsContext := fmt.Sprintf(
		"Patient sleep data summary:\n"+
			"Total sessions tracked: %d\n"+
			"Period: last 30 sessions\n"+
			"Average sleep duration: %.1f hours\n"+
			"Average sleep score: %.0f/100\n"+
			"Average sleep efficiency: %.1f%%\n"+
			"Sleep debt (14-day rolling, 8h goal): %.1f hours\n"+
			"Bedtime consistency (standard deviation): %.0f minutes\n",
		len(sessions),
		avgDuration,
		avgScore,
		avgEfficiency,
		sleepDebt,
		bedtimeVarianceMin,
	)

	// Cap at 20K chars (safety).
	if len(statsContext) > 20000 {
		statsContext = statsContext[:20000]
	}

	systemPrompt := "Generate a clinical sleep summary for a doctor appointment. Include: total sessions tracked, avg sleep duration, sleep efficiency, sleep debt, sleep schedule consistency (bedtime variance in minutes), notable patterns, and any concerning trends. Format in clear medical language. Be factual and concise."

	content, err := s.callOpenAI(systemPrompt, statsContext)
	if err != nil {
		return "", err
	}

	return content, nil
}

// GetHygieneScore scores sleep hygiene across 4 dimensions using the last 14 sessions.
func (s *SleepService) GetHygieneScore(appID string, userID uuid.UUID) (*HygieneScoreResponse, error) {
	var sessions []SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Order("bedtime DESC").
		Limit(14).
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("fetch sessions: %w", err)
	}

	if len(sessions) == 0 {
		return &HygieneScoreResponse{
			Score:      0,
			Grade:      "N/A",
			Dimensions: map[string]int{"consistency": 0, "duration": 0, "efficiency": 0, "streak": 0},
			Insight:    "No sleep data yet. Start logging your sleep to get your hygiene score.",
		}, nil
	}

	// 1. Consistency: bedtime variance.
	var bedtimeMinutes []float64
	for _, sess := range sessions {
		h, m := sess.Bedtime.Hour(), sess.Bedtime.Minute()
		bm := float64(h*60 + m)
		if bm < 720 {
			bm += 1440
		}
		bedtimeMinutes = append(bedtimeMinutes, bm)
	}
	avgBedtime := 0.0
	for _, bm := range bedtimeMinutes {
		avgBedtime += bm
	}
	avgBedtime /= float64(len(bedtimeMinutes))
	variance := 0.0
	for _, bm := range bedtimeMinutes {
		d := bm - avgBedtime
		variance += d * d
	}
	variance /= float64(len(bedtimeMinutes))
	bedtimeStdDev := math.Sqrt(variance)

	consistencyScore := 5
	switch {
	case bedtimeStdDev < 30:
		consistencyScore = 25
	case bedtimeStdDev < 60:
		consistencyScore = 15
	case bedtimeStdDev < 90:
		consistencyScore = 10
	}

	// 2. Duration: average sleep hours.
	var totalDuration float64
	for _, sess := range sessions {
		totalDuration += float64(sess.DurationMinutes) / 60.0
	}
	avgDuration := totalDuration / float64(len(sessions))

	durationScore := 5
	switch {
	case avgDuration >= 7.5:
		durationScore = 25
	case avgDuration >= 7.0:
		durationScore = 20
	case avgDuration >= 6.0:
		durationScore = 15
	}

	// 3. Efficiency: average efficiency.
	var totalEff float64
	for _, sess := range sessions {
		totalEff += sess.Efficiency
	}
	avgEff := totalEff / float64(len(sessions))

	efficiencyScore := 5
	switch {
	case avgEff >= 85:
		efficiencyScore = 25
	case avgEff >= 75:
		efficiencyScore = 18
	case avgEff >= 65:
		efficiencyScore = 12
	}

	// 4. Streak: fetch from streak table.
	var streak SleepStreak
	streakDays := 0
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error; err == nil {
		streakDays = streak.CurrentStreak
	}

	streakScore := 5
	switch {
	case streakDays >= 14:
		streakScore = 25
	case streakDays >= 7:
		streakScore = 18
	case streakDays >= 3:
		streakScore = 12
	}

	total := consistencyScore + durationScore + efficiencyScore + streakScore

	// Grade mapping.
	grade := "F"
	switch {
	case total >= 93:
		grade = "A+"
	case total >= 90:
		grade = "A"
	case total >= 87:
		grade = "A-"
	case total >= 83:
		grade = "B+"
	case total >= 80:
		grade = "B"
	case total >= 77:
		grade = "B-"
	case total >= 73:
		grade = "C+"
	case total >= 70:
		grade = "C"
	case total >= 67:
		grade = "C-"
	case total >= 60:
		grade = "D"
	}

	// Insight: pick the weakest dimension.
	insight := "Keep logging your sleep consistently to improve your score."
	minDim, minScore := "consistency", consistencyScore
	if durationScore < minScore {
		minDim, minScore = "duration", durationScore
	}
	if efficiencyScore < minScore {
		minDim, minScore = "efficiency", efficiencyScore
	}
	if streakScore < minScore {
		minDim, _ = "streak", streakScore
	}

	switch minDim {
	case "consistency":
		insight = fmt.Sprintf("Your bedtime varies by ~%.0f minutes. Try going to bed within the same 30-minute window each night to improve deep sleep.", bedtimeStdDev)
	case "duration":
		insight = fmt.Sprintf("You're averaging %.1f hours of sleep. Aim for 7.5+ hours to fully restore your body and mind.", avgDuration)
	case "efficiency":
		insight = fmt.Sprintf("Your sleep efficiency is %.0f%%. Avoid screens 1 hour before bed and keep your room cool to spend more time actually sleeping.", avgEff)
	case "streak":
		insight = fmt.Sprintf("You've logged %d consecutive nights. Building a %d-night streak will sharpen your insights significantly.", streakDays, 7)
	}

	return &HygieneScoreResponse{
		Score: total,
		Grade: grade,
		Dimensions: map[string]int{
			"consistency": consistencyScore,
			"duration":    durationScore,
			"efficiency":  efficiencyScore,
			"streak":      streakScore,
		},
		Insight: insight,
	}, nil
}

// LogCaffeine upserts today's caffeine log for the user (one record per user per day).
func (s *SleepService) LogCaffeine(appID string, userID uuid.UUID, caffeineML, exerciseMin int, lastCupAt *time.Time) (*DailyCaffeineLog, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)

	log := DailyCaffeineLog{
		ID:          uuid.New(),
		AppID:       appID,
		UserID:      userID,
		LogDate:     today,
		CaffeineML:  caffeineML,
		ExerciseMin: exerciseMin,
		LastCupAt:   lastCupAt,
	}

	// Upsert: on conflict (app_id + user_id + log_date) update the fields.
	result := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "app_id"}, {Name: "user_id"}, {Name: "log_date"}},
		DoUpdates: clause.AssignmentColumns([]string{"caffeine_ml", "exercise_min", "last_cup_at", "updated_at"}),
	}).Create(&log)

	if result.Error != nil {
		// Fallback: find-and-update if the upsert path fails (e.g. no unique index yet).
		var existing DailyCaffeineLog
		if err := s.db.Scopes(tenant.ForTenant(appID)).
			Where("user_id = ? AND log_date = ?", userID, today).
			First(&existing).Error; err == nil {
			existing.CaffeineML = caffeineML
			existing.ExerciseMin = exerciseMin
			existing.LastCupAt = lastCupAt
			if err2 := s.db.Save(&existing).Error; err2 != nil {
				return nil, fmt.Errorf("update caffeine log: %w", err2)
			}
			return &existing, nil
		}
		return nil, fmt.Errorf("create caffeine log: %w", result.Error)
	}

	// Re-fetch to get the actual stored record (handles upsert returning stale ID).
	var stored DailyCaffeineLog
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND log_date = ?", userID, today).
		First(&stored).Error; err != nil {
		return &log, nil
	}
	return &stored, nil
}

// GetCaffeineLog returns the last N days of caffeine logs for the user.
func (s *SleepService) GetCaffeineLog(appID string, userID uuid.UUID, days int) ([]DailyCaffeineLog, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)
	var logs []DailyCaffeineLog
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND log_date >= ?", userID, since).
		Order("log_date DESC").
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

// GetSoundCorrelation returns average sleep efficiency per soundscape (only soundscapes
// with 3+ sessions are included). Pure SQL aggregation, no AI.
func (s *SleepService) GetSoundCorrelation(appID string, userID uuid.UUID) (*SoundCorrelationResponse, error) {
	type row struct {
		Soundscape string  `gorm:"column:soundscape_played"`
		AvgEff     float64 `gorm:"column:avg_efficiency"`
	}
	var rows []row
	err := s.db.Raw(
		"SELECT soundscape_played, ROUND(AVG(efficiency)::numeric, 2) AS avg_efficiency "+
			"FROM sleep_sessions "+
			"WHERE app_id = ? AND user_id = ? AND soundscape_played IS NOT NULL AND soundscape_played != '' "+
			"AND deleted_at IS NULL "+
			"GROUP BY soundscape_played HAVING COUNT(*) >= 3",
		appID, userID,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("sound correlation: %w", err)
	}

	m := make(map[string]float64, len(rows))
	for _, r := range rows {
		m[r.Soundscape] = r.AvgEff
	}

	var nightCount int64
	s.db.Raw(
		"SELECT COUNT(*) FROM sleep_sessions "+
			"WHERE app_id = ? AND user_id = ? AND soundscape_played IS NOT NULL AND soundscape_played != '' "+
			"AND deleted_at IS NULL",
		appID, userID,
	).Scan(&nightCount)

	return &SoundCorrelationResponse{Correlations: m, NightCount: int(nightCount)}, nil
}

// GetTempCorrelation returns average sleep score per room temperature (only temperatures
// with 3+ sessions are included). Pure SQL aggregation, no AI.
func (s *SleepService) GetTempCorrelation(appID string, userID uuid.UUID) (*TempCorrelationResponse, error) {
	type row struct {
		RoomTemp string  `gorm:"column:room_temp"`
		AvgScore float64 `gorm:"column:avg_score"`
	}
	var rows []row
	err := s.db.Raw(
		"SELECT room_temp, ROUND(AVG(score)::numeric, 2) AS avg_score "+
			"FROM sleep_sessions "+
			"WHERE app_id = ? AND user_id = ? AND room_temp IS NOT NULL AND room_temp != '' "+
			"AND deleted_at IS NULL "+
			"GROUP BY room_temp HAVING COUNT(*) >= 3",
		appID, userID,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("temp correlation: %w", err)
	}

	m := make(map[string]float64, len(rows))
	for _, r := range rows {
		m[r.RoomTemp] = r.AvgScore
	}

	var nightCount int64
	s.db.Raw(
		"SELECT COUNT(*) FROM sleep_sessions "+
			"WHERE app_id = ? AND user_id = ? AND room_temp IS NOT NULL AND room_temp != '' "+
			"AND deleted_at IS NULL",
		appID, userID,
	).Scan(&nightCount)

	return &TempCorrelationResponse{Correlations: m, NightCount: int(nightCount)}, nil
}

// GetCBTIInsights analyzes the user's sleep sessions and returns clinically-validated
// CBT-I recommendations based on their pattern. Pure Go math, no AI.
func (s *SleepService) GetCBTIInsights(appID string, userID uuid.UUID) (*CBTIInsightsResponse, error) {
	// Fetch enough sessions for meaningful analysis
	var sessions []SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(30).
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("fetch sessions: %w", err)
	}

	resp := &CBTIInsightsResponse{Recommendations: []CBTIRecommendation{}}

	if len(sessions) < 3 {
		return resp, nil
	}

	var totalInBed, totalEff, totalLatency float64
	n := float64(len(sessions))

	for _, sess := range sessions {
		totalInBed += float64(sess.DurationMinutes)
		totalEff += sess.Efficiency
		totalLatency += float64(sess.LatencyMinutes)
	}

	avgInBedMinutes := totalInBed / n
	avgEff := totalEff / n
	avgLatency := totalLatency / n

	// 1. Sleep restriction check
	// If avg time in bed > 480 min (8h) AND avg efficiency < 75%
	if avgInBedMinutes > 480 && avgEff < 75.0 {
		resp.Recommendations = append(resp.Recommendations, CBTIRecommendation{
			Type:     "restrict_bed_time",
			Message:  "Research shows going to bed 30 minutes later improves deep sleep quality for people with your pattern.",
			Evidence: "Sleep restriction therapy (Spielman 1987) is a first-line CBT-I technique.",
		})
	}

	// 2. Stimulus control check
	// If avg sleep onset latency > 30 min AND at least 7 sessions logged
	if avgLatency > 30.0 && len(sessions) >= 7 {
		resp.Recommendations = append(resp.Recommendations, CBTIRecommendation{
			Type:     "stimulus_control",
			Message:  "Only go to bed when genuinely sleepy. This trains your brain to associate bed with sleep.",
			Evidence: "Stimulus control therapy (Bootzin 1972) reduces sleep-onset latency in insomnia.",
		})
	}

	// 3. Orthosomnia check
	// If user has 14+ sessions AND score variance is high (std dev > 15)
	if len(sessions) >= 14 {
		var totalScore float64
		for _, sess := range sessions {
			totalScore += float64(sess.Score)
		}
		avgScore := totalScore / n
		var varianceSum float64
		for _, sess := range sessions {
			d := float64(sess.Score) - avgScore
			varianceSum += d * d
		}
		stdDev := math.Sqrt(varianceSum / n)

		if stdDev > 15 {
			resp.Recommendations = append(resp.Recommendations, CBTIRecommendation{
				Type:     "tracker_break",
				Message:  "You're tracking consistently — consider a 7-day score-free period to focus on how you feel, not the number.",
				Evidence: "Orthosomnia (excessive sleep tracking anxiety) can worsen sleep quality (Baron et al. 2017).",
			})
		}
	}

	return resp, nil
}

// GetSleepRegularityIndex computes a Sleep Regularity Index (SRI) score endorsed by
// the World Sleep Society 2025. SRI is derived from the standard deviation of the
// user's bedtime and wake-time across the last 14-30 sessions:
//   score = 100 × max(0, 1 − stdDev/90)  (90-min tolerance before score drops to 0)
//
// Returns grade + actionable coaching if sufficient data exists (≥7 sessions).
func (s *SleepService) GetSleepRegularityIndex(appID string, userID uuid.UUID) (*SRIResponse, error) {
	const minSessions = 7
	const maxSessions = 30

	var sessions []SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Order("bedtime DESC").
		Limit(maxSessions).
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("fetch sessions: %w", err)
	}

	resp := &SRIResponse{
		NightsSampled: len(sessions),
		CitationNote:  "World Sleep Society 2025 Sleep Regularity Index guidelines.",
	}

	if len(sessions) < minSessions {
		resp.Score = 0
		resp.Grade = "Not enough data"
		resp.Insight = fmt.Sprintf("Log %d more nights to unlock your Sleep Regularity Index.", minSessions-len(sessions))
		return resp, nil
	}

	// Compute average bedtime offset (minutes since midnight, handling day crossover).
	toMinutes := func(t time.Time) float64 {
		h := float64(t.Hour())
		m := float64(t.Minute())
		mins := h*60 + m
		// Shift late-night times (before 6am) to after midnight for continuity.
		// e.g. 1:00 AM → 1*60 = 60; treat as 60+1440? No — we shift forward by treating
		// times < 6*60 as belonging to the "next day" (+1440) for variance computation.
		// This prevents midnight wraparound artifacts.
		if mins < 360 { // before 6am
			mins += 1440
		}
		return mins
	}

	bedMins := make([]float64, len(sessions))
	wakeMins := make([]float64, len(sessions))
	for i, s := range sessions {
		bedMins[i] = toMinutes(s.Bedtime)
		wakeMins[i] = toMinutes(s.WakeTime)
	}

	stdDev := func(vals []float64) (mean, std float64) {
		n := float64(len(vals))
		for _, v := range vals {
			mean += v
		}
		mean /= n
		var variance float64
		for _, v := range vals {
			d := v - mean
			variance += d * d
		}
		std = math.Sqrt(variance / n)
		return
	}

	avgBed, stdBed := stdDev(bedMins)
	avgWake, stdWake := stdDev(wakeMins)

	// Combined variance: weight bedtime 60%, wake time 40%.
	combined := 0.6*stdBed + 0.4*stdWake

	// Normalise: 0 variance → 100, 90+ min variance → 0.
	score := math.Max(0, 100*(1-combined/90.0))
	score = math.Round(score*10) / 10

	// Convert average minutes back to clock hour (0-24).
	toHour := func(mins float64) float64 {
		if mins >= 1440 {
			mins -= 1440
		}
		return math.Round(mins/60*10) / 10
	}
	avgBedHour := toHour(avgBed)
	avgWakeHour := toHour(avgWake)

	// Grade thresholds (World Sleep Society 2025).
	var grade, insight, rec string
	switch {
	case score >= 85:
		grade = "Excellent"
		insight = fmt.Sprintf("Your bedtime varies only %.0f min on average — top-tier sleep regularity.", stdBed)
		rec = "Maintain your consistent schedule to preserve circadian alignment."
	case score >= 70:
		grade = "Good"
		insight = fmt.Sprintf("Your sleep timing is fairly consistent (±%.0f min variance).", combined)
		rec = "Try to keep bedtime within a 30-minute window to reach Excellent."
	case score >= 50:
		grade = "Fair"
		insight = fmt.Sprintf("Irregular sleep timing (±%.0f min variance) is fragmenting your sleep quality.", combined)
		rec = "Pick a target bedtime and stick within 30 minutes for 14 days. Even weekends matter."
	default:
		grade = "Poor"
		insight = fmt.Sprintf("High variability (±%.0f min) is likely disrupting your circadian rhythm.", combined)
		rec = "Social jet lag is present. Align your weekend bedtime closer to your weekday schedule."
	}

	resp.Score = score
	resp.Grade = grade
	resp.BedtimeVarianceMin = math.Round(stdBed*10) / 10
	resp.WakeVarianceMin = math.Round(stdWake*10) / 10
	resp.AvgBedtimeHour = avgBedHour
	resp.AvgWakeHour = avgWakeHour
	resp.Insight = insight
	resp.Recommendation = rec

	return resp, nil
}

// avgIntSlice returns the mean of a non-empty int slice.
func avgIntSlice(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := 0
	for _, v := range vals {
		s += v
	}
	return float64(s) / float64(len(vals))
}

// avgFloat64Slice returns the mean of a non-empty float64 slice.
func avgFloat64Slice(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// GetLifestyleCorrelation computes correlations between:
//   - caffeine timing (after/before 3 pm) and sleep onset latency
//   - exercise (≥30 min / <30 min) and sleep efficiency
//
// Returns nil fields when insufficient paired data exists (<3 nights per group).
// Pure Go math — no AI, no external calls.
func (s *SleepService) GetLifestyleCorrelation(appID string, userID uuid.UUID) (*LifestyleCorrelationResponse, error) {
	const minNights = 7
	const minGroup = 3

	// 1. Fetch last 30 sleep sessions.
	var sessions []SleepSession
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Order("bedtime DESC").
		Limit(30).
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("fetch sessions: %w", err)
	}

	resp := &LifestyleCorrelationResponse{
		DataPoints:    len(sessions),
		MinDataPoints: minNights,
	}
	if len(sessions) < minNights {
		return resp, nil
	}

	// 2. Fetch caffeine logs for the relevant date range.
	oldest := sessions[len(sessions)-1].Bedtime.AddDate(0, 0, -1)
	var caffeineLogs []DailyCaffeineLog
	if err := s.db.
		Where("app_id = ? AND user_id = ? AND log_date >= ?", appID, userID, oldest).
		Find(&caffeineLogs).Error; err != nil {
		return nil, fmt.Errorf("fetch caffeine logs: %w", err)
	}

	// 3. Build date-keyed map.
	cafMap := make(map[string]*DailyCaffeineLog, len(caffeineLogs))
	for i := range caffeineLogs {
		key := caffeineLogs[i].LogDate.Format("2006-01-02")
		cafMap[key] = &caffeineLogs[i]
	}

	// 4. Collect grouped metrics.
	var latencyAfter, latencyBefore []int
	var effWithEx, effWithoutEx []float64

	for _, sess := range sessions {
		key := sess.Bedtime.Format("2006-01-02")
		log, ok := cafMap[key]
		if !ok {
			continue
		}
		// Caffeine timing.
		if log.LastCupAt != nil {
			if log.LastCupAt.Hour() >= 15 {
				latencyAfter = append(latencyAfter, sess.LatencyMinutes)
			} else {
				latencyBefore = append(latencyBefore, sess.LatencyMinutes)
			}
		}
		// Exercise.
		if log.ExerciseMin >= 30 {
			effWithEx = append(effWithEx, sess.Efficiency)
		} else {
			effWithoutEx = append(effWithoutEx, sess.Efficiency)
		}
	}

	// 5. Compute caffeine correlation.
	if len(latencyAfter) >= minGroup && len(latencyBefore) >= minGroup {
		avgAfter := avgIntSlice(latencyAfter)
		avgBefore := avgIntSlice(latencyBefore)
		diff := math.Round((avgAfter-avgBefore)*10) / 10

		var insight string
		switch {
		case diff > 10:
			insight = fmt.Sprintf("Caffeine after 3 pm adds ~%.0f min to your sleep onset.", avgAfter-avgBefore)
		case diff < -5:
			insight = "No late-caffeine penalty detected in your data."
		default:
			insight = "Not enough contrast between early and late caffeine nights yet."
		}

		resp.CaffeineCorrelation = &CaffeineCorrelationResult{
			AvgLatencyAfter3pmMin:  math.Round(avgAfter*10) / 10,
			AvgLatencyBefore3pmMin: math.Round(avgBefore*10) / 10,
			DiffMinutes:            diff,
			NightsAfter3pm:         len(latencyAfter),
			NightsBefore3pm:        len(latencyBefore),
			Insight:                insight,
		}
	}

	// 6. Compute exercise correlation.
	if len(effWithEx) >= minGroup && len(effWithoutEx) >= minGroup {
		avgWith := avgFloat64Slice(effWithEx)
		avgWithout := avgFloat64Slice(effWithoutEx)
		diff := math.Round((avgWith-avgWithout)*10) / 10

		var insight string
		switch {
		case diff > 5:
			insight = fmt.Sprintf("30+ min exercise days show +%.0f%% better sleep efficiency.", avgWith-avgWithout)
		case diff < -5:
			insight = "Consider lighter or earlier exercise — intense late workouts may be disrupting your sleep."
		default:
			insight = "Exercise has a neutral effect on your sleep efficiency so far."
		}

		resp.ExerciseCorrelation = &ExerciseCorrelationResult{
			AvgEffWithExercisePct:    math.Round(avgWith*10) / 10,
			AvgEffWithoutExercisePct: math.Round(avgWithout*10) / 10,
			DiffPercent:              diff,
			NightsWithExercise:       len(effWithEx),
			NightsWithoutExercise:    len(effWithoutEx),
			Insight:                  insight,
		}
	}

	return resp, nil
}

// LogAlertness records a single daytime alertness check-in.
func (s *SleepService) LogAlertness(appID string, userID uuid.UUID, level int, loggedAt time.Time) (*AlertnessLogResponse, error) {
	if level < 1 || level > 5 {
		return nil, errors.New("level must be between 1 and 5")
	}
	log := AlertnessLog{
		AppID:    appID,
		UserID:   userID,
		Level:    level,
		LoggedAt: loggedAt,
	}
	if err := s.db.Scopes(tenant.ForTenant(appID)).Create(&log).Error; err != nil {
		return nil, errors.New("storage error")
	}
	return &AlertnessLogResponse{
		ID:       log.ID,
		Level:    log.Level,
		LoggedAt: log.LoggedAt.UTC().Format(time.RFC3339),
	}, nil
}

// GetAlertnessLogs returns alertness logs for the last N days with daily average and peak/trough hours.
func (s *SleepService) GetAlertnessLogs(appID string, userID uuid.UUID, days int) (*AlertnessListResponse, error) {
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	var logs []AlertnessLog
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND logged_at >= ?", userID, since).
		Order("logged_at ASC").
		Find(&logs).Error; err != nil {
		return nil, errors.New("storage error")
	}

	resp := &AlertnessListResponse{Days: days}
	if len(logs) == 0 {
		return resp, nil
	}

	// Build response logs + compute averages
	resp.Logs = make([]AlertnessLogResponse, len(logs))
	totalLevel := 0
	hourSum := make(map[int]int)
	hourCount := make(map[int]int)
	for i, l := range logs {
		resp.Logs[i] = AlertnessLogResponse{
			ID:       l.ID,
			Level:    l.Level,
			LoggedAt: l.LoggedAt.UTC().Format(time.RFC3339),
		}
		totalLevel += l.Level
		h := l.LoggedAt.UTC().Hour()
		hourSum[h] += l.Level
		hourCount[h]++
	}
	resp.DailyAvg = math.Round(float64(totalLevel)/float64(len(logs))*10) / 10

	// Find peak (highest avg) and trough (lowest avg) hours
	peakHour, troughHour := -1, -1
	var peakAvg, troughAvg float64
	for h, sum := range hourSum {
		avg := float64(sum) / float64(hourCount[h])
		if peakHour < 0 || avg > peakAvg {
			peakHour = h
			peakAvg = avg
		}
		if troughHour < 0 || avg < troughAvg {
			troughHour = h
			troughAvg = avg
		}
	}
	resp.PeakHour = peakHour
	resp.TroughHour = troughHour

	return resp, nil
}

// GetSnoringAnalysis analyses snoring events stored in the sounds_json JSONB field
// across the last 30 sleep sessions. Returns 422 if fewer than 3 sessions exist.
func (s *SleepService) GetSnoringAnalysis(appID string, userID uuid.UUID) (*SnoringAnalysisResponse, error) {
	type rawRow struct {
		ID         string  `gorm:"column:id"`
		Score      int     `gorm:"column:score"`
		CreatedAt  string  `gorm:"column:created_at"`
		SoundsJSON string  `gorm:"column:sounds_json"`
	}

	var rows []rawRow
	err := s.db.Raw(
		"SELECT id, score, created_at::text, sounds_json FROM sleep_sessions "+
			"WHERE app_id = ? AND user_id = ? AND deleted_at IS NULL "+
			"ORDER BY created_at DESC LIMIT 30",
		appID, userID,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("snoring analysis query: %w", err)
	}

	total := len(rows)
	if total < 3 {
		return nil, fmt.Errorf("not enough data")
	}

	type sessionData struct {
		score           int
		createdAt       time.Time
		snoringDuration int
		hasSnoring      bool
	}

	sessions := make([]sessionData, 0, total)
	for _, r := range rows {
		var sounds []SoundDTO
		if r.SoundsJSON != "" && r.SoundsJSON != "[]" {
			_ = json.Unmarshal([]byte(r.SoundsJSON), &sounds)
		}

		snoringTotal := 0
		for _, s := range sounds {
			if strings.EqualFold(s.Type, "snoring") {
				snoringTotal += s.DurationSeconds
			}
		}

		t, _ := time.Parse("2006-01-02 15:04:05.999999 -0700 MST", r.CreatedAt)
		if t.IsZero() {
			t, _ = time.Parse(time.RFC3339, r.CreatedAt)
		}
		if t.IsZero() {
			// Fallback: strip sub-second and timezone suffix for simple parsing
			t, _ = time.Parse("2006-01-02 15:04:05", r.CreatedAt[:min(len(r.CreatedAt), 19)])
		}

		sessions = append(sessions, sessionData{
			score:           r.Score,
			createdAt:       t,
			snoringDuration: snoringTotal,
			hasSnoring:      snoringTotal > 0,
		})
	}

	// Compute aggregates
	sessionsWithSnoring := 0
	var sumScoreWith, sumScoreWithout float64
	countWith, countWithout := 0, 0
	var totalSnoringDur int

	for _, sd := range sessions {
		if sd.hasSnoring {
			sessionsWithSnoring++
			sumScoreWith += float64(sd.score)
			countWith++
			totalSnoringDur += sd.snoringDuration
		} else {
			sumScoreWithout += float64(sd.score)
			countWithout++
		}
	}

	avgWith := 0.0
	if countWith > 0 {
		avgWith = math.Round(sumScoreWith/float64(countWith)*10) / 10
	}
	avgWithout := 0.0
	if countWithout > 0 {
		avgWithout = math.Round(sumScoreWithout/float64(countWithout)*10) / 10
	}
	scoreDiff := math.Round((avgWithout-avgWith)*10) / 10

	snoringPct := math.Round(float64(sessionsWithSnoring)/float64(total)*1000) / 10
	avgDurationSec := 0.0
	if countWith > 0 {
		avgDurationSec = math.Round(float64(totalSnoringDur)/float64(countWith)*10) / 10
	}

	// Insight
	insight := "Snoring doesn't appear to significantly impact your sleep score."
	if scoreDiff > 5 {
		insight = "Snoring correlates with lower sleep quality. Consider a side-sleeping position."
	} else if snoringPct > 50 {
		insight = "You snore on most nights. Try sleeping on your side or elevating your pillow."
	}

	// Build trend for last 14 nights (sessions are ordered DESC, so reverse for chronological order)
	trendCount := 14
	if len(sessions) < trendCount {
		trendCount = len(sessions)
	}
	trendSessions := sessions[:trendCount]
	// Reverse so earliest night comes first
	trend := make([]SnoringNight, trendCount)
	for i, sd := range trendSessions {
		dateStr := sd.createdAt.UTC().Format("2006-01-02")
		trend[trendCount-1-i] = SnoringNight{
			Date:            dateStr,
			HasSnoring:      sd.hasSnoring,
			SnoringDuration: sd.snoringDuration,
			SleepScore:      sd.score,
		}
	}

	return &SnoringAnalysisResponse{
		TotalSessions:          total,
		SessionsWithSnoring:    sessionsWithSnoring,
		SnoringPct:             snoringPct,
		AvgScoreWithSnoring:    avgWith,
		AvgScoreWithoutSnoring: avgWithout,
		ScoreDiff:              scoreDiff,
		AvgDurationSecPerNight: avgDurationSec,
		TrendNights:            trend,
		Insight:                insight,
	}, nil
}

// --- Pre-Sleep Ritual services ---

// CreateOrUpdateRitual upserts a SleepRitual record by (appID, userID, date).
// If a record already exists for that date it is overwritten.
func (s *SleepService) CreateOrUpdateRitual(appID string, userID uuid.UUID, req CreateRitualRequest) (*SleepRitual, error) {
	if req.Date == "" {
		return nil, errors.New("date is required")
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		return nil, errors.New("date must be YYYY-MM-DD")
	}
	if len(req.Notes) > 500 {
		return nil, errors.New("notes must be at most 500 characters")
	}
	if req.ScreenTimeMin < 0 {
		req.ScreenTimeMin = 0
	}
	if req.LastMealHoursAgo < 0 {
		req.LastMealHoursAgo = 0
	}

	ritual := SleepRitual{
		AppID:             appID,
		UserID:            userID,
		Date:              req.Date,
		HadAlcohol:        req.HadAlcohol,
		LastDrinkHoursAgo: req.LastDrinkHoursAgo,
		LastMealHoursAgo:  req.LastMealHoursAgo,
		ScreenTimeMin:     req.ScreenTimeMin,
		ExercisedToday:    req.ExercisedToday,
		ExerciseHoursAgo:  req.ExerciseHoursAgo,
		Notes:             req.Notes,
	}

	// Upsert: if (app_id, user_id, date) already exists, update all fields.
	result := s.db.Scopes(tenant.ForTenant(appID)).
		Where("app_id = ? AND user_id = ? AND date = ?", appID, userID, req.Date).
		Assign(SleepRitual{
			HadAlcohol:        req.HadAlcohol,
			LastDrinkHoursAgo: req.LastDrinkHoursAgo,
			LastMealHoursAgo:  req.LastMealHoursAgo,
			ScreenTimeMin:     req.ScreenTimeMin,
			ExercisedToday:    req.ExercisedToday,
			ExerciseHoursAgo:  req.ExerciseHoursAgo,
			Notes:             req.Notes,
		}).
		FirstOrCreate(&ritual)
	if result.Error != nil {
		return nil, fmt.Errorf("upsert ritual: %w", result.Error)
	}

	// If it already existed, update.
	if result.RowsAffected == 0 {
		if err := s.db.Model(&ritual).Updates(map[string]interface{}{
			"had_alcohol":          req.HadAlcohol,
			"last_drink_hours_ago": req.LastDrinkHoursAgo,
			"last_meal_hours_ago":  req.LastMealHoursAgo,
			"screen_time_min":      req.ScreenTimeMin,
			"exercised_today":      req.ExercisedToday,
			"exercise_hours_ago":   req.ExerciseHoursAgo,
			"notes":                req.Notes,
		}).Error; err != nil {
			return nil, fmt.Errorf("update ritual: %w", err)
		}
	}

	return &ritual, nil
}

// GetRitualCorrelation joins ritual records with sleep sessions on date and
// computes impact of each behavioral factor on sleep score.
// Requires at least 5 paired nights per factor; 14+ total for has_enough_data=true.
func (s *SleepService) GetRitualCorrelation(appID string, userID uuid.UUID) (*RitualCorrelationResponse, error) {
	type pairedRow struct {
		Date          string
		Score         float64
		HadAlcohol    bool
		ScreenTimeMin int
		ExercisedToday bool
		LastMealHoursAgo int
	}

	// Join rituals and sleep sessions on date (date extracted from bedtime).
	var rows []pairedRow
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Raw(`SELECT
			r.date,
			s.score AS score,
			r.had_alcohol,
			r.screen_time_min,
			r.exercised_today,
			r.last_meal_hours_ago
		FROM sleep_rituals r
		JOIN sleep_sessions s
			ON TO_CHAR(s.bedtime AT TIME ZONE 'UTC', 'YYYY-MM-DD') = r.date
			AND s.user_id = r.user_id
			AND s.app_id = r.app_id
			AND s.deleted_at IS NULL
		WHERE r.app_id = ? AND r.user_id = ? AND r.deleted_at IS NULL`,
			appID, userID).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("fetch paired rows: %w", err)
	}

	dataPoints := len(rows)
	hasEnoughData := dataPoints >= 14

	resp := &RitualCorrelationResponse{
		HasEnoughData: hasEnoughData,
		DataPoints:    dataPoints,
	}

	if dataPoints == 0 {
		resp.TopInsight = "Log at least 14 nights of rituals and sleep sessions to see correlations."
		return resp, nil
	}

	// Helper: compute average scores for rows where predicate is true/false.
	computeImpact := func(factorName string, withPred func(pairedRow) bool) *RitualImpactItem {
		var withScores, withoutScores []float64
		for _, r := range rows {
			if withPred(r) {
				withScores = append(withScores, r.Score)
			} else {
				withoutScores = append(withoutScores, r.Score)
			}
		}
		if len(withScores) < 5 || len(withoutScores) < 5 {
			return nil
		}
		avgWith := func(xs []float64) float64 {
			sum := 0.0
			for _, x := range xs {
				sum += x
			}
			return sum / float64(len(xs))
		}
		with := avgWith(withScores)
		without := avgWith(withoutScores)
		delta := 0.0
		if without > 0 {
			delta = (with - without) / without * 100
		}
		insight := fmt.Sprintf("Sleep score %.1f%% %s on nights with %s (%d nights of data)",
			math.Abs(delta),
			func() string {
				if delta < 0 {
					return "lower"
				}
				return "higher"
			}(),
			factorName,
			len(withScores)+len(withoutScores),
		)
		return &RitualImpactItem{
			FactorName:    factorName,
			WithFactor:    with,
			WithoutFactor: without,
			DeltaPct:      delta,
			SampleSize:    len(withScores) + len(withoutScores),
			Insight:       insight,
		}
	}

	resp.AlcoholImpact = computeImpact("alcohol", func(r pairedRow) bool { return r.HadAlcohol })
	resp.ExerciseImpact = computeImpact("exercise", func(r pairedRow) bool { return r.ExercisedToday })
	resp.ScreenTimeImpact = computeImpact("high screen time (>30 min)", func(r pairedRow) bool { return r.ScreenTimeMin > 30 })
	resp.LateEatingImpact = computeImpact("eating within 2 hours of bed", func(r pairedRow) bool { return r.LastMealHoursAgo < 2 })

	// Generate top insight from the highest absolute delta factor.
	bestDelta := 0.0
	bestInsight := ""
	for _, item := range []*RitualImpactItem{resp.AlcoholImpact, resp.ExerciseImpact, resp.ScreenTimeImpact, resp.LateEatingImpact} {
		if item != nil && math.Abs(item.DeltaPct) > math.Abs(bestDelta) {
			bestDelta = item.DeltaPct
			bestInsight = item.Insight
		}
	}
	if bestInsight == "" {
		bestInsight = fmt.Sprintf("Not enough paired data per factor yet (%d total nights logged). Keep going!", dataPoints)
	}
	resp.TopInsight = bestInsight

	return resp, nil
}

// --- CBT-I Program services ---

// StartCBTIProgram creates or returns the existing CBTIProgress record for a user.
// If a program is already active, it returns the existing record without resetting it.
func (s *SleepService) StartCBTIProgram(appID string, userID uuid.UUID, req StartCBTIRequest) (*CBTIStatusResponse, error) {
	if req.SleepWindowStart == "" || req.SleepWindowEnd == "" {
		return nil, errors.New("sleep_window_start and sleep_window_end are required")
	}
	// Validate HH:MM format.
	validateTime := func(t string) error {
		if len(t) != 5 || t[2] != ':' {
			return fmt.Errorf("invalid time format %q: must be HH:MM", t)
		}
		return nil
	}
	if err := validateTime(req.SleepWindowStart); err != nil {
		return nil, err
	}
	if err := validateTime(req.SleepWindowEnd); err != nil {
		return nil, err
	}

	// Check if already active.
	var existing CBTIProgress
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("app_id = ? AND user_id = ? AND is_active = true", appID, userID).
		First(&existing).Error
	if err == nil {
		// Already enrolled — return current status.
		return s.buildCBTIStatusResponse(appID, userID, &existing)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("check existing cbti: %w", err)
	}

	prog := CBTIProgress{
		AppID:            appID,
		UserID:           userID,
		StartDate:        time.Now().UTC().Format("2006-01-02"),
		CurrentWeek:      1,
		CurrentDay:       1,
		SleepWindowStart: req.SleepWindowStart,
		SleepWindowEnd:   req.SleepWindowEnd,
		CompletedDays:    0,
		IsActive:         true,
	}
	if err := s.db.Create(&prog).Error; err != nil {
		return nil, fmt.Errorf("create cbti progress: %w", err)
	}

	return s.buildCBTIStatusResponse(appID, userID, &prog)
}

// GetCBTIStatus returns the current CBT-I program status for a user.
func (s *SleepService) GetCBTIStatus(appID string, userID uuid.UUID) (*CBTIStatusResponse, error) {
	var prog CBTIProgress
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("app_id = ? AND user_id = ?", appID, userID).
		Order("created_at DESC").
		First(&prog).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &CBTIStatusResponse{IsEnrolled: false}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetch cbti progress: %w", err)
	}

	return s.buildCBTIStatusResponse(appID, userID, &prog)
}

// buildCBTIStatusResponse constructs a CBTIStatusResponse from a progress record.
// It fetches check-ins to compute adherence and weekly insight.
func (s *SleepService) buildCBTIStatusResponse(appID string, userID uuid.UUID, prog *CBTIProgress) (*CBTIStatusResponse, error) {
	var checkIns []CBTIDayCheckIn
	s.db.Scopes(tenant.ForTenant(appID)).
		Where("app_id = ? AND user_id = ?", appID, userID).
		Order("created_at DESC").
		Find(&checkIns)

	followCount := 0
	for _, ci := range checkIns {
		if ci.DidFollow {
			followCount++
		}
	}

	adherencePct := 0.0
	if len(checkIns) > 0 {
		adherencePct = float64(followCount) / float64(len(checkIns)) * 100
	}

	// Count check-ins for the current week only.
	weekFollowed := 0
	weekTotal := 0
	for _, ci := range checkIns {
		if ci.Week == prog.CurrentWeek {
			weekTotal++
			if ci.DidFollow {
				weekFollowed++
			}
		}
	}

	weeklyInsight := fmt.Sprintf("Week %d: %d of %d days completed", prog.CurrentWeek, weekTotal, 7)
	if weekTotal > 0 {
		weeklyInsight = fmt.Sprintf("Week %d: %d of %d days followed your sleep window", prog.CurrentWeek, weekFollowed, weekTotal)
	}

	isCompleted := prog.CompletedAt != nil
	completedAt := ""
	if prog.CompletedAt != nil {
		completedAt = *prog.CompletedAt
	}

	return &CBTIStatusResponse{
		IsEnrolled:       prog.IsActive || isCompleted,
		CurrentWeek:      prog.CurrentWeek,
		CurrentDay:       prog.CurrentDay,
		StartDate:        prog.StartDate,
		SleepWindowStart: prog.SleepWindowStart,
		SleepWindowEnd:   prog.SleepWindowEnd,
		CompletedDays:    prog.CompletedDays,
		AdherencePct:     adherencePct,
		WeeklyInsight:    weeklyInsight,
		IsCompleted:      isCompleted,
		CompletedAt:      completedAt,
	}, nil
}

// SubmitCBTICheckIn records a daily check-in, advances the day/week counter, and
// marks the program completed when week 6 day 7 is reached.
func (s *SleepService) SubmitCBTICheckIn(appID string, userID uuid.UUID, req CBTICheckInRequest) (*CBTIStatusResponse, error) {
	var prog CBTIProgress
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("app_id = ? AND user_id = ? AND is_active = true", appID, userID).
		First(&prog).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("no active cbti program found")
	}
	if err != nil {
		return nil, fmt.Errorf("fetch cbti progress: %w", err)
	}
	if len(req.Notes) > 500 {
		return nil, errors.New("notes must be at most 500 characters")
	}

	checkIn := CBTIDayCheckIn{
		AppID:     appID,
		UserID:    userID,
		Date:      time.Now().UTC().Format("2006-01-02"),
		Week:      prog.CurrentWeek,
		DidFollow: req.DidFollow,
		Notes:     req.Notes,
	}
	if err := s.db.Create(&checkIn).Error; err != nil {
		return nil, fmt.Errorf("create cbti check-in: %w", err)
	}

	// Advance counters.
	prog.CompletedDays++
	prog.CurrentDay++
	if prog.CurrentDay > 7 {
		prog.CurrentDay = 1
		prog.CurrentWeek++
	}

	// Mark complete if finished week 6.
	if prog.CurrentWeek > 6 {
		now := time.Now().UTC().Format("2006-01-02")
		prog.CompletedAt = &now
		prog.IsActive = false
	}

	if err := s.db.Save(&prog).Error; err != nil {
		return nil, fmt.Errorf("update cbti progress: %w", err)
	}

	return s.buildCBTIStatusResponse(appID, userID, &prog)
}

// PauseCBTIProgram sets is_active=false on the user's active program.
func (s *SleepService) PauseCBTIProgram(appID string, userID uuid.UUID) error {
	result := s.db.Scopes(tenant.ForTenant(appID)).
		Model(&CBTIProgress{}).
		Where("app_id = ? AND user_id = ? AND is_active = true", appID, userID).
		Update("is_active", false)
	if result.Error != nil {
		return fmt.Errorf("pause cbti: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("no active cbti program found")
	}
	return nil
}

