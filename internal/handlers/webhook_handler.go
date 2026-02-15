package handlers

import (
	"crypto/subtle"
	"log/slog"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/services"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
)

type WebhookHandler struct {
	subscriptionService *services.SubscriptionService
	registry            *tenant.Registry
}

func NewWebhookHandler(subscriptionService *services.SubscriptionService, registry *tenant.Registry) *WebhookHandler {
	return &WebhookHandler{
		subscriptionService: subscriptionService,
		registry:            registry,
	}
}

// HandleRevenueCat routes webhooks by :app_id path param with per-app auth.
func (h *WebhookHandler) HandleRevenueCat(c *fiber.Ctx) error {
	appID := c.Params("app_id")
	if appID == "" || !h.registry.Exists(appID) {
		return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
			Error: true, Message: "Unknown app",
		})
	}

	expectedAuth := h.registry.GetWebhookAuth(appID)
	if expectedAuth == "" {
		return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
			Error: true, Message: "Webhooks not configured for this app",
		})
	}

	authHeader := c.Get("Authorization")
	if subtle.ConstantTimeCompare([]byte(authHeader), []byte(expectedAuth)) != 1 {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	var webhook dto.RevenueCatWebhook
	if err := c.BodyParser(&webhook); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid webhook payload",
		})
	}

	if err := h.subscriptionService.HandleWebhookEvent(appID, &webhook.Event); err != nil {
		slog.Error("webhook processing failed", "app_id", appID, "event_type", webhook.Event.Type, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to process webhook event",
		})
	}

	slog.Info("webhook processed", "app_id", appID, "event_type", webhook.Event.Type)
	return c.JSON(fiber.Map{"received": true})
}
