package paletteai

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrPaletteNotFound = errors.New("palette not found")
	ErrNotOwner        = errors.New("you do not own this palette")
)

type PaletteService struct {
	db *gorm.DB
}

func NewPaletteService(db *gorm.DB) *PaletteService {
	return &PaletteService{db: db}
}

func (s *PaletteService) CreatePalette(appID string, userID uuid.UUID, req *CreatePaletteRequest) (*Palette, error) {
	name := req.Name
	if name == "" {
		name = "Untitled Palette"
	}

	palette := Palette{
		AppID:  appID,
		UserID: userID,
		Color1: req.Color1,
		Color2: req.Color2,
		Color3: req.Color3,
		Color4: req.Color4,
		Color5: req.Color5,
		Name:   name,
	}

	if err := s.db.Create(&palette).Error; err != nil {
		return nil, fmt.Errorf("failed to create palette: %w", err)
	}

	// Increment user stats
	var stats PaletteStats
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&stats).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		stats = PaletteStats{
			AppID:         appID,
			UserID:        userID,
			TotalPalettes: 1,
		}
		s.db.Create(&stats)
	} else if err == nil {
		s.db.Model(&stats).Update("total_palettes", stats.TotalPalettes+1)
	}

	return &palette, nil
}

func (s *PaletteService) GetUserPalettes(appID string, userID uuid.UUID, limit, offset int) ([]Palette, int64, error) {
	var palettes []Palette
	var total int64

	query := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID)
	query.Model(&Palette{}).Count(&total)

	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&palettes).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch palettes: %w", err)
	}

	return palettes, total, nil
}

func (s *PaletteService) GetPaletteByID(appID string, id uuid.UUID) (*Palette, error) {
	var palette Palette
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&palette, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaletteNotFound
		}
		return nil, fmt.Errorf("failed to fetch palette: %w", err)
	}
	return &palette, nil
}

func (s *PaletteService) DeletePalette(appID string, userID, paletteID uuid.UUID) error {
	var palette Palette
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&palette, "id = ?", paletteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPaletteNotFound
		}
		return fmt.Errorf("failed to fetch palette: %w", err)
	}

	if palette.UserID != userID {
		return ErrNotOwner
	}

	return s.db.Delete(&palette).Error
}

func (s *PaletteService) IncrementShareCount(appID string, paletteID uuid.UUID) error {
	result := s.db.Model(&Palette{}).Scopes(tenant.ForTenant(appID)).Where("id = ?", paletteID).
		UpdateColumn("share_count", gorm.Expr("share_count + 1"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPaletteNotFound
	}

	// Also increment stats for the palette owner
	var palette Palette
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&palette, "id = ?", paletteID).Error; err == nil {
		s.db.Model(&PaletteStats{}).Scopes(tenant.ForTenant(appID)).Where("user_id = ?", palette.UserID).
			UpdateColumn("total_shares", gorm.Expr("total_shares + 1"))
	}

	return nil
}

func (s *PaletteService) GetUserStats(appID string, userID uuid.UUID) (*PaletteStats, error) {
	var stats PaletteStats
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&stats).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		stats = PaletteStats{
			AppID:         appID,
			UserID:        userID,
			TotalPalettes: 0,
			TotalShares:   0,
		}
		s.db.Create(&stats)
		return &stats, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stats: %w", err)
	}
	return &stats, nil
}

func (s *PaletteService) SaveGuestPalette(appID string, colors []string, name string) (*Palette, error) {
	if name == "" {
		name = "Guest Palette"
	}
	c1, c2, c3, c4, c5 := "", "", "", "", ""
	if len(colors) > 0 {
		c1 = colors[0]
	}
	if len(colors) > 1 {
		c2 = colors[1]
	}
	if len(colors) > 2 {
		c3 = colors[2]
	}
	if len(colors) > 3 {
		c4 = colors[3]
	}
	if len(colors) > 4 {
		c5 = colors[4]
	}

	palette := Palette{
		AppID:  appID,
		Color1: c1,
		Color2: c2,
		Color3: c3,
		Color4: c4,
		Color5: c5,
		Name:   name,
	}

	if err := s.db.Create(&palette).Error; err != nil {
		return nil, fmt.Errorf("failed to save guest palette: %w", err)
	}

	return &palette, nil
}

func (s *PaletteService) ToggleFavorite(appID string, userID, paletteID uuid.UUID) (bool, error) {
	var palette Palette
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&palette, "id = ?", paletteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrPaletteNotFound
		}
		return false, fmt.Errorf("failed to fetch palette: %w", err)
	}

	if palette.UserID != userID {
		return false, ErrNotOwner
	}

	palette.IsFavorite = !palette.IsFavorite
	if err := s.db.Save(&palette).Error; err != nil {
		return false, fmt.Errorf("failed to update palette: %w", err)
	}

	return palette.IsFavorite, nil
}

func (s *PaletteService) GetFavorites(appID string, userID uuid.UUID, limit, offset int) ([]Palette, int64, error) {
	var palettes []Palette
	var total int64

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	if err := s.db.Model(&Palette{}).Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND is_favorite = ?", userID, true).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count favorites: %w", err)
	}

	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND is_favorite = ?", userID, true).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&palettes).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to fetch favorites: %w", err)
	}

	return palettes, total, nil
}

// CheckContrast calculates WCAG 2.1 contrast ratio between two hex colors.
func (s *PaletteService) CheckContrast(hex1, hex2 string) (*ContrastCheckResponse, error) {
	hex1 = strings.TrimPrefix(hex1, "#")
	hex2 = strings.TrimPrefix(hex2, "#")

	r1, g1, b1, err := hexToRGB(hex1)
	if err != nil {
		return nil, fmt.Errorf("invalid hex1: %w", err)
	}

	r2, g2, b2, err := hexToRGB(hex2)
	if err != nil {
		return nil, fmt.Errorf("invalid hex2: %w", err)
	}

	L1 := relativeLuminance(r1, g1, b1)
	L2 := relativeLuminance(r2, g2, b2)

	lighter := math.Max(L1, L2)
	darker := math.Min(L1, L2)

	ratio := (lighter + 0.05) / (darker + 0.05)
	ratio = math.Round(ratio*100) / 100

	level := "Fail"
	if ratio >= 7.0 {
		level = "AAA"
	} else if ratio >= 4.5 {
		level = "AA"
	}

	return &ContrastCheckResponse{
		Ratio:          ratio,
		Level:          level,
		LargeTextPass:  ratio >= 3.0,
		NormalTextPass: ratio >= 4.5,
	}, nil
}

// ExportPalette generates text export formats for a color palette.
func (s *PaletteService) ExportPalette(colors []string, name string) (*PaletteExportResponse, error) {
	if len(colors) == 0 {
		return nil, fmt.Errorf("at least one color is required")
	}

	for i, hex := range colors {
		if len(hex) != 6 {
			return nil, fmt.Errorf("color at index %d must be 6 characters", i)
		}
		for _, c := range hex {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return nil, fmt.Errorf("color at index %d contains invalid hex character", i)
			}
		}
	}

	hexList := "#" + strings.Join(colors, ", #")

	var cssVars strings.Builder
	cssVars.WriteString(":root {\n")
	for i, hex := range colors {
		cssVars.WriteString(fmt.Sprintf("  --color-%d: #%s;\n", i+1, hex))
	}
	cssVars.WriteString("}")

	var tailwind strings.Builder
	tailwind.WriteString("colors: {\n")
	if name != "" {
		tailwind.WriteString(fmt.Sprintf("  %s: {\n", strings.ToLower(strings.ReplaceAll(name, " ", "_"))))
		for i, hex := range colors {
			tailwind.WriteString(fmt.Sprintf("    %d: '#%s',\n", i+1, hex))
		}
		tailwind.WriteString("  },\n")
	} else {
		for i, hex := range colors {
			tailwind.WriteString(fmt.Sprintf("  custom-%d: '#%s',\n", i+1, hex))
		}
	}
	tailwind.WriteString("}")

	var text strings.Builder
	if name != "" {
		text.WriteString(fmt.Sprintf("Palette: %s\n\n", name))
	} else {
		text.WriteString("Color Palette\n\n")
	}
	for i, hex := range colors {
		text.WriteString(fmt.Sprintf("Color %d: #%s\n", i+1, hex))
	}
	text.WriteString(fmt.Sprintf("\nHex List: %s", hexList))

	return &PaletteExportResponse{
		Text:     text.String(),
		HexList:  hexList,
		CssVars:  cssVars.String(),
		Tailwind: tailwind.String(),
	}, nil
}

// AnalyzeColor computes detailed color analysis from a hex string.
func (s *PaletteService) AnalyzeColor(hex string) (*ColorAnalysisResponse, error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return nil, errors.New("hex must be 6 characters (e.g. FF5733)")
	}

	r, g, b, err := hexToRGB(hex)
	if err != nil {
		return nil, err
	}

	h, sat, l := rgbToHSL(r, g, b)

	compH := math.Mod(h+180, 360)
	complement := hslToHex(compH, sat, l)

	analog1 := hslToHex(math.Mod(h+30+360, 360), sat, l)
	analog2 := hslToHex(math.Mod(h-30+360, 360), sat, l)

	triad1 := hslToHex(math.Mod(h+120, 360), sat, l)
	triad2 := hslToHex(math.Mod(h+240, 360), sat, l)

	mood := determineMood(h, sat, l)
	temp := determineTemperature(h)
	name := getColorName(r, g, b)

	return &ColorAnalysisResponse{
		Hex:  "#" + strings.ToUpper(hex),
		Name: name,
		RGB: RGBValue{
			R: r,
			G: g,
			B: b,
		},
		HSL: HSLValue{
			H: math.Round(h*10) / 10,
			S: math.Round(sat*1000) / 10,
			L: math.Round(l*1000) / 10,
		},
		Complement:  complement,
		Analogous:   []string{analog1, analog2},
		Triadic:     []string{triad1, triad2},
		Mood:        mood,
		Temperature: temp,
	}, nil
}

// --- Color helpers ---

func linearize(value float64) float64 {
	if value <= 0.03928 {
		return value / 12.92
	}
	return math.Pow((value+0.055)/1.055, 2.4)
}

func relativeLuminance(r, g, b int) float64 {
	rLinear := linearize(float64(r) / 255.0)
	gLinear := linearize(float64(g) / 255.0)
	bLinear := linearize(float64(b) / 255.0)
	return 0.2126*rLinear + 0.7152*gLinear + 0.0722*bLinear
}

func hexToRGB(hex string) (int, int, int, error) {
	r, err := strconv.ParseInt(hex[0:2], 16, 64)
	if err != nil {
		return 0, 0, 0, errors.New("invalid hex value")
	}
	g, err := strconv.ParseInt(hex[2:4], 16, 64)
	if err != nil {
		return 0, 0, 0, errors.New("invalid hex value")
	}
	b, err := strconv.ParseInt(hex[4:6], 16, 64)
	if err != nil {
		return 0, 0, 0, errors.New("invalid hex value")
	}
	return int(r), int(g), int(b), nil
}

func rgbToHSL(r, g, b int) (float64, float64, float64) {
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0

	max := math.Max(rf, math.Max(gf, bf))
	min := math.Min(rf, math.Min(gf, bf))
	l := (max + min) / 2.0

	if max == min {
		return 0, 0, l
	}

	d := max - min
	s := d / (1 - math.Abs(2*l-1))

	var h float64
	switch max {
	case rf:
		h = (gf - bf) / d
		if gf < bf {
			h += 6
		}
	case gf:
		h = (bf-rf)/d + 2
	case bf:
		h = (rf-gf)/d + 4
	}
	h *= 60

	return h, s, l
}

func hslToRGB(h, s, l float64) (int, int, int) {
	if s == 0 {
		v := int(math.Round(l * 255))
		return v, v, v
	}

	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q

	r := hueToRGB(p, q, h/360+1.0/3.0)
	g := hueToRGB(p, q, h/360)
	b := hueToRGB(p, q, h/360-1.0/3.0)

	return int(math.Round(r * 255)), int(math.Round(g * 255)), int(math.Round(b * 255))
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}

func hslToHex(h, s, l float64) string {
	r, g, b := hslToRGB(h, s, l)
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

func determineMood(h, s, l float64) string {
	if l < 0.15 {
		return "mysterious"
	}
	if l > 0.9 {
		return "airy"
	}
	if s < 0.1 {
		return "balanced"
	}

	if h < 60 || h > 330 {
		if s > 0.6 {
			return "energetic"
		}
		return "warm"
	}
	if h >= 60 && h < 170 {
		return "natural"
	}
	if h >= 170 && h <= 330 {
		if s > 0.6 {
			return "serene"
		}
		return "calm"
	}

	return "balanced"
}

func determineTemperature(h float64) string {
	if h < 60 || h > 330 {
		return "warm"
	}
	if h >= 60 && h < 150 {
		return "neutral"
	}
	return "cool"
}

type namedColor struct {
	name    string
	r, g, b int
}

var cssColors = []namedColor{
	{"Red", 255, 0, 0},
	{"Crimson", 220, 20, 60},
	{"Coral", 255, 127, 80},
	{"Orange", 255, 165, 0},
	{"Gold", 255, 215, 0},
	{"Yellow", 255, 255, 0},
	{"Lime", 0, 255, 0},
	{"Green", 0, 128, 0},
	{"Teal", 0, 128, 128},
	{"Cyan", 0, 255, 255},
	{"Sky Blue", 135, 206, 235},
	{"Blue", 0, 0, 255},
	{"Navy", 0, 0, 128},
	{"Indigo", 75, 0, 130},
	{"Purple", 128, 0, 128},
	{"Violet", 238, 130, 238},
	{"Magenta", 255, 0, 255},
	{"Pink", 255, 192, 203},
	{"Rose", 255, 0, 127},
	{"Brown", 139, 69, 19},
	{"Chocolate", 210, 105, 30},
	{"Tan", 210, 180, 140},
	{"Beige", 245, 245, 220},
	{"Ivory", 255, 255, 240},
	{"White", 255, 255, 255},
	{"Silver", 192, 192, 192},
	{"Gray", 128, 128, 128},
	{"Charcoal", 54, 69, 79},
	{"Black", 0, 0, 0},
	{"Maroon", 128, 0, 0},
}

func getColorName(r, g, b int) string {
	bestName := "Unknown"
	bestDist := math.MaxFloat64

	for _, c := range cssColors {
		dr := float64(r - c.r)
		dg := float64(g - c.g)
		db := float64(b - c.b)
		dist := math.Sqrt(dr*dr + dg*dg + db*db)
		if dist < bestDist {
			bestDist = dist
			bestName = c.name
		}
	}

	return bestName
}
