package moodpulse

import (
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"gorm.io/gorm"
)

type MoodPulsePlugin struct{}

func New() *MoodPulsePlugin {
	return &MoodPulsePlugin{}
}

func (p *MoodPulsePlugin) ID() string { return "moodpulse" }

func (p *MoodPulsePlugin) Models() []interface{} {
	return []interface{}{
		&MoodCheckIn{},
		&MoodStreak{},
		&CustomEmotion{},
		&CustomTrigger{},
		&CustomActivity{},
	}
}

func (p *MoodPulsePlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewMoodService(db, cfg)
	uploadHandler := NewUploadHandler(cfg.OpenAIAPIKey, cfg.AITimeout, cfg.UploadsRoot)
	handler := NewMoodHandler(svc, uploadHandler)

	// Per-user rate limiter for AI-backed endpoints.
	// Keyed on JWT token prefix so each authenticated user has their own bucket.
	aiLimiter := limiter.New(limiter.Config{
		Max:               10,
		Expiration:        1 * time.Hour,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c *fiber.Ctx) string {
			auth := c.Get("Authorization")
			if len(auth) > 39 { // "Bearer " (7) + 32 chars minimum
				return "mood_ai:" + auth[7:39]
			}
			return "mood_ai:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "AI rate limit exceeded. Please try again in an hour.",
			})
		},
	})

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
				return "mood_upload_photo:" + auth[7:39]
			}
			return "mood_upload_photo:ip:" + c.IP()
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
				return "mood_transcribe:" + auth[7:39]
			}
			return "mood_transcribe:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "Transcription rate limit exceeded. Please try again in an hour.",
			})
		},
	})

	// Core mood CRUD routes
	router.Post("/moods", handler.Create)
	router.Get("/moods", handler.List)
	router.Get("/moods/search", handler.Search)
	router.Get("/moods/calendar", handler.Calendar)
	router.Get("/moods/streak", handler.GetStreak)
	router.Get("/moods/stats", handler.GetStats)
	router.Post("/moods/batch", handler.BatchCreate)
	router.Post("/moods/batch-delete", handler.BatchDelete)

	// AI routes (MUST come before :id catch-all)
	router.Get("/moods/ai-insights", aiLimiter, handler.AIInsights)
	router.Post("/moods/ask", aiLimiter, handler.Ask)

	// Upload routes — photo storage and audio transcription (MUST come before :id catch-all).
	router.Post("/moods/upload-photo", uploadPhotoLimiter, handler.UploadPhoto)
	router.Post("/moods/transcribe", transcribeLimiter, handler.Transcribe)

	// Feature endpoints — CBT exercises, mood drivers, mood forecast
	router.Post("/moods/cbt", aiLimiter, handler.GetCBTExercise)
	router.Get("/moods/drivers", handler.GetMoodDrivers)
	router.Get("/moods/forecast", handler.GetMoodForecast)

	// Context tagging + medication tracking insights (MUST be before :id catch-all)
	router.Get("/moods/context-insights", handler.GetContextInsights)
	router.Get("/moods/med-correlation", handler.GetMedCorrelation)
	router.Get("/moods/sub-emotions", handler.GetSubEmotions)

	// Crisis check — DB-only, no AI, no extra rate limit (MUST be before :id catch-all)
	router.Get("/moods/crisis-check", handler.CrisisCheck)

	// Parameterized routes last
	router.Get("/moods/:id", handler.Get)
	router.Put("/moods/:id", handler.Update)
	router.Delete("/moods/:id", handler.Delete)

	// Custom vocabulary
	vocSvc := NewVocabularyService(db)
	vocHandler := NewVocabularyHandler(vocSvc)

	router.Get("/vocabulary/emotions", vocHandler.ListEmotions)
	router.Post("/vocabulary/emotions", vocHandler.CreateEmotion)
	router.Delete("/vocabulary/emotions/:id", vocHandler.DeleteEmotion)
	router.Get("/vocabulary/triggers", vocHandler.ListTriggers)
	router.Post("/vocabulary/triggers", vocHandler.CreateTrigger)
	router.Delete("/vocabulary/triggers/:id", vocHandler.DeleteTrigger)
	router.Get("/vocabulary/activities", vocHandler.ListActivities)
	router.Post("/vocabulary/activities", vocHandler.CreateActivity)
	router.Delete("/vocabulary/activities/:id", vocHandler.DeleteActivity)
	router.Post("/vocabulary/sync", vocHandler.BulkSync)
}
