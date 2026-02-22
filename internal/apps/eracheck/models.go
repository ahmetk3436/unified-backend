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
	// Photo challenge fields
	PhotoURL      string         `json:"-"`
	CorrectDecade string         `json:"-"` // never exposed directly — use ToPublicView()
	FunFact       string         `gorm:"type:text" json:"-"`
	Options       datatypes.JSON `gorm:"type:jsonb" json:"-"`
	UserAnswer    string         `json:"-"`
	IsCorrect     bool           `json:"-"`
	// Legacy fields kept for DB compat
	Prompt        string         `json:"-"`
	Response      string         `gorm:"type:text" json:"-"`
	Era           string         `json:"-"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// ChallengePublicView is the safe serialization — correct_decade/fun_fact only revealed after answering.
type ChallengePublicView struct {
	ID            uuid.UUID      `json:"id"`
	ChallengeDate time.Time      `json:"challenge_date"`
	PhotoURL      string         `json:"photo_url"`
	Options       datatypes.JSON `json:"options"`
	UserAnswer    string         `json:"user_answer"`
	IsCorrect     bool           `json:"is_correct"`
	CorrectDecade string         `json:"correct_decade,omitempty"`
	FunFact       string         `json:"fun_fact,omitempty"`
}

func (c *EraChallenge) ToPublicView() ChallengePublicView {
	view := ChallengePublicView{
		ID:            c.ID,
		ChallengeDate: c.ChallengeDate,
		PhotoURL:      c.PhotoURL,
		Options:       c.Options,
		UserAnswer:    c.UserAnswer,
		IsCorrect:     c.IsCorrect,
	}
	if c.UserAnswer != "" {
		view.CorrectDecade = c.CorrectDecade
		view.FunFact = c.FunFact
	}
	return view
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
