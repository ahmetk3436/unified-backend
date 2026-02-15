package ecomonitor

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// --- Errors ---

var (
	ErrCoordinateNotFound = errors.New("coordinate not found")
	ErrInvalidLatitude    = errors.New("latitude must be between -90 and 90")
	ErrInvalidLongitude   = errors.New("longitude must be between -180 and 180")
	ErrLabelRequired      = errors.New("label is required")
	ErrLabelTooLong       = errors.New("label must be at most 255 characters")
	ErrDescriptionTooLong = errors.New("description must be at most 500 characters")
	ErrAINotConfigured    = errors.New("AI analysis service not configured")
)

// --- OpenAI types (internal) ---

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type environmentalChange struct {
	ChangeType  string  `json:"change_type"`
	Confidence  float64 `json:"confidence"`
	Summary     string  `json:"summary"`
	Severity    string  `json:"severity"`
	Description string  `json:"description"`
	DetectedAt  string  `json:"detected_at"`
}

// =============================================================================
// CoordinateService
// =============================================================================

type CoordinateService struct {
	db *gorm.DB
}

func NewCoordinateService(db *gorm.DB) *CoordinateService {
	return &CoordinateService{db: db}
}

func (s *CoordinateService) Create(appID string, userID uuid.UUID, req *CreateCoordinateRequest) (*CoordinateResponse, error) {
	if err := validateCoordinateInput(req.Latitude, req.Longitude, req.Label, req.Description); err != nil {
		return nil, err
	}

	coord := Coordinate{
		ID:          uuid.New(),
		AppID:       appID,
		UserID:      userID,
		Latitude:    req.Latitude,
		Longitude:   req.Longitude,
		Label:       req.Label,
		Description: req.Description,
	}

	if err := s.db.Create(&coord).Error; err != nil {
		return nil, fmt.Errorf("failed to create coordinate: %w", err)
	}

	return mapCoordinateToResponse(&coord), nil
}

func (s *CoordinateService) List(appID string, userID uuid.UUID, page, limit int, search string) (*CoordinatesListResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var coords []Coordinate
	var total int64

	query := s.db.Model(&Coordinate{}).Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID)
	if search != "" {
		searchLower := "%" + strings.ToLower(search) + "%"
		query = query.Where("(LOWER(label) LIKE ? OR LOWER(description) LIKE ?)", searchLower, searchLower)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&coords).Error; err != nil {
		return nil, err
	}

	resp := &CoordinatesListResponse{
		Coordinates: make([]CoordinateResponse, len(coords)),
		Total:       total,
		Page:        page,
		Limit:       limit,
		TotalPages:  int(math.Ceil(float64(total) / float64(limit))),
	}
	for i, c := range coords {
		resp.Coordinates[i] = *mapCoordinateToResponse(&c)
	}

	return resp, nil
}

func (s *CoordinateService) Get(appID string, id uuid.UUID) (*CoordinateResponse, error) {
	var coord Coordinate
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&coord, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCoordinateNotFound
		}
		return nil, err
	}
	return mapCoordinateToResponse(&coord), nil
}

func (s *CoordinateService) Update(appID string, id, userID uuid.UUID, req *UpdateCoordinateRequest) (*CoordinateResponse, error) {
	updates := map[string]interface{}{}

	if req.Latitude != nil {
		if *req.Latitude < -90 || *req.Latitude > 90 {
			return nil, ErrInvalidLatitude
		}
		updates["latitude"] = *req.Latitude
	}
	if req.Longitude != nil {
		if *req.Longitude < -180 || *req.Longitude > 180 {
			return nil, ErrInvalidLongitude
		}
		updates["longitude"] = *req.Longitude
	}
	if req.Label != nil {
		trimmed := strings.TrimSpace(*req.Label)
		if trimmed == "" {
			return nil, ErrLabelRequired
		}
		if len(trimmed) > 255 {
			return nil, ErrLabelTooLong
		}
		updates["label"] = trimmed
	}
	if req.Description != nil {
		if len(*req.Description) > 500 {
			return nil, ErrDescriptionTooLong
		}
		updates["description"] = *req.Description
	}

	if len(updates) == 0 {
		return s.Get(appID, id)
	}

	result := s.db.Model(&Coordinate{}).Scopes(tenant.ForTenant(appID)).Where("id = ? AND user_id = ?", id, userID).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrCoordinateNotFound
	}

	return s.Get(appID, id)
}

func (s *CoordinateService) Delete(appID string, id, userID uuid.UUID) error {
	result := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ? AND user_id = ?", id, userID).Delete(&Coordinate{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCoordinateNotFound
	}
	return nil
}

func validateCoordinateInput(lat, lng float64, label, description string) error {
	if lat < -90 || lat > 90 {
		return ErrInvalidLatitude
	}
	if lng < -180 || lng > 180 {
		return ErrInvalidLongitude
	}
	if strings.TrimSpace(label) == "" {
		return ErrLabelRequired
	}
	if len(label) > 255 {
		return ErrLabelTooLong
	}
	if len(description) > 500 {
		return ErrDescriptionTooLong
	}
	return nil
}

func mapCoordinateToResponse(c *Coordinate) *CoordinateResponse {
	return &CoordinateResponse{
		ID:          c.ID,
		UserID:      c.UserID,
		Latitude:    c.Latitude,
		Longitude:   c.Longitude,
		Label:       c.Label,
		Description: c.Description,
		CreatedAt:   c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   c.UpdatedAt.Format(time.RFC3339),
	}
}

// =============================================================================
// SatelliteService
// =============================================================================

type SatelliteService struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewSatelliteService(db *gorm.DB, cfg *config.Config) *SatelliteService {
	return &SatelliteService{db: db, cfg: cfg}
}

func (s *SatelliteService) AnalyzeCoordinate(appID string, coordinateID, userID uuid.UUID) ([]SatelliteDataResponse, error) {
	if s.cfg.OpenAIAPIKey == "" {
		return nil, ErrAINotConfigured
	}

	var coord Coordinate
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ? AND user_id = ?", coordinateID, userID).First(&coord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCoordinateNotFound
		}
		return nil, fmt.Errorf("failed to fetch coordinate: %w", err)
	}

	model := s.cfg.OpenAIModel
	if model == "" {
		model = "gpt-4o-mini"
	}

	prompt := fmt.Sprintf(`You are an environmental analysis AI. Analyze the area at latitude %.6f, longitude %.6f (%s). Based on your knowledge of this geographic region, provide a realistic environmental change assessment.

Return a JSON array with objects containing:
- change_type: one of [construction, vegetation_loss, water_change, urban_expansion, deforestation, pollution, flooding, erosion, wildfire_risk, biodiversity_loss]
- confidence: a float between 0.0 and 1.0
- summary: a 2-3 sentence description of the specific change detected at this location
- severity: one of [low, medium, high, critical]
- description: a detailed paragraph with in-depth analysis
- detected_at: an ISO8601 date within the last 30 days

Provide 1-4 realistic entries. Return ONLY valid JSON.`, coord.Latitude, coord.Longitude, coord.Label)

	oaiReq := openAIRequest{
		Model: model,
		Messages: []openAIMessage{
			{Role: "system", Content: "You are an environmental analysis AI that returns only valid JSON arrays."},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.7,
		MaxTokens:   1500,
	}

	reqBody, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.OpenAIAPIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call OpenAI API: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", httpResp.StatusCode, string(bodyBytes))
	}

	var oaiResp openAIResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&oaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	if len(oaiResp.Choices) == 0 {
		return nil, errors.New("no response from OpenAI")
	}

	content := cleanJSONContent(oaiResp.Choices[0].Message.Content)

	var changes []environmentalChange
	if err := json.Unmarshal([]byte(content), &changes); err != nil {
		return nil, fmt.Errorf("failed to parse environmental changes: %w", err)
	}

	validChangeTypes := map[string]bool{
		"construction": true, "vegetation_loss": true, "water_change": true,
		"urban_expansion": true, "deforestation": true, "pollution": true,
		"flooding": true, "erosion": true, "wildfire_risk": true, "biodiversity_loss": true,
	}
	validSeverities := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}

	var results []SatelliteDataResponse

	for _, change := range changes {
		if !validChangeTypes[change.ChangeType] {
			continue
		}
		if change.Confidence < 0.0 || change.Confidence > 1.0 {
			change.Confidence = 0.5
		}

		detectedAt, err := time.Parse(time.RFC3339, change.DetectedAt)
		if err != nil {
			detectedAt, err = time.Parse("2006-01-02", change.DetectedAt)
			if err != nil {
				detectedAt = time.Now().AddDate(0, 0, -1)
			}
		}
		thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
		if detectedAt.Before(thirtyDaysAgo) || detectedAt.After(time.Now()) {
			detectedAt = time.Now().AddDate(0, 0, -1)
		}

		severity := change.Severity
		if !validSeverities[severity] {
			severity = "medium"
		}

		satData := SatelliteData{
			AppID:        appID,
			CoordinateID: coordinateID,
			ChangeType:   change.ChangeType,
			Confidence:   change.Confidence,
			Summary:      change.Summary,
			Severity:     severity,
			AIModel:      model,
			Description:  change.Description,
			DetectedAt:   detectedAt,
		}

		if err := s.db.Create(&satData).Error; err != nil {
			return nil, fmt.Errorf("failed to create satellite data: %w", err)
		}

		results = append(results, mapSatelliteToResponse(&satData))
	}

	if len(results) == 0 {
		return nil, errors.New("no valid environmental changes detected for this location")
	}

	return results, nil
}

func (s *SatelliteService) GetAnalysis(appID string, coordinateID uuid.UUID, page, limit int) (*PaginatedSatelliteResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit

	var totalCount int64
	query := s.db.Model(&SatelliteData{}).Scopes(tenant.ForTenant(appID)).Where("coordinate_id = ?", coordinateID)
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, err
	}

	var data []SatelliteData
	if err := query.Order("detected_at DESC").Offset(offset).Limit(limit).Find(&data).Error; err != nil {
		return nil, err
	}

	resp := make([]SatelliteDataResponse, len(data))
	for i, d := range data {
		resp[i] = mapSatelliteToResponse(&d)
	}

	return &PaginatedSatelliteResponse{
		Data:       resp,
		Page:       page,
		Limit:      limit,
		TotalCount: totalCount,
		TotalPages: int(math.Ceil(float64(totalCount) / float64(limit))),
	}, nil
}

func (s *SatelliteService) GetAlerts(appID string, userID uuid.UUID, limit int, severityFilter string) ([]AlertResponse, error) {
	if limit < 1 || limit > 50 {
		limit = 10
	}

	var results []struct {
		ID           uuid.UUID
		CoordinateID uuid.UUID
		ChangeType   string
		Confidence   float64
		DetectedAt   time.Time
		Summary      string
		Severity     string
		Description  string
		Latitude     float64
		Longitude    float64
	}

	query := s.db.Table("eco_satellite_data").
		Select("eco_satellite_data.id, eco_satellite_data.coordinate_id, eco_satellite_data.change_type, eco_satellite_data.confidence, eco_satellite_data.detected_at, eco_satellite_data.summary, eco_satellite_data.severity, eco_satellite_data.description, coordinates.latitude, coordinates.longitude").
		Joins("JOIN coordinates ON eco_satellite_data.coordinate_id = coordinates.id").
		Where("coordinates.user_id = ? AND coordinates.deleted_at IS NULL AND coordinates.app_id = ?", userID, appID)

	if severityFilter != "" && severityFilter != "all" {
		query = query.Where("eco_satellite_data.severity = ?", severityFilter)
	}

	if err := query.Order("eco_satellite_data.detected_at DESC").Limit(limit).Find(&results).Error; err != nil {
		return nil, err
	}

	alerts := make([]AlertResponse, len(results))
	for i, r := range results {
		alerts[i] = AlertResponse{
			ID: r.ID, CoordinateID: r.CoordinateID,
			Latitude: r.Latitude, Longitude: r.Longitude,
			ChangeType: r.ChangeType, Confidence: r.Confidence,
			DetectedAt: r.DetectedAt, Summary: r.Summary,
			Severity: r.Severity, Description: r.Description,
		}
	}

	return alerts, nil
}

func cleanJSONContent(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
	}
	if strings.HasSuffix(content, "```") {
		content = strings.TrimSuffix(content, "```")
	}
	return strings.TrimSpace(content)
}

func mapSatelliteToResponse(d *SatelliteData) SatelliteDataResponse {
	return SatelliteDataResponse{
		ID: d.ID, CoordinateID: d.CoordinateID,
		ChangeType: d.ChangeType, Confidence: d.Confidence,
		DetectedAt: d.DetectedAt, ImageURL: d.ImageURL,
		Summary: d.Summary, Severity: d.Severity,
		AIModel: d.AIModel, Description: d.Description,
		CreatedAt: d.CreatedAt,
	}
}

// =============================================================================
// HistoryService
// =============================================================================

type HistoryService struct {
	db *gorm.DB
}

func NewHistoryService(db *gorm.DB) *HistoryService {
	return &HistoryService{db: db}
}

func (s *HistoryService) RecordAnalysis(appID string, userID, coordinateID uuid.UUID, analysisType string, results []SatelliteDataResponse) error {
	if len(results) == 0 {
		return nil
	}

	var totalConfidence float64
	summaries := make([]string, 0, len(results))
	for _, r := range results {
		totalConfidence += r.Confidence
		summaries = append(summaries, fmt.Sprintf("%s (%.0f%%)", r.ChangeType, r.Confidence*100))
	}

	avgConfidence := totalConfidence / float64(len(results))
	resultSummary := strings.Join(summaries, "; ")
	if len(resultSummary) > 2000 {
		resultSummary = resultSummary[:1997] + "..."
	}

	history := AnalysisHistory{
		ID:            uuid.New(),
		AppID:         appID,
		UserID:        userID,
		CoordinateID:  coordinateID,
		AnalysisType:  analysisType,
		ResultSummary: resultSummary,
		ConfidenceAvg: math.Round(avgConfidence*100) / 100,
		ChangeCount:   len(results),
	}

	if err := s.db.Create(&history).Error; err != nil {
		return fmt.Errorf("failed to record analysis history: %w", err)
	}

	return nil
}

func (s *HistoryService) GetUserHistory(appID string, userID uuid.UUID, page, limit int) (*PaginatedHistoryResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	offset := (page - 1) * limit

	var totalCount int64
	if err := s.db.Model(&AnalysisHistory{}).Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).Count(&totalCount).Error; err != nil {
		return nil, err
	}

	var results []struct {
		ID            uuid.UUID
		AnalysisType  string
		ResultSummary string
		ConfidenceAvg float64
		ChangeCount   int
		CreatedAt     time.Time
		Label         string
	}

	err := s.db.Table("eco_analysis_histories").
		Select("eco_analysis_histories.id, eco_analysis_histories.analysis_type, eco_analysis_histories.result_summary, eco_analysis_histories.confidence_avg, eco_analysis_histories.change_count, eco_analysis_histories.created_at, coordinates.label").
		Joins("LEFT JOIN coordinates ON eco_analysis_histories.coordinate_id = coordinates.id").
		Where("eco_analysis_histories.user_id = ? AND eco_analysis_histories.app_id = ?", userID, appID).
		Order("eco_analysis_histories.created_at DESC").
		Offset(offset).Limit(limit).Find(&results).Error
	if err != nil {
		return nil, err
	}

	data := make([]HistoryResponse, len(results))
	for i, r := range results {
		data[i] = HistoryResponse{
			ID: r.ID, CoordinateLabel: r.Label,
			AnalysisType: r.AnalysisType, ResultSummary: r.ResultSummary,
			ConfidenceAvg: r.ConfidenceAvg, ChangeCount: r.ChangeCount,
			CreatedAt: r.CreatedAt.Format(time.RFC3339),
		}
	}

	return &PaginatedHistoryResponse{
		Data:       data,
		Page:       page,
		Limit:      limit,
		TotalCount: totalCount,
		TotalPages: int(math.Ceil(float64(totalCount) / float64(limit))),
	}, nil
}

// =============================================================================
// ExportService
// =============================================================================

type ExportService struct {
	db *gorm.DB
}

func NewExportService(db *gorm.DB) *ExportService {
	return &ExportService{db: db}
}

type coordinateWithSatelliteData struct {
	ID         uuid.UUID `gorm:"column:id"`
	Label      string    `gorm:"column:label"`
	Latitude   float64   `gorm:"column:latitude"`
	Longitude  float64   `gorm:"column:longitude"`
	ChangeType string    `gorm:"column:change_type"`
	Confidence float64   `gorm:"column:confidence"`
	Severity   string    `gorm:"column:severity"`
	Summary    string    `gorm:"column:summary"`
	DetectedAt time.Time `gorm:"column:detected_at"`
}

func (s *ExportService) ExportCSV(appID string, userID uuid.UUID) ([]byte, error) {
	var data []coordinateWithSatelliteData

	err := s.db.Table("coordinates").
		Select(`
			coordinates.id,
			coordinates.label,
			coordinates.latitude,
			coordinates.longitude,
			eco_satellite_data.change_type,
			eco_satellite_data.confidence,
			eco_satellite_data.severity,
			eco_satellite_data.summary,
			eco_satellite_data.detected_at
		`).
		Joins("LEFT JOIN eco_satellite_data ON eco_satellite_data.coordinate_id = coordinates.id AND eco_satellite_data.app_id = ?", appID).
		Where("coordinates.user_id = ? AND coordinates.deleted_at IS NULL AND coordinates.app_id = ?", userID, appID).
		Order("eco_satellite_data.detected_at DESC").
		Find(&data).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query user data: %w", err)
	}

	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)

	headers := []string{"Location", "Latitude", "Longitude", "Change Type", "Confidence", "Severity", "Summary", "Detected At"}
	if err := writer.Write(headers); err != nil {
		return nil, fmt.Errorf("failed to write CSV headers: %w", err)
	}

	for _, row := range data {
		detectedAtStr := ""
		if !row.DetectedAt.IsZero() {
			detectedAtStr = row.DetectedAt.Format("2006-01-02 15:04:05")
		}
		record := []string{
			row.Label,
			fmt.Sprintf("%.6f", row.Latitude),
			fmt.Sprintf("%.6f", row.Longitude),
			row.ChangeType,
			fmt.Sprintf("%.2f", row.Confidence),
			row.Severity,
			row.Summary,
			detectedAtStr,
		}
		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV writer error: %w", err)
	}

	return buffer.Bytes(), nil
}
