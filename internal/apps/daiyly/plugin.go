package daiyly

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type DaiylyPlugin struct{}

func New() *DaiylyPlugin {
	return &DaiylyPlugin{}
}

func (p *DaiylyPlugin) ID() string { return "daiyly" }

func (p *DaiylyPlugin) Models() []interface{} {
	return []interface{}{
		&JournalEntry{},
		&JournalStreak{},
		&EntryAnalysis{},
		&WeeklyReport{},
		&DailyPromptCache{},
		&NotificationConfigCache{},
	}
}

func (p *DaiylyPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewJournalService(db, cfg.GLMAPIKey, cfg.GLMAPIURL, cfg.GLMModel, cfg.AITimeout)
	handler := NewJournalHandler(svc)

	// Journal CRUD routes
	router.Post("/journals", handler.Create)
	router.Get("/journals", handler.List)
	router.Get("/journals/search", handler.Search)
	router.Get("/journals/streak", handler.GetStreak)
	router.Get("/journals/insights", handler.GetWeeklyInsights)

	// AI routes (MUST come before :id catch-all)
	router.Get("/journals/prompts", handler.GetPrompts)
	router.Get("/journals/weekly-report", handler.GetWeeklyReport)
	router.Get("/journals/flashbacks", handler.GetFlashbacks)
	router.Get("/journals/notification-config", handler.GetNotificationConfig)

	// Parameterized routes (MUST be last)
	router.Get("/journals/:id", handler.Get)
	router.Put("/journals/:id", handler.Update)
	router.Delete("/journals/:id", handler.Delete)
	router.Post("/journals/:id/analyze", handler.AnalyzeEntry)
	router.Get("/journals/:id/analysis", handler.GetEntryAnalysis)
}
