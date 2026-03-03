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
	// Note: "null" is not a valid Fiber origin format — use .invalid TLD (RFC 2606).
	origins := cfg.CORSOrigins
	if origins == "" || origins == "*" {
		// Wildcard is never valid here — this is a mobile-only API.
		// If someone sets CORS_ORIGINS=* in Coolify by mistake, lock it down.
		origins = "https://no-origin.invalid"
	}
	return cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowHeaders:     "Origin, Content-Type, Authorization, Accept, X-App-ID",
		AllowMethods:     "GET, POST, PUT, DELETE, PATCH, OPTIONS",
		AllowCredentials: false,
	})
}
