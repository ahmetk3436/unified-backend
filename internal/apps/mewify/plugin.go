package mewify

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Mewify is a shared-only app — no custom models or routes.
type Plugin struct{}

func New() *Plugin { return &Plugin{} }

func (p *Plugin) ID() string { return "mewify" }

func (p *Plugin) Models() []interface{} { return nil }

func (p *Plugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {}
