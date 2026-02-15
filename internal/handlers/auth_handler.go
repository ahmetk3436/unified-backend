package handlers

import (
	"errors"
	"strings"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/services"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
)

type AuthHandler struct {
	authService *services.AuthService
	registry    *tenant.Registry
}

func NewAuthHandler(authService *services.AuthService, registry *tenant.Registry) *AuthHandler {
	return &AuthHandler{authService: authService, registry: registry}
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	var req dto.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	resp, err := h.authService.Register(appID, &req)
	if err != nil {
		if errors.Is(err, services.ErrEmailTaken) {
			return c.Status(fiber.StatusConflict).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(resp)
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	var req dto.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	resp, err := h.authService.Login(appID, &req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidCredentials) {
			return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Internal server error",
		})
	}

	return c.JSON(resp)
}

func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	var req dto.RefreshRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	resp, err := h.authService.Refresh(appID, &req)
	if err != nil {
		if errors.Is(err, services.ErrInvalidToken) {
			return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
				Error: true, Message: err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Internal server error",
		})
	}

	return c.JSON(resp)
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	var req dto.LogoutRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	if err := h.authService.Logout(appID, &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to logout",
		})
	}

	return c.JSON(fiber.Map{"message": "Logged out successfully"})
}

func (h *AuthHandler) DeleteAccount(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	var req dto.DeleteAccountRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	if err := h.authService.DeleteAccount(appID, userID, req.Password); err != nil {
		if errors.Is(err, services.ErrInvalidCredentials) {
			return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
				Error: true, Message: "Incorrect password. Please try again.",
			})
		}
		if errors.Is(err, services.ErrUserNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: "User not found",
			})
		}
		if strings.Contains(err.Error(), "password is required") {
			return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
				Error: true, Message: "Password is required",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to delete account",
		})
	}

	return c.JSON(fiber.Map{"message": "Account deleted successfully"})
}

func (h *AuthHandler) AppleSignIn(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	var req dto.AppleSignInRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	if req.IdentityToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Identity token is required",
		})
	}

	bundleID := h.registry.GetBundleID(appID)
	if bundleID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Apple Sign In not configured for this app",
		})
	}

	resp, err := h.authService.AppleSignIn(appID, bundleID, &req)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: err.Error(),
		})
	}

	return c.JSON(resp)
}
