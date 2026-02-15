package vibecheck

import (
	"strconv"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
)

// VibeHandler handles HTTP requests for vibe check-ins.
type VibeHandler struct {
	service *VibeService
}

// NewVibeHandler creates a new VibeHandler.
func NewVibeHandler(service *VibeService) *VibeHandler {
	return &VibeHandler{service: service}
}

// CreateVibeCheck handles POST /api/vibes
func (h *VibeHandler) CreateVibeCheck(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	var req struct {
		MoodText string `json:"mood_text"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request body",
		})
	}

	check, err := h.service.CreateVibeCheck(appID, userID, req.MoodText)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(check)
}

// CreateGuestVibeCheck handles POST /api/vibes/guest
func (h *VibeHandler) CreateGuestVibeCheck(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)

	var req struct {
		MoodText string `json:"mood_text"`
		DeviceID string `json:"device_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request body",
		})
	}

	if req.MoodText == "" || req.DeviceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "mood_text and device_id are required",
		})
	}

	check, err := h.service.CreateGuestVibeCheck(appID, req.MoodText, req.DeviceID)
	if err != nil {
		status := fiber.StatusBadRequest
		if err.Error() == "free limit reached, sign up for unlimited vibes" {
			status = fiber.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(check)
}

// GetTodayCheck handles GET /api/vibes/today
func (h *VibeHandler) GetTodayCheck(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	check, err := h.service.GetTodayCheck(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true, "message": "No vibe check today",
		})
	}

	return c.JSON(check)
}

// GetVibeHistory handles GET /api/vibes/history
func (h *VibeHandler) GetVibeHistory(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	if limit > 100 {
		limit = 100
	}

	checks, total, err := h.service.GetVibeHistory(appID, userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to fetch history",
		})
	}

	return c.JSON(fiber.Map{
		"data": checks, "total": total,
		"limit": limit, "offset": offset,
	})
}

// GetVibeStats handles GET /api/vibes/stats
func (h *VibeHandler) GetVibeStats(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	stats, err := h.service.GetVibeStats(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to fetch stats",
		})
	}

	return c.JSON(stats)
}

// GetVibeTrend handles GET /api/vibes/trend
func (h *VibeHandler) GetVibeTrend(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	days, err := strconv.Atoi(c.Query("days", "7"))
	if err != nil || days < 1 {
		days = 7
	}
	if days > 30 {
		days = 30
	}

	trendData, err := h.service.GetVibeTrend(appID, userID, days)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to fetch vibe trend",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    trendData,
		"meta":    fiber.Map{"days": days, "data_type": "vibe_trend"},
	})
}
