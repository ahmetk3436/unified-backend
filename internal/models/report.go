package models

import (
	"time"

	"github.com/google/uuid"
)

// Report implements UGC safety governance (Apple Guideline 1.2).
type Report struct {
	ID          uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID       string    `gorm:"size:50;not null;index" json:"-"`
	ReporterID  uuid.UUID `gorm:"type:uuid;not null;index" json:"reporter_id"`
	ContentType string    `gorm:"not null;size:50" json:"content_type"`
	ContentID   string    `gorm:"not null;size:255;index" json:"content_id"`
	Reason      string    `gorm:"not null;size:500" json:"reason"`
	Status      string    `gorm:"not null;default:'pending';size:50" json:"status"`
	AdminNote   string    `gorm:"size:1000" json:"admin_note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Reporter    User      `gorm:"foreignKey:ReporterID" json:"-"`
}
