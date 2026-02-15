package feelsy

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FeelCheck represents a daily mood/energy check-in.
type FeelCheck struct {
	ID           uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID        string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID       uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	MoodScore    int            `gorm:"not null" json:"mood_score"`
	EnergyScore  int            `gorm:"not null" json:"energy_score"`
	FeelScore    int            `gorm:"not null" json:"feel_score"`
	MoodEmoji    string         `gorm:"size:10" json:"mood_emoji"`
	Note         string         `gorm:"size:280" json:"note"`
	JournalEntry string         `gorm:"type:text" json:"journal_entry"`
	ColorHex     string         `gorm:"size:7" json:"color_hex"`
	CheckDate    time.Time      `gorm:"type:date;not null;index" json:"check_date"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// CalculateFeelScore computes the combined feel score.
func (f *FeelCheck) CalculateFeelScore() {
	f.FeelScore = (f.MoodScore + f.EnergyScore) / 2
}

// GetColorHex returns a color based on the feel score.
func (f *FeelCheck) GetColorHex() string {
	switch {
	case f.FeelScore >= 90:
		return "#22c55e"
	case f.FeelScore >= 75:
		return "#84cc16"
	case f.FeelScore >= 60:
		return "#eab308"
	case f.FeelScore >= 45:
		return "#f97316"
	case f.FeelScore >= 30:
		return "#ef4444"
	default:
		return "#8b5cf6"
	}
}

// FeelStreak tracks daily check-in streaks.
type FeelStreak struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID          string         `gorm:"size:50;not null;index;uniqueIndex:idx_feel_streak_app_user" json:"app_id"`
	UserID         uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_feel_streak_app_user" json:"user_id"`
	CurrentStreak  int            `gorm:"default:0" json:"current_streak"`
	LongestStreak  int            `gorm:"default:0" json:"longest_streak"`
	TotalCheckIns  int            `gorm:"default:0" json:"total_check_ins"`
	LastCheckDate  *time.Time     `gorm:"type:date" json:"last_check_date"`
	AverageScore   float64        `gorm:"default:0" json:"average_score"`
	UnlockedBadges string         `gorm:"type:text;default:''" json:"unlocked_badges"`
	LastMessageIdx int            `gorm:"default:0" json:"last_message_idx"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// FeelFriend represents friend connections for comparing feels.
type FeelFriend struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID     string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	FriendID  uuid.UUID      `gorm:"type:uuid;not null;index" json:"friend_id"`
	Status    string         `gorm:"size:20;default:'pending'" json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// GoodVibe represents positive energy sent between friends.
type GoodVibe struct {
	ID         uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID      string         `gorm:"size:50;not null;index" json:"app_id"`
	SenderID   uuid.UUID      `gorm:"type:uuid;not null;index" json:"sender_id"`
	ReceiverID uuid.UUID      `gorm:"type:uuid;not null;index" json:"receiver_id"`
	Message    string         `gorm:"size:100" json:"message"`
	VibeType   string         `gorm:"size:20" json:"vibe_type"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

// --- DTO types embedded in this package ---

// WeeklyInsight represents mood data aggregated for a single week.
type WeeklyInsight struct {
	WeekStart     string  `json:"week_start"`
	WeekEnd       string  `json:"week_end"`
	AverageMood   float64 `json:"average_mood"`
	AverageEnergy float64 `json:"average_energy"`
	AverageFeel   float64 `json:"average_feel"`
	TotalCheckIns int     `json:"total_checkins"`
	BestDay       string  `json:"best_day"`
	WorstDay      string  `json:"worst_day"`
	MoodTrend     string  `json:"mood_trend"`
	DominantEmoji string  `json:"dominant_emoji"`
	StreakAtEnd   int     `json:"streak_at_end"`
}

// InsightsResponse represents the weekly insights API response.
type InsightsResponse struct {
	CurrentWeek  WeeklyInsight `json:"current_week"`
	PreviousWeek WeeklyInsight `json:"previous_week"`
	Improvement  float64       `json:"improvement"`
	Message      string        `json:"message"`
}

// WeeklyRecapResponse represents the weekly recap data.
type WeeklyRecapResponse struct {
	TotalCheckins int     `json:"total_checkins"`
	AverageScore  float64 `json:"average_score"`
	BestScore     int     `json:"best_score"`
	BestDay       string  `json:"best_day"`
	TopEmoji      string  `json:"top_emoji"`
	DailyScores   []int   `json:"daily_scores"`
	CurrentStreak int     `json:"current_streak"`
	WeekStart     string  `json:"week_start"`
	WeekEnd       string  `json:"week_end"`
}
