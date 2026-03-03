package handlers

import (
	"errors"
	"log/slog"
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
			// Return 201 with a generic message instead of 409 to prevent email enumeration.
			// A 409 "email already registered" response tells an attacker exactly which
			// emails have accounts (OWASP A07 Identification and Authentication Failures).
			// The client shows the same "account created" UI regardless; legitimate users
			// who already have accounts will discover this on their next login attempt.
			return c.Status(fiber.StatusCreated).JSON(fiber.Map{
				"message": "Registration processed. Check your email to continue.",
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
		slog.Error("login failed", "app", appID, "error", err)
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
		slog.Error("token refresh failed", "app", appID, "error", err)
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
		slog.Error("logout failed", "error", err)
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

	bundleID := h.registry.GetBundleID(appID)
	if err := h.authService.DeleteAccount(appID, userID, req.Password, req.AuthorizationCode, bundleID); err != nil {
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
		slog.Error("delete account failed", "app", appID, "user", userID, "error", err)
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
			Error: true, Message: "Sign-in not available",
		})
	}

	resp, err := h.authService.AppleSignIn(appID, bundleID, &req)
	if err != nil {
		slog.Error("apple sign-in failed", "app", appID, "error", err)
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Authentication failed",
		})
	}

	return c.JSON(resp)
}
