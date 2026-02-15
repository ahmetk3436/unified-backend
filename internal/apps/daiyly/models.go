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
	PhotoURL  string         `gorm:"type:text" json:"photo_url"`
	CardColor string         `gorm:"type:varchar(7)" json:"card_color"`
	EntryDate time.Time      `gorm:"index" json:"entry_date"`
	IsPrivate bool           `gorm:"default:true" json:"is_private"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type JournalStreak struct {
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID         string    `gorm:"size:50;not null;index;uniqueIndex:idx_journal_streak_app_user" json:"app_id"`
	UserID        uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_journal_streak_app_user" json:"user_id"`
	CurrentStreak int       `gorm:"default:0" json:"current_streak"`
	LongestStreak int       `gorm:"default:0" json:"longest_streak"`
	TotalEntries  int       `gorm:"default:0" json:"total_entries"`
	LastEntryDate time.Time `json:"last_entry_date"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

var MoodEmojis = []string{"😊", "😢", "😡", "😰", "😴", "🥳", "😌", "🤔", "😍", "😤"}
var CardColors = []string{"#fef3c7", "#dbeafe", "#dcfce7", "#fce7f3", "#ede9fe", "#fef2f2"}

// --- DTOs ---

type CreateJournalRequest struct {
	MoodEmoji string `json:"mood_emoji"`
	MoodScore int    `json:"mood_score"`
	Content   string `json:"content"`
	PhotoURL  string `json:"photo_url"`
	CardColor string `json:"card_color"`
	IsPrivate bool   `json:"is_private"`
}

type UpdateJournalRequest struct {
	MoodEmoji *string `json:"mood_emoji"`
	MoodScore *int    `json:"mood_score"`
	Content   *string `json:"content"`
	PhotoURL  *string `json:"photo_url"`
	CardColor *string `json:"card_color"`
	IsPrivate *bool   `json:"is_private"`
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
