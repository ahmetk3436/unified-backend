package daiyly

import (
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"gorm.io/gorm"
)

type DaiylyPlugin struct{}

func New() *DaiylyPlugin {
	return &DaiylyPlugin{}
}

func (p *DaiylyPlugin) ID() string { return "daiyly" }

func (p *DaiylyPlugin) Models() []interface{} {
	return []interface{}{
		&JournalEntry{},
		&JournalStreak{},
		&EntryAnalysis{},
		&WeeklyReport{},
		&DailyPromptCache{},
		&NotificationConfigCache{},
		&TherapistExportCache{},
	}
}

func (p *DaiylyPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewJournalService(db, cfg.GLMAPIKey, cfg.GLMAPIURL, cfg.GLMModel, cfg.AITimeout, cfg.OpenAIAPIKey, cfg.OpenAIModel)
	handler := NewJournalHandler(svc)

	// Per-user rate limiter for AI-backed endpoints. Keyed on JWT token prefix so each
	// authenticated user gets their own bucket — prevents a single user from exhausting
	// AI compute quota or running up unbounded GLM costs.
	// weekly-report and notification-config: 5 per hour (heavy AI, some have ?refresh bypass)
	// prompts and per-entry analyze: 10 per hour (lighter, but still AI-backed)
	aiHeavyLimiter := limiter.New(limiter.Config{
		Max:               5,
		Expiration:        1 * time.Hour,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c *fiber.Ctx) string {
			auth := c.Get("Authorization")
			if len(auth) > 39 { // "Bearer " (7) + 32 chars minimum
				return "ai_heavy:" + auth[7:39]
			}
			return "ai_heavy:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "AI rate limit exceeded. Please try again in an hour.",
			})
		},
	})
	aiLightLimiter := limiter.New(limiter.Config{
		Max:               10,
		Expiration:        1 * time.Hour,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c *fiber.Ctx) string {
			auth := c.Get("Authorization")
			if len(auth) > 39 {
				return "ai_light:" + auth[7:39]
			}
			return "ai_light:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "AI rate limit exceeded. Please try again in an hour.",
			})
		},
	})

	// Journal CRUD routes
	router.Post("/journals", handler.Create)
	router.Get("/journals", handler.List)
	router.Get("/journals/search", handler.Search)
	router.Get("/journals/streak", handler.GetStreak)
	router.Get("/journals/insights", handler.GetWeeklyInsights)

	// AI routes (MUST come before :id catch-all)
	router.Get("/journals/prompts", aiLightLimiter, handler.GetPrompts)
	router.Get("/journals/weekly-report", aiHeavyLimiter, handler.GetWeeklyReport)
	router.Get("/journals/flashbacks", handler.GetFlashbacks)
	router.Get("/journals/notification-config", aiHeavyLimiter, handler.GetNotificationConfig)
	router.Get("/journals/therapist-export", aiHeavyLimiter, handler.TherapistExport)
	// /journals/therapist-report is the spec-required alias for the same feature.
	router.Get("/journals/therapist-report", aiHeavyLimiter, handler.TherapistReport)
	router.Get("/journals/notification-timing", handler.GetNotificationTiming)

	// AI semantic search and ask-your-journal (MUST come before :id catch-all)
	router.Get("/journals/ai-search", aiLightLimiter, handler.AISearch)
	router.Post("/journals/ask", aiLightLimiter, handler.AskJournal)

	// Per-user rate limiters for upload endpoints.
	// Photo: 20 uploads/hour — prevents disk exhaustion from a single authenticated user.
	// Transcribe: 10/hour — each call proxies to paid Whisper API (cost control + disk).
	uploadPhotoLimiter := limiter.New(limiter.Config{
		Max:               20,
		Expiration:        1 * time.Hour,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c *fiber.Ctx) string {
			auth := c.Get("Authorization")
			if len(auth) > 39 {
				return "upload_photo:" + auth[7:39]
			}
			return "upload_photo:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "Upload rate limit exceeded. Please try again in an hour.",
			})
		},
	})
	transcribeLimiter := limiter.New(limiter.Config{
		Max:               10,
		Expiration:        1 * time.Hour,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c *fiber.Ctx) string {
			auth := c.Get("Authorization")
			if len(auth) > 39 {
				return "transcribe:" + auth[7:39]
			}
			return "transcribe:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "Transcription rate limit exceeded. Please try again in an hour.",
			})
		},
	})

	// Upload routes — photo storage and audio transcription (MUST come before :id catch-all).
	// Protected by JWT (upstream middleware) + per-user rate limiters above.
	// Use cfg.UploadsRoot (set via UPLOADS_ROOT env var) so the path is correct
	// regardless of the process working directory. Defaults to "./uploads".
	uploadHandler := NewUploadHandler(cfg.OpenAIAPIKey, cfg.AITimeout, cfg.UploadsRoot)
	router.Post("/journals/upload-photo", uploadPhotoLimiter, uploadHandler.UploadPhoto)
	router.Post("/journals/transcribe", transcribeLimiter, uploadHandler.Transcribe)

	// Parameterized routes (MUST be last)
	router.Get("/journals/:id", handler.Get)
	router.Put("/journals/:id", handler.Update)
	router.Delete("/journals/:id", handler.Delete)
	router.Post("/journals/:id/analyze", aiLightLimiter, handler.AnalyzeEntry)
	router.Get("/journals/:id/analysis", handler.GetEntryAnalysis)
}
