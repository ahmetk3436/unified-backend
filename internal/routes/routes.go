package routes

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/lucky_draw"
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
		// c.IP() is now safe: TrustedProxies in main.go ensures real client IP
		KeyGenerator: func(c *fiber.Ctx) string { return c.IP() },
	}))
	auth.Post("/register", authHandler.Register)

	// Login gets an additional per-email limiter (5 attempts/min per email).
	// IP-based limits alone can be bypassed via X-Forwarded-For spoofing, but
	// per-email limits cannot — attackers can't spoof the target account's email.
	loginEmailLimiter := limiter.New(limiter.Config{
		Max:               5,
		Expiration:        1 * time.Minute,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c *fiber.Ctx) string {
			var body struct {
				Email string `json:"email"`
			}
			if err := json.Unmarshal(c.Body(), &body); err == nil && body.Email != "" {
				return "login:email:" + strings.ToLower(strings.TrimSpace(body.Email))
			}
			return "login:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "Too many login attempts. Please try again later.",
			})
		},
	})
	auth.Post("/login", loginEmailLimiter, authHandler.Login)
	auth.Post("/refresh", authHandler.Refresh)

	// Apple Sign In gets an additional per-token limiter (5 attempts/min per token prefix)
	// to prevent rapid replay of stolen Apple identity tokens.
	appleSignInLimiter := limiter.New(limiter.Config{
		Max:               5,
		Expiration:        1 * time.Minute,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c *fiber.Ctx) string {
			var body struct {
				IdentityToken string `json:"identity_token"`
			}
			if err := json.Unmarshal(c.Body(), &body); err == nil && len(body.IdentityToken) > 0 {
				tok := body.IdentityToken
				if len(tok) > 32 {
					tok = tok[:32]
				}
				return "apple:tok:" + tok
			}
			return "apple:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "Too many sign-in attempts. Please try again later.",
			})
		},
	})
	auth.Post("/apple", appleSignInLimiter, authHandler.AppleSignIn)

	// Protected routes (JWT required) - apply middleware to individual routes
	api.Post("/auth/logout", middleware.JWTProtected(cfg), authHandler.Logout)

	// Account deletion: 1 successful attempt per user per day is more than enough.
	// Per-user key (from JWT sub claim in body or fallback to IP) prevents DoS
	// where an attacker fires rapid DELETEs with a stolen token.
	deleteAccountLimiter := limiter.New(limiter.Config{
		Max:               3,
		Expiration:        24 * time.Hour,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c *fiber.Ctx) string {
			// Use Authorization header prefix as user-scoped key — the full token
			// is too long to use as a key; the first 32 chars are sufficient
			// to distinguish users without leaking the full credential.
			auth := c.Get("Authorization")
			if len(auth) > 39 { // "Bearer " (7) + 32 chars minimum
				return "del_acct:" + auth[7:39]
			}
			return "del_acct:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "Too many deletion attempts. Please try again later.",
			})
		},
	})
	api.Delete("/auth/account", middleware.JWTProtected(cfg), deleteAccountLimiter, authHandler.DeleteAccount)

	// Moderation — user endpoints (protected)
	// Per-user report limiter (5/hour). The global 60 req/min per-IP is insufficient;
	// a single authenticated user could spam 60 reports before it triggers. Keyed on
	// JWT token prefix so each user gets their own bucket.
	reportLimiter := limiter.New(limiter.Config{
		Max:               5,
		Expiration:        1 * time.Hour,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c *fiber.Ctx) string {
			auth := c.Get("Authorization")
			if len(auth) > 39 { // "Bearer " (7) + 32 chars minimum
				return "report:" + auth[7:39]
			}
			return "report:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "Too many reports submitted. Please try again later.",
			})
		},
	})
	api.Post("/reports", middleware.JWTProtected(cfg), reportLimiter, moderationHandler.CreateReport)
	api.Post("/blocks", middleware.JWTProtected(cfg), moderationHandler.BlockUser)
	api.Delete("/blocks/:id", middleware.JWTProtected(cfg), moderationHandler.UnblockUser)

	// Admin moderation panel (protected + admin required)
	// Strict rate limiter (10 req/min per IP) protects admin token brute-force.
	adminLimiter := limiter.New(limiter.Config{
		Max:               10,
		Expiration:        1 * time.Minute,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator:      func(c *fiber.Ctx) string { return c.IP() },
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "Too many requests. Please try again later.",
			})
		},
	})
	admin := api.Group("/admin", adminLimiter, middleware.JWTProtected(cfg), middleware.AdminRequired(db, cfg))
	admin.Get("/moderation/reports", moderationHandler.ListReports)
	admin.Put("/moderation/reports/:id", moderationHandler.ActionReport)

	// Admin config management (protected + admin required)
	admin.Put("/config/:key", configHandler.SetConfigKey)
	admin.Delete("/config/:key", configHandler.DeleteConfigKey)

	// Webhooks — per-app auth via :app_id path param (no JWT)
	webhooks := api.Group("/webhooks")
	webhooks.Post("/revenuecat/:app_id", webhookHandler.HandleRevenueCat)

	// Public plugin routes (no JWT required) - for guest-friendly plugins like lucky_draw
	public := api.Group("/p")

	// LuckyDraw public endpoint for guest mode (rate limited separately)
	luckyDrawPublicLimiter := limiter.New(limiter.Config{
		Max:               20,
		Expiration:        1 * time.Hour,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator:      func(c *fiber.Ctx) string { return "lucky_draw:public:" + c.IP() },
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "Rate limit exceeded. Please try again in an hour.",
			})
		},
	})
	luckyDrawSvc := lucky_draw.NewLuckyDrawService(db, cfg)
	luckyDrawHandler := lucky_draw.NewLuckyDrawHandler(luckyDrawSvc)
	// Public: /api/p/lucky_draw/draw (guest mode - no JWT required)
	public.Post("/lucky_draw/draw", luckyDrawPublicLimiter, luckyDrawHandler.Create)

	// Protected plugin routes (JWT required)
	protected := api.Group("/p", middleware.JWTProtected(cfg))

	// Register lucky_draw protected endpoints
	protected.Get("/lucky_draw", luckyDrawHandler.List)
	protected.Get("/lucky_draw/:id", luckyDrawHandler.Get)
	protected.Delete("/lucky_draw/:id", luckyDrawHandler.Delete)
	protected.Get("/lucky_draw/stats", luckyDrawHandler.GetStats)
	protected.Get("/lucky_draw/history", luckyDrawHandler.GetHistory)

	// Register other plugins (non-lucky_draw)
	for _, p := range plugins {
		if p.ID() != "lucky_draw" {
			p.RegisterRoutes(protected, db, cfg)
		}
		// If the plugin also implements AdminPlugin, register admin routes
		if ap, ok := p.(apps.AdminPlugin); ok {
			ap.RegisterAdminRoutes(admin, db, cfg)
		}
	}
}
