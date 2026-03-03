package models

import "time"

// ProcessedWebhookEvent tracks webhook event IDs that have already been handled.
// RevenueCat retries failed webhooks — without this deduplication layer, a retried
// INITIAL_PURCHASE would create a second subscription record (free subscription exploit).
// Uses event_id as the primary key so a PostgreSQL unique-constraint violation is
// the only possible outcome for a duplicate; no SELECT needed before INSERT.
type ProcessedWebhookEvent struct {
	EventID     string    `gorm:"primaryKey;column:event_id" json:"event_id"`
	ProcessedAt time.Time `gorm:"column:processed_at;index" json:"processed_at"`
}
