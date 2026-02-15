package tenant

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// GetAppID extracts the app_id from Fiber context locals.
func GetAppID(c *fiber.Ctx) string {
	if appID, ok := c.Locals("app_id").(string); ok {
		return appID
	}
	return ""
}

// GetUserID extracts the user UUID from JWT claims in context.
func GetUserID(c *fiber.Ctx) (uuid.UUID, error) {
	token, ok := c.Locals("user").(*jwt.Token)
	if !ok {
		return uuid.Nil, errors.New("invalid token in context")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, errors.New("invalid claims")
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		return uuid.Nil, errors.New("missing sub claim")
	}

	return uuid.Parse(sub)
}
