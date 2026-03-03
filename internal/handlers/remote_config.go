package handlers

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// RemoteConfigHandler handles remote configuration operations
type RemoteConfigHandler struct {
	db *gorm.DB
}

// NewRemoteConfigHandler creates a new remote config handler
func NewRemoteConfigHandler(db *gorm.DB) *RemoteConfigHandler {
	return &RemoteConfigHandler{db: db}
}

// GetConfig returns all config for the current app (public endpoint)
// Tenant is identified via X-App-ID header by TenantMiddleware
func (h *RemoteConfigHandler) GetConfig(c *fiber.Ctx) error {
	appID := c.Locals("app_id")
	if appID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   true,
			"message": "X-App-ID header is required",
		})
	}

	var configs []models.RemoteConfig
	if err := h.db.Where("app_id = ?", appID).Find(&configs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   true,
			"message": "Database error",
		})
	}

	result := make(map[string]interface{})
	var maxUpdated time.Time

	for _, cfg := range configs {
		switch cfg.Type {
		case "bool":
			result[cfg.Key] = cfg.Value == "true" || cfg.Value == "1"
		case "int":
			if v, err := strconv.Atoi(cfg.Value); err == nil {
				result[cfg.Key] = v
			} else {
				result[cfg.Key] = cfg.Value
			}
		case "json":
			var parsed interface{}
			if err := json.Unmarshal([]byte(cfg.Value), &parsed); err == nil {
				result[cfg.Key] = parsed
			} else {
				slog.Warn("remote config value is not valid JSON, using raw string", "key", cfg.Key, "error", err)
				result[cfg.Key] = cfg.Value
			}
		default:
			result[cfg.Key] = cfg.Value
		}
		if cfg.UpdatedAt.After(maxUpdated) {
			maxUpdated = cfg.UpdatedAt
		}
	}

	// Add config version for cache invalidation
	result["config_version"] = maxUpdated.Unix()

	// Set cache headers
	c.Set("Cache-Control", "public, max-age=60")
	if !maxUpdated.IsZero() {
		c.Set("Last-Modified", maxUpdated.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))
	}

	return c.JSON(result)
}

// SetConfigKey creates or updates a config value (admin only)
func (h *RemoteConfigHandler) SetConfigKey(c *fiber.Ctx) error {
	appID := c.Locals("app_id")
	if appID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   true,
			"message": "X-App-ID header is required",
		})
	}

	key := c.Params("key")
	if key == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   true,
			"message": "Key is required",
		})
	}

	var req struct {
		Value string `json:"value"`
		Type  string `json:"type"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   true,
			"message": "Invalid request body",
		})
	}

	if req.Value == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   true,
			"message": "Value is required",
		})
	}

	if req.Type == "" {
		req.Type = "string"
	}

	var cfg models.RemoteConfig
	result := h.db.Where("app_id = ? AND key = ?", appID, key).First(&cfg)

	if result.Error != nil {
		// Create new
		cfg = models.RemoteConfig{
			AppID: appID.(string),
			Key:   key,
			Value: req.Value,
			Type:  req.Type,
		}
		if err := h.db.Create(&cfg).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   true,
				"message": "Failed to create config",
			})
		}
	} else {
		// Update existing
		if err := h.db.Model(&cfg).Updates(map[string]interface{}{
			"value":      req.Value,
			"type":       req.Type,
			"updated_at": time.Now(),
		}).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   true,
				"message": "Failed to update config",
			})
		}
	}

	return c.JSON(fiber.Map{
		"key":        key,
		"value":      req.Value,
		"type":       req.Type,
		"updated_at": cfg.UpdatedAt,
	})
}

// DeleteConfigKey removes a config key (admin only)
func (h *RemoteConfigHandler) DeleteConfigKey(c *fiber.Ctx) error {
	appID := c.Locals("app_id")
	if appID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   true,
			"message": "X-App-ID header is required",
		})
	}

	key := c.Params("key")
	if key == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   true,
			"message": "Key is required",
		})
	}

	if err := h.db.Where("app_id = ? AND key = ?", appID, key).Delete(&models.RemoteConfig{}).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   true,
			"message": "Failed to delete config",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Config key deleted",
	})
}

// paywallConfigJSON is the default paywall config for subscription apps.
// Edit via admin API: PUT /api/admin/config/paywall_config
// with body: {"value": "<escaped JSON>", "type": "json"}
const paywallConfigJSON = `{
  "variant": "default",
  "headline": "Unlock Everything",
  "headline_named": "{{name}}, unlock your potential",
  "subtitle": "Join thousands who understand their emotions better",
  "social_proof": "50,000+ people tracking their mood",
  "show_urgency": true,
  "urgency_text": "Limited time offer — Save 50% today",
  "urgency_badge": "SAVE 50%",
  "cta_primary": "Start 7-Day Free Trial",
  "cta_processing": "Processing...",
  "trial_days": 7,
  "features": [
    {"icon": "infinite-outline",    "text": "Unlimited mood logging"},
    {"icon": "analytics-outline",   "text": "Weekly & monthly insights"},
    {"icon": "git-network-outline", "text": "Pattern recognition & trends"},
    {"icon": "color-palette-outline","text": "Custom emotions & themes"},
    {"icon": "leaf-outline",        "text": "Guided breathing exercises"},
    {"icon": "share-social-outline","text": "Beautiful shareable cards"},
    {"icon": "download-outline",    "text": "Export your data anytime"},
    {"icon": "headset-outline",     "text": "Priority support"}
  ],
  "plans": [
    {"id": "annual",  "label": "Annual Plan",  "price": "$29.99/year", "per_month": "$2.50/mo", "badge": "Best Value", "is_default": true},
    {"id": "monthly", "label": "Monthly Plan", "price": "$4.99/month", "badge": null,            "is_default": false}
  ]
}`

// subscriptionApps is the set of app IDs that use the subscription paywall.
var subscriptionApps = map[string]bool{
	"moodpulse": true,
	"daiyly":    true,
}

// SeedDefaults creates default config values for all apps
func (h *RemoteConfigHandler) SeedDefaults(appRegistry map[string]string) {
	// appRegistry maps app_id -> app_name
	langsJSON := `["en","tr","de","fr","es","it","pt","ru","ar","zh"]`

	for appID, appName := range appRegistry {
		defaults := []models.RemoteConfig{
			{AppID: appID, Key: "app_name", Value: appName, Type: "string"},
			{AppID: appID, Key: "default_language", Value: "en", Type: "string"},
			{AppID: appID, Key: "supported_languages", Value: langsJSON, Type: "json"},
			{AppID: appID, Key: "maintenance_mode", Value: "false", Type: "bool"},
			{AppID: appID, Key: "announcement_title", Value: "", Type: "string"},
			{AppID: appID, Key: "announcement_message", Value: "", Type: "string"},
			{AppID: appID, Key: "announcement_type", Value: "info", Type: "string"},
			{AppID: appID, Key: "min_app_version", Value: "1.0.0", Type: "string"},
		}

		// Add paywall config for subscription apps
		if subscriptionApps[appID] {
			defaults = append(defaults, models.RemoteConfig{
				AppID: appID,
				Key:   "paywall_config",
				Value: paywallConfigJSON,
				Type:  "json",
			})
		}

		for _, def := range defaults {
			var existing models.RemoteConfig
			if h.db.Where("app_id = ? AND key = ?", def.AppID, def.Key).First(&existing).Error != nil {
				if err := h.db.Create(&def).Error; err != nil {
					slog.Error("failed to seed config", "key", def.Key, "error", err)
				}
			}
		}
	}
}
