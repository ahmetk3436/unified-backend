package streakkeeper

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// StreakKeeper is a habit tracking and streak maintenance app.
type Plugin struct{}

func New() *Plugin { return &Plugin{} }

func (p *Plugin) ID() string { return "streakkeeper" }

func (p *Plugin) Models() []interface{} { return nil }

func (p *Plugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {}
