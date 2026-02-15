package handlers

import (
	"errors"
	"strconv"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/services"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ModerationHandler struct {
	moderationService *services.ModerationService
}

func NewModerationHandler(moderationService *services.ModerationService) *ModerationHandler {
	return &ModerationHandler{moderationService: moderationService}
}

func (h *ModerationHandler) CreateReport(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	var req dto.CreateReportRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	report, err := h.moderationService.CreateReport(appID, userID, &req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(report)
}

func (h *ModerationHandler) BlockUser(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	blockerID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	var req dto.BlockUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	if err := h.moderationService.BlockUser(appID, blockerID, req.BlockedID); err != nil {
		if errors.Is(err, services.ErrSelfBlock) || errors.Is(err, services.ErrAlreadyBlocked) {
			return c.Status(fiber.StatusConflict).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to block user",
		})
	}

	return c.JSON(fiber.Map{"message": "User blocked successfully"})
}

func (h *ModerationHandler) UnblockUser(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	blockerID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	blockedID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid user ID",
		})
	}

	if err := h.moderationService.UnblockUser(appID, blockerID, blockedID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to unblock user",
		})
	}

	return c.JSON(fiber.Map{"message": "User unblocked successfully"})
}

func (h *ModerationHandler) ListReports(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	status := c.Query("status", "")
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	if limit > 100 {
		limit = 100
	}

	reports, total, err := h.moderationService.ListReports(appID, status, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch reports",
		})
	}

	return c.JSON(fiber.Map{
		"reports": reports,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

func (h *ModerationHandler) ActionReport(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	reportID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid report ID",
		})
	}

	var req dto.ActionReportRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	if err := h.moderationService.ActionReport(appID, reportID, &req); err != nil {
		if errors.Is(err, services.ErrReportNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: err.Error(),
		})
	}

	return c.JSON(fiber.Map{"message": "Report updated successfully"})
}
