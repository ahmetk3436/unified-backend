package driftoff

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalidScore    = errors.New("score must be between 0 and 100")
	ErrInvalidDuration = errors.New("duration must be positive")
	ErrMissingTimes    = errors.New("bedtime and wake_time are required")
	ErrNotFound        = errors.New("sleep session not found")
	ErrNotOwner        = errors.New("not the owner of this session")
)

type SleepService struct {
	db *gorm.DB
}

func NewSleepService(db *gorm.DB) *SleepService {
	return &SleepService{db: db}
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

	phasesJSON, _ := json.Marshal(req.Phases)
	soundsJSON, _ := json.Marshal(req.Sounds)

	session := SleepSession{
		AppID:           appID,
		UserID:          userID,
		Score:           req.Score,
		DurationMinutes: req.DurationMinutes,
		Efficiency:      req.Efficiency,
		LatencyMinutes:  req.LatencyMinutes,
		Bedtime:         bedtime,
		WakeTime:        wakeTime,
		PhasesJSON:      string(phasesJSON),
		SoundsJSON:      string(soundsJSON),
		AlarmPhase:      req.AlarmPhase,
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
		ID:              sess.ID,
		Score:           sess.Score,
		DurationMinutes: sess.DurationMinutes,
		Efficiency:      sess.Efficiency,
		LatencyMinutes:  sess.LatencyMinutes,
		Bedtime:         sess.Bedtime.Format(time.RFC3339),
		WakeTime:        sess.WakeTime.Format(time.RFC3339),
		Phases:          phases,
		Sounds:          sounds,
		AlarmPhase:      sess.AlarmPhase,
		CreatedAt:       sess.CreatedAt.Format(time.RFC3339),
	}

	if sess.AlarmTime != nil {
		t := sess.AlarmTime.Format(time.RFC3339)
		resp.AlarmTime = &t
	}

	return resp
}
