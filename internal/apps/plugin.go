package apps

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Plugin defines the interface every app must implement.
type Plugin interface {
	// ID returns the unique app identifier (must match apps.json app_id).
	ID() string

	// Models returns the list of GORM model pointers for AutoMigrate.
	Models() []interface{}

	// RegisterRoutes mounts app-specific routes on the given Fiber group.
	// The group is already prefixed with /api and has JWT middleware applied.
	RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config)
}

// AdminPlugin extends Plugin with admin-specific route registration.
// Plugins that implement this interface can register additional admin-only routes.
type AdminPlugin interface {
	Plugin

	// RegisterAdminRoutes mounts admin-only routes on the given Fiber group.
	// The group has both JWT and Admin middleware applied.
	RegisterAdminRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config)
}
