package moodtracker

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// MoodTracker is a mood tracking app with journals and insights.
type Plugin struct{}

func New() *Plugin { return &Plugin{} }

func (p *Plugin) ID() string { return "moodtracker" }

func (p *Plugin) Models() []interface{} { return nil }

func (p *Plugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {}
