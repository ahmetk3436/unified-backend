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

	// WouldYou challenge routes (prefixed with /wy to avoid collision with eracheck /challenges)
	router.Get("/wy/challenges/daily", handler.GetDailyChallenge)
	router.Post("/wy/challenges/vote", handler.Vote)
	router.Get("/wy/challenges/random", handler.GetRandom)
	router.Get("/wy/challenges/category/:category", handler.GetByCategory)
	router.Get("/wy/challenges/stats", handler.GetStats)
	router.Get("/wy/challenges/history", handler.GetHistory)
}

// RegisterAdminRoutes implements apps.AdminPlugin for admin-only routes.
func (p *Plugin) RegisterAdminRoutes(admin fiber.Router, db *gorm.DB, cfg *config.Config) {
	qg := NewQuestionGeneratorService(db, cfg.GLMAPIURL, cfg.GLMAPIKey, cfg.GLMModel)
	svc := NewChallengeService(db, qg)
	handler := NewChallengeHandler(svc, qg)

	admin.Post("/wy/challenges/generate", handler.GenerateQuestions)
	admin.Post("/wy/challenges/generate-all", handler.GenerateAllCategories)
}
