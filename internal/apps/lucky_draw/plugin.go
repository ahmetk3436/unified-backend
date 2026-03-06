package lucky_draw

import (
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"gorm.io/gorm"
)

type LuckyDrawPlugin struct{}

func New() *LuckyDrawPlugin {
	return &LuckyDrawPlugin{}
}

func (p *LuckyDrawPlugin) ID() string { return "lucky_draw" }

func (p *LuckyDrawPlugin) Models() []interface{} {
	return []interface{}{
		&LuckyDraw{},
		&UserHistory{},
	}
}

func (p *LuckyDrawPlugin) RegisterRoutes(router fiber.Router, db *gorm.DB, cfg *config.Config) {
	svc := NewLuckyDrawService(db, cfg)
	handler := NewLuckyDrawHandler(svc)

	// Rate limiter for draw creation (20/hour per user)
	drawLimiter := limiter.New(limiter.Config{
		Max:               20,
		Expiration:        1 * time.Hour,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c *fiber.Ctx) string {
			auth := c.Get("Authorization")
			if len(auth) > 39 {
				return "lucky_draw:" + auth[7:39]
			}
			return "lucky_draw:ip:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   true,
				"message": "Rate limit exceeded. Please try again in an hour.",
			})
		},
	})

	// Draw endpoints
	router.Post("/draw", drawLimiter, handler.Create)
	router.Get("/draw", handler.List)
	router.Get("/draw/:id", handler.Get)
	router.Delete("/draw/:id", handler.Delete)

	// Stats endpoints
	router.Get("/stats", handler.GetStats)
	router.Get("/history", handler.GetHistory)
}