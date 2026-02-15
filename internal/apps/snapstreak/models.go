package snapstreak

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Snap struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID     string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID    uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	ImageURL  string         `gorm:"type:text" json:"image_url"`
	Caption   string         `gorm:"type:varchar(280)" json:"caption"`
	Filter    string         `gorm:"type:varchar(50)" json:"filter"`
	SnapDate  time.Time      `gorm:"index" json:"snap_date"`
	LikeCount int            `gorm:"default:0" json:"like_count"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type SnapStreak struct {
	ID               uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID            string    `gorm:"size:50;not null;index;uniqueIndex:idx_snapstreak_app_user" json:"app_id"`
	UserID           uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_snapstreak_app_user" json:"user_id"`
	CurrentStreak    int       `gorm:"default:0" json:"current_streak"`
	LongestStreak    int       `gorm:"default:0" json:"longest_streak"`
	TotalSnaps       int       `gorm:"default:0" json:"total_snaps"`
	LastSnapDate     time.Time `json:"last_snap_date"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	FreezesAvailable int       `json:"freezes_available" gorm:"default:0"`
	FreezesUsed      int       `json:"freezes_used" gorm:"default:0"`
	LastFreezeDate   time.Time `json:"last_freeze_date"`
}

var SnapFilters = []string{"none", "vintage", "warm", "cool", "dramatic", "minimal", "vibrant", "noir"}

// --- DTOs ---

type CreateSnapRequest struct {
	ImageURL string `json:"image_url"`
	Caption  string `json:"caption"`
	Filter   string `json:"filter"`
}

type SnapResponse struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ImageURL  string    `json:"image_url"`
	Caption   string    `json:"caption"`
	Filter    string    `json:"filter"`
	SnapDate  time.Time `json:"snap_date"`
	LikeCount int       `json:"like_count"`
	CreatedAt time.Time `json:"created_at"`
}

type StreakResponse struct {
	CurrentStreak    int       `json:"current_streak"`
	LongestStreak    int       `json:"longest_streak"`
	TotalSnaps       int       `json:"total_snaps"`
	LastSnapDate     time.Time `json:"last_snap_date"`
	HasSnappedToday  bool      `json:"has_snapped_today"`
	FreezesAvailable int       `json:"freezes_available"`
	FreezesUsed      int       `json:"freezes_used"`
}

type SnapsListResponse struct {
	Snaps []SnapResponse `json:"snaps"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Limit int            `json:"limit"`
}
