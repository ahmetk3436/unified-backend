package daiyly

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type JournalEntry struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID     string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID    uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	MoodEmoji string         `gorm:"type:varchar(10)" json:"mood_emoji"`
	MoodScore int            `gorm:"default:50" json:"mood_score"`
	Content   string         `gorm:"type:text" json:"content"`
	PhotoURL   string         `gorm:"type:text" json:"photo_url"`
	AudioURL   string         `gorm:"type:text" json:"audio_url"`
	Transcript string         `gorm:"type:text" json:"transcript"`
	CardColor  string         `gorm:"type:varchar(7)" json:"card_color"`
	EntryDate time.Time      `gorm:"index" json:"entry_date"`
	IsPrivate bool           `gorm:"default:true" json:"is_private"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type JournalStreak struct {
	ID                uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID             string     `gorm:"size:50;not null;index;uniqueIndex:idx_journal_streak_app_user" json:"app_id"`
	UserID            uuid.UUID  `gorm:"type:uuid;uniqueIndex:idx_journal_streak_app_user" json:"user_id"`
	CurrentStreak     int        `gorm:"default:0" json:"current_streak"`
	LongestStreak     int        `gorm:"default:0" json:"longest_streak"`
	TotalEntries      int        `gorm:"default:0" json:"total_entries"`
	LastEntryDate     time.Time  `json:"last_entry_date"`
	GracePeriodActive bool       `gorm:"default:false" json:"grace_period_active"`
	GracePeriodUsedAt *time.Time `gorm:"default:null" json:"grace_period_used_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

var MoodEmojis = []string{"😊", "😢", "😡", "😰", "😴", "🥳", "😌", "🤔", "😍", "😤", "😔", "😐"}
var CardColors = []string{"#fef3c7", "#dbeafe", "#dcfce7", "#fce7f3", "#ede9fe", "#fef2f2"}

// --- DTOs ---

type CreateJournalRequest struct {
	MoodEmoji  string `json:"mood_emoji"`
	MoodScore  int    `json:"mood_score"`
	Content    string `json:"content"`
	PhotoURL   string `json:"photo_url"`
	AudioURL   string `json:"audio_url"`
	Transcript string `json:"transcript"`
	CardColor  string `json:"card_color"`
	IsPrivate  bool   `json:"is_private"`
	EntryDate  string `json:"entry_date"` // "YYYY-MM-DD" from client's local timezone; optional
}

type UpdateJournalRequest struct {
	MoodEmoji  *string `json:"mood_emoji"`
	MoodScore  *int    `json:"mood_score"`
	Content    *string `json:"content"`
	PhotoURL   *string `json:"photo_url"`
	AudioURL   *string `json:"audio_url"`
	Transcript *string `json:"transcript"`
	CardColor  *string `json:"card_color"`
	IsPrivate  *bool   `json:"is_private"`
}

type JournalListResponse struct {
	Entries []JournalEntry `json:"entries"`
	Total   int64          `json:"total"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
}

type WeeklyInsights struct {
	AverageMoodScore int            `json:"average_mood_score"`
	MoodTrend        string         `json:"mood_trend"`
	TopMood          string         `json:"top_mood"`
	TotalEntries     int            `json:"total_entries"`
	DailyScores      []DailyScore   `json:"daily_scores"`
	MoodDistribution map[string]int `json:"mood_distribution"`
	WritingStats     WritingStats   `json:"writing_stats"`
	TimePattern      map[string]int `json:"time_pattern"`
	StreakData       StreakData     `json:"streak_data"`
}

type WritingStats struct {
	AvgWordCount int `json:"avg_word_count"`
	TotalWords   int `json:"total_words"`
}

type StreakData struct {
	Current int `json:"current"`
	Longest int `json:"longest"`
	Total   int `json:"total"`
}

type DailyScore struct {
	Date  string `json:"date"`
	Score int    `json:"score"`
}

type SearchJournalResponse struct {
	Entries []JournalEntry `json:"entries"`
	Total   int64          `json:"total"`
	Query   string         `json:"query"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
}

type DeleteJournalResponse struct {
	Message string `json:"message"`
}

// --- AI Models ---

type EntryAnalysis struct {
	ID                uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID             string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID            uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	EntryID           uuid.UUID      `gorm:"type:uuid;uniqueIndex" json:"entry_id"`
	Themes            string         `gorm:"type:text" json:"themes"`
	SentimentLabel    string         `gorm:"size:20" json:"sentiment_label"`
	SentimentScore    float64        `json:"sentiment_score"`
	CognitivePatterns string         `gorm:"type:text" json:"cognitive_patterns"`
	Insight           string         `gorm:"type:text" json:"insight"`
	Status            string         `gorm:"size:20;default:'pending'" json:"status"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

type WeeklyReport struct {
	ID              uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID           string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID          uuid.UUID      `gorm:"type:uuid;index;uniqueIndex:idx_weekly_report_user_week" json:"user_id"`
	WeekStart       time.Time      `gorm:"type:date;uniqueIndex:idx_weekly_report_user_week" json:"week_start"`
	Narrative       string         `gorm:"type:text" json:"narrative"`
	KeyThemes       string         `gorm:"type:text" json:"key_themes"`
	MoodExplanation string         `gorm:"type:text" json:"mood_explanation"`
	Suggestion      string         `gorm:"type:text" json:"suggestion"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// NotificationConfigCache stores AI-generated notification messages per user per day.
type NotificationConfigCache struct {
	ID           uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID        string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID       uuid.UUID      `gorm:"type:uuid;index;uniqueIndex:idx_notif_cache_user_date" json:"user_id"`
	ConfigDate   time.Time      `gorm:"type:date;uniqueIndex:idx_notif_cache_user_date" json:"config_date"`
	MessagesJSON string         `gorm:"type:text" json:"messages_json"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// DailyPromptCache stores AI-generated prompts per user per day (24h cache).
type DailyPromptCache struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID       string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID      uuid.UUID      `gorm:"type:uuid;index;uniqueIndex:idx_prompt_cache_user_date" json:"user_id"`
	PromptDate  time.Time      `gorm:"type:date;uniqueIndex:idx_prompt_cache_user_date" json:"prompt_date"`
	PromptsJSON string         `gorm:"type:text" json:"prompts_json"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// --- AI DTOs ---

type EntryAnalysisResponse struct {
	Themes            []string `json:"themes"`
	SentimentLabel    string   `json:"sentiment_label"`
	SentimentScore    float64  `json:"sentiment_score"`
	CognitivePatterns []string `json:"cognitive_patterns"`
	Insight           string   `json:"insight"`
	Status            string   `json:"status"`
}

type JournalPrompt struct {
	Text     string `json:"text"`
	Category string `json:"category"`
}

type PromptsResponse struct {
	Prompts []JournalPrompt `json:"prompts"`
}

type WeeklyReportResponse struct {
	Narrative       string         `json:"narrative"`
	KeyThemes       []string       `json:"key_themes"`
	MoodExplanation string         `json:"mood_explanation"`
	Suggestion      string         `json:"suggestion"`
	WeekStart       string         `json:"week_start"`
	Stats           WeeklyInsights `json:"stats"`
}

type FlashbackEntry struct {
	Entry   JournalEntry `json:"entry"`
	Period  string       `json:"period"`
	DaysAgo int          `json:"days_ago"`
}

type FlashbacksResponse struct {
	Entries []FlashbackEntry `json:"entries"`
}

// TherapistExportCache stores the AI-generated therapist export per user (6h TTL).
type TherapistExportCache struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID       string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID      uuid.UUID      `gorm:"type:uuid;index;uniqueIndex:idx_therapist_export_user" json:"user_id"`
	ReportJSON  string         `gorm:"type:text" json:"report_json"`
	GeneratedAt time.Time      `json:"generated_at"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// --- Notification Config DTOs ---

type NotificationMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type NotificationConfigResponse struct {
	SuggestedHour   int                   `json:"suggested_hour"`
	SuggestedMinute int                   `json:"suggested_minute"`
	DailyMessages   []NotificationMessage `json:"daily_messages"`
	StreakMessages   []NotificationMessage `json:"streak_messages"`
}

// --- Therapist Export DTOs ---

type NotableEntry struct {
	Date      string `json:"date"`
	Excerpt   string `json:"excerpt"`
	MoodScore int    `json:"mood_score"`
}

// TherapistExportResponse is the structured therapist-ready summary returned by /journals/therapist-export.
// This is a PREMIUM feature; non-subscribers should be gated at the handler level once subscription
// checking is wired up. For now the service generates the report unconditionally.
type TherapistExportResponse struct {
	Period            string         `json:"period"`
	EntryCount        int            `json:"entry_count"`
	AvgMoodScore      int            `json:"avg_mood_score"`
	MoodTrend         string         `json:"mood_trend"`
	DominantThemes    []string       `json:"dominant_themes"`
	EmotionalPatterns string         `json:"emotional_patterns"`
	NotableEntries    []NotableEntry `json:"notable_entries"`
	AINarrative       string         `json:"ai_narrative"`
	Suggestions       string         `json:"suggestions"`
	GeneratedAt       string         `json:"generated_at"`
}

// TherapistReportDateRange is used by TherapistReportResponse.
type TherapistReportDateRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// TherapistReportResponse is the spec-compatible response shape for GET /journals/therapist-report.
// It wraps the rich AI narrative from TherapistExportResponse into a simpler envelope.
type TherapistReportResponse struct {
	Report      string                   `json:"report"`
	GeneratedAt string                   `json:"generated_at"`
	EntryCount  int                      `json:"entry_count"`
	DateRange   TherapistReportDateRange `json:"date_range"`
}

// --- Notification Timing DTOs ---

// NotificationTimingResponse is returned by /journals/notification-timing.
type NotificationTimingResponse struct {
	OptimalHour      *int    `json:"optimal_hour"`
	OptimalHourLabel string  `json:"optimal_hour_label"`
	Confidence       float64 `json:"confidence"`
	DaysAnalyzed     int     `json:"days_analyzed"`
}

// --- AI Search / Ask DTOs ---

// AISearchResult is a single result entry returned by /journals/ai-search.
type AISearchResult struct {
	ID               string `json:"id"`
	Date             string `json:"date"`
	Content          string `json:"content"`
	MoodScore        int    `json:"mood_score"`
	RelevanceExcerpt string `json:"relevance_excerpt"`
}

// AISearchResponse is returned by GET /journals/ai-search.
type AISearchResponse struct {
	Query   string           `json:"query"`
	Results []AISearchResult `json:"results"`
	Total   int              `json:"total"`
}

// AskJournalRequest is the request body for POST /journals/ask.
type AskJournalRequest struct {
	Question string `json:"question"`
}

// AskJournalResponse is returned by POST /journals/ask.
type AskJournalResponse struct {
	Answer          string   `json:"answer"`
	ReferencedDates []string `json:"referenced_dates"`
}
