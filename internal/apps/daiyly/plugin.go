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
	router.Post("/journal", handler.Create)
	router.Get("/journal", handler.List)
	router.Get("/journal/search", handler.Search)
	router.Get("/journal/streak", handler.GetStreak)
	router.Get("/journal/insights", handler.GetWeeklyInsights)

	// AI routes (MUST come before :id catch-all)
	router.Get("/journal/prompts", handler.GetPrompts)
	router.Get("/journal/weekly-report", handler.GetWeeklyReport)
	router.Get("/journal/flashbacks", handler.GetFlashbacks)
	router.Get("/journal/notification-config", handler.GetNotificationConfig)

	// Parameterized routes (MUST be last)
	router.Get("/journal/:id", handler.Get)
	router.Put("/journal/:id", handler.Update)
	router.Delete("/journal/:id", handler.Delete)
	router.Post("/journal/:id/analyze", handler.AnalyzeEntry)
	router.Get("/journal/:id/analysis", handler.GetEntryAnalysis)
}
