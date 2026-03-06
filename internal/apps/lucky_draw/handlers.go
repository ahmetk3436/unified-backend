package lucky_draw

import (
	"errors"
	"strconv"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type LuckyDrawHandler struct {
	svc *LuckyDrawService
}

func NewLuckyDrawHandler(svc *LuckyDrawService) *LuckyDrawHandler {
	return &LuckyDrawHandler{svc: svc}
}

func (h *LuckyDrawHandler) Create(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	var userID *uuid.UUID

	// Get user ID if authenticated
	uid, err := tenant.GetUserID(c)
	if err == nil {
		userID = &uid
	}

	var req CreateDrawRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	// Validate input length
	if len(req.Input) > 5000 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":   true,
			"message": "input_too_long",
		})
	}

	// Set IsGuest based on authentication
	if userID == nil {
		req.IsGuest = true
	}

	result, err := h.svc.Create(appID, userID, req)
	if err != nil {
		if errors.Is(err, ErrInvalidInput) || errors.Is(err, ErrInvalidGuestID) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return fiber.NewError(fiber.StatusInternalServerError, "create failed")
	}

	return c.Status(fiber.StatusCreated).JSON(result)
}

func (h *LuckyDrawHandler) Get(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	var userID *uuid.UUID

	// Get user ID if authenticated
	uid, err := tenant.GetUserID(c)
	if err == nil {
		userID = &uid
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	result, err := h.svc.Get(appID, userID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return fiber.NewError(fiber.StatusInternalServerError, "get failed")
	}

	return c.JSON(result)
}

func (h *LuckyDrawHandler) List(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	var userID *uuid.UUID

	// Get user ID if authenticated
	uid, err := tenant.GetUserID(c)
	if err == nil {
		userID = &uid
	}

	// Validate and clamp query parameters
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if offset > 10000 {
		offset = 0
	}

	results, total, err := h.svc.List(appID, userID, limit, offset)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "list failed")
	}

	return c.JSON(ListDrawsResponse{
		Results: results,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	})
}

func (h *LuckyDrawHandler) Delete(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	var userID *uuid.UUID

	// Get user ID if authenticated
	uid, err := tenant.GetUserID(c)
	if err == nil {
		userID = &uid
	} else {
		// Guests cannot delete specific results
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	if err := h.svc.Delete(appID, userID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return fiber.NewError(fiber.StatusInternalServerError, "delete failed")
	}

	return c.JSON(fiber.Map{
		"message": "draw result deleted successfully",
	})
}

func (h *LuckyDrawHandler) GetStats(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	stats, err := h.svc.GetStats(appID, userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "stats failed")
	}

	return c.JSON(stats)
}

func (h *LuckyDrawHandler) GetHistory(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	days, _ := strconv.Atoi(c.Query("days", "30"))
	if days < 1 || days > 90 {
		days = 30
	}

	history, err := h.svc.GetHistory(appID, userID, days)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "history failed")
	}

	return c.JSON(history)
}