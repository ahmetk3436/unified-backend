package middleware

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func CORS(cfg *config.Config) fiber.Handler {
	// When AllowOrigins is empty, Fiber defaults to "*" (wildcard).
	// This backend is mobile-only — no browser client. Restrict to a no-match
	// origin so browsers cannot make arbitrary cross-origin requests.
	// Mobile SDK clients do not send an Origin header and are unaffected.
	origins := cfg.CORSOrigins
	if origins == "" {
		origins = "null" // Matches no real origin; denies browser cross-origin access.
	}
	return cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowHeaders:     "Origin, Content-Type, Authorization, Accept, X-App-ID",
		AllowMethods:     "GET, POST, PUT, DELETE, PATCH, OPTIONS",
		AllowCredentials: false,
	})
}
