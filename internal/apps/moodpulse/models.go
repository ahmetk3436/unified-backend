package moodpulse

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MoodCheckIn is the core mood entry.
type MoodCheckIn struct {
	ID                uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID             string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID            uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	EmotionID         string         `gorm:"size:50;not null" json:"emotion_id"`
	EmotionName       string         `gorm:"size:100;not null" json:"emotion_name"`
	EmotionEmoji      string         `gorm:"type:varchar(10)" json:"emotion_emoji"`
	EmotionColor      string         `gorm:"type:varchar(10)" json:"emotion_color"`
	EmotionCustom     bool           `gorm:"default:false" json:"emotion_custom"`
	Intensity         int            `gorm:"default:5;not null" json:"intensity"`
	Note              string         `gorm:"type:text" json:"note"`
	TriggersJSON      string         `gorm:"type:jsonb;default:'[]'" json:"-"`
	ActivitiesJSON    string         `gorm:"type:jsonb;default:'[]'" json:"-"`
	PhotoURL          string         `json:"photo_url" gorm:"type:text"`
	AudioURL          string         `json:"audio_url" gorm:"type:text"`
	Transcript        *string        `json:"transcript" gorm:"type:text"`
	DetectedEmotion   string         `json:"detected_emotion" gorm:"type:varchar(50)"`
	EmotionScores     *string        `json:"emotion_scores" gorm:"type:text"` // JSON array
	EmotionAnalyzedAt *time.Time     `json:"emotion_analyzed_at"`
	// Context tagging (nullable — all optional)
	WhereContext    *string `json:"where_context" gorm:"size:50"`    // home/work/outside/commute/social/gym
	WithContext     *string `json:"with_context" gorm:"size:50"`     // alone/partner/friends/family/colleagues/strangers
	ActivityContext *string `json:"activity_context" gorm:"size:50"` // working/relaxing/exercising/eating/socializing/commuting
	SubEmotion      *string `json:"sub_emotion" gorm:"size:50"`      // granular emotion (hopeless/frustrated/overwhelmed etc.)
	// Medication/supplement tracking (nullable — all optional)
	MedTaken *bool   `json:"med_taken"`                  // did user take their tracked medication today?
	MedName  *string `json:"med_name" gorm:"size:100"`   // what medication/supplement
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
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
	Emotion         EmotionDTO `json:"emotion"`
	Intensity       int        `json:"intensity"`
	Note            string     `json:"note"`
	Triggers        []TagItem  `json:"triggers"`
	Activities      []TagItem  `json:"activities"`
	PhotoURL        string     `json:"photo_url"`
	AudioURL        string     `json:"audio_url"`
	Transcript      *string    `json:"transcript"`
	WhereContext    *string    `json:"where_context"`
	WithContext     *string    `json:"with_context"`
	ActivityContext *string    `json:"activity_context"`
	SubEmotion      *string    `json:"sub_emotion"`
	MedTaken        *bool      `json:"med_taken"`
	MedName         *string    `json:"med_name"`
}

type UpdateMoodRequest struct {
	Emotion         *EmotionDTO `json:"emotion"`
	Intensity       *int        `json:"intensity"`
	Note            *string     `json:"note"`
	Triggers        *[]TagItem  `json:"triggers"`
	Activities      *[]TagItem  `json:"activities"`
	PhotoURL        *string     `json:"photo_url"`
	AudioURL        *string     `json:"audio_url"`
	Transcript      *string     `json:"transcript"`
	WhereContext    *string     `json:"where_context"`
	WithContext     *string     `json:"with_context"`
	ActivityContext *string     `json:"activity_context"`
	SubEmotion      *string     `json:"sub_emotion"`
	MedTaken        *bool       `json:"med_taken"`
	MedName         *string     `json:"med_name"`
}

type AIInsightsResponse struct {
	Insights string `json:"insights"`
}

type AskMoodRequest struct {
	Question string `json:"question"`
}

type AskMoodResponse struct {
	Answer string `json:"answer"`
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

// --- Batch Sync DTOs ---

type BatchCreateMoodRequest struct {
	Entries []BatchMoodEntry `json:"entries"`
}

type BatchMoodEntry struct {
	ClientID   string     `json:"client_id"`
	Emotion    EmotionDTO `json:"emotion"`
	Intensity  int        `json:"intensity"`
	Note       string     `json:"note"`
	Triggers   []TagItem  `json:"triggers"`
	Activities []TagItem  `json:"activities"`
	CreatedAt  string     `json:"created_at"`
}

type BatchCreateMoodResponse struct {
	Imported int               `json:"imported"`
	Skipped  int               `json:"skipped"`
	Results  []BatchMoodResult `json:"results"`
}

type BatchMoodResult struct {
	ClientID string `json:"client_id"`
	ServerID string `json:"server_id,omitempty"`
	Status   string `json:"status"` // "created", "duplicate", "error"
	Error    string `json:"error,omitempty"`
}

type BatchDeleteMoodRequest struct {
	IDs []string `json:"ids"`
}

type BatchDeleteMoodResponse struct {
	Deleted int `json:"deleted"`
	Skipped int `json:"skipped"`
}

type BulkSyncVocabularyRequest struct {
	Emotions   []CreateCustomEmotionRequest  `json:"emotions"`
	Triggers   []CreateCustomTriggerRequest  `json:"triggers"`
	Activities []CreateCustomActivityRequest `json:"activities"`
}

type CalendarEntry struct {
	ID    uuid.UUID `json:"id"`
	Date  string    `json:"date"`
	Color string    `json:"color"`
	Emoji string    `json:"emoji"`
}

type CalendarResponse struct {
	Entries []CalendarEntry `json:"entries"`
	Month   int             `json:"month"`
	Year    int             `json:"year"`
}

type BulkSyncVocabularyResponse struct {
	Emotions   []CustomEmotion  `json:"emotions"`
	Triggers   []CustomTrigger  `json:"triggers"`
	Activities []CustomActivity `json:"activities"`
}

// ContextInsightsResponse holds avg intensity per context category.
type ContextInsightsResponse struct {
	Where    map[string]float64 `json:"where"`
	With     map[string]float64 `json:"with"`
	Activity map[string]float64 `json:"activity"`
	Days     int                `json:"days"`
}

// MedCorrelationResponse holds avg intensity on med-taken vs not-taken days.
type MedCorrelationResponse struct {
	MedName     string  `json:"med_name"`
	TakenAvg    float64 `json:"taken_avg"`
	NotTakenAvg float64 `json:"not_taken_avg"`
	TakenCount  int     `json:"taken_count"`
	NotTakenCount int   `json:"not_taken_count"`
	Days        int     `json:"days"`
}

// SubEmotionsResponse holds the static sub-emotion vocabulary map.
type SubEmotionsResponse struct {
	SubEmotions map[string][]string `json:"sub_emotions"`
}

// TherapistReportResponse is a structured clinical mood summary.
type TherapistReportResponse struct {
	Period       string        `json:"period"`        // e.g. "Feb 3 – Mar 4, 2026"
	TotalEntries int           `json:"total_entries"`
	AverageMood  float64       `json:"average_mood"` // 1-10
	MoodTrend    string        `json:"mood_trend"`   // "improving" / "declining" / "stable"
	TopEmotions  []EmotionFreq `json:"top_emotions"`
	LowDays      int           `json:"low_days"`    // days with mood <= 3
	HighDays     int           `json:"high_days"`   // days with mood >= 8
	Narrative    string        `json:"narrative"`   // AI-generated clinical summary
	ExportedAt   string        `json:"exported_at"`
}

type EmotionFreq struct {
	Emotion string  `json:"emotion"`
	Count   int     `json:"count"`
	Pct     float64 `json:"pct"`
}

type WeeklyNarrativeResponse struct {
	Narrative       string  `json:"narrative"`
	WeekStart       string  `json:"week_start"`
	WeekEnd         string  `json:"week_end"`
	DominantEmotion string  `json:"dominant_emotion"`
	EntryCount      int     `json:"entry_count"`
	AverageMood     float64 `json:"average_mood"`
}
