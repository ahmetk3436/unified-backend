package eracheck

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type EraQuiz struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AppID     string         `gorm:"size:50;not null;index" json:"app_id"`
	Question  string         `gorm:"not null" json:"question"`
	Options   datatypes.JSON `gorm:"type:jsonb" json:"options"`
	Category  string         `json:"category"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

type EraResult struct {
	ID             uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AppID          string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID         uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Era            string         `gorm:"not null" json:"era"`
	EraTitle       string         `json:"era_title"`
	EraDescription string         `gorm:"type:text" json:"era_description"`
	EraColor       string         `json:"era_color"`
	EraEmoji       string         `json:"era_emoji"`
	MusicTaste     string         `gorm:"type:text" json:"music_taste"`
	StyleTraits    string         `gorm:"type:text" json:"style_traits"`
	Scores         datatypes.JSON `gorm:"type:jsonb" json:"scores"`
	ShareCount     int            `gorm:"default:0" json:"share_count"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

type EraChallenge struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AppID         string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID        uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:idx_era_user_challenge_date" json:"user_id"`
	ChallengeDate time.Time      `gorm:"type:date;not null;uniqueIndex:idx_era_user_challenge_date" json:"challenge_date"`
	Prompt        string         `json:"prompt"`
	Response      string         `gorm:"type:text" json:"response"`
	Era           string         `json:"era"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

type EraStreak struct {
	ID              uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	AppID           string         `gorm:"size:50;not null;uniqueIndex:idx_era_streak_app_user" json:"app_id"`
	UserID          uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_era_streak_app_user" json:"user_id"`
	CurrentStreak   int            `gorm:"default:0" json:"current_streak"`
	LongestStreak   int            `gorm:"default:0" json:"longest_streak"`
	LastActiveDate  time.Time      `gorm:"type:date" json:"last_active_date"`
	TotalQuizzes    int            `gorm:"default:0" json:"total_quizzes"`
	TotalChallenges int            `gorm:"default:0" json:"total_challenges"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}
