package middleware

import (
	"strings"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Paths that don't require tenant identification.
var tenantSkipPaths = []string{
	"/api/health",
	"/api/legal/",
	"/api/webhooks/", // webhooks use :app_id path param instead
}

// TenantMiddleware extracts app_id from JWT claims, X-App-ID header, or query param.
func TenantMiddleware(registry *tenant.Registry) fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Path()

		// Skip paths that handle tenant differently (health, legal, webhooks)
		for _, skip := range tenantSkipPaths {
			if strings.HasPrefix(path, skip) {
				return c.Next()
			}
		}

		// 1. Try JWT claim (already authenticated)
		if token, ok := c.Locals("user").(*jwt.Token); ok {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if appID, ok := claims["app_id"].(string); ok && appID != "" {
					c.Locals("app_id", appID)
					return c.Next()
				}
			}
		}

		// 2. Try X-App-ID header
		appID := c.Get("X-App-ID")
		if appID != "" {
			if !registry.Exists(appID) {
				return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
					Error:   true,
					Message: "Invalid X-App-ID: " + appID,
				})
			}
			c.Locals("app_id", appID)
			return c.Next()
		}

		// 3. Try query param (backward compat)
		appID = c.Query("app_id")
		if appID != "" {
			if !registry.Exists(appID) {
				return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
					Error:   true,
					Message: "Invalid app_id: " + appID,
				})
			}
			c.Locals("app_id", appID)
			return c.Next()
		}

		// 4. Missing app_id
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "X-App-ID header is required",
		})
	}
}
