package driftoff

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SleepSession is a nightly sleep recording.
type SleepSession struct {
	ID              uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID           string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID          uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	Score           int            `gorm:"default:0" json:"score"`
	DurationMinutes int            `gorm:"default:0" json:"duration_minutes"`
	Efficiency      float64        `gorm:"default:0" json:"efficiency"`
	LatencyMinutes  int            `gorm:"default:0" json:"latency_minutes"`
	Bedtime         time.Time      `json:"bedtime"`
	WakeTime        time.Time      `json:"wake_time"`
	PhasesJSON      string         `gorm:"type:jsonb;default:'[]'" json:"-"`
	SoundsJSON      string         `gorm:"type:jsonb;default:'[]'" json:"-"`
	AlarmTime       *time.Time     `json:"alarm_time"`
	AlarmPhase      string         `gorm:"size:20" json:"alarm_phase"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// SleepStreak tracks consecutive nights of sleep logging.
type SleepStreak struct {
	ID              uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID           string    `gorm:"size:50;not null;index;uniqueIndex:idx_sleep_streak_app_user" json:"app_id"`
	UserID          uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_sleep_streak_app_user" json:"user_id"`
	CurrentStreak   int       `gorm:"default:0" json:"current_streak"`
	LongestStreak   int       `gorm:"default:0" json:"longest_streak"`
	TotalSessions   int       `gorm:"default:0" json:"total_sessions"`
	LastSessionDate time.Time `json:"last_session_date"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// --- DTOs ---

type PhaseDTO struct {
	Type            string  `json:"type"`
	StartTime       string  `json:"start_time"`
	EndTime         string  `json:"end_time"`
	DurationMinutes int     `json:"duration_minutes"`
	Percent         float64 `json:"percent"`
}

type SoundDTO struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	Timestamp       string `json:"timestamp"`
	DurationSeconds int    `json:"duration_seconds"`
}

type CreateSleepRequest struct {
	Score           int        `json:"score"`
	DurationMinutes int        `json:"duration_minutes"`
	Efficiency      float64    `json:"efficiency"`
	LatencyMinutes  int        `json:"latency_minutes"`
	Bedtime         string     `json:"bedtime"`
	WakeTime        string     `json:"wake_time"`
	Phases          []PhaseDTO `json:"phases"`
	Sounds          []SoundDTO `json:"sounds"`
	AlarmTime       *string    `json:"alarm_time"`
	AlarmPhase      string     `json:"alarm_phase"`
}

type UpdateSleepRequest struct {
	Score           *int        `json:"score"`
	DurationMinutes *int        `json:"duration_minutes"`
	Efficiency      *float64    `json:"efficiency"`
	LatencyMinutes  *int        `json:"latency_minutes"`
	Phases          *[]PhaseDTO `json:"phases"`
	Sounds          *[]SoundDTO `json:"sounds"`
}

type SleepResponse struct {
	ID              uuid.UUID  `json:"id"`
	Score           int        `json:"score"`
	DurationMinutes int        `json:"duration_minutes"`
	Efficiency      float64    `json:"efficiency"`
	LatencyMinutes  int        `json:"latency_minutes"`
	Bedtime         string     `json:"bedtime"`
	WakeTime        string     `json:"wake_time"`
	Phases          []PhaseDTO `json:"phases"`
	Sounds          []SoundDTO `json:"sounds"`
	AlarmTime       *string    `json:"alarm_time"`
	AlarmPhase      string     `json:"alarm_phase"`
	CreatedAt       string     `json:"created_at"`
}

type SleepListResponse struct {
	Sessions []SleepResponse `json:"sessions"`
	Total    int64           `json:"total"`
	Limit    int             `json:"limit"`
	Offset   int             `json:"offset"`
}

type SearchSleepResponse struct {
	Sessions []SleepResponse `json:"sessions"`
	Total    int64           `json:"total"`
	Query    string          `json:"query"`
}

type StreakResponse struct {
	CurrentStreak   int    `json:"current_streak"`
	LongestStreak   int    `json:"longest_streak"`
	TotalSessions   int    `json:"total_sessions"`
	LastSessionDate string `json:"last_session_date"`
}

type StatsResponse struct {
	TotalSessions     int                `json:"total_sessions"`
	AverageScore      float64            `json:"average_score"`
	AverageDuration   float64            `json:"average_duration"`
	AverageEfficiency float64            `json:"average_efficiency"`
	AverageBedtime    string             `json:"average_bedtime"`
	ScoreTrend        string             `json:"score_trend"`
	DailyScores       []DailyScore       `json:"daily_scores"`
	PhaseBreakdown    map[string]float64 `json:"phase_breakdown"`
}

type DailyScore struct {
	Date            string `json:"date"`
	Score           int    `json:"score"`
	DurationMinutes int    `json:"duration_minutes"`
}

type SleepDebtResponse struct {
	CurrentDebtHours float64 `json:"current_debt_hours"`
	Trend            string  `json:"trend"`
	DailyGoalHours   float64 `json:"daily_goal_hours"`
	RollingDays      int     `json:"rolling_days"`
}
