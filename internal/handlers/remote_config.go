package handlers

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RemoteConfigHandler struct {
	db       *gorm.DB
	registry *tenant.Registry
}

func NewRemoteConfigHandler(db *gorm.DB, registry *tenant.Registry) *RemoteConfigHandler {
	return &RemoteConfigHandler{
		db:       db,
		registry: registry,
	}
}

// GetConfig returns app-specific configuration (public, requires X-App-ID header)
func (h *RemoteConfigHandler) GetConfig(c *fiber.Ctx) error {
	appID := c.Get("X-App-ID")
	if appID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "X-App-ID header is required",
		})
	}

	// Validate app exists
	if !h.registry.Exists(appID) {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "Invalid X-App-ID: " + appID,
		})
	}

	var configs []models.RemoteConfig
	if err := h.db.Where("app_id = ?", appID).Find(&configs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "Failed to fetch configuration",
		})
	}

	// Convert to map for easier consumption
	result := make(map[string]interface{})
	for _, cfg := range configs {
		var value interface{}
		switch cfg.Type {
		case "bool":
			value, _ = strconv.ParseBool(cfg.Value)
		case "int":
			value, _ = strconv.Atoi(cfg.Value)
		case "json":
			json.Unmarshal([]byte(cfg.Value), &value)
		default:
			value = cfg.Value
		}
		result[cfg.Key] = value
	}

	return c.JSON(result)
}

// SetConfigKey sets or updates a config key (admin only)
func (h *RemoteConfigHandler) SetConfigKey(c *fiber.Ctx) error {
	appID := c.Params("app_id", "")
	if appID == "" {
		// If no app_id in path, use from header
		appID = c.Get("X-App-ID", "")
	}

	key := c.Params("key", "")
	if key == "" {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "Key parameter is required",
		})
	}

	var payload struct {
		Value string `json:"value"`
		Type  string `json:"type"` // string, bool, int, json
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "Invalid request body",
		})
	}

	if payload.Value == "" {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "Value is required",
		})
	}

	if payload.Type == "" {
		payload.Type = "string"
	}

	// Validate app exists
	if !h.registry.Exists(appID) {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "Invalid app_id: " + appID,
		})
	}

	// Upsert config
	var config models.RemoteConfig
	err := h.db.Where("app_id = ? AND key = ?", appID, key).First(&config).Error
	if err == gorm.ErrRecordNotFound {
		// Create new
		config = models.RemoteConfig{
			ID:        uuid.New(),
			AppID:     appID,
			Key:       key,
			Value:     payload.Value,
			Type:      payload.Type,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := h.db.Create(&config).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
				Error:   true,
				Message: "Failed to create config",
			})
		}
	} else if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "Failed to query config",
		})
	} else {
		// Update existing
		config.Value = payload.Value
		config.Type = payload.Type
		config.UpdatedAt = time.Now()
		if err := h.db.Save(&config).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
				Error:   true,
				Message: "Failed to update config",
			})
		}
	}

	return c.JSON(fiber.Map{
		"error":   false,
		"message": "Config updated successfully",
		"config": fiber.Map{
			"app_id": config.AppID,
			"key":    config.Key,
			"value":  config.Value,
			"type":   config.Type,
		},
	})
}

// DeleteConfigKey deletes a config key (admin only)
func (h *RemoteConfigHandler) DeleteConfigKey(c *fiber.Ctx) error {
	appID := c.Params("app_id", "")
	if appID == "" {
		appID = c.Get("X-App-ID", "")
	}

	key := c.Params("key", "")
	if key == "" {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "Key parameter is required",
		})
	}

	result := h.db.Where("app_id = ? AND key = ?", appID, key).Delete(&models.RemoteConfig{})
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "Failed to delete config",
		})
	}

	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
			Error:   true,
			Message: "Config not found",
		})
	}

	return c.JSON(fiber.Map{
		"error":   false,
		"message": "Config deleted successfully",
	})
}

// SeedDefaults creates default configuration for all apps
func (h *RemoteConfigHandler) SeedDefaults(appRegistry map[string]string) error {
	defaultLang := "en"
	_ = defaultLang // Used in configs below

	for appID, appName := range appRegistry {
		configs := []map[string]interface{}{
			{
				"key":   "app_name",
				"value": appName,
				"type":  "string",
			},
			{
				"key":   "default_language",
				"value": defaultLang,
				"type":  "string",
			},
			{
				"key":   "supported_languages",
				"value": "en,tr,de,fr,es,it,pt,ru,ar,zh",
				"type":  "string",
			},
			{
				"key":   "maintenance_mode",
				"value": "false",
				"type":  "bool",
			},
			{
				"key":   "announcement_title",
				"value": "",
				"type":  "string",
			},
			{
				"key":   "announcement_message",
				"value": "",
				"type":  "string",
			},
		}

		for _, cfg := range configs {
			var existing models.RemoteConfig
			err := h.db.Where("app_id = ? AND key = ?", appID, cfg["key"]).First(&existing).Error
			if err == gorm.ErrRecordNotFound {
				newConfig := models.RemoteConfig{
					ID:    uuid.New(),
					AppID: appID,
					Key:   cfg["key"].(string),
					Value: cfg["value"].(string),
					Type:  cfg["type"].(string),
				}
				if err := h.db.Create(&newConfig).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// GetConfigByAppID returns config for a specific app (internal use)
func (h *RemoteConfigHandler) GetConfigByAppID(appID string) (map[string]interface{}, error) {
	var configs []models.RemoteConfig
	if err := h.db.Where("app_id = ?", appID).Find(&configs).Error; err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for _, cfg := range configs {
		var value interface{}
		switch cfg.Type {
		case "bool":
			value, _ = strconv.ParseBool(cfg.Value)
		case "int":
			value, _ = strconv.Atoi(cfg.Value)
		case "json":
			json.Unmarshal([]byte(cfg.Value), &value)
		default:
			value = cfg.Value
		}
		result[cfg.Key] = value
	}
	return result, nil
}
