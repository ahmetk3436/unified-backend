package moodpulse

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
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
	}
}

func (p *MoodPulsePlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewMoodService(db)
	handler := NewMoodHandler(svc)

	router.Post("/moods", handler.Create)
	router.Get("/moods", handler.List)
	router.Get("/moods/search", handler.Search)
	router.Get("/moods/streak", handler.GetStreak)
	router.Get("/moods/stats", handler.GetStats)

	// Parameterized routes last
	router.Get("/moods/:id", handler.Get)
	router.Put("/moods/:id", handler.Update)
	router.Delete("/moods/:id", handler.Delete)
}
