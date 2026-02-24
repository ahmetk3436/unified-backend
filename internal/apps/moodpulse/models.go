package moodpulse

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MoodCheckIn is the core mood entry.
type MoodCheckIn struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID          string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID         uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	EmotionID      string         `gorm:"size:50;not null" json:"emotion_id"`
	EmotionName    string         `gorm:"size:100;not null" json:"emotion_name"`
	EmotionEmoji   string         `gorm:"type:varchar(10)" json:"emotion_emoji"`
	EmotionColor   string         `gorm:"type:varchar(10)" json:"emotion_color"`
	EmotionCustom  bool           `gorm:"default:false" json:"emotion_custom"`
	Intensity      int            `gorm:"default:5;not null" json:"intensity"`
	Note           string         `gorm:"type:text" json:"note"`
	TriggersJSON   string         `gorm:"type:jsonb;default:'[]'" json:"-"`
	ActivitiesJSON string         `gorm:"type:jsonb;default:'[]'" json:"-"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// MoodStreak tracks consecutive days of logging.
type MoodStreak struct {
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID         string    `gorm:"size:50;not null;index;uniqueIndex:idx_mood_streak_app_user" json:"app_id"`
	UserID        uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_mood_streak_app_user" json:"user_id"`
	CurrentStreak int       `gorm:"default:0" json:"current_streak"`
	LongestStreak int       `gorm:"default:0" json:"longest_streak"`
	TotalEntries  int       `gorm:"default:0" json:"total_entries"`
	LastEntryDate time.Time `json:"last_entry_date"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// --- DTOs ---

type TagItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	IsCustom bool   `json:"isCustom"`
}

type EmotionDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Emoji    string `json:"emoji"`
	Color    string `json:"color"`
	IsCustom bool   `json:"isCustom"`
}

type CreateMoodRequest struct {
	Emotion    EmotionDTO `json:"emotion"`
	Intensity  int        `json:"intensity"`
	Note       string     `json:"note"`
	Triggers   []TagItem  `json:"triggers"`
	Activities []TagItem  `json:"activities"`
}

type UpdateMoodRequest struct {
	Emotion    *EmotionDTO `json:"emotion"`
	Intensity  *int        `json:"intensity"`
	Note       *string     `json:"note"`
	Triggers   *[]TagItem  `json:"triggers"`
	Activities *[]TagItem  `json:"activities"`
}

type MoodEntryResponse struct {
	ID         uuid.UUID  `json:"id"`
	Emotion    EmotionDTO `json:"emotion"`
	Intensity  int        `json:"intensity"`
	Note       string     `json:"note"`
	Triggers   []TagItem  `json:"triggers"`
	Activities []TagItem  `json:"activities"`
	CreatedAt  string     `json:"createdAt"`
}

type MoodListResponse struct {
	Entries []MoodEntryResponse `json:"entries"`
	Total   int64               `json:"total"`
	Limit   int                 `json:"limit"`
	Offset  int                 `json:"offset"`
}

type SearchMoodResponse struct {
	Entries []MoodEntryResponse `json:"entries"`
	Total   int64               `json:"total"`
	Query   string              `json:"query"`
}

type StreakResponse struct {
	CurrentStreak int    `json:"current_streak"`
	LongestStreak int    `json:"longest_streak"`
	TotalEntries  int    `json:"total_entries"`
	LastEntryDate string `json:"last_entry_date"`
}

type StatsResponse struct {
	TotalCheckIns     int                `json:"total_check_ins"`
	AverageIntensity  float64            `json:"average_intensity"`
	TopEmotion        string             `json:"top_emotion"`
	TopEmotionEmoji   string             `json:"top_emotion_emoji"`
	EmotionBreakdown  map[string]int     `json:"emotion_breakdown"`
	DayOfWeekPattern  map[string]int     `json:"day_of_week_pattern"`
	TimeOfDayPattern  map[string]float64 `json:"time_of_day_pattern"`
	TopTriggers       []TriggerStat      `json:"top_triggers"`
}

type TriggerStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// CustomEmotion stores user-defined emotions.
type CustomEmotion struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID     string         `gorm:"size:50;not null;uniqueIndex:idx_custom_emotion_unique" json:"app_id"`
	UserID    uuid.UUID      `gorm:"type:uuid;uniqueIndex:idx_custom_emotion_unique" json:"user_id"`
	Name      string         `gorm:"size:100;not null;uniqueIndex:idx_custom_emotion_unique" json:"name"`
	Emoji     string         `gorm:"type:varchar(10);not null" json:"emoji"`
	Color     string         `gorm:"type:varchar(10);not null" json:"color"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// CustomTrigger stores user-defined triggers.
type CustomTrigger struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID     string         `gorm:"size:50;not null;uniqueIndex:idx_custom_trigger_unique" json:"app_id"`
	UserID    uuid.UUID      `gorm:"type:uuid;uniqueIndex:idx_custom_trigger_unique" json:"user_id"`
	Name      string         `gorm:"size:100;not null;uniqueIndex:idx_custom_trigger_unique" json:"name"`
	Icon      string         `gorm:"size:100;default:'flash-outline'" json:"icon"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// CustomActivity stores user-defined activities.
type CustomActivity struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID     string         `gorm:"size:50;not null;uniqueIndex:idx_custom_activity_unique" json:"app_id"`
	UserID    uuid.UUID      `gorm:"type:uuid;uniqueIndex:idx_custom_activity_unique" json:"user_id"`
	Name      string         `gorm:"size:100;not null;uniqueIndex:idx_custom_activity_unique" json:"name"`
	Icon      string         `gorm:"size:100;default:'ellipse-outline'" json:"icon"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// --- Custom Vocabulary DTOs ---

type CreateCustomEmotionRequest struct {
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
	Color string `json:"color"`
}

type CreateCustomTriggerRequest struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
}

type CreateCustomActivityRequest struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
}

type BulkSyncVocabularyRequest struct {
	Emotions   []CreateCustomEmotionRequest  `json:"emotions"`
	Triggers   []CreateCustomTriggerRequest  `json:"triggers"`
	Activities []CreateCustomActivityRequest `json:"activities"`
}

type BulkSyncVocabularyResponse struct {
	Emotions   []CustomEmotion  `json:"emotions"`
	Triggers   []CustomTrigger  `json:"triggers"`
	Activities []CustomActivity `json:"activities"`
}
