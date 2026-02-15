package ecomonitor

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type EcoMonitorPlugin struct{}

func New() *EcoMonitorPlugin {
	return &EcoMonitorPlugin{}
}

func (p *EcoMonitorPlugin) ID() string { return "ecomonitor" }

func (p *EcoMonitorPlugin) Models() []interface{} {
	return []interface{}{
		&Coordinate{},
		&SatelliteData{},
		&AnalysisHistory{},
	}
}

func (p *EcoMonitorPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	coordService := NewCoordinateService(db)
	satelliteService := NewSatelliteService(db, cfg)
	historyService := NewHistoryService(db)
	exportService := NewExportService(db)

	coordHandler := NewCoordinateHandler(coordService)
	satelliteHandler := NewSatelliteHandler(satelliteService, historyService)
	historyHandler := NewHistoryHandler(historyService)
	exportHandler := NewExportHandler(exportService, db)

	// Coordinate routes
	router.Post("/coordinates", coordHandler.CreateCoordinate)
	router.Get("/coordinates", coordHandler.ListCoordinates)
	router.Get("/coordinates/:id", coordHandler.GetCoordinate)
	router.Put("/coordinates/:id", coordHandler.UpdateCoordinate)
	router.Delete("/coordinates/:id", coordHandler.DeleteCoordinate)

	// Satellite analysis routes
	router.Post("/coordinates/:id/analyze", satelliteHandler.GenerateAnalysis)
	router.Get("/coordinates/:id/analysis", satelliteHandler.GetAnalysis)
	router.Get("/alerts", satelliteHandler.GetAlerts)

	// History routes
	router.Get("/history", historyHandler.GetHistory)

	// Export routes
	router.Get("/export/csv", exportHandler.ExportCSV)
}
