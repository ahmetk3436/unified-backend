package rizzcheck

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Plugin implements the apps.Plugin interface for the RizzCheck app.
type Plugin struct{}

// New creates a new rizzcheck Plugin.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string { return "rizzcheck" }

func (p *Plugin) Models() []interface{} {
	return []interface{}{
		&RizzResponse{},
		&RizzStreak{},
	}
}

func (p *Plugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewRizzService(db, cfg)
	handler := NewRizzHandler(svc)

	router.Post("/rizz/generate", handler.Generate)
	router.Get("/rizz/stats", handler.GetStats)
	router.Get("/rizz/history", handler.GetHistory)
	router.Post("/rizz/select", handler.SelectResponse)
}
