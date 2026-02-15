package aurascan

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type AuraScanPlugin struct{}

func New() *AuraScanPlugin {
	return &AuraScanPlugin{}
}

func (p *AuraScanPlugin) ID() string { return "aurascan" }

func (p *AuraScanPlugin) Models() []interface{} {
	return []interface{}{
		&AuraReading{},
		&AuraMatch{},
		&AuraStreak{},
	}
}

func (p *AuraScanPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	auraService := NewAuraService(db, cfg)
	matchService := NewAuraMatchService(db, cfg)
	streakService := NewStreakService(db)

	auraHandler := NewAuraHandler(auraService)
	matchHandler := NewAuraMatchHandler(matchService)
	streakHandler := NewStreakHandler(streakService)

	// Aura scan routes
	router.Get("/aura/eligibility", auraHandler.CheckScanEligibility)
	router.Post("/aura/scan", auraHandler.Scan)
	router.Post("/aura/scan/upload", auraHandler.ScanWithUpload)
	router.Get("/aura/readings", auraHandler.List)
	router.Get("/aura/readings/:id", auraHandler.GetByID)
	router.Get("/aura/stats", auraHandler.Stats)

	// Match routes
	router.Post("/aura/match", matchHandler.CreateMatch)
	router.Get("/aura/matches", matchHandler.GetMatches)
	router.Get("/aura/matches/:friend_id", matchHandler.GetMatchByFriend)

	// Streak routes
	router.Get("/aura/streak", streakHandler.GetStreak)
	router.Post("/aura/streak", streakHandler.UpdateStreak)
}
