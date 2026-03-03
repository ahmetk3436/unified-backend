package driftoff

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
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
	}
}

func (p *DriftoffPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewSleepService(db, cfg)
	handler := NewSleepHandler(svc)

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

	// AI-powered routes (MUST be before parameterized routes)
	router.Get("/sleeps/coach", handler.GetSleepCoach)
	router.Get("/sleeps/doctor-report", handler.GetDoctorReport)
	router.Get("/sleeps/hygiene", handler.GetHygieneScore)
	router.Post("/sleeps/caffeine", handler.LogCaffeine)
	router.Get("/sleeps/caffeine", handler.GetCaffeineLogs)

	// Correlation + CBT-I insights (MUST be before parameterized routes)
	router.Get("/sleeps/sound-correlation", handler.GetSoundCorrelation)
	router.Get("/sleeps/temp-correlation", handler.GetTempCorrelation)
	router.Get("/sleeps/cbti-insights", handler.GetCBTIInsights)

	// Parameterized routes (MUST be last)
	router.Get("/sleeps/:id", handler.Get)
	router.Put("/sleeps/:id", handler.Update)
	router.Delete("/sleeps/:id", handler.Delete)
}
