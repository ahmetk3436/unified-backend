package moodpulse

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalidIntensity = errors.New("intensity must be between 1 and 10")
	ErrMissingEmotion   = errors.New("emotion is required")
	ErrNotFound         = errors.New("mood entry not found")
	ErrNotOwner         = errors.New("not the owner of this entry")
)

type MoodService struct {
	db *gorm.DB
}

func NewMoodService(db *gorm.DB) *MoodService {
	return &MoodService{db: db}
}

func (s *MoodService) Create(appID string, userID uuid.UUID, req CreateMoodRequest) (*MoodEntryResponse, error) {
	if req.Emotion.ID == "" || req.Emotion.Name == "" {
		return nil, ErrMissingEmotion
	}
	if req.Intensity < 1 || req.Intensity > 10 {
		return nil, ErrInvalidIntensity
	}

	triggersJSON, _ := json.Marshal(req.Triggers)
	activitiesJSON, _ := json.Marshal(req.Activities)

	entry := MoodCheckIn{
		AppID:          appID,
		UserID:         userID,
		EmotionID:      req.Emotion.ID,
		EmotionName:    req.Emotion.Name,
		EmotionEmoji:   req.Emotion.Emoji,
		EmotionColor:   req.Emotion.Color,
		EmotionCustom:  req.Emotion.IsCustom,
		Intensity:      req.Intensity,
		Note:           req.Note,
		TriggersJSON:   string(triggersJSON),
		ActivitiesJSON: string(activitiesJSON),
	}

	if err := s.db.Create(&entry).Error; err != nil {
		return nil, fmt.Errorf("create failed: %w", err)
	}

	// Update streak
	go s.updateStreak(appID, userID)

	return s.toResponse(entry), nil
}

func (s *MoodService) List(appID string, userID uuid.UUID, limit, offset int) (*MoodListResponse, error) {
	var entries []MoodCheckIn
	var total int64

	base := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID)
	base.Model(&MoodCheckIn{}).Count(&total)

	if err := base.Order("created_at DESC").Limit(limit).Offset(offset).Find(&entries).Error; err != nil {
		return nil, err
	}

	resp := &MoodListResponse{
		Entries: make([]MoodEntryResponse, len(entries)),
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}
	for i, e := range entries {
		resp.Entries[i] = *s.toResponse(e)
	}
	return resp, nil
}

func (s *MoodService) Get(appID string, userID uuid.UUID, id uuid.UUID) (*MoodEntryResponse, error) {
	var entry MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", id).First(&entry).Error; err != nil {
		return nil, ErrNotFound
	}
	if entry.UserID != userID {
		return nil, ErrNotOwner
	}
	return s.toResponse(entry), nil
}

func (s *MoodService) Update(appID string, userID uuid.UUID, id uuid.UUID, req UpdateMoodRequest) (*MoodEntryResponse, error) {
	var entry MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", id).First(&entry).Error; err != nil {
		return nil, ErrNotFound
	}
	if entry.UserID != userID {
		return nil, ErrNotOwner
	}

	if req.Emotion != nil {
		if req.Emotion.ID == "" || req.Emotion.Name == "" {
			return nil, ErrMissingEmotion
		}
		entry.EmotionID = req.Emotion.ID
		entry.EmotionName = req.Emotion.Name
		entry.EmotionEmoji = req.Emotion.Emoji
		entry.EmotionColor = req.Emotion.Color
		entry.EmotionCustom = req.Emotion.IsCustom
	}
	if req.Intensity != nil {
		if *req.Intensity < 1 || *req.Intensity > 10 {
			return nil, ErrInvalidIntensity
		}
		entry.Intensity = *req.Intensity
	}
	if req.Note != nil {
		entry.Note = *req.Note
	}
	if req.Triggers != nil {
		j, _ := json.Marshal(*req.Triggers)
		entry.TriggersJSON = string(j)
	}
	if req.Activities != nil {
		j, _ := json.Marshal(*req.Activities)
		entry.ActivitiesJSON = string(j)
	}

	if err := s.db.Save(&entry).Error; err != nil {
		return nil, fmt.Errorf("update failed: %w", err)
	}

	return s.toResponse(entry), nil
}

func (s *MoodService) Delete(appID string, userID uuid.UUID, id uuid.UUID) error {
	var entry MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", id).First(&entry).Error; err != nil {
		return ErrNotFound
	}
	if entry.UserID != userID {
		return ErrNotOwner
	}
	return s.db.Delete(&entry).Error
}

func (s *MoodService) Search(appID string, userID uuid.UUID, q string) (*SearchMoodResponse, error) {
	q = strings.TrimSpace(q)
	pattern := "%" + strings.ToLower(q) + "%"

	var entries []MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Where("LOWER(note) LIKE ? OR LOWER(emotion_name) LIKE ? OR LOWER(triggers_json) LIKE ? OR LOWER(activities_json) LIKE ?",
			pattern, pattern, pattern, pattern).
		Order("created_at DESC").
		Limit(50).
		Find(&entries).Error; err != nil {
		return nil, err
	}

	resp := &SearchMoodResponse{
		Entries: make([]MoodEntryResponse, len(entries)),
		Total:   int64(len(entries)),
		Query:   q,
	}
	for i, e := range entries {
		resp.Entries[i] = *s.toResponse(e)
	}
	return resp, nil
}

func (s *MoodService) GetStreak(appID string, userID uuid.UUID) (*StreakResponse, error) {
	var streak MoodStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &StreakResponse{}, nil
	}
	if err != nil {
		return nil, err
	}

	lastDate := ""
	if !streak.LastEntryDate.IsZero() {
		lastDate = streak.LastEntryDate.Format(time.RFC3339)
	}

	return &StreakResponse{
		CurrentStreak: streak.CurrentStreak,
		LongestStreak: streak.LongestStreak,
		TotalEntries:  streak.TotalEntries,
		LastEntryDate: lastDate,
	}, nil
}

func (s *MoodService) GetStats(appID string, userID uuid.UUID, days int) (*StatsResponse, error) {
	since := time.Now().AddDate(0, 0, -days)

	var entries []MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at DESC").
		Find(&entries).Error; err != nil {
		return nil, err
	}

	resp := &StatsResponse{
		TotalCheckIns:    len(entries),
		EmotionBreakdown: make(map[string]int),
		DayOfWeekPattern: make(map[string]int),
		TimeOfDayPattern: make(map[string]float64),
	}

	if len(entries) == 0 {
		return resp, nil
	}

	var totalIntensity float64
	emotionCounts := make(map[string]int)
	emotionEmojis := make(map[string]string)
	triggerCounts := make(map[string]int)
	timeSlots := map[string]struct{ total float64; count int }{
		"Morning":   {},
		"Afternoon": {},
		"Evening":   {},
		"Night":     {},
	}

	for _, e := range entries {
		totalIntensity += float64(e.Intensity)
		emotionCounts[e.EmotionName]++
		emotionEmojis[e.EmotionName] = e.EmotionEmoji
		resp.EmotionBreakdown[e.EmotionName]++

		day := e.CreatedAt.Weekday().String()
		resp.DayOfWeekPattern[day]++

		hour := e.CreatedAt.Hour()
		var slot string
		switch {
		case hour < 12:
			slot = "Morning"
		case hour < 17:
			slot = "Afternoon"
		case hour < 21:
			slot = "Evening"
		default:
			slot = "Night"
		}
		ts := timeSlots[slot]
		ts.total += float64(e.Intensity)
		ts.count++
		timeSlots[slot] = ts

		var triggers []TagItem
		_ = json.Unmarshal([]byte(e.TriggersJSON), &triggers)
		for _, t := range triggers {
			triggerCounts[t.Name]++
		}
	}

	resp.AverageIntensity = totalIntensity / float64(len(entries))

	// Top emotion
	maxCount := 0
	for name, count := range emotionCounts {
		if count > maxCount {
			maxCount = count
			resp.TopEmotion = name
			resp.TopEmotionEmoji = emotionEmojis[name]
		}
	}

	// Time of day averages
	for slot, data := range timeSlots {
		if data.count > 0 {
			resp.TimeOfDayPattern[slot] = data.total / float64(data.count)
		}
	}

	// Top triggers
	type kv struct {
		k string
		v int
	}
	var sorted []kv
	for k, v := range triggerCounts {
		sorted = append(sorted, kv{k, v})
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].v > sorted[i].v {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	limit := 5
	if len(sorted) < limit {
		limit = len(sorted)
	}
	for _, kv := range sorted[:limit] {
		resp.TopTriggers = append(resp.TopTriggers, TriggerStat{Name: kv.k, Count: kv.v})
	}

	return resp, nil
}

// updateStreak recalculates the user's streak after a new entry.
func (s *MoodService) updateStreak(appID string, userID uuid.UUID) {
	var streak MoodStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error

	now := time.Now().Truncate(24 * time.Hour)

	if errors.Is(err, gorm.ErrRecordNotFound) {
		streak = MoodStreak{
			AppID:         appID,
			UserID:        userID,
			CurrentStreak: 1,
			LongestStreak: 1,
			TotalEntries:  1,
			LastEntryDate: now,
		}
		s.db.Create(&streak)
		return
	}

	streak.TotalEntries++
	lastDate := streak.LastEntryDate.Truncate(24 * time.Hour)

	switch {
	case now.Equal(lastDate):
		// Same day, just increment total
	case now.Sub(lastDate) <= 48*time.Hour && now.Sub(lastDate) > 0:
		// Next day (or within 48h window to be safe)
		streak.CurrentStreak++
	default:
		// Streak broken
		streak.CurrentStreak = 1
	}

	if streak.CurrentStreak > streak.LongestStreak {
		streak.LongestStreak = streak.CurrentStreak
	}
	streak.LastEntryDate = now

	s.db.Save(&streak)
}

func (s *MoodService) toResponse(e MoodCheckIn) *MoodEntryResponse {
	var triggers []TagItem
	var activities []TagItem
	_ = json.Unmarshal([]byte(e.TriggersJSON), &triggers)
	_ = json.Unmarshal([]byte(e.ActivitiesJSON), &activities)

	if triggers == nil {
		triggers = []TagItem{}
	}
	if activities == nil {
		activities = []TagItem{}
	}

	return &MoodEntryResponse{
		ID: e.ID,
		Emotion: EmotionDTO{
			ID:       e.EmotionID,
			Name:     e.EmotionName,
			Emoji:    e.EmotionEmoji,
			Color:    e.EmotionColor,
			IsCustom: e.EmotionCustom,
		},
		Intensity:  e.Intensity,
		Note:       e.Note,
		Triggers:   triggers,
		Activities: activities,
		CreatedAt:  e.CreatedAt.Format(time.RFC3339),
	}
}
