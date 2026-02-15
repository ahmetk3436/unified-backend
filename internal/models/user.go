package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User is the unified user model (superset of all 11 app variants).
type User struct {
	ID           uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID        string         `gorm:"size:50;not null;uniqueIndex:idx_users_app_email" json:"-"`
	Email        string         `gorm:"not null;size:255;uniqueIndex:idx_users_app_email" json:"email"`
	Password     string         `gorm:"not null" json:"-"`
	Role         string         `gorm:"size:20;default:'user'" json:"role"`
	AppleUserID  *string        `gorm:"size:255;index" json:"-"`
	AuthProvider string         `gorm:"size:50;default:'email'" json:"-"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}
