package feelsy

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/services"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Plugin implements the apps.Plugin interface for the Feelsy app.
type Plugin struct {
	moderationService *services.ModerationService
}

// New creates a new feelsy Plugin.
func New(moderationService *services.ModerationService) *Plugin {
	return &Plugin{moderationService: moderationService}
}

func (p *Plugin) ID() string { return "feelsy" }

func (p *Plugin) Models() []interface{} {
	return []interface{}{
		&FeelCheck{},
		&FeelStreak{},
		&FeelFriend{},
		&GoodVibe{},
	}
}

func (p *Plugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewFeelService(db, p.moderationService)
	handler := NewFeelHandler(svc)

	// Check-in routes
	router.Post("/feels", handler.CreateFeelCheck)
	router.Get("/feels/today", handler.GetTodayCheck)
	router.Get("/feels/history", handler.GetFeelHistory)
	router.Get("/feels/stats", handler.GetFeelStats)
	router.Get("/feels/insights", handler.GetWeeklyInsights)
	router.Get("/feels/recap", handler.GetWeeklyRecap)
	router.Put("/feels/:id/journal", handler.UpdateJournal)

	// Good Vibes routes
	router.Post("/feels/vibe", handler.SendGoodVibe)
	router.Get("/feels/vibes", handler.GetReceivedVibes)

	// Friends routes
	router.Get("/feels/friends", handler.GetFriendFeels)
	router.Post("/feels/friends/add", handler.SendFriendRequest)
	router.Get("/feels/friends/requests", handler.ListFriendRequests)
	router.Get("/feels/friends/list", handler.ListFriends)
	router.Put("/feels/friends/:id/accept", handler.AcceptFriendRequest)
	router.Delete("/feels/friends/:id/reject", handler.RejectFriendRequest)
	router.Delete("/feels/friends/:id", handler.RemoveFriend)
}
