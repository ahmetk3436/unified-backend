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
	AlarmTime         *time.Time     `json:"alarm_time"`
	AlarmPhase        string         `gorm:"size:20" json:"alarm_phase"`
	Notes             string         `gorm:"type:text" json:"notes"`              // Optional user note for session
	HygieneScore      *int           `json:"hygiene_score"`                       // 0-100 score computed async
	SoundscapePlayed  *string        `json:"soundscape_played" gorm:"size:100"`   // e.g. "brown_noise", "rain"
	RoomTemp          *string        `json:"room_temp" gorm:"size:20"`            // "cool"/"comfortable"/"warm"
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// DailyCaffeineLog records caffeine intake and exercise for a single day.
type DailyCaffeineLog struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID       string     `gorm:"size:50;not null;index" json:"app_id"`
	UserID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	LogDate     time.Time  `gorm:"not null;index" json:"log_date"`
	CaffeineML  int        `json:"caffeine_ml"`  // mg of caffeine
	LastCupAt   *time.Time `json:"last_cup_at"`  // time of last cup (used for correlation)
	ExerciseMin int        `json:"exercise_min"` // minutes of exercise that day
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
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
	Score            int        `json:"score"`
	DurationMinutes  int        `json:"duration_minutes"`
	Efficiency       float64    `json:"efficiency"`
	LatencyMinutes   int        `json:"latency_minutes"`
	Bedtime          string     `json:"bedtime"`
	WakeTime         string     `json:"wake_time"`
	Phases           []PhaseDTO `json:"phases"`
	Sounds           []SoundDTO `json:"sounds"`
	AlarmTime        *string    `json:"alarm_time"`
	AlarmPhase       string     `json:"alarm_phase"`
	SoundscapePlayed *string    `json:"soundscape_played"`
	RoomTemp         *string    `json:"room_temp"`
}

type UpdateSleepRequest struct {
	Score            *int        `json:"score"`
	DurationMinutes  *int        `json:"duration_minutes"`
	Efficiency       *float64    `json:"efficiency"`
	LatencyMinutes   *int        `json:"latency_minutes"`
	Phases           *[]PhaseDTO `json:"phases"`
	Sounds           *[]SoundDTO `json:"sounds"`
	SoundscapePlayed *string     `json:"soundscape_played"`
	RoomTemp         *string     `json:"room_temp"`
}

type SleepResponse struct {
	ID               uuid.UUID  `json:"id"`
	Score            int        `json:"score"`
	DurationMinutes  int        `json:"duration_minutes"`
	Efficiency       float64    `json:"efficiency"`
	LatencyMinutes   int        `json:"latency_minutes"`
	Bedtime          string     `json:"bedtime"`
	WakeTime         string     `json:"wake_time"`
	Phases           []PhaseDTO `json:"phases"`
	Sounds           []SoundDTO `json:"sounds"`
	AlarmTime        *string    `json:"alarm_time"`
	AlarmPhase       string     `json:"alarm_phase"`
	SoundscapePlayed *string    `json:"soundscape_played"`
	RoomTemp         *string    `json:"room_temp"`
	CreatedAt        string     `json:"created_at"`
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
	TotalSessions          int                `json:"total_sessions"`
	AverageScore           float64            `json:"average_score"`
	AverageDuration        float64            `json:"average_duration"`
	AverageEfficiency      float64            `json:"average_efficiency"`
	AverageBedtime         string             `json:"average_bedtime"`
	ScoreTrend             string             `json:"score_trend"`
	DailyScores            []DailyScore       `json:"daily_scores"`
	PhaseBreakdown         map[string]float64 `json:"phase_breakdown"`
	BedtimeVarianceMinutes float64            `json:"bedtime_variance_minutes"`
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

// --- Batch Import (guest-to-auth migration) ---

type BatchImportRequest struct {
	Sessions []BatchImportEntry `json:"sessions"`
}

type BatchImportEntry struct {
	ClientID        string     `json:"client_id"`
	Score           int        `json:"score"`
	DurationMinutes int        `json:"duration_minutes"`
	Efficiency      float64    `json:"efficiency"`
	LatencyMinutes  int        `json:"latency_minutes"`
	Bedtime         string     `json:"bedtime"`
	WakeTime        string     `json:"wake_time"`
	Phases          []PhaseDTO `json:"phases"`
	Sounds          []SoundDTO `json:"sounds"`
	AlarmPhase      string     `json:"alarm_phase"`
	CreatedAt       string     `json:"created_at"`
}

type BatchImportResponse struct {
	Imported int                 `json:"imported"`
	Skipped  int                 `json:"skipped"`
	Results  []BatchImportResult `json:"results"`
}

type BatchImportResult struct {
	ClientID string `json:"client_id"`
	ServerID string `json:"server_id"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

// --- Caffeine Log DTOs ---

type LogCaffeineRequest struct {
	CaffeineML  int     `json:"caffeine_ml"`
	ExerciseMin int     `json:"exercise_min"`
	LastCupAt   *string `json:"last_cup_at"` // RFC3339
}

// --- Hygiene Score ---

type HygieneScoreResponse struct {
	Score      int               `json:"score"`
	Grade      string            `json:"grade"`
	Dimensions map[string]int    `json:"dimensions"`
	Insight    string            `json:"insight"`
}

// --- Sound Correlation ---

type SoundCorrelationResponse struct {
	Correlations map[string]float64 `json:"correlations"` // soundscape -> avg efficiency%
	NightCount   int                `json:"night_count"`  // total sessions with a soundscape logged
}

// --- Temperature Correlation ---

type TempCorrelationResponse struct {
	Correlations map[string]float64 `json:"correlations"` // room_temp -> avg score
	NightCount   int                `json:"night_count"`  // total sessions with a room_temp logged
}

// --- CBT-I Insights ---

type CBTIRecommendation struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	Evidence string `json:"evidence"`
}

type CBTIInsightsResponse struct {
	Recommendations []CBTIRecommendation `json:"recommendations"`
}

// --- Sleep Regularity Index ---

type SRIResponse struct {
	Score               float64  `json:"score"`               // 0-100; higher = more regular
	Grade               string   `json:"grade"`               // "Excellent" / "Good" / "Fair" / "Poor"
	BedtimeVarianceMin  float64  `json:"bedtime_variance_min"` // std dev of bedtime offset in minutes
	WakeVarianceMin     float64  `json:"wake_variance_min"`    // std dev of wake time offset in minutes
	NightsSampled       int      `json:"nights_sampled"`
	AvgBedtimeHour      float64  `json:"avg_bedtime_hour"`  // 0-24
	AvgWakeHour         float64  `json:"avg_wake_hour"`     // 0-24
	Insight             string   `json:"insight"`
	Recommendation      string   `json:"recommendation"`
	CitationNote        string   `json:"citation_note"`
}

// --- Lifestyle Correlation (caffeine + exercise) ---

type CaffeineCorrelationResult struct {
	AvgLatencyAfter3pmMin  float64 `json:"avg_latency_after_3pm_min"`
	AvgLatencyBefore3pmMin float64 `json:"avg_latency_before_3pm_min"`
	DiffMinutes            float64 `json:"diff_minutes"`
	NightsAfter3pm         int     `json:"nights_after_3pm"`
	NightsBefore3pm        int     `json:"nights_before_3pm"`
	Insight                string  `json:"insight"`
}

type ExerciseCorrelationResult struct {
	AvgEffWithExercisePct    float64 `json:"avg_efficiency_with_exercise_pct"`
	AvgEffWithoutExercisePct float64 `json:"avg_efficiency_without_exercise_pct"`
	DiffPercent              float64 `json:"diff_percent"`
	NightsWithExercise       int     `json:"nights_with_exercise"`
	NightsWithoutExercise    int     `json:"nights_without_exercise"`
	Insight                  string  `json:"insight"`
}

type LifestyleCorrelationResponse struct {
	DataPoints          int                        `json:"data_points"`
	MinDataPoints       int                        `json:"min_data_points"`
	CaffeineCorrelation *CaffeineCorrelationResult `json:"caffeine_correlation"`
	ExerciseCorrelation *ExerciseCorrelationResult `json:"exercise_correlation"`
}

// --- Daytime Alertness (UMD 2026 clinical trial pattern) ---

// AlertnessLog records a single daytime alertness/energy check-in.
type AlertnessLog struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID     string    `gorm:"size:50;not null;index" json:"app_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Level     int       `gorm:"not null" json:"level"`     // 1-5: 1=very tired, 5=very alert
	LoggedAt  time.Time `gorm:"not null" json:"logged_at"` // when the check-in occurred
	CreatedAt time.Time `json:"created_at"`
}

type LogAlertnessRequest struct {
	Level    int    `json:"level"`     // 1–5
	LoggedAt string `json:"logged_at"` // RFC3339
}

type AlertnessLogResponse struct {
	ID       uuid.UUID `json:"id"`
	Level    int       `json:"level"`
	LoggedAt string    `json:"logged_at"`
}

type AlertnessListResponse struct {
	Logs        []AlertnessLogResponse `json:"logs"`
	Days        int                    `json:"days"`
	DailyAvg    float64                `json:"daily_avg"`   // average level over the period
	PeakHour    int                    `json:"peak_hour"`   // hour of day (0-23) with highest avg level
	TroughHour  int                    `json:"trough_hour"` // hour of day with lowest avg level
}

// --- Nap Optimizer ---

type NapOptimizerResponse struct {
	NapNeeded       bool    `json:"nap_needed"`
	OptimalStart    string  `json:"optimal_start"`    // "14:00"
	OptimalEnd      string  `json:"optimal_end"`      // "14:20"
	DurationMinutes int     `json:"duration_minutes"` // 20 or 90
	Reason          string  `json:"reason"`
	ChronotypeHour  float64 `json:"chronotype_hour"` // avg bedtime hour (0-24)
}

// --- Dream Journal ---

type DreamEntry struct {
	ID        uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID     string     `gorm:"size:50;not null;index" json:"app_id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	SessionID *uuid.UUID `gorm:"type:uuid" json:"session_id,omitempty"` // optional linked sleep session
	Text      string     `gorm:"type:text;not null" json:"text"`
	Mood      string     `gorm:"size:20" json:"mood"` // scary/happy/neutral/peaceful/strange
	Tags      string     `gorm:"type:jsonb;default:'[]'" json:"-"` // stored as JSON array
	DreamDate string     `gorm:"size:10;not null;index" json:"dream_date"` // YYYY-MM-DD
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type CreateDreamRequest struct {
	Text      string   `json:"text"`       // required, max 5000 chars
	Mood      string   `json:"mood"`       // optional
	Tags      []string `json:"tags"`       // optional
	DreamDate string   `json:"dream_date"` // YYYY-MM-DD, defaults to today
	SessionID *string  `json:"session_id"` // optional
}

type DreamResponse struct {
	ID        string   `json:"id"`
	Text      string   `json:"text"`
	Mood      string   `json:"mood"`
	Tags      []string `json:"tags"`
	DreamDate string   `json:"dream_date"`
	CreatedAt string   `json:"created_at"`
}

type DreamListResponse struct {
	Dreams []DreamResponse `json:"dreams"`
	Total  int             `json:"total"`
}
