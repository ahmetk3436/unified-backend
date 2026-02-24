package moodpulse

import (
	"strconv"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type MoodHandler struct {
	svc *MoodService
}

func NewMoodHandler(svc *MoodService) *MoodHandler {
	return &MoodHandler{svc: svc}
}

func (h *MoodHandler) Create(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	var req CreateMoodRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	entry, err := h.svc.Create(appID, userID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(entry)
}

func (h *MoodHandler) List(c *fiber.Ctx) error {
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

func (h *MoodHandler) Get(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	entry, err := h.svc.Get(appID, userID, id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	return c.JSON(entry)
}

func (h *MoodHandler) Update(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}

	var req UpdateMoodRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	entry, err := h.svc.Update(appID, userID, id, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(entry)
}

func (h *MoodHandler) Delete(c *fiber.Ctx) error {
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

func (h *MoodHandler) Search(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	q := c.Query("q")
	if len(q) < 2 {
		return fiber.NewError(fiber.StatusBadRequest, "query too short")
	}

	resp, err := h.svc.Search(appID, userID, q)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "search failed")
	}

	return c.JSON(resp)
}

func (h *MoodHandler) GetStreak(c *fiber.Ctx) error {
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

func (h *MoodHandler) GetStats(c *fiber.Ctx) error {
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

// VocabularyHandler handles custom vocabulary endpoints.
type VocabularyHandler struct {
	svc *VocabularyService
}

func NewVocabularyHandler(svc *VocabularyService) *VocabularyHandler {
	return &VocabularyHandler{svc: svc}
}

func (h *VocabularyHandler) ListEmotions(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}
	items, err := h.svc.ListEmotions(appID, userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(items)
}

func (h *VocabularyHandler) CreateEmotion(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}
	var req CreateCustomEmotionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	item, err := h.svc.UpsertEmotion(appID, userID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *VocabularyHandler) DeleteEmotion(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	if err := h.svc.DeleteEmotion(appID, userID, id); err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *VocabularyHandler) ListTriggers(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}
	items, err := h.svc.ListTriggers(appID, userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(items)
}

func (h *VocabularyHandler) CreateTrigger(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}
	var req CreateCustomTriggerRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	item, err := h.svc.UpsertTrigger(appID, userID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *VocabularyHandler) DeleteTrigger(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	if err := h.svc.DeleteTrigger(appID, userID, id); err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *VocabularyHandler) ListActivities(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}
	items, err := h.svc.ListActivities(appID, userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(items)
}

func (h *VocabularyHandler) CreateActivity(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}
	var req CreateCustomActivityRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	item, err := h.svc.UpsertActivity(appID, userID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *VocabularyHandler) DeleteActivity(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	if err := h.svc.DeleteActivity(appID, userID, id); err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *VocabularyHandler) BulkSync(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}
	var req BulkSyncVocabularyRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	resp, err := h.svc.BulkSync(appID, userID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(resp)
}
