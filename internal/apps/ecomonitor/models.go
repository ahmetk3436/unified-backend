package ecomonitor

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Coordinate represents a user-saved geographic coordinate.
type Coordinate struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID       string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Latitude    float64        `gorm:"type:decimal(10,8);not null" json:"latitude"`
	Longitude   float64        `gorm:"type:decimal(11,8);not null" json:"longitude"`
	Label       string         `gorm:"not null;size:255" json:"label"`
	Description string         `gorm:"size:500" json:"description,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// SatelliteData stores AI-generated environmental analysis results.
type SatelliteData struct {
	ID           uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID        string    `gorm:"size:50;not null;index" json:"app_id"`
	CoordinateID uuid.UUID `gorm:"type:uuid;not null;index" json:"coordinate_id"`
	ChangeType   string    `gorm:"type:varchar(100);not null" json:"change_type"`
	Confidence   float64   `gorm:"not null;check:confidence >= 0 AND confidence <= 1" json:"confidence"`
	DetectedAt   time.Time `gorm:"not null" json:"detected_at"`
	ImageURL     string    `gorm:"type:text" json:"image_url"`
	Summary      string    `gorm:"type:varchar(1000)" json:"summary"`
	Severity     string    `gorm:"type:varchar(20);default:'medium'" json:"severity"`
	AIModel      string    `gorm:"type:varchar(50)" json:"ai_model"`
	Description  string    `gorm:"type:text" json:"description"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (SatelliteData) TableName() string {
	return "eco_satellite_data"
}

// AnalysisHistory tracks past analysis runs.
type AnalysisHistory struct {
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID         string    `gorm:"size:50;not null;index" json:"app_id"`
	UserID        uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	CoordinateID  uuid.UUID `gorm:"type:uuid;not null" json:"coordinate_id"`
	AnalysisType  string    `gorm:"type:varchar(50);not null" json:"analysis_type"`
	ResultSummary string    `gorm:"type:varchar(2000)" json:"result_summary"`
	ConfidenceAvg float64   `gorm:"not null" json:"confidence_avg"`
	ChangeCount   int       `gorm:"not null" json:"change_count"`
	CreatedAt     time.Time `json:"created_at"`
}

func (AnalysisHistory) TableName() string {
	return "eco_analysis_histories"
}

// --- DTOs ---

type CreateCoordinateRequest struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
}

type UpdateCoordinateRequest struct {
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
	Label       *string  `json:"label"`
	Description *string  `json:"description"`
}

type CoordinateResponse struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	Label       string    `json:"label"`
	Description string    `json:"description,omitempty"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

type CoordinatesListResponse struct {
	Coordinates []CoordinateResponse `json:"coordinates"`
	Total       int64                `json:"total"`
	Page        int                  `json:"page"`
	Limit       int                  `json:"limit"`
	TotalPages  int                  `json:"total_pages"`
}

type SatelliteDataResponse struct {
	ID           uuid.UUID `json:"id"`
	CoordinateID uuid.UUID `json:"coordinate_id"`
	ChangeType   string    `json:"change_type"`
	Confidence   float64   `json:"confidence"`
	DetectedAt   time.Time `json:"detected_at"`
	ImageURL     string    `json:"image_url"`
	Summary      string    `json:"summary"`
	Severity     string    `json:"severity"`
	AIModel      string    `json:"ai_model"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"created_at"`
}

type PaginatedSatelliteResponse struct {
	Data       []SatelliteDataResponse `json:"data"`
	Page       int                     `json:"page"`
	Limit      int                     `json:"limit"`
	TotalCount int64                   `json:"total_count"`
	TotalPages int                     `json:"total_pages"`
}

type AlertResponse struct {
	ID           uuid.UUID `json:"id"`
	CoordinateID uuid.UUID `json:"coordinate_id"`
	Latitude     float64   `json:"latitude"`
	Longitude    float64   `json:"longitude"`
	ChangeType   string    `json:"change_type"`
	Confidence   float64   `json:"confidence"`
	DetectedAt   time.Time `json:"detected_at"`
	Summary      string    `json:"summary"`
	Severity     string    `json:"severity"`
	Description  string    `json:"description"`
}

type HistoryResponse struct {
	ID              uuid.UUID `json:"id"`
	CoordinateLabel string    `json:"coordinate_label"`
	AnalysisType    string    `json:"analysis_type"`
	ResultSummary   string    `json:"result_summary"`
	ConfidenceAvg   float64   `json:"confidence_avg"`
	ChangeCount     int       `json:"change_count"`
	CreatedAt       string    `json:"created_at"`
}

type PaginatedHistoryResponse struct {
	Data       []HistoryResponse `json:"data"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
	TotalCount int64             `json:"total_count"`
	TotalPages int               `json:"total_pages"`
}

type ErrorResponse struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
}
