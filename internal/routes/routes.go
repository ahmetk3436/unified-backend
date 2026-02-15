package routes

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/handlers"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/middleware"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func Setup(
	app *fiber.App,
	cfg *config.Config,
	db *gorm.DB,
	authHandler *handlers.AuthHandler,
	healthHandler *handlers.HealthHandler,
	webhookHandler *handlers.WebhookHandler,
	moderationHandler *handlers.ModerationHandler,
	legalHandler *handlers.LegalHandler,
	plugins []apps.Plugin,
) {
	api := app.Group("/api")

	// Health (no tenant required)
	api.Get("/health", healthHandler.Check)

	// Legal pages (tenant optional for display)
	api.Get("/legal/privacy", legalHandler.PrivacyPolicy)
	api.Get("/legal/terms", legalHandler.TermsOfService)

	// Auth — public (tenant middleware already applied globally)
	auth := api.Group("/auth")
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.Refresh)
	auth.Post("/apple", authHandler.AppleSignIn)

	// Protected routes (JWT required)
	protected := api.Group("", middleware.JWTProtected(cfg))
	protected.Post("/auth/logout", authHandler.Logout)
	protected.Delete("/auth/account", authHandler.DeleteAccount)

	// Moderation — user endpoints (protected)
	protected.Post("/reports", moderationHandler.CreateReport)
	protected.Post("/blocks", moderationHandler.BlockUser)
	protected.Delete("/blocks/:id", moderationHandler.UnblockUser)

	// Admin moderation panel (protected + admin required)
	admin := api.Group("/admin", middleware.JWTProtected(cfg), middleware.AdminRequired(db, cfg))
	admin.Get("/moderation/reports", moderationHandler.ListReports)
	admin.Put("/moderation/reports/:id", moderationHandler.ActionReport)

	// Webhooks — per-app auth via :app_id path param (no JWT)
	webhooks := api.Group("/webhooks")
	webhooks.Post("/revenuecat/:app_id", webhookHandler.HandleRevenueCat)

	// Plugin routes — each plugin registers its own routes on the protected group
	for _, p := range plugins {
		p.RegisterRoutes(protected, db, cfg)
	}
}
