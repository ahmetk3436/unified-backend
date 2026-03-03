package daiyly

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type JournalHandler struct {
	service *JournalService
}

func NewJournalHandler(service *JournalService) *JournalHandler {
	return &JournalHandler{service: service}
}

func (h *JournalHandler) Search(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	query := c.Query("q")
	if len(query) < 2 {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "search query must be at least 2 characters",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	if offset < 0 {
		offset = 0
	}
	if offset > 10000 {
		offset = 0
	}

	response, err := h.service.SearchEntries(appID, userID, query, limit, offset)
	if err != nil {
		slog.Error("search entries failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "search failed",
		})
	}

	return c.JSON(response)
}

func (h *JournalHandler) Create(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	var req CreateJournalRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	entry, err := h.service.CreateEntry(appID, userID, req)
	if err != nil {
		if errors.Is(err, ErrInvalidMoodEmoji) ||
			errors.Is(err, ErrInvalidMoodScore) ||
			errors.Is(err, ErrInvalidCardColor) ||
			errors.Is(err, ErrInvalidPhotoURL) ||
			errors.Is(err, ErrInvalidAudioURL) ||
			errors.Is(err, ErrContentInappropriate) {
			return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		slog.Error("create journal entry failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to create journal entry",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(entry)
}

func (h *JournalHandler) List(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if offset > 10000 {
		offset = 0
	}

	entries, total, err := h.service.GetEntries(appID, userID, limit, offset)
	if err != nil {
		slog.Error("list journal entries failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch journal entries",
		})
	}

	return c.JSON(JournalListResponse{
		Entries: entries,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	})
}

func (h *JournalHandler) Get(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	entryID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid entry ID",
		})
	}

	entry, err := h.service.GetEntry(appID, userID, entryID)
	if err != nil {
		if errors.Is(err, ErrJournalNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		if errors.Is(err, ErrNotOwner) {
			return c.Status(fiber.StatusForbidden).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		slog.Error("get journal entry failed", "app", appID, "user", userID, "entry", entryID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch journal entry",
		})
	}

	return c.JSON(entry)
}

func (h *JournalHandler) Update(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	entryID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid entry ID",
		})
	}

	var req UpdateJournalRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	entry, err := h.service.UpdateEntry(appID, userID, entryID, req)
	if err != nil {
		if errors.Is(err, ErrJournalNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		if errors.Is(err, ErrNotOwner) {
			return c.Status(fiber.StatusForbidden).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		if errors.Is(err, ErrInvalidMoodEmoji) ||
			errors.Is(err, ErrInvalidMoodScore) ||
			errors.Is(err, ErrInvalidCardColor) ||
			errors.Is(err, ErrInvalidPhotoURL) ||
			errors.Is(err, ErrInvalidAudioURL) ||
			errors.Is(err, ErrContentInappropriate) {
			return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		slog.Error("update journal entry failed", "app", appID, "user", userID, "entry", entryID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to update journal entry",
		})
	}

	return c.JSON(entry)
}

func (h *JournalHandler) Delete(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	entryID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid entry ID",
		})
	}

	err = h.service.DeleteEntry(appID, userID, entryID)
	if err != nil {
		if errors.Is(err, ErrJournalNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		if errors.Is(err, ErrNotOwner) {
			return c.Status(fiber.StatusForbidden).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		slog.Error("delete journal entry failed", "app", appID, "user", userID, "entry", entryID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to delete journal entry",
		})
	}

	return c.JSON(DeleteJournalResponse{
		Message: "Entry deleted successfully",
	})
}

func (h *JournalHandler) GetStreak(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	streak, err := h.service.GetStreak(appID, userID)
	if err != nil {
		slog.Error("get streak failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch streak",
		})
	}

	return c.JSON(streak)
}

func (h *JournalHandler) GetWeeklyInsights(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	insights, err := h.service.GetWeeklyInsights(appID, userID)
	if err != nil {
		slog.Error("get weekly insights failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch weekly insights",
		})
	}

	return c.JSON(insights)
}

func (h *JournalHandler) GetPrompts(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	prompts, err := h.service.GetPersonalizedPrompts(appID, userID)
	if err != nil {
		slog.Error("get prompts failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to generate prompts",
		})
	}

	return c.JSON(prompts)
}

func (h *JournalHandler) GetWeeklyReport(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	forceRefresh := c.Query("refresh") == "true"
	report, err := h.service.GetWeeklyReport(appID, userID, forceRefresh)
	if err != nil {
		slog.Error("get weekly report failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to generate weekly report",
		})
	}

	return c.JSON(report)
}

func (h *JournalHandler) GetFlashbacks(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	flashbacks, err := h.service.GetFlashbacks(appID, userID)
	if err != nil {
		slog.Error("get flashbacks failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch flashbacks",
		})
	}

	return c.JSON(flashbacks)
}

func (h *JournalHandler) GetNotificationConfig(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	config, err := h.service.GetNotificationConfig(appID, userID)
	if err != nil {
		slog.Error("get notification config failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to generate notification config",
		})
	}

	return c.JSON(config)
}

func (h *JournalHandler) AnalyzeEntry(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	entryID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid entry ID",
		})
	}

	// Verify ownership
	if _, err := h.service.GetEntry(appID, userID, entryID); err != nil {
		if errors.Is(err, ErrJournalNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		if errors.Is(err, ErrNotOwner) {
			return c.Status(fiber.StatusForbidden).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		slog.Error("verify entry for analysis failed", "app", appID, "user", userID, "entry", entryID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to verify entry",
		})
	}

	if err := h.service.TriggerAnalysis(appID, userID, entryID); err != nil {
		slog.Error("trigger analysis failed", "app", appID, "user", userID, "entry", entryID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to trigger analysis",
		})
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"message": "Analysis started",
	})
}

func (h *JournalHandler) GetEntryAnalysis(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	entryID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid entry ID",
		})
	}

	analysis, err := h.service.GetEntryAnalysis(appID, userID, entryID)
	if err != nil {
		if errors.Is(err, ErrAnalysisNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: "Analysis not available yet",
			})
		}
		slog.Error("get entry analysis failed", "app", appID, "user", userID, "entry", entryID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch analysis",
		})
	}

	return c.JSON(analysis)
}

// TherapistExport returns an AI-generated therapist-ready summary of the last 30 days.
// PREMIUM feature: gating is noted with a TODO below pending subscription check wiring.
func (h *JournalHandler) TherapistExport(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	// TODO: Check subscription entitlement here once RevenueCat webhook is wired up.
	// if !isPremium(c) { return c.Status(fiber.StatusPaymentRequired).JSON(...) }

	report, err := h.service.TherapistExport(appID, userID)
	if err != nil {
		slog.Error("therapist export failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to generate therapist export",
		})
	}

	return c.JSON(report)
}

// TherapistReport returns the spec-compatible therapist report envelope for GET /journals/therapist-report.
// It wraps TherapistExport output into a simpler {report, generated_at, entry_count, date_range} shape.
// PREMIUM feature: gating is noted with a TODO below pending subscription check wiring.
func (h *JournalHandler) TherapistReport(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	// TODO: Check subscription entitlement here once RevenueCat webhook is wired up.
	// if !isPremium(c) { return c.Status(fiber.StatusPaymentRequired).JSON(...) }

	report, err := h.service.TherapistReport(appID, userID)
	if err != nil {
		slog.Error("therapist report failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to generate therapist report",
		})
	}

	return c.JSON(report)
}

// GetNotificationTiming returns the user's optimal journaling hour based on the last 30 days.
func (h *JournalHandler) GetNotificationTiming(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	timing, err := h.service.GetNotificationTiming(appID, userID)
	if err != nil {
		slog.Error("get notification timing failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to compute notification timing",
		})
	}

	return c.JSON(timing)
}

// AISearch performs semantic journal search using GPT-4o-mini.
// GET /journals/ai-search?q=...&limit=10&days=90
func (h *JournalHandler) AISearch(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	query := c.Query("q")
	if len(query) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "query parameter 'q' is required",
		})
	}
	if len(query) > 500 {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "query must be at most 500 characters",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	days, _ := strconv.Atoi(c.Query("days", "90"))
	if days <= 0 || days > 365 {
		days = 90
	}

	result, err := h.service.AISearchEntries(appID, userID, query, limit, days)
	if err != nil {
		slog.Error("ai search failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "AI search failed",
		})
	}

	return c.JSON(result)
}

// AskJournal answers a natural-language question about the user's journal.
// POST /journals/ask
func (h *JournalHandler) AskJournal(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	var req AskJournalRequest
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
	if len(req.Question) > 1000 {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "question must be at most 1000 characters",
		})
	}

	result, err := h.service.AskJournal(appID, userID, req.Question)
	if err != nil {
		slog.Error("ask journal failed", "app", appID, "user", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to answer question",
		})
	}

	return c.JSON(result)
}
