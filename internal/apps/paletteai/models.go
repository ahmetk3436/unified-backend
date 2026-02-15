package paletteai

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Palette struct {
	ID         uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID      string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID     uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	ImageURL   string         `gorm:"type:text" json:"image_url"`
	Color1     string         `gorm:"type:varchar(7)" json:"color_1"`
	Color2     string         `gorm:"type:varchar(7)" json:"color_2"`
	Color3     string         `gorm:"type:varchar(7)" json:"color_3"`
	Color4     string         `gorm:"type:varchar(7)" json:"color_4"`
	Color5     string         `gorm:"type:varchar(7)" json:"color_5"`
	Name       string         `gorm:"type:varchar(100)" json:"name"`
	ShareCount int            `gorm:"default:0" json:"share_count"`
	IsFavorite bool           `gorm:"default:false" json:"is_favorite"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

type PaletteStats struct {
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID         string    `gorm:"size:50;not null;index;uniqueIndex:idx_palette_stats_app_user" json:"app_id"`
	UserID        uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_palette_stats_app_user" json:"user_id"`
	TotalPalettes int      `gorm:"default:0" json:"total_palettes"`
	TotalShares   int      `gorm:"default:0" json:"total_shares"`
	FavoriteColor string   `gorm:"type:varchar(7)" json:"favorite_color"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// --- DTOs ---

type CreatePaletteRequest struct {
	ImageBase64 string `json:"image_base64"`
	Name        string `json:"name"`
	Color1      string `json:"color_1"`
	Color2      string `json:"color_2"`
	Color3      string `json:"color_3"`
	Color4      string `json:"color_4"`
	Color5      string `json:"color_5"`
}

type PaletteResponse struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	ImageURL   string    `json:"image_url"`
	Color1     string    `json:"color_1"`
	Color2     string    `json:"color_2"`
	Color3     string    `json:"color_3"`
	Color4     string    `json:"color_4"`
	Color5     string    `json:"color_5"`
	Name       string    `json:"name"`
	ShareCount int       `json:"share_count"`
	IsFavorite bool      `json:"is_favorite"`
	CreatedAt  time.Time `json:"created_at"`
}

type ToggleFavoriteResponse struct {
	IsFavorite bool `json:"is_favorite"`
}

type PaletteListResponse struct {
	Palettes []PaletteResponse `json:"palettes"`
	Total    int64             `json:"total"`
}

type StatsResponse struct {
	TotalPalettes int    `json:"total_palettes"`
	TotalShares   int    `json:"total_shares"`
	FavoriteColor string `json:"favorite_color"`
}

type ContrastCheckResponse struct {
	Ratio          float64 `json:"ratio"`
	Level          string  `json:"level"`
	LargeTextPass  bool    `json:"large_text_pass"`
	NormalTextPass bool    `json:"normal_text_pass"`
}

type PaletteExportResponse struct {
	Text     string `json:"text"`
	HexList  string `json:"hex_list"`
	CssVars  string `json:"css_vars"`
	Tailwind string `json:"tailwind"`
}

type ColorAnalysisResponse struct {
	Hex         string   `json:"hex"`
	Name        string   `json:"name"`
	RGB         RGBValue `json:"rgb"`
	HSL         HSLValue `json:"hsl"`
	Complement  string   `json:"complement"`
	Analogous   []string `json:"analogous"`
	Triadic     []string `json:"triadic"`
	Mood        string   `json:"mood"`
	Temperature string   `json:"temperature"`
}

type RGBValue struct {
	R int `json:"r"`
	G int `json:"g"`
	B int `json:"b"`
}

type HSLValue struct {
	H float64 `json:"h"`
	S float64 `json:"s"`
	L float64 `json:"l"`
}
