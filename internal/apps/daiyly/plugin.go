package daiyly

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
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
	}
}

func (p *DaiylyPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewJournalService(db)
	handler := NewJournalHandler(svc)

	// Journal CRUD routes
	router.Post("/journal", handler.Create)
	router.Get("/journal", handler.List)
	router.Get("/journal/search", handler.Search)
	router.Get("/journal/streak", handler.GetStreak)
	router.Get("/journal/insights", handler.GetWeeklyInsights)
	router.Get("/journal/:id", handler.Get)
	router.Put("/journal/:id", handler.Update)
	router.Delete("/journal/:id", handler.Delete)
}
