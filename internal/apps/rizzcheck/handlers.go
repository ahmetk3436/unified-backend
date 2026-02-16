package rizzcheck

import (
	"strconv"
	"strings"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// RizzHandler handles HTTP requests for the RizzCheck app.
type RizzHandler struct {
	service *RizzService
}

// NewRizzHandler creates a new RizzHandler.
func NewRizzHandler(service *RizzService) *RizzHandler {
	return &RizzHandler{service: service}
}

// Generate handles POST /api/rizz/generate
func (h *RizzHandler) Generate(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Authentication required",
		})
	}

	var req struct {
		InputText string `json:"input_text"`
		Tone      string `json:"tone"`
		Category  string `json:"category"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request body",
		})
	}

	response, err := h.service.GenerateResponses(userID, appID, req.InputText, req.Tone, req.Category)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "daily free limit") {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"error": true, "message": msg, "code": "LIMIT_REACHED",
			})
		}
		if strings.Contains(msg, "invalid tone") || strings.Contains(msg, "invalid category") || strings.Contains(msg, "required") || strings.Contains(msg, "too long") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": true, "message": msg,
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to generate responses",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data": response,
	})
}

// GetStats handles GET /api/rizz/stats
func (h *RizzHandler) GetStats(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Authentication required",
		})
	}

	streak, err := h.service.GetStreak(userID, appID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to get stats",
		})
	}

	// Rizz Score = total * 10 + longest_streak * 5
	rizzScore := streak.TotalRizzes*10 + streak.LongestStreak*5

	return c.JSON(fiber.Map{
		"current_streak":  streak.CurrentStreak,
		"longest_streak":  streak.LongestStreak,
		"total_rizzes":    streak.TotalRizzes,
		"free_uses_today": streak.FreeUsesToday,
		"free_limit":      freeDailyLimit,
		"rizz_score":      rizzScore,
	})
}

// GetHistory handles GET /api/rizz/history
func (h *RizzHandler) GetHistory(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Authentication required",
		})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	responses, total, err := h.service.GetHistory(userID, appID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to get history",
		})
	}

	return c.JSON(fiber.Map{
		"data":  responses,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// SelectResponse handles POST /api/rizz/select
func (h *RizzHandler) SelectResponse(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Authentication required",
		})
	}

	var req struct {
		ResponseID string `json:"response_id"`
		SelectedIdx int   `json:"selected_idx"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request body",
		})
	}

	responseID, err := uuid.Parse(req.ResponseID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid response ID",
		})
	}

	if err := h.service.SelectResponse(userID, appID, responseID, req.SelectedIdx); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{"message": "Response selected"})
}

