package middleware

import (
	"log/slog"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	jwtware "github.com/gofiber/contrib/jwt"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

func JWTProtected(cfg *config.Config) fiber.Handler {
	return jwtware.New(jwtware.Config{
		SigningKey: jwtware.SigningKey{JWTAlg: "HS256", Key: []byte(cfg.JWTSecret)},
		// Cross-app scope enforcement: the JWT's app_id claim must match the request's
		// X-App-ID header (set by TenantMiddleware). Without this check, a valid JWT
		// issued for DriftOff is cryptographically accepted on MoodPulse endpoints
		// because all apps share the same HS256 signing secret.
		SuccessHandler: func(c *fiber.Ctx) error {
			tok, ok := c.Locals("user").(*jwt.Token)
			if !ok {
				return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
					Error:   true,
					Message: "Unauthorized: invalid token",
				})
			}
			claims, ok := tok.Claims.(jwt.MapClaims)
			if !ok {
				return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
					Error:   true,
					Message: "Unauthorized: invalid token claims",
				})
			}
			tokenAppID, _ := claims["app_id"].(string)
			requestAppID := tenant.GetAppID(c)
			if tokenAppID == "" || tokenAppID != requestAppID {
				slog.Warn("cross-app token rejected",
					"token_app_id", tokenAppID,
					"request_app_id", requestAppID,
					"path", c.Path(),
				)
				return c.Status(fiber.StatusForbidden).JSON(dto.ErrorResponse{
					Error:   true,
					Message: "Forbidden: token not valid for this app",
				})
			}
			return c.Next()
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
				Error:   true,
				Message: "Unauthorized: invalid or expired token",
			})
		},
	})
}
