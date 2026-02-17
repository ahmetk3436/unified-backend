package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RemoteConfig stores per-app configuration values
type RemoteConfig struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID     string    `gorm:"size:50;not null;uniqueIndex:idx_remote_config_app_key,priority:1;index:idx_remote_config_app" json:"app_id"`
	Key       string    `gorm:"size:100;not null;uniqueIndex:idx_remote_config_app_key,priority:2" json:"key"`
	Value     string    `gorm:"type:text;not null" json:"value"`
	Type      string    `gorm:"size:20;default:'string'" json:"type"` // string, bool, int, json
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate ensures UUID is set before creation
func (rc *RemoteConfig) BeforeCreate(tx *gorm.DB) error {
	if rc.ID == uuid.Nil {
		rc.ID = uuid.New()
	}
	return nil
}

// TableName specifies the table name for RemoteConfig
func (RemoteConfig) TableName() string {
	return "remote_configs"
}
