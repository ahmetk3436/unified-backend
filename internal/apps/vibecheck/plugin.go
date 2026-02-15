package vibecheck

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Plugin implements the apps.Plugin interface for the VibeCheck app.
type Plugin struct{}

// New creates a new vibecheck Plugin.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string { return "vibecheck" }

func (p *Plugin) Models() []interface{} {
	return []interface{}{
		&VibeCheck{},
		&VibeStreak{},
	}
}

func (p *Plugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewVibeService(db, cfg.OpenAIAPIKey)
	handler := NewVibeHandler(svc)

	router.Post("/vibes", handler.CreateVibeCheck)
	router.Post("/vibes/guest", handler.CreateGuestVibeCheck)
	router.Get("/vibes/today", handler.GetTodayCheck)
	router.Get("/vibes/history", handler.GetVibeHistory)
	router.Get("/vibes/stats", handler.GetVibeStats)
	router.Get("/vibes/trend", handler.GetVibeTrend)
}
