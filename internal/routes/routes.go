package routes

import (
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/handlers"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
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
	configHandler *handlers.RemoteConfigHandler,
	plugins []apps.Plugin,
) {
	api := app.Group("/api")

	// General API rate limiter: 60 req/min per IP
	api.Use(limiter.New(limiter.Config{
		Max:               60,
		Expiration:        1 * time.Minute,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator:      func(c *fiber.Ctx) string { return c.IP() },
	}))

	// Health (no tenant required)
	api.Get("/health", healthHandler.Check)

	// Remote Config (public, tenant-scoped via X-App-ID header)
	api.Get("/config", configHandler.GetConfig)

	// Legal pages (tenant optional for display)
	api.Get("/legal/privacy", legalHandler.PrivacyPolicy)
	api.Get("/legal/terms", legalHandler.TermsOfService)

	// Auth — public (tenant middleware already applied globally)
	// Auth-specific rate limit: 10 req/min per IP (stricter)
	auth := api.Group("/auth")
	auth.Use(limiter.New(limiter.Config{
		Max:               10,
		Expiration:        1 * time.Minute,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator:      func(c *fiber.Ctx) string { return c.IP() },
	}))
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.Refresh)
	auth.Post("/apple", authHandler.AppleSignIn)

	// Protected routes (JWT required) - apply middleware to individual routes
	// This prevents JWT middleware from affecting public routes
	api.Post("/auth/logout", middleware.JWTProtected(cfg), authHandler.Logout)
	api.Delete("/auth/account", middleware.JWTProtected(cfg), authHandler.DeleteAccount)

	// Moderation — user endpoints (protected)
	api.Post("/reports", middleware.JWTProtected(cfg), moderationHandler.CreateReport)
	api.Post("/blocks", middleware.JWTProtected(cfg), moderationHandler.BlockUser)
	api.Delete("/blocks/:id", middleware.JWTProtected(cfg), moderationHandler.UnblockUser)

	// Admin moderation panel (protected + admin required)
	admin := api.Group("/admin", middleware.JWTProtected(cfg), middleware.AdminRequired(db, cfg))
	admin.Get("/moderation/reports", moderationHandler.ListReports)
	admin.Put("/moderation/reports/:id", moderationHandler.ActionReport)

	// Admin config management (protected + admin required)
	admin.Put("/config/:app_id/:key", configHandler.SetConfigKey)
	admin.Delete("/config/:app_id/:key", configHandler.DeleteConfigKey)

	// Webhooks — per-app auth via :app_id path param (no JWT)
	webhooks := api.Group("/webhooks")
	webhooks.Post("/revenuecat/:app_id", webhookHandler.HandleRevenueCat)

	// Plugin routes - create a protected group for plugins only
	// This ensures JWT middleware doesn't affect public routes
	protected := api.Group("/p", middleware.JWTProtected(cfg))
	for _, p := range plugins {
		p.RegisterRoutes(protected, db, cfg)
		// If the plugin also implements AdminPlugin, register admin routes
		if ap, ok := p.(apps.AdminPlugin); ok {
			ap.RegisterAdminRoutes(admin, db, cfg)
		}
	}
}
