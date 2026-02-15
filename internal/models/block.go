package models

import (
	"time"

	"github.com/google/uuid"
)

// Block implements user blocking (Apple Guideline 1.2 - immediate content hiding).
type Block struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID     string    `gorm:"size:50;not null;index" json:"-"`
	BlockerID uuid.UUID `gorm:"type:uuid;not null;index" json:"blocker_id"`
	BlockedID uuid.UUID `gorm:"type:uuid;not null;index" json:"blocked_id"`
	CreatedAt time.Time `json:"created_at"`
	Blocker   User      `gorm:"foreignKey:BlockerID" json:"-"`
	Blocked   User      `gorm:"foreignKey:BlockedID" json:"-"`
}

func (Block) TableName() string {
	return "blocks"
}
