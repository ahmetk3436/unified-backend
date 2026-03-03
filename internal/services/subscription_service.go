package services

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SubscriptionService struct {
	db *gorm.DB
}

func NewSubscriptionService(db *gorm.DB) *SubscriptionService {
	return &SubscriptionService{db: db}
}

func (s *SubscriptionService) HandleWebhookEvent(appID string, event *dto.RevenueCatEvent) error {
	// Idempotency guard: RevenueCat retries failed webhooks. Without deduplication,
	// a retried INITIAL_PURCHASE creates a second subscription row (free subscription exploit),
	// and a retried CANCELLATION could incorrectly cancel an active subscriber who re-subscribed.
	// We use event.ID as a unique key and INSERT ... ON CONFLICT DO NOTHING so only the first
	// delivery wins — atomically, at the DB level.
	if event.ID != "" {
		result := s.db.Exec(
			`INSERT INTO processed_webhook_events (event_id, processed_at) VALUES (?, ?) ON CONFLICT (event_id) DO NOTHING`,
			event.ID, time.Now().UTC(),
		)
		if result.Error != nil {
			return fmt.Errorf("webhook idempotency check failed: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			slog.Info("duplicate webhook event skipped", "event_id", event.ID, "type", event.Type)
			return nil
		}
	}

	switch event.Type {
	case "INITIAL_PURCHASE":
		return s.handleInitialPurchase(appID, event)
	case "RENEWAL":
		return s.handleRenewal(appID, event)
	case "CANCELLATION":
		return s.handleCancellation(appID, event)
	case "EXPIRATION", "EXPIRED_FROM_BILLING_ISSUE":
		return s.handleExpiration(appID, event)
	case "ENTERED_GRACE_PERIOD", "BILLING_ISSUE":
		// Grace period: subscriber's payment failed but RevenueCat gives them time to fix it.
		// Keep subscription active but mark it as grace_period so the app can surface a notice.
		return s.handleGracePeriod(appID, event)
	default:
		return nil
	}
}

func (s *SubscriptionService) handleInitialPurchase(appID string, event *dto.RevenueCatEvent) error {
	// Upsert: if a subscription already exists for this RevenueCat user (e.g. from a trial
	// that converted), update it rather than inserting a duplicate row.
	var existing models.Subscription
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("revenuecat_id = ?", event.AppUserID).First(&existing).Error
	if err == nil {
		return s.db.Model(&existing).Updates(map[string]interface{}{
			"status":               "active",
			"product_id":           event.ProductID,
			"current_period_start": msToTime(event.PurchasedAtMs),
			"current_period_end":   msToTime(event.ExpirationAtMs),
		}).Error
	}

	sub := models.Subscription{
		ID:                 uuid.New(),
		AppID:              appID,
		RevenueCatID:       event.AppUserID,
		ProductID:          event.ProductID,
		Status:             "active",
		CurrentPeriodStart: msToTime(event.PurchasedAtMs),
		CurrentPeriodEnd:   msToTime(event.ExpirationAtMs),
	}

	var user models.User
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", event.AppUserID).First(&user).Error; err == nil {
		sub.UserID = user.ID
	}

	return s.db.Create(&sub).Error
}

func (s *SubscriptionService) handleGracePeriod(appID string, event *dto.RevenueCatEvent) error {
	// Keep the subscription accessible during grace period but mark status so the
	// app can display a "payment issue" banner. If no subscription row exists yet,
	// this is a no-op (RevenueCat may send BILLING_ISSUE before INITIAL_PURCHASE in edge cases).
	result := s.db.Model(&models.Subscription{}).
		Scopes(tenant.ForTenant(appID)).
		Where("revenuecat_id = ?", event.AppUserID).
		Update("status", "grace_period")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		slog.Warn("grace period event for unknown subscription", "app_id", appID, "revenuecat_id", event.AppUserID)
	}
	return nil
}

func (s *SubscriptionService) handleRenewal(appID string, event *dto.RevenueCatEvent) error {
	var sub models.Subscription
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("revenuecat_id = ?", event.AppUserID).First(&sub).Error; err != nil {
		return fmt.Errorf("subscription not found for renewal: %w", err)
	}

	return s.db.Model(&sub).Updates(map[string]interface{}{
		"status":               "active",
		"current_period_end":   msToTime(event.ExpirationAtMs),
		"current_period_start": msToTime(event.PurchasedAtMs),
	}).Error
}

func (s *SubscriptionService) handleCancellation(appID string, event *dto.RevenueCatEvent) error {
	return s.db.Model(&models.Subscription{}).
		Scopes(tenant.ForTenant(appID)).
		Where("revenuecat_id = ?", event.AppUserID).
		Update("status", "cancelled").Error
}

func (s *SubscriptionService) handleExpiration(appID string, event *dto.RevenueCatEvent) error {
	return s.db.Model(&models.Subscription{}).
		Scopes(tenant.ForTenant(appID)).
		Where("revenuecat_id = ?", event.AppUserID).
		Update("status", "expired").Error
}

func msToTime(ms int64) time.Time {
	return time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond))
}
