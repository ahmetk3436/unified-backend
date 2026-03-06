package lucky_draw

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// LuckyDraw represents a single draw/analysis result
// Supports both authenticated users and guests via nullable UserID
type LuckyDraw struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    *uuid.UUID     `gorm:"type:uuid;index" json:"user_id"`
	Input     string         `gorm:"type:text" json:"input"`
	Result    string         `gorm:"type:text" json:"result"`
	Score     *int           `json:"score"`
	Category  string         `gorm:"type:varchar(100)" json:"category"`
	Metadata  datatypes.JSON `gorm:"type:jsonb" json:"metadata"`
	IsGuest   bool           `gorm:"default:false" json:"is_guest"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName specifies the table name for LuckyDraw
func (LuckyDraw) TableName() string {
	return "lucky_draws"
}

// UserHistory tracks daily usage count and streak for each user
type UserHistory struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;index;not null" json:"user_id"`
	Date      string    `gorm:"type:date;not null" json:"date"` // Format: YYYY-MM-DD
	Count     int       `gorm:"default:0" json:"count"`
	Streak    int       `gorm:"default:0" json:"streak"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies the table name for UserHistory
func (UserHistory) TableName() string {
	return "lucky_draw_user_histories"
}

// DTOs for request/response

type CreateDrawRequest struct {
	Input    string                 `json:"input" validate:"required,max=5000"`
	IsGuest  bool                   `json:"is_guest"`
	Metadata map[string]interface{} `json:"metadata"`
}

type DrawResultResponse struct {
	ID        uuid.UUID              `json:"id"`
	Input     string                 `json:"input"`
	Result    string                 `json:"result"`
	Score     *int                   `json:"score,omitempty"`
	Category  string                 `json:"category,omitempty"`
	Metadata  map[string]interface{} `json:"metadata"`
	IsGuest   bool                   `json:"is_guest"`
	CreatedAt time.Time              `json:"created_at"`
}

type ListDrawsResponse struct {
	Results []LuckyDraw `json:"results"`
	Total   int64       `json:"total"`
	Limit   int         `json:"limit"`
	Offset  int         `json:"offset"`
}

type HistoryResponse struct {
	Date   string `json:"date"`
	Count  int    `json:"count"`
	Streak int    `json:"streak"`
}

type UserStatsResponse struct {
	TotalDraws    int64            `json:"total_draws"`
	CurrentStreak int             `json:"current_streak"`
	LongestStreak int             `json:"longest_streak"`
	DailyHistory  []HistoryResponse `json:"daily_history"`
}

// Models returns all models for AutoMigrate
func Models() []interface{} {
	return []interface{}{
		&LuckyDraw{},
		&UserHistory{},
	}
}
