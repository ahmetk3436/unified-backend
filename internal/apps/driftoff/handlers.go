package driftoff

import (
	"strconv"
	"time"

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
	if days < 1 {
		days = 1
	}
	if days > 90 {
		days = 90
	}

	resp, err := h.svc.GetStats(appID, userID, days)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "stats fetch failed")
	}

	return c.JSON(resp)
}

func (h *SleepHandler) BatchImport(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	var req BatchImportRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	if len(req.Sessions) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "no sessions provided")
	}
	if len(req.Sessions) > 100 {
		return fiber.NewError(fiber.StatusBadRequest, "max 100 sessions per batch")
	}

	resp, err := h.svc.BatchImport(appID, userID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "batch import failed")
	}

	return c.Status(fiber.StatusCreated).JSON(resp)
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

// ExportSleepData returns all sleep sessions as CSV or JSON download.
func (h *SleepHandler) ExportSleepData(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	format := c.Query("format", "csv")
	if format != "csv" && format != "json" {
		return fiber.NewError(fiber.StatusBadRequest, "format must be csv or json")
	}

	data, mimeType, err := h.svc.ExportSleepData(appID, userID, format)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "export failed")
	}

	ext := format
	filename := "driftoff_sleep_export_" + time.Now().Format("2006-01-02") + "." + ext
	c.Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Set("Content-Type", mimeType)
	return c.Send(data)
}

// GetSleepCoach returns AI-generated personalised sleep coaching. Cached 6h per user.
func (h *SleepHandler) GetSleepCoach(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	coaching, err := h.svc.GetSleepCoach(appID, userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "coaching unavailable")
	}

	return c.JSON(fiber.Map{"coaching": coaching})
}

// GetDoctorReport returns a clinical sleep summary. PREMIUM feature.
func (h *SleepHandler) GetDoctorReport(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	// PREMIUM feature: gating is noted with a TODO below pending subscription check wiring.
	// TODO: Check subscription entitlement here once RevenueCat webhook is wired up.
	// if !isPremium(c) { return c.Status(fiber.StatusPaymentRequired).JSON(...) }

	report, err := h.svc.GetDoctorReport(appID, userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "report unavailable")
	}

	return c.JSON(fiber.Map{"report": report})
}

// GetHygieneScore returns the sleep hygiene score breakdown. Free feature.
func (h *SleepHandler) GetHygieneScore(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	score, err := h.svc.GetHygieneScore(appID, userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "hygiene score unavailable")
	}

	return c.JSON(score)
}

// LogCaffeine upserts today's caffeine and exercise log.
func (h *SleepHandler) LogCaffeine(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	var req LogCaffeineRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	if req.CaffeineML < 0 || req.CaffeineML > 10000 {
		return fiber.NewError(fiber.StatusBadRequest, "caffeine_ml must be between 0 and 10000")
	}
	if req.ExerciseMin < 0 || req.ExerciseMin > 1440 {
		return fiber.NewError(fiber.StatusBadRequest, "exercise_min must be between 0 and 1440")
	}

	var lastCupAt *time.Time
	if req.LastCupAt != nil && *req.LastCupAt != "" {
		t, err := time.Parse(time.RFC3339, *req.LastCupAt)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid last_cup_at format (use RFC3339)")
		}
		lastCupAt = &t
	}

	log, err := h.svc.LogCaffeine(appID, userID, req.CaffeineML, req.ExerciseMin, lastCupAt)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "caffeine log failed")
	}

	return c.Status(fiber.StatusCreated).JSON(log)
}

// GetCaffeineLogs returns caffeine logs for the last N days.
func (h *SleepHandler) GetCaffeineLogs(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	days, _ := strconv.Atoi(c.Query("days", "30"))
	if days < 1 {
		days = 1
	}
	if days > 90 {
		days = 90
	}

	logs, err := h.svc.GetCaffeineLog(appID, userID, days)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "fetch caffeine logs failed")
	}

	return c.JSON(fiber.Map{"logs": logs, "days": days})
}
