package logging

import (
	"log/slog"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"gorm.io/gorm"
)

// StartCleanup runs a daily goroutine that deletes system_logs older than 30 days.
func StartCleanup(db *gorm.DB, done chan struct{}) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cutoff := time.Now().AddDate(0, 0, -30)
				result := db.Where("timestamp < ?", cutoff).Delete(&models.SystemLog{})
				if result.Error != nil {
					slog.Error("log cleanup failed", "error", result.Error)
				} else if result.RowsAffected > 0 {
					slog.Info("log cleanup completed", "deleted", result.RowsAffected)
				}
			case <-done:
				return
			}
		}
	}()
}
