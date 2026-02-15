package vibecheck

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// VibeCheck represents a daily vibe/mood check.
type VibeCheck struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID          string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID         *uuid.UUID     `gorm:"type:uuid;index" json:"user_id"`
	DeviceID       *string        `gorm:"type:varchar(100);index" json:"device_id,omitempty"`
	MoodText       string         `gorm:"size:500" json:"mood_text"`
	Aesthetic      string         `gorm:"size:100" json:"aesthetic"`
	ColorPrimary   string         `gorm:"size:7" json:"color_primary"`
	ColorSecondary string         `gorm:"size:7" json:"color_secondary"`
	ColorAccent    string         `gorm:"size:7" json:"color_accent"`
	VibeScore      int            `gorm:"default:50" json:"vibe_score"`
	Emoji          string         `gorm:"size:10" json:"emoji"`
	Insight        string         `gorm:"size:500" json:"insight"`
	CheckDate      time.Time      `gorm:"type:date;not null" json:"check_date"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// VibeStreak tracks user's vibe check streak.
type VibeStreak struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID         string         `gorm:"size:50;not null;index;uniqueIndex:idx_vibe_streak_app_user" json:"app_id"`
	UserID        uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_vibe_streak_app_user" json:"user_id"`
	CurrentStreak int            `gorm:"default:0" json:"current_streak"`
	LongestStreak int            `gorm:"default:0" json:"longest_streak"`
	TotalChecks   int            `gorm:"default:0" json:"total_checks"`
	LastCheckDate time.Time      `gorm:"type:date" json:"last_check_date"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// Aesthetic presets
var Aesthetics = map[string]struct {
	Name           string
	Emoji          string
	ColorPrimary   string
	ColorSecondary string
	ColorAccent    string
}{
	"chill":       {"Chill Vibes", "😌", "#6366f1", "#a5b4fc", "#e0e7ff"},
	"energetic":   {"High Energy", "⚡", "#f97316", "#fdba74", "#fff7ed"},
	"romantic":    {"Hopeless Romantic", "💕", "#ec4899", "#f9a8d4", "#fdf2f8"},
	"melancholy":  {"Melancholy Soul", "🌧️", "#64748b", "#94a3b8", "#f1f5f9"},
	"adventurous": {"Adventure Mode", "🏔️", "#22c55e", "#86efac", "#f0fdf4"},
	"creative":    {"Creative Flow", "🎨", "#8b5cf6", "#c4b5fd", "#f5f3ff"},
	"peaceful":    {"Inner Peace", "🧘", "#06b6d4", "#67e8f9", "#ecfeff"},
	"confident":   {"Main Character", "👑", "#eab308", "#fde047", "#fefce8"},
	"cozy":        {"Cozy Era", "☕", "#92400e", "#fbbf24", "#fffbeb"},
	"mysterious":  {"Dark Academia", "🌙", "#1e1b4b", "#4338ca", "#312e81"},
}
