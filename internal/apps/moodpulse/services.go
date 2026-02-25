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

func (s *MoodService) List(appID string, userID uuid.UUID, limit, offset, month, year int) (*MoodListResponse, error) {
	var entries []MoodCheckIn
	var total int64

	base := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID)

	// Optional month/year filter for calendar efficiency
	if month >= 1 && month <= 12 && year >= 2000 {
		start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		base = base.Where("created_at >= ? AND created_at < ?", start, end)
	}

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

func (s *MoodService) Calendar(appID string, userID uuid.UUID, month, year int) (*CalendarResponse, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	var entries []MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
		Select("id, emotion_color, emotion_emoji, created_at").
		Order("created_at ASC").
		Find(&entries).Error; err != nil {
		return nil, err
	}

	resp := &CalendarResponse{
		Entries: make([]CalendarEntry, len(entries)),
		Month:   month,
		Year:    year,
	}
	for i, e := range entries {
		resp.Entries[i] = CalendarEntry{
			ID:    e.ID,
			Date:  e.CreatedAt.Format("2006-01-02"),
			Color: e.EmotionColor,
			Emoji: e.EmotionEmoji,
		}
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

// VocabularyService handles custom vocabulary CRUD.
type VocabularyService struct {
	db *gorm.DB
}

func NewVocabularyService(db *gorm.DB) *VocabularyService {
	return &VocabularyService{db: db}
}

// --- Emotions ---

func (s *VocabularyService) ListEmotions(appID string, userID uuid.UUID) ([]CustomEmotion, error) {
	var items []CustomEmotion
	if err := s.db.Where("app_id = ? AND user_id = ?", appID, userID).Order("created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *VocabularyService) UpsertEmotion(appID string, userID uuid.UUID, req CreateCustomEmotionRequest) (*CustomEmotion, error) {
	if req.Name == "" || req.Emoji == "" || req.Color == "" {
		return nil, errors.New("name, emoji, and color are required")
	}
	item := CustomEmotion{
		AppID:  appID,
		UserID: userID,
		Name:   req.Name,
		Emoji:  req.Emoji,
		Color:  req.Color,
	}
	result := s.db.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, req.Name).First(&item)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := s.db.Create(&item).Error; err != nil {
			return nil, err
		}
	} else {
		item.Emoji = req.Emoji
		item.Color = req.Color
		if err := s.db.Save(&item).Error; err != nil {
			return nil, err
		}
	}
	return &item, nil
}

func (s *VocabularyService) DeleteEmotion(appID string, userID uuid.UUID, id uuid.UUID) error {
	result := s.db.Where("app_id = ? AND user_id = ? AND id = ?", appID, userID, id).Delete(&CustomEmotion{})
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return result.Error
}

// --- Triggers ---

func (s *VocabularyService) ListTriggers(appID string, userID uuid.UUID) ([]CustomTrigger, error) {
	var items []CustomTrigger
	if err := s.db.Where("app_id = ? AND user_id = ?", appID, userID).Order("created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *VocabularyService) UpsertTrigger(appID string, userID uuid.UUID, req CreateCustomTriggerRequest) (*CustomTrigger, error) {
	if req.Name == "" {
		return nil, errors.New("name is required")
	}
	if req.Icon == "" {
		req.Icon = "flash-outline"
	}
	item := CustomTrigger{
		AppID:  appID,
		UserID: userID,
		Name:   req.Name,
		Icon:   req.Icon,
	}
	result := s.db.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, req.Name).First(&item)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := s.db.Create(&item).Error; err != nil {
			return nil, err
		}
	} else {
		item.Icon = req.Icon
		if err := s.db.Save(&item).Error; err != nil {
			return nil, err
		}
	}
	return &item, nil
}

func (s *VocabularyService) DeleteTrigger(appID string, userID uuid.UUID, id uuid.UUID) error {
	result := s.db.Where("app_id = ? AND user_id = ? AND id = ?", appID, userID, id).Delete(&CustomTrigger{})
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return result.Error
}

// --- Activities ---

func (s *VocabularyService) ListActivities(appID string, userID uuid.UUID) ([]CustomActivity, error) {
	var items []CustomActivity
	if err := s.db.Where("app_id = ? AND user_id = ?", appID, userID).Order("created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *VocabularyService) UpsertActivity(appID string, userID uuid.UUID, req CreateCustomActivityRequest) (*CustomActivity, error) {
	if req.Name == "" {
		return nil, errors.New("name is required")
	}
	if req.Icon == "" {
		req.Icon = "ellipse-outline"
	}
	item := CustomActivity{
		AppID:  appID,
		UserID: userID,
		Name:   req.Name,
		Icon:   req.Icon,
	}
	result := s.db.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, req.Name).First(&item)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := s.db.Create(&item).Error; err != nil {
			return nil, err
		}
	} else {
		item.Icon = req.Icon
		if err := s.db.Save(&item).Error; err != nil {
			return nil, err
		}
	}
	return &item, nil
}

func (s *VocabularyService) DeleteActivity(appID string, userID uuid.UUID, id uuid.UUID) error {
	result := s.db.Where("app_id = ? AND user_id = ? AND id = ?", appID, userID, id).Delete(&CustomActivity{})
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return result.Error
}

// --- Bulk Sync ---

func (s *VocabularyService) BulkSync(appID string, userID uuid.UUID, req BulkSyncVocabularyRequest) (*BulkSyncVocabularyResponse, error) {
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Upsert emotions
	for _, e := range req.Emotions {
		if e.Name == "" || e.Emoji == "" || e.Color == "" {
			continue
		}
		var existing CustomEmotion
		err := tx.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, e.Name).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			tx.Create(&CustomEmotion{AppID: appID, UserID: userID, Name: e.Name, Emoji: e.Emoji, Color: e.Color})
		} else if err == nil {
			existing.Emoji = e.Emoji
			existing.Color = e.Color
			tx.Save(&existing)
		}
	}

	// Upsert triggers
	for _, t := range req.Triggers {
		if t.Name == "" {
			continue
		}
		icon := t.Icon
		if icon == "" {
			icon = "flash-outline"
		}
		var existing CustomTrigger
		err := tx.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, t.Name).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			tx.Create(&CustomTrigger{AppID: appID, UserID: userID, Name: t.Name, Icon: icon})
		} else if err == nil {
			existing.Icon = icon
			tx.Save(&existing)
		}
	}

	// Upsert activities
	for _, a := range req.Activities {
		if a.Name == "" {
			continue
		}
		icon := a.Icon
		if icon == "" {
			icon = "ellipse-outline"
		}
		var existing CustomActivity
		err := tx.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, a.Name).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			tx.Create(&CustomActivity{AppID: appID, UserID: userID, Name: a.Name, Icon: icon})
		} else if err == nil {
			existing.Icon = icon
			tx.Save(&existing)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("bulk sync commit failed: %w", err)
	}

	// Fetch all after sync
	emotions, _ := s.ListEmotions(appID, userID)
	triggers, _ := s.ListTriggers(appID, userID)
	activities, _ := s.ListActivities(appID, userID)

	return &BulkSyncVocabularyResponse{
		Emotions:   emotions,
		Triggers:   triggers,
		Activities: activities,
	}, nil
}
