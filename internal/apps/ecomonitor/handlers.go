package ecomonitor

import (
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// =============================================================================
// CoordinateHandler
// =============================================================================

type CoordinateHandler struct {
	coordService *CoordinateService
}

func NewCoordinateHandler(coordService *CoordinateService) *CoordinateHandler {
	return &CoordinateHandler{coordService: coordService}
}

func (h *CoordinateHandler) CreateCoordinate(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: true, Message: "Unauthorized"})
	}

	var req CreateCoordinateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: true, Message: "Invalid request body"})
	}

	coord, err := h.coordService.Create(appID, userID, &req)
	if err != nil {
		if errors.Is(err, ErrInvalidLatitude) || errors.Is(err, ErrInvalidLongitude) ||
			errors.Is(err, ErrLabelRequired) || errors.Is(err, ErrLabelTooLong) ||
			errors.Is(err, ErrDescriptionTooLong) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: true, Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: true, Message: "Failed to create coordinate"})
	}

	return c.Status(fiber.StatusCreated).JSON(coord)
}

func (h *CoordinateHandler) ListCoordinates(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: true, Message: "Unauthorized"})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	search := c.Query("search", "")

	coords, err := h.coordService.List(appID, userID, page, limit, search)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: true, Message: "Failed to fetch coordinates"})
	}

	return c.JSON(coords)
}

func (h *CoordinateHandler) GetCoordinate(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: true, Message: "Invalid coordinate ID"})
	}

	coord, err := h.coordService.Get(appID, id)
	if err != nil {
		if errors.Is(err, ErrCoordinateNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: true, Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: true, Message: "Failed to fetch coordinate"})
	}

	return c.JSON(coord)
}

func (h *CoordinateHandler) UpdateCoordinate(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: true, Message: "Unauthorized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: true, Message: "Invalid coordinate ID"})
	}

	var req UpdateCoordinateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: true, Message: "Invalid request body"})
	}

	coord, err := h.coordService.Update(appID, id, userID, &req)
	if err != nil {
		if errors.Is(err, ErrCoordinateNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: true, Message: err.Error()})
		}
		if errors.Is(err, ErrInvalidLatitude) || errors.Is(err, ErrInvalidLongitude) ||
			errors.Is(err, ErrLabelRequired) || errors.Is(err, ErrLabelTooLong) ||
			errors.Is(err, ErrDescriptionTooLong) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: true, Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: true, Message: "Failed to update coordinate"})
	}

	return c.JSON(coord)
}

func (h *CoordinateHandler) DeleteCoordinate(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: true, Message: "Unauthorized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: true, Message: "Invalid coordinate ID"})
	}

	if err := h.coordService.Delete(appID, id, userID); err != nil {
		if errors.Is(err, ErrCoordinateNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: true, Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: true, Message: "Failed to delete coordinate"})
	}

	return c.JSON(fiber.Map{"message": "Coordinate deleted successfully"})
}

// =============================================================================
// SatelliteHandler
// =============================================================================

type SatelliteHandler struct {
	satelliteService *SatelliteService
	historyService   *HistoryService
}

func NewSatelliteHandler(satelliteService *SatelliteService, historyService *HistoryService) *SatelliteHandler {
	return &SatelliteHandler{satelliteService: satelliteService, historyService: historyService}
}

func (h *SatelliteHandler) GenerateAnalysis(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: true, Message: "Unauthorized"})
	}

	coordinateID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: true, Message: "Invalid coordinate ID"})
	}

	results, err := h.satelliteService.AnalyzeCoordinate(appID, coordinateID, userID)
	if err != nil {
		if errors.Is(err, ErrAINotConfigured) {
			return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{Error: true, Message: "AI analysis service not configured"})
		}
		if errors.Is(err, ErrCoordinateNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: true, Message: "Coordinate not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: true, Message: "Analysis failed: " + err.Error()})
	}

	if err := h.historyService.RecordAnalysis(appID, userID, coordinateID, "satellite", results); err != nil {
		log.Printf("Failed to record analysis history: %v", err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "Analysis generated successfully", "data": results})
}

func (h *SatelliteHandler) GetAnalysis(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	coordinateID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: true, Message: "Invalid coordinate ID"})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))

	results, err := h.satelliteService.GetAnalysis(appID, coordinateID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: true, Message: "Failed to fetch analysis"})
	}

	return c.JSON(results)
}

func (h *SatelliteHandler) GetAlerts(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: true, Message: "Unauthorized"})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	severity := c.Query("severity")

	alerts, err := h.satelliteService.GetAlerts(appID, userID, limit, severity)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: true, Message: "Failed to fetch alerts"})
	}

	return c.JSON(fiber.Map{"alerts": alerts})
}

// =============================================================================
// HistoryHandler
// =============================================================================

type HistoryHandler struct {
	historyService *HistoryService
}

func NewHistoryHandler(historyService *HistoryService) *HistoryHandler {
	return &HistoryHandler{historyService: historyService}
}

func (h *HistoryHandler) GetHistory(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: true, Message: "Unauthorized"})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit > 50 {
		limit = 50
	}

	history, err := h.historyService.GetUserHistory(appID, userID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: true, Message: "Failed to fetch history"})
	}

	return c.JSON(history)
}

// =============================================================================
// ExportHandler
// =============================================================================

type ExportHandler struct {
	exportService *ExportService
	db            *gorm.DB
}

func NewExportHandler(exportService *ExportService, db *gorm.DB) *ExportHandler {
	return &ExportHandler{exportService: exportService, db: db}
}

func (h *ExportHandler) ExportCSV(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: true, Message: "Unauthorized"})
	}

	// Check subscription status (premium only)
	var subscription models.Subscription
	err = h.db.Where("user_id = ? AND app_id = ? AND status = ?", userID, appID, "active").First(&subscription).Error
	if err != nil || subscription.CurrentPeriodEnd.Before(time.Now()) {
		if errors.Is(err, gorm.ErrRecordNotFound) || err == nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "CSV export is a premium feature. Please upgrade to export your data.",
				"code":  "PREMIUM_REQUIRED",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: true, Message: "Failed to check subscription status"})
	}

	csvBytes, err := h.exportService.ExportCSV(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: true, Message: "Failed to generate export"})
	}

	timestamp := time.Now().Format("2006-01-02")
	filename := "ecomonitor-export-" + timestamp + ".csv"

	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", "attachment; filename="+filename)
	c.Set("Cache-Control", "no-cache")

	return c.Send(csvBytes)
}
