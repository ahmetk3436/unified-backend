package rizzcheck

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RizzResponse stores an AI-generated response set.
type RizzResponse struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID       string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	InputText   string         `gorm:"type:text;not null" json:"input_text"`
	Tone        string         `gorm:"size:30;not null" json:"tone"`     // flirty,professional,funny,chill,savage,romantic,confident,mysterious
	Category    string         `gorm:"size:30;not null" json:"category"` // dating,work,casual,family,friends
	Response1   string         `gorm:"type:text" json:"response_1"`
	Response2   string         `gorm:"type:text" json:"response_2"`
	Response3   string         `gorm:"type:text" json:"response_3"`
	SelectedIdx int            `gorm:"default:0" json:"selected_idx"`
	CreatedAt   time.Time      `json:"created_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// RizzStreak tracks user's daily usage and streak gamification.
type RizzStreak struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID         string         `gorm:"size:50;not null;index;uniqueIndex:idx_rizz_streak_app_user" json:"app_id"`
	UserID        uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_rizz_streak_app_user" json:"user_id"`
	CurrentStreak int            `gorm:"default:0" json:"current_streak"`
	LongestStreak int            `gorm:"default:0" json:"longest_streak"`
	TotalRizzes   int            `gorm:"default:0" json:"total_rizzes"`
	FreeUsesToday int            `gorm:"default:0" json:"free_uses_today"`
	LastUseDate   *time.Time     `gorm:"type:date" json:"last_use_date"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// ValidTones lists all valid tone options.
var ValidTones = map[string]bool{
	"flirty": true, "professional": true, "funny": true, "chill": true,
	"savage": true, "romantic": true, "confident": true, "mysterious": true,
}

// ValidCategories lists all valid category options.
var ValidCategories = map[string]bool{
	"dating": true, "work": true, "casual": true, "family": true, "friends": true,
}
