package paletteai

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type PaletteAIPlugin struct{}

func New() *PaletteAIPlugin {
	return &PaletteAIPlugin{}
}

func (p *PaletteAIPlugin) ID() string { return "paletteai" }

func (p *PaletteAIPlugin) Models() []interface{} {
	return []interface{}{
		&Palette{},
		&PaletteStats{},
	}
}

func (p *PaletteAIPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewPaletteService(db)
	handler := NewPaletteHandler(svc)

	// Palette CRUD routes
	router.Post("/palettes", handler.CreatePalette)
	router.Get("/palettes", handler.ListPalettes)
	router.Get("/palettes/stats", handler.GetStats)
	router.Get("/palettes/favorites", handler.ListFavorites)
	router.Get("/palettes/:id", handler.GetPalette)
	router.Delete("/palettes/:id", handler.DeletePalette)
	router.Post("/palettes/:id/share", handler.SharePalette)
	router.Post("/palettes/:id/favorite", handler.ToggleFavorite)

	// Color utility routes
	router.Get("/colors/analyze", handler.AnalyzeColor)
	router.Get("/colors/contrast", handler.CheckContrast)
	router.Post("/colors/export", handler.ExportPalette)
	router.Get("/colors/explore", handler.GetCuratedPalettes)
}
