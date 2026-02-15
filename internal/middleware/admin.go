package middleware

import (
	"strings"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AdminRequired is a unified admin middleware that checks:
// 1. Config-based admin emails/IDs/token
// 2. DB-based user Role field
func AdminRequired(db *gorm.DB, cfg *config.Config) fiber.Handler {
	adminEmails := parseCSV(cfg.AdminEmails)
	adminUserIDs := parseCSV(cfg.AdminUserIDs)

	return func(c *fiber.Ctx) error {
		// Check admin token header
		if cfg.AdminToken != "" {
			if c.Get("X-Admin-Token") == cfg.AdminToken {
				return c.Next()
			}
		}

		token, ok := c.Locals("user").(*jwt.Token)
		if !ok || token == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
				Error: true, Message: "Unauthorized",
			})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(dto.ErrorResponse{
				Error: true, Message: "Invalid claims",
			})
		}

		email, _ := claims["email"].(string)
		sub, _ := claims["sub"].(string)

		// Check config-based admin lists
		if contains(adminEmails, email) || contains(adminUserIDs, sub) {
			return c.Next()
		}

		// Check DB-based role
		if sub != "" {
			userID, err := uuid.Parse(sub)
			if err == nil {
				appID := tenant.GetAppID(c)
				var user models.User
				if err := db.Scopes(tenant.ForTenant(appID)).First(&user, "id = ?", userID).Error; err == nil {
					if user.Role == "admin" {
						return c.Next()
					}
				}
			}
		}

		return c.Status(fiber.StatusForbidden).JSON(dto.ErrorResponse{
			Error: true, Message: "Admin access required",
		})
	}
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func contains(list []string, val string) bool {
	for _, item := range list {
		if item == val {
			return true
		}
	}
	return false
}
