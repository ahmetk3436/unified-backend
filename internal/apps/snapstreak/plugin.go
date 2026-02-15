package snapstreak

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type SnapstreakPlugin struct{}

func New() *SnapstreakPlugin {
	return &SnapstreakPlugin{}
}

func (p *SnapstreakPlugin) ID() string { return "snapstreak" }

func (p *SnapstreakPlugin) Models() []interface{} {
	return []interface{}{
		&Snap{},
		&SnapStreak{},
	}
}

func (p *SnapstreakPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewSnapService(db)
	handler := NewSnapHandler(svc)

	// Snap CRUD routes
	router.Post("/snaps", handler.CreateSnap)
	router.Get("/snaps", handler.GetMySnaps)
	router.Get("/snaps/streak", handler.GetStreak)
	router.Post("/snaps/streak/freeze", handler.AddFreeze)
	router.Get("/snaps/calendar", handler.GetSnapCalendar)
	router.Delete("/snaps/:id", handler.DeleteSnap)
	router.Post("/snaps/:id/like", handler.LikeSnap)
}
