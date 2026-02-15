package tenant

import "gorm.io/gorm"

// ForTenant returns a GORM scope that filters by app_id.
func ForTenant(appID string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("app_id = ?", appID)
	}
}
