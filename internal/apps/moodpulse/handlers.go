package moodpulse

import (
	"log/slog"
	"strconv"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type MoodHandler struct {
	svc            *MoodService
	uploadHandler  *UploadHandler
}

func NewMoodHandler(svc *MoodService, uploadHandler *UploadHandler) *MoodHandler {
	return &MoodHandler{svc: svc, uploadHandler: uploadHandler}
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
	month, _ := strconv.Atoi(c.Query("month", "0"))
	year, _ := strconv.Atoi(c.Query("year", "0"))
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

	resp, err := h.svc.List(appID, userID, limit, offset, month, year)
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

func (h *MoodHandler) BatchCreate(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	var req BatchCreateMoodRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	if len(req.Entries) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "no entries provided")
	}
	if len(req.Entries) > 100 {
		return fiber.NewError(fiber.StatusBadRequest, "max 100 entries per batch")
	}

	resp, err := h.svc.BatchCreate(appID, userID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(resp)
}

func (h *MoodHandler) BatchDelete(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	var req BatchDeleteMoodRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	resp, err := h.svc.BatchDelete(appID, userID, req)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "batch delete failed")
	}

	return c.JSON(resp)
}

func (h *MoodHandler) Calendar(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid auth")
	}

	month, _ := strconv.Atoi(c.Query("month"))
	year, _ := strconv.Atoi(c.Query("year"))
	if month < 1 || month > 12 {
		return fiber.NewError(fiber.StatusBadRequest, "month must be 1-12")
	}
	if year < 2000 || year > 2100 {
		return fiber.NewError(fiber.StatusBadRequest, "invalid year")
	}

	resp, err := h.svc.Calendar(appID, userID, month, year)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "calendar fetch failed")
	}

	return c.JSON(resp)
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

// AIInsights handles GET /moods/ai-insights?days=30
// Returns longitudinal mood analysis from GPT-4o-mini. Pro-gated via JWT auth.
func (h *MoodHandler) AIInsights(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	days, _ := strconv.Atoi(c.Query("days", "30"))
	if days < 1 {
		days = 1
	}
	if days > 90 {
		days = 90
	}

	insights, err := h.svc.AIInsights(appID, userID, days)
	if err != nil {
		slog.Error("[moodpulse] ai insights failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "AI insights unavailable",
		})
	}

	return c.JSON(AIInsightsResponse{Insights: insights})
}

// Ask handles POST /moods/ask with body {"question": "..."}
// Answers a natural-language question about the user's mood history. Pro-gated via JWT auth.
func (h *MoodHandler) Ask(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	var req AskMoodRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	if len(req.Question) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "question is required",
		})
	}
	if len(req.Question) > 500 {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "question must be at most 500 characters",
		})
	}

	answer, err := h.svc.AskMood(appID, userID, req.Question)
	if err != nil {
		slog.Error("[moodpulse] ask mood failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to answer question",
		})
	}

	return c.JSON(AskMoodResponse{Answer: answer})
}

// UploadPhoto delegates to the UploadHandler (POST /moods/upload-photo).
func (h *MoodHandler) UploadPhoto(c *fiber.Ctx) error {
	return h.uploadHandler.UploadPhoto(c)
}

// Transcribe delegates to the UploadHandler (POST /moods/transcribe).
func (h *MoodHandler) Transcribe(c *fiber.Ctx) error {
	return h.uploadHandler.Transcribe(c)
}

// GetCBTExercise handles POST /moods/cbt
// Accepts {"emotion": "Anxiety", "intensity": 8} and returns a tailored CBT exercise.
func (h *MoodHandler) GetCBTExercise(c *fiber.Ctx) error {
	_, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	var body struct {
		Emotion   string `json:"emotion"`
		Intensity int    `json:"intensity"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}
	if body.Emotion == "" {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "emotion is required",
		})
	}
	if body.Intensity < 1 || body.Intensity > 10 {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "intensity must be between 1 and 10",
		})
	}

	result, err := h.svc.GetCBTExercise(body.Emotion, body.Intensity)
	if err != nil {
		slog.Error("[moodpulse] cbt exercise failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "CBT exercise unavailable",
		})
	}

	return c.JSON(result)
}

// GetMoodDrivers handles GET /moods/drivers?days=90
// Returns trigger and activity correlations with mood intensity over the given period.
func (h *MoodHandler) GetMoodDrivers(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	days, _ := strconv.Atoi(c.Query("days", "90"))
	if days < 1 {
		days = 1
	}
	if days > 90 {
		days = 90
	}

	drivers, err := h.svc.GetMoodDrivers(appID, userID, days)
	if err != nil {
		slog.Error("[moodpulse] mood drivers failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Mood drivers unavailable",
		})
	}

	return c.JSON(drivers)
}

// GetMoodForecast handles GET /moods/forecast
// Returns a simple mood forecast for the next 3 days based on historical patterns.
func (h *MoodHandler) GetMoodForecast(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	forecast, err := h.svc.GetMoodForecast(appID, userID)
	if err != nil {
		slog.Error("[moodpulse] mood forecast failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Mood forecast unavailable",
		})
	}

	return c.JSON(forecast)
}

// GetContextInsights handles GET /moods/context-insights?days=30
// Returns average mood intensity per context category (where/with/activity).
func (h *MoodHandler) GetContextInsights(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	days, _ := strconv.Atoi(c.Query("days", "30"))
	if days < 1 {
		days = 1
	}
	if days > 90 {
		days = 90
	}

	resp, err := h.svc.GetContextInsights(appID, userID, days)
	if err != nil {
		slog.Error("[moodpulse] context insights failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Context insights unavailable",
		})
	}

	return c.JSON(resp)
}

// GetMedCorrelation handles GET /moods/med-correlation?med_name=X&days=30
// Returns average mood intensity on medication-taken days vs not-taken days.
func (h *MoodHandler) GetMedCorrelation(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	medName := c.Query("med_name")
	if medName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "med_name is required",
		})
	}
	if len(medName) > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "med_name must be at most 100 characters",
		})
	}

	days, _ := strconv.Atoi(c.Query("days", "30"))
	if days < 1 {
		days = 1
	}
	if days > 90 {
		days = 90
	}

	resp, err := h.svc.GetMedCorrelation(appID, userID, medName, days)
	if err != nil {
		slog.Error("[moodpulse] med correlation failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Medication correlation unavailable",
		})
	}

	return c.JSON(resp)
}

// GetSubEmotions handles GET /moods/sub-emotions
// Returns the static sub-emotion vocabulary map. No DB access, no auth needed
// beyond the existing JWT middleware applied at router level.
func (h *MoodHandler) GetSubEmotions(c *fiber.Ctx) error {
	_, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}
	return c.JSON(h.svc.GetSubEmotions())
}

// CrisisCheck handles GET /moods/crisis-check.
// Returns whether the user is in a crisis pattern (5+ consecutive low-mood days).
// Light DB-only endpoint — no AI call, no extra rate limit needed.
func (h *MoodHandler) CrisisCheck(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	resp, err := h.svc.GetCrisisCheck(appID, userID)
	if err != nil {
		slog.Error("[moodpulse] crisis check failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Crisis check unavailable",
		})
	}

	return c.JSON(resp)
}
