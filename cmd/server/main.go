package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	sentryfiber "github.com/getsentry/sentry-go/fiber"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/aurascan"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/confessit"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/daiyly"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/ecomonitor"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/eracheck"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/feelsy"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/mewify"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/paletteai"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/snapstreak"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/vibecheck"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/rizzcheck"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/apps/wouldyou"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/database"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/handlers"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/logging"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/middleware"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/routes"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/services"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

func main() {
	// Structured logging (JSON to stdout)
	logging.Setup()

	cfg := config.Load()

	if cfg.JWTSecret == "" {
		slog.Error("JWT_SECRET environment variable is required")
		os.Exit(1)
	}
	if cfg.DBPassword == "" {
		slog.Error("DB_PASSWORD environment variable is required")
		os.Exit(1)
	}

	// App registry
	registry, err := tenant.LoadFromFile(cfg.AppsConfigPath)
	if err != nil {
		slog.Error("failed to load app registry", "path", cfg.AppsConfigPath, "error", err)
		os.Exit(1)
	}
	slog.Info("app registry loaded", "apps", len(registry.All()))

	// Database
	if err := database.Connect(cfg); err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}

	// Migrate shared models
	if err := database.MigrateShared(); err != nil {
		slog.Error("shared migration failed", "error", err)
		os.Exit(1)
	}

	// PostgreSQL log handler (ERROR+ async batch)
	pgLogHandler := logging.NewPGHandler(database.DB)
	slog.SetDefault(slog.New(logging.NewMultiHandler(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		pgLogHandler,
	)))

	// Log cleanup (30-day retention)
	cleanupDone := make(chan struct{})
	logging.StartCleanup(database.DB, cleanupDone)

	// Services
	authService := services.NewAuthService(database.DB, cfg)
	subscriptionService := services.NewSubscriptionService(database.DB)
	moderationService := services.NewModerationService(database.DB)

	// Register plugins (all 11 apps)
	plugins := []apps.Plugin{
		eracheck.New(moderationService),
		mewify.New(),
		paletteai.New(),
		snapstreak.New(),
		daiyly.New(),
		vibecheck.New(),
		feelsy.New(moderationService),
		wouldyou.New(),
		confessit.New(),
		ecomonitor.New(),
		aurascan.New(),
		rizzcheck.New(),
	}

	// Migrate plugin models
	for _, p := range plugins {
		if models := p.Models(); len(models) > 0 {
			if err := database.MigrateModels(models); err != nil {
				slog.Error("plugin migration failed", "plugin", p.ID(), "error", err)
				os.Exit(1)
			}
			slog.Info("plugin migrated", "plugin", p.ID(), "models", len(models))
		}
	}

	// Handlers
	authHandler := handlers.NewAuthHandler(authService, registry)
	healthHandler := handlers.NewHealthHandler(registry)
	webhookHandler := handlers.NewWebhookHandler(subscriptionService, registry)
	moderationHandler := handlers.NewModerationHandler(moderationService)
	legalHandler := handlers.NewLegalHandler(registry)
	configHandler := handlers.NewRemoteConfigHandler(database.DB, registry)

	// Seed default remote config values
	slog.Info("seeding remote config defaults")
	configHandler.SeedDefaults(registry.ToMap())

	// Sentry error tracking
	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              dsn,
			EnableTracing:    true,
			TracesSampleRate: 0.2,
			Environment:      os.Getenv("APP_ENV"),
		}); err != nil {
			slog.Error("sentry init failed", "error", err)
		} else {
			defer sentry.Flush(2 * time.Second)
		}
	}

	// Fiber app
	app := fiber.New(fiber.Config{
		BodyLimit:    4 * 1024 * 1024,
		ErrorHandler: customErrorHandler,
	})

	// Sentry middleware
	app.Use(sentryfiber.New(sentryfiber.Options{
		Repanic:         true,
		WaitForDelivery: false,
	}))

	// Global middleware
	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(fiberlogger.New(fiberlogger.Config{
		Format: "${time} | ${status} | ${latency} | ${ip} | ${method} | ${path}\n",
	}))
	app.Use(middleware.CORS(cfg))
	app.Use(func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		return c.Next()
	})
	app.Use(middleware.TenantMiddleware(registry))

	// Routes
	routes.Setup(app, cfg, database.DB, authHandler, healthHandler, webhookHandler, moderationHandler, legalHandler, configHandler, plugins)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := app.Listen(":" + cfg.Port); err != nil {
			slog.Error("server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down server...")

	close(cleanupDone)
	pgLogHandler.Stop()
	sentry.Flush(2 * time.Second)

	if err := app.Shutdown(); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	// Close database connections
	if sqlDB, err := database.DB.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			slog.Error("database close error", "error", err)
		}
	}

	slog.Info("server stopped")
}

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal server error"
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	// Only expose error details for client errors (4xx), not server errors (5xx)
	if code >= 500 {
		slog.Error("unhandled server error", "method", c.Method(), "path", c.Path(), "error", err.Error())
		message = "Internal server error"
	}

	return c.Status(code).JSON(fiber.Map{
		"error":   true,
		"message": message,
	})
}
