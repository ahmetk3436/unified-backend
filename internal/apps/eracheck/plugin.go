package eracheck

import (
	"log/slog"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/services"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type Plugin struct {
	moderationService *services.ModerationService
}

func New(moderationService *services.ModerationService) *Plugin {
	return &Plugin{moderationService: moderationService}
}

func (p *Plugin) ID() string { return "eracheck" }

func (p *Plugin) Models() []interface{} {
	return []interface{}{
		&EraQuiz{},
		&EraResult{},
		&EraChallenge{},
		&EraStreak{},
		&PhotoAnalysis{},
	}
}

func (p *Plugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	// Services
	eraService := NewEraService(db)
	streakService := NewStreakService(db)
	challengeService := NewChallengeService(db, cfg.GLMAPIURL, cfg.GLMAPIKey, p.moderationService)

	// Photo analysis service
	photoService := NewPhotoService(db, cfg.GLMAPIURL, cfg.GLMAPIKey)

	// Handlers
	eraHandler := NewEraHandler(eraService)
	challengeHandler := NewChallengeHandler(challengeService, streakService)
	photoHandler := NewPhotoHandler(photoService)

	// Seed quiz questions for this app
	if err := SeedQuizQuestions(db, "eracheck"); err != nil {
		slog.Error("failed to seed eracheck quiz questions", "error", err)
	}

	// Era Quiz routes
	router.Get("/era/questions", eraHandler.GetQuestions)
	router.Post("/era/quiz", eraHandler.SubmitQuiz)
	router.Get("/era/results", eraHandler.GetResults)
	router.Get("/era/results/:id", eraHandler.GetResult)
	router.Post("/era/results/:id/share", eraHandler.ShareResult)
	router.Get("/era/stats", eraHandler.GetStats)

	// Challenge routes (prefixed with /era to avoid collision with wouldyou /challenges)
	router.Get("/era/challenges/daily", challengeHandler.GetDailyChallenge)
	router.Post("/era/challenges/submit", challengeHandler.SubmitChallenge)
	router.Get("/era/challenges/history", challengeHandler.GetHistory)
	router.Get("/era/challenges/streak", challengeHandler.GetStreak)
	router.Post("/era/challenges/use-streak-freeze", challengeHandler.UseStreakFreeze)

	// Photo analysis route
	router.Post("/photos/analyze", photoHandler.AnalyzePhoto)

	// Alias routes without /era prefix (used by EraCheck mobile app)
	router.Get("/challenges/daily", challengeHandler.GetDailyChallenge)
	router.Post("/challenges/submit", challengeHandler.SubmitChallenge)
	router.Get("/challenges/history", challengeHandler.GetHistory)
	router.Get("/challenges/streak", challengeHandler.GetStreak)
	router.Post("/challenges/use-streak-freeze", challengeHandler.UseStreakFreeze)
}
