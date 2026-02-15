package handlers

import (
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/database"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
)

type HealthHandler struct {
	registry *tenant.Registry
}

func NewHealthHandler(registry *tenant.Registry) *HealthHandler {
	return &HealthHandler{registry: registry}
}

func (h *HealthHandler) Check(c *fiber.Ctx) error {
	dbStatus := "ok"
	if err := database.Ping(); err != nil {
		dbStatus = "unhealthy: " + err.Error()
	}

	return c.JSON(dto.HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		DB:        dbStatus,
		AppCount:  len(h.registry.All()),
	})
}
