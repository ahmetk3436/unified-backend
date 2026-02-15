package confessit

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Confession represents an anonymous confession.
type Confession struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID         string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID        uuid.UUID      `gorm:"type:uuid;index" json:"-"`
	Content       string         `gorm:"type:text;not null" json:"content"`
	Category      string         `gorm:"type:varchar(50);index" json:"category"`
	Mood          string         `gorm:"type:varchar(30)" json:"mood"`
	IsAnonymous   bool           `gorm:"default:true" json:"is_anonymous"`
	LikeCount     int            `gorm:"default:0" json:"like_count"`
	CommentCount  int            `gorm:"default:0" json:"comment_count"`
	ShareCount    int            `gorm:"default:0" json:"share_count"`
	ViewCount     int            `gorm:"default:0" json:"view_count"`
	ReactionCount int            `gorm:"default:0" json:"reaction_count"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// ConfessionLike tracks who liked a confession.
type ConfessionLike struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID        string    `gorm:"size:50;not null;index" json:"app_id"`
	ConfessionID uuid.UUID `gorm:"type:uuid;index" json:"confession_id"`
	UserID       uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// ConfessionComment represents a comment on a confession.
type ConfessionComment struct {
	ID           uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID        string         `gorm:"size:50;not null;index" json:"app_id"`
	ConfessionID uuid.UUID      `gorm:"type:uuid;index" json:"confession_id"`
	UserID       uuid.UUID      `gorm:"type:uuid" json:"-"`
	Content      string         `gorm:"type:text;not null" json:"content"`
	IsAnonymous  bool           `gorm:"default:true" json:"is_anonymous"`
	LikeCount    int            `gorm:"default:0" json:"like_count"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// ConfessionStreak tracks user's daily confession streak.
type ConfessionStreak struct {
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID         string    `gorm:"size:50;not null;index" json:"app_id"`
	UserID        uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_confessit_streak_app_user" json:"user_id"`
	CurrentStreak int       `gorm:"default:0" json:"current_streak"`
	LongestStreak int       `gorm:"default:0" json:"longest_streak"`
	TotalPosts    int       `gorm:"default:0" json:"total_posts"`
	LastPostDate  time.Time `json:"last_post_date"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ConfessionReaction tracks emoji reactions on a confession.
type ConfessionReaction struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID        string    `gorm:"size:50;not null;index" json:"app_id"`
	ConfessionID uuid.UUID `gorm:"type:uuid;index" json:"confession_id"`
	UserID       uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	Emoji        string    `gorm:"type:varchar(10);not null" json:"emoji"`
	CreatedAt    time.Time `json:"created_at"`
}

// Category constants.
var ConfessionCategories = []string{
	"love", "work", "family", "friends", "secret",
	"funny", "sad", "embarrassing", "proud", "random",
}

// Mood constants.
var ConfessionMoods = []string{
	"happy", "sad", "anxious", "relieved",
	"guilty", "excited", "confused", "peaceful",
}
