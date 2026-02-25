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
	}
}

func (p *DriftoffPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewSleepService(db)
	handler := NewSleepHandler(svc)

	// Sleep CRUD routes
	router.Post("/sleeps", handler.Create)
	router.Get("/sleeps", handler.List)
	router.Get("/sleeps/search", handler.Search)
	router.Get("/sleeps/streak", handler.GetStreak)
	router.Get("/sleeps/stats", handler.GetStats)
	router.Get("/sleeps/debt", handler.GetSleepDebt)
	router.Post("/sleeps/batch", handler.BatchImport)

	// Parameterized routes (MUST be last)
	router.Get("/sleeps/:id", handler.Get)
	router.Put("/sleeps/:id", handler.Update)
	router.Delete("/sleeps/:id", handler.Delete)
}
