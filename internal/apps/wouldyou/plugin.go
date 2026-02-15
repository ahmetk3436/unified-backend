package wouldyou

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Plugin implements the apps.Plugin interface for the WouldYou app.
type Plugin struct{}

// New creates a new wouldyou Plugin.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ID() string { return "wouldyou" }

func (p *Plugin) Models() []interface{} {
	return []interface{}{
		&Challenge{},
		&Vote{},
		&ChallengeStreak{},
	}
}

func (p *Plugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	qg := NewQuestionGeneratorService(db, cfg.GLMAPIURL, cfg.GLMAPIKey, cfg.GLMModel)
	svc := NewChallengeService(db, qg)
	handler := NewChallengeHandler(svc, qg)

	// Public-ish routes (JWT is on the router group already, but guests can also vote)
	router.Get("/challenges/daily", handler.GetDailyChallenge)
	router.Post("/challenges/vote", handler.Vote)
	router.Get("/challenges/random", handler.GetRandom)
	router.Get("/challenges/category/:category", handler.GetByCategory)
	router.Get("/challenges/stats", handler.GetStats)
	router.Get("/challenges/history", handler.GetHistory)

	// Admin routes (would need additional admin middleware in routes.go)
	router.Post("/admin/challenges/generate", handler.GenerateQuestions)
	router.Post("/admin/challenges/generate-all", handler.GenerateAllCategories)
}
