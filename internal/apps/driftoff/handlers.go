package driftoff

import (
	"strconv"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type SleepHandler struct {
	svc *SleepService
}

func NewSleepHandler(svc *SleepService) *SleepHandler {
	return &SleepHandler{svc: svc}
}

func (h *SleepHandler) Create(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	var req CreateSleepRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	session, err := h.svc.Create(appID, userID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(session)
}

func (h *SleepHandler) List(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	if limit > 100 {
		limit = 100
	}

	resp, err := h.svc.List(appID, userID, limit, offset)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "list failed")
	}

	return c.JSON(resp)
}

func (h *SleepHandler) Get(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	session, err := h.svc.Get(appID, userID, id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	return c.JSON(session)
}

func (h *SleepHandler) Update(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var req UpdateSleepRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	session, err := h.svc.Update(appID, userID, id, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(session)
}

func (h *SleepHandler) Delete(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	if err := h.svc.Delete(appID, userID, id); err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	return c.JSON(fiber.Map{"message": "deleted"})
}

func (h *SleepHandler) Search(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	q := c.Query("q")
	if len(q) < 1 {
		return fiber.NewError(fiber.StatusBadRequest, "query required")
	}

	resp, err := h.svc.Search(appID, userID, q)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "search failed")
	}

	return c.JSON(resp)
}

func (h *SleepHandler) GetStreak(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	resp, err := h.svc.GetStreak(appID, userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "streak fetch failed")
	}

	return c.JSON(resp)
}

func (h *SleepHandler) GetStats(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	days, _ := strconv.Atoi(c.Query("days", "7"))
	if days > 90 {
		days = 90
	}

	resp, err := h.svc.GetStats(appID, userID, days)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "stats fetch failed")
	}

	return c.JSON(resp)
}

func (h *SleepHandler) GetSleepDebt(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	goalStr := c.Query("goal", "8")
	goal := 8.0
	if g, err := strconv.ParseFloat(goalStr, 64); err == nil && g > 0 && g <= 12 {
		goal = g
	}

	resp, err := h.svc.GetSleepDebt(appID, userID, goal)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "sleep debt fetch failed")
	}

	return c.JSON(resp)
}
