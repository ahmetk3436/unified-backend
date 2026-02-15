package aurascan

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuraReading stores a single aura scan result.
type AuraReading struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID          string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID         uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	ImageURL       string         `gorm:"type:text;not null" json:"image_url"`
	AuraColor      string         `gorm:"type:varchar(50);not null" json:"aura_color"`
	SecondaryColor *string        `gorm:"type:varchar(50);default:NULL" json:"secondary_color,omitempty"`
	EnergyLevel    int            `gorm:"type:integer;check:energy_level >= 1 AND energy_level <= 100" json:"energy_level"`
	MoodScore      int            `gorm:"type:integer;check:mood_score >= 1 AND mood_score <= 10" json:"mood_score"`
	Personality    string         `gorm:"type:text" json:"personality"`
	Strengths      []string       `gorm:"type:jsonb;serializer:json" json:"strengths"`
	Challenges     []string       `gorm:"type:jsonb;serializer:json" json:"challenges"`
	DailyAdvice    string         `gorm:"type:text" json:"daily_advice"`
	AnalyzedAt     time.Time      `gorm:"not null" json:"analyzed_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (AuraReading) TableName() string {
	return "aura_readings"
}

// AuraMatch stores compatibility results between two users.
type AuraMatch struct {
	ID                 uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID              string    `gorm:"size:50;not null;index" json:"app_id"`
	UserID             uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	FriendID           uuid.UUID `gorm:"type:uuid;not null;index" json:"friend_id"`
	UserAuraID         uuid.UUID `gorm:"type:uuid;not null" json:"user_aura_id"`
	FriendAuraID       uuid.UUID `gorm:"type:uuid;not null" json:"friend_aura_id"`
	CompatibilityScore int       `gorm:"type:integer;check:compatibility_score >= 0 AND compatibility_score <= 100" json:"compatibility_score"`
	Synergy            string    `gorm:"type:text" json:"synergy"`
	Tension            string    `gorm:"type:text" json:"tension"`
	Advice             string    `gorm:"type:text" json:"advice"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (AuraMatch) TableName() string {
	return "aura_matches"
}

// AuraStreak tracks daily scanning streaks and unlocked colors.
type AuraStreak struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID          string    `gorm:"size:50;not null;uniqueIndex:idx_aura_streak_app_user" json:"app_id"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_aura_streak_app_user" json:"user_id"`
	CurrentStreak  int       `gorm:"type:integer;default:0" json:"current_streak"`
	LongestStreak  int       `gorm:"type:integer;default:0" json:"longest_streak"`
	TotalScans     int       `gorm:"type:integer;default:0" json:"total_scans"`
	LastScanDate   time.Time `gorm:"type:date" json:"last_scan_date"`
	UnlockedColors []string  `gorm:"type:jsonb;serializer:json;default:'[]'" json:"unlocked_colors"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (AuraStreak) TableName() string {
	return "aura_streaks"
}

// --- DTOs ---

type CreateAuraRequest struct {
	ImageURL  string `json:"image_url"`
	ImageData string `json:"image_data"`
}

type AuraReadingResponse struct {
	ID             uuid.UUID `json:"id"`
	UserID         uuid.UUID `json:"user_id"`
	AuraColor      string    `json:"aura_color"`
	SecondaryColor *string   `json:"secondary_color,omitempty"`
	EnergyLevel    int       `json:"energy_level"`
	MoodScore      int       `json:"mood_score"`
	Personality    string    `json:"personality"`
	Strengths      []string  `json:"strengths"`
	Challenges     []string  `json:"challenges"`
	DailyAdvice    string    `json:"daily_advice"`
	ImageURL       string    `json:"image_url"`
	AnalyzedAt     time.Time `json:"analyzed_at"`
	CreatedAt      time.Time `json:"created_at"`
}

type AuraListResponse struct {
	Data       []AuraReadingResponse `json:"data"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	TotalCount int64                 `json:"total_count"`
}

type AuraStatsResponse struct {
	ColorDistribution map[string]int `json:"color_distribution"`
	TotalReadings     int64          `json:"total_readings"`
	AverageEnergy     float64        `json:"average_energy"`
	AverageMood       float64        `json:"average_mood"`
}

type ScanEligibilityResponse struct {
	CanScan      bool `json:"canScan"`
	Remaining    int  `json:"remaining"`
	IsSubscribed bool `json:"isSubscribed"`
}

type CreateMatchRequest struct {
	FriendID string `json:"friend_id"`
}

type AuraMatchResponse struct {
	ID                 uuid.UUID `json:"id"`
	UserID             uuid.UUID `json:"user_id"`
	FriendID           uuid.UUID `json:"friend_id"`
	UserAuraID         uuid.UUID `json:"user_aura_id"`
	FriendAuraID       uuid.UUID `json:"friend_aura_id"`
	CompatibilityScore int       `json:"compatibility_score"`
	Synergy            string    `json:"synergy"`
	Tension            string    `json:"tension"`
	Advice             string    `json:"advice"`
	UserAuraColor      string    `json:"user_aura_color"`
	FriendAuraColor    string    `json:"friend_aura_color"`
	CreatedAt          time.Time `json:"created_at"`
}

type StreakResponse struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"user_id"`
	CurrentStreak   int       `json:"current_streak"`
	LongestStreak   int       `json:"longest_streak"`
	TotalScans      int       `json:"total_scans"`
	LastScanDate    time.Time `json:"last_scan_date"`
	UnlockedColors  []string  `json:"unlocked_colors"`
	NextUnlock      string    `json:"next_unlock,omitempty"`
	DaysUntilUnlock int       `json:"days_until_unlock,omitempty"`
}

type StreakUpdateResponse struct {
	Streak       StreakResponse `json:"streak"`
	NewUnlock    string         `json:"new_unlock,omitempty"`
	StreakBroken bool           `json:"streak_broken"`
	Message      string         `json:"message"`
}
