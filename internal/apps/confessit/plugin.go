package confessit

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type ConfessitPlugin struct{}

func New() *ConfessitPlugin {
	return &ConfessitPlugin{}
}

func (p *ConfessitPlugin) ID() string { return "confessit" }

func (p *ConfessitPlugin) Models() []interface{} {
	return []interface{}{
		&Confession{},
		&ConfessionLike{},
		&ConfessionComment{},
		&ConfessionStreak{},
		&ConfessionReaction{},
	}
}

func (p *ConfessitPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, _ *config.Config) {
	svc := NewConfessionService(db)
	h := NewConfessionHandler(svc)

	// Public feed routes (no JWT required, but app_id comes from tenant middleware)
	router.Get("/confessions/feed", h.GetFeed)
	router.Get("/confessions/trending", h.GetTrending)
	router.Get("/confessions/category/:category", h.GetByCategory)
	router.Get("/confessions/:id", h.GetByID)
	router.Get("/confessions/:id/comments", h.GetComments)
	router.Get("/confessions/:id/reactions", h.GetReactions)

	// Protected routes (require JWT)
	router.Post("/confessions", h.Create)
	router.Post("/confessions/:id/like", h.Like)
	router.Post("/confessions/:id/comment", h.AddComment)
	router.Post("/confessions/:id/share", h.Share)
	router.Post("/confessions/:id/react", h.React)
	router.Delete("/confessions/:id", h.Delete)
	router.Get("/confessions/my/list", h.GetMyConfessions)
	router.Get("/confessions/my/stats", h.GetStats)
}
