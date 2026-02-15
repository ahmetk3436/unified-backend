package database

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Connect(cfg *config.Config) error {
	var err error
	DB, err = gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(25)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	slog.Info("database connected")
	return nil
}

// MigrateShared runs AutoMigrate for shared models.
func MigrateShared() error {
	return DB.AutoMigrate(
		&models.User{},
		&models.RefreshToken{},
		&models.Subscription{},
		&models.Report{},
		&models.Block{},
		&models.SystemLog{},
	)
}

// MigrateModels runs AutoMigrate for arbitrary models (used by plugins).
func MigrateModels(modelList []interface{}) error {
	if len(modelList) == 0 {
		return nil
	}
	return DB.AutoMigrate(modelList...)
}

func Ping() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
