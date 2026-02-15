package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// SystemLog stores structured error logs for AI agent querying.
type SystemLog struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Timestamp time.Time      `gorm:"not null;index" json:"timestamp"`
	Level     string         `gorm:"size:10;not null;index" json:"level"`
	Message   string         `gorm:"type:text" json:"message"`
	AppID     string         `gorm:"size:50;index" json:"app_id"`
	TraceID   string         `gorm:"size:36;index" json:"trace_id"`
	UserID    *string        `gorm:"size:36" json:"user_id"`
	Action    string         `gorm:"size:100" json:"action"`
	Error     string         `gorm:"type:text" json:"error"`
	LatencyMs int            `json:"latency_ms"`
	Extra     datatypes.JSON `gorm:"type:jsonb;default:'{}'" json:"extra"`
	CreatedAt time.Time      `json:"created_at"`
}
