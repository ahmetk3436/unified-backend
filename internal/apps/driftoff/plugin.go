package driftoff

import (
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"gorm.io/gorm"
)

type DriftoffPlugin struct{}

func New() *DriftoffPlugin {
	return &DriftoffPlugin{}
}

func (p *DriftoffPlugin) ID() string { return "driftoff" }

func (p *DriftoffPlugin) Models() []interface{} {
	return []interface{}{
		&SleepSession{},
		&SleepStreak{},
		&DailyCaffeineLog{},
		&AlertnessLog{},
		&DreamEntry{},
	}
}

func (p *DriftoffPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewSleepService(db, cfg)
	handler := NewSleepHandler(svc)

	// Per-user rate limiter for AI-backed endpoints. Keyed on JWT token prefix so each
	// authenticated user gets their own bucket — prevents AI cost abuse.
	// Heavy AI (coach, doctor-report): 5 per hour
	// Light AI (cbti-insights): 10 per hour
	aiHeavyLimiter := limiter.New(limiter.Config{
		Max:               5,
		Expiration:        1 * time.Hour,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c *fiber.Ctx) string {
			auth := c.Get("Authorization")
			if len(auth) > 39 { // "Bearer " (7) + 32 chars minimum
				return "sleep_ai_heavy:" + auth[7:39]
			}
			return "sleep_ai_heavy:ip:" + c.IP()
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
				return "sleep_ai_light:" + auth[7:39]
			}
			return "sleep_ai_light:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "AI rate limit exceeded. Please try again in an hour.",
			})
		},
	})

	// Sleep CRUD routes
	router.Post("/sleeps", handler.Create)
	router.Get("/sleeps", handler.List)
	router.Get("/sleeps/search", handler.Search)
	router.Get("/sleeps/streak", handler.GetStreak)
	router.Get("/sleeps/stats", handler.GetStats)
	router.Get("/sleeps/debt", handler.GetSleepDebt)
	router.Post("/sleeps/batch", handler.BatchImport)

	// Export
	router.Get("/sleeps/export", handler.ExportSleepData)

	// AI-powered routes (rate-limited; MUST be before parameterized routes)
	router.Get("/sleeps/coach", aiHeavyLimiter, handler.GetSleepCoach)
	router.Get("/sleeps/doctor-report", aiHeavyLimiter, handler.GetDoctorReport)
	router.Get("/sleeps/hygiene", handler.GetHygieneScore)
	router.Post("/sleeps/caffeine", handler.LogCaffeine)
	router.Get("/sleeps/caffeine", handler.GetCaffeineLogs)

	// Correlation + CBT-I + SRI insights (MUST be before parameterized routes)
	router.Get("/sleeps/sound-correlation", handler.GetSoundCorrelation)
	router.Get("/sleeps/temp-correlation", handler.GetTempCorrelation)
	router.Get("/sleeps/cbti-insights", aiLightLimiter, handler.GetCBTIInsights)
	router.Get("/sleeps/lifestyle-correlation", handler.GetLifestyleCorrelation)
	router.Get("/sleeps/sri", handler.GetSleepRegularityIndex)

	// Daytime alertness check-ins (UMD 2026 clinical trial pattern)
	router.Post("/sleeps/alertness", handler.LogAlertness)
	router.Get("/sleeps/alertness", handler.GetAlertnessLogs)

	// Nap optimizer (MUST be before parameterized routes)
	router.Get("/sleeps/nap-optimizer", handler.GetNapOptimizer)

	// Dream journal (MUST be before parameterized routes)
	router.Post("/sleeps/dream", handler.CreateDream)
	router.Get("/sleeps/dreams", handler.ListDreams)

	// Parameterized routes (MUST be last)
	router.Get("/sleeps/:id", handler.Get)
	router.Put("/sleeps/:id", handler.Update)
	router.Delete("/sleeps/:id", handler.Delete)
}
