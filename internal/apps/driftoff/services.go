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
	aiModel      string
	aiTimeout    time.Duration
	coachCache   map[string]coachCacheEntry
	coachCacheMu sync.Mutex
}

func NewSleepService(db *gorm.DB, cfg *config.Config) *SleepService {
	model := cfg.OpenAIModel
	if model == "" {
		model = "gpt-4o-mini"
	}
	timeout := cfg.AITimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &SleepService{
		db:         db,
		aiAPIKey:   cfg.OpenAIAPIKey,
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

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonData))
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
		return "", err
	}

	s.coachCacheMu.Lock()
	s.coachCache[cacheKey] = coachCacheEntry{content: content, cachedAt: time.Now()}
	s.coachCacheMu.Unlock()

	return content, nil
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
