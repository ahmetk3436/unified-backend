package daiyly

import (
	"errors"
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

	response, err := h.service.SearchEntries(appID, userID, query, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: err.Error(),
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
			errors.Is(err, ErrContentInappropriate) {
			return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
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

	entries, total, err := h.service.GetEntries(appID, userID, limit, offset)
	if err != nil {
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
			errors.Is(err, ErrContentInappropriate) {
			return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
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
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch weekly insights",
		})
	}

	return c.JSON(insights)
}
