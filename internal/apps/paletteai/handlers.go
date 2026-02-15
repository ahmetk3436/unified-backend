package paletteai

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type PaletteHandler struct {
	paletteService *PaletteService
}

func NewPaletteHandler(paletteService *PaletteService) *PaletteHandler {
	return &PaletteHandler{paletteService: paletteService}
}

// --- Explore types ---

type CuratedPalette struct {
	Name   string   `json:"name"`
	Colors []string `json:"colors"`
}

type ColorFamily struct {
	Name string `json:"name"`
	Hex  string `json:"hex"`
}

type ExploreResponse struct {
	Curated    []CuratedPalette `json:"curated"`
	Families   []ColorFamily    `json:"families"`
	ColorOfDay ColorOfDay       `json:"color_of_day"`
}

type ColorOfDay struct {
	Hex  string `json:"hex"`
	Name string `json:"name"`
}

// CreatePalette handles POST /palettes
func (h *PaletteHandler) CreatePalette(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	var req CreatePaletteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid request body",
		})
	}

	palette, err := h.paletteService.CreatePalette(appID, userID, &req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to create palette",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(toPaletteResponse(palette))
}

// ListPalettes handles GET /palettes
func (h *PaletteHandler) ListPalettes(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	if limit > 100 {
		limit = 100
	}

	palettes, total, err := h.paletteService.GetUserPalettes(appID, userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch palettes",
		})
	}

	responses := make([]PaletteResponse, len(palettes))
	for i, p := range palettes {
		responses[i] = toPaletteResponse(&p)
	}

	return c.JSON(PaletteListResponse{
		Palettes: responses,
		Total:    total,
	})
}

// GetPalette handles GET /palettes/:id
func (h *PaletteHandler) GetPalette(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid palette ID",
		})
	}

	palette, err := h.paletteService.GetPaletteByID(appID, id)
	if err != nil {
		if errors.Is(err, ErrPaletteNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: "Palette not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch palette",
		})
	}

	return c.JSON(toPaletteResponse(palette))
}

// DeletePalette handles DELETE /palettes/:id
func (h *PaletteHandler) DeletePalette(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	paletteID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid palette ID",
		})
	}

	if err := h.paletteService.DeletePalette(appID, userID, paletteID); err != nil {
		if errors.Is(err, ErrPaletteNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: "Palette not found",
			})
		}
		if errors.Is(err, ErrNotOwner) {
			return c.Status(fiber.StatusForbidden).JSON(dto.ErrorResponse{
				Error: true, Message: "You do not own this palette",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to delete palette",
		})
	}

	return c.JSON(fiber.Map{"message": "Palette deleted successfully"})
}

// SharePalette handles POST /palettes/:id/share
func (h *PaletteHandler) SharePalette(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	paletteID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid palette ID",
		})
	}

	if err := h.paletteService.IncrementShareCount(appID, paletteID); err != nil {
		if errors.Is(err, ErrPaletteNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: "Palette not found",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to share palette",
		})
	}

	return c.JSON(fiber.Map{"message": "Share count incremented"})
}

// GetStats handles GET /palettes/stats
func (h *PaletteHandler) GetStats(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	stats, err := h.paletteService.GetUserStats(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch stats",
		})
	}

	return c.JSON(StatsResponse{
		TotalPalettes: stats.TotalPalettes,
		TotalShares:   stats.TotalShares,
		FavoriteColor: stats.FavoriteColor,
	})
}

// ToggleFavorite handles POST /palettes/:id/favorite
func (h *PaletteHandler) ToggleFavorite(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	paletteID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "Invalid palette ID",
		})
	}

	newStatus, err := h.paletteService.ToggleFavorite(appID, userID, paletteID)
	if err != nil {
		if errors.Is(err, ErrPaletteNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
				Error: true, Message: "Palette not found",
			})
		}
		if errors.Is(err, ErrNotOwner) {
			return c.Status(fiber.StatusForbidden).JSON(dto.ErrorResponse{
				Error: true, Message: "You do not own this palette",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to toggle favorite",
		})
	}

	return c.JSON(ToggleFavoriteResponse{
		IsFavorite: newStatus,
	})
}

// ListFavorites handles GET /palettes/favorites
func (h *PaletteHandler) ListFavorites(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
			Error: true, Message: "Unauthorized",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	palettes, total, err := h.paletteService.GetFavorites(appID, userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error: true, Message: "Failed to fetch favorites",
		})
	}

	responses := make([]PaletteResponse, len(palettes))
	for i, p := range palettes {
		responses[i] = toPaletteResponse(&p)
	}

	return c.JSON(PaletteListResponse{
		Palettes: responses,
		Total:    total,
	})
}

// AnalyzeColor handles GET /colors/analyze?hex=FF5733
func (h *PaletteHandler) AnalyzeColor(c *fiber.Ctx) error {
	hex := c.Query("hex", "")
	if hex == "" {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: "hex query parameter is required",
		})
	}

	analysis, err := h.paletteService.AnalyzeColor(hex)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error: true, Message: err.Error(),
		})
	}

	return c.JSON(analysis)
}

// CheckContrast handles GET /colors/contrast?hex1=FFFFFF&hex2=000000
func (h *PaletteHandler) CheckContrast(c *fiber.Ctx) error {
	hex1 := c.Query("hex1")
	hex2 := c.Query("hex2")

	if hex1 == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "hex1 query parameter is required",
		})
	}

	if hex2 == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "hex2 query parameter is required",
		})
	}

	hex1 = strings.TrimPrefix(hex1, "#")
	hex2 = strings.TrimPrefix(hex2, "#")

	if len(hex1) != 6 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "hex1 must be exactly 6 characters (without #)",
		})
	}

	if len(hex2) != 6 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "hex2 must be exactly 6 characters (without #)",
		})
	}

	result, err := h.paletteService.CheckContrast(hex1, hex2)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data":    result,
	})
}

// ExportPalette handles POST /colors/export
func (h *PaletteHandler) ExportPalette(c *fiber.Ctx) error {
	var req struct {
		Colors []string `json:"colors"`
		Name   string   `json:"name"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if len(req.Colors) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "colors array is required and must not be empty",
		})
	}

	if len(req.Colors) > 10 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "maximum 10 colors allowed",
		})
	}

	cleanColors := make([]string, len(req.Colors))
	for i, color := range req.Colors {
		cleanColors[i] = strings.TrimPrefix(color, "#")
	}

	result, err := h.paletteService.ExportPalette(cleanColors, req.Name)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data":    result,
	})
}

// GetCuratedPalettes returns curated palettes, color families, and color of the day
func (h *PaletteHandler) GetCuratedPalettes(c *fiber.Ctx) error {
	colorNames := []string{
		"Crimson Flame", "Amber Glow", "Golden Sun", "Emerald Dream",
		"Ocean Depths", "Royal Azure", "Violet Mist", "Rose Petal",
		"Coral Reef", "Mint Fresh", "Lavender Field", "Peach Blossom",
	}

	curatedPalettes := []CuratedPalette{
		{Name: "Sunset Glow", Colors: []string{"#FF6B6B", "#FFA07A", "#FFD700", "#FF8C00", "#FF4500"}},
		{Name: "Ocean Breeze", Colors: []string{"#00CED1", "#20B2AA", "#48D1CC", "#40E0D0", "#00BFFF"}},
		{Name: "Forest Dreams", Colors: []string{"#228B22", "#32CD32", "#90EE90", "#8FBC8F", "#006400"}},
		{Name: "Berry Bliss", Colors: []string{"#8B008B", "#DA70D6", "#EE82EE", "#DDA0DD", "#BA55D3"}},
		{Name: "Earth Tones", Colors: []string{"#8B4513", "#A0522D", "#CD853F", "#DEB887", "#D2B48C"}},
	}

	colorFamilies := []ColorFamily{
		{Name: "Red", Hex: "#EF4444"},
		{Name: "Orange", Hex: "#F97316"},
		{Name: "Yellow", Hex: "#EAB308"},
		{Name: "Green", Hex: "#22C55E"},
		{Name: "Teal", Hex: "#14B8A6"},
		{Name: "Blue", Hex: "#3B82F6"},
		{Name: "Purple", Hex: "#8B5CF6"},
		{Name: "Pink", Hex: "#EC4899"},
	}

	colorOfDay := calculateColorOfDay(colorNames)

	return c.JSON(ExploreResponse{
		Curated:    curatedPalettes,
		Families:   colorFamilies,
		ColorOfDay: colorOfDay,
	})
}

// --- helpers ---

func calculateColorOfDay(colorNames []string) ColorOfDay {
	now := time.Now()
	dayOfYear := now.YearDay()
	hue := float64((dayOfYear * 137) % 360)
	saturation := 0.65
	lightness := 0.55
	hex := hslToHexExplore(hue, saturation, lightness)

	nameIndex := int(hue / 30)
	if nameIndex >= len(colorNames) {
		nameIndex = len(colorNames) - 1
	}

	return ColorOfDay{Hex: hex, Name: colorNames[nameIndex]}
}

func hslToHexExplore(h, s, l float64) string {
	cc := (1 - math.Abs(2*l-1)) * s
	x := cc * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - cc/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = cc, x, 0
	case h < 120:
		r, g, b = x, cc, 0
	case h < 180:
		r, g, b = 0, cc, x
	case h < 240:
		r, g, b = 0, x, cc
	case h < 300:
		r, g, b = x, 0, cc
	default:
		r, g, b = cc, 0, x
	}

	r = math.Round((r + m) * 255)
	g = math.Round((g + m) * 255)
	b = math.Round((b + m) * 255)

	return fmt.Sprintf("#%02X%02X%02X", int(r), int(g), int(b))
}

func toPaletteResponse(p *Palette) PaletteResponse {
	return PaletteResponse{
		ID:         p.ID,
		UserID:     p.UserID,
		ImageURL:   p.ImageURL,
		Color1:     p.Color1,
		Color2:     p.Color2,
		Color3:     p.Color3,
		Color4:     p.Color4,
		Color5:     p.Color5,
		Name:       p.Name,
		ShareCount: p.ShareCount,
		IsFavorite: p.IsFavorite,
		CreatedAt:  p.CreatedAt,
	}
}
