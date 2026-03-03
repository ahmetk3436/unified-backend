package config

import (
	"os"
	"time"
)

type Config struct {
	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// JWT (shared across all apps)
	JWTSecret        string
	JWTAccessExpiry  time.Duration
	JWTRefreshExpiry time.Duration

	// AI Providers
	GLMAPIKey      string
	GLMAPIURL      string
	GLMModel       string
	GLMVisionModel string

	DeepSeekAPIKey string
	DeepSeekAPIURL string
	DeepSeekModel  string

	OpenAIAPIKey string
	OpenAIModel  string

	FalAPIKey string

	AITimeout time.Duration

	// Admin
	AdminEmails  string
	AdminUserIDs string
	AdminToken   string

	// Server
	Port        string
	CORSOrigins string

	// Apple Sign In (for token revocation on account delete)
	AppleTeamID    string
	AppleKeyID     string
	ApplePrivateKey string

	// App registry
	AppsConfigPath string

	// File uploads root directory (absolute path preferred; defaults to ./uploads)
	UploadsRoot string

	// EmotionSenseML service URL for async emotion analysis on journal entries
	EmotionSenseMLURL string
}

func Load() *Config {
	return &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "unified_db"),
		// Default "require" enforces TLS for all DB connections (GDPR / FTC HBNR).
		// Override with DB_SSLMODE=disable only for local dev without a TLS-capable DB.
		DBSSLMode: getEnv("DB_SSLMODE", "require"),

		JWTSecret:        getEnv("JWT_SECRET", ""),
		JWTAccessExpiry:  parseDuration(getEnv("JWT_ACCESS_EXPIRY", "15m")),
		JWTRefreshExpiry: parseDuration(getEnv("JWT_REFRESH_EXPIRY", "168h")),

		GLMAPIKey:      getEnv("GLM_API_KEY", ""),
		GLMAPIURL:      getEnv("GLM_API_URL", "https://api.z.ai/api/paas/v4/chat/completions"),
		GLMModel:       getEnv("GLM_MODEL", "glm-5"),
		GLMVisionModel: getEnv("GLM_VISION_MODEL", "glm-4v-plus"),

		DeepSeekAPIKey: getEnv("DEEPSEEK_API_KEY", ""),
		DeepSeekAPIURL: getEnv("DEEPSEEK_API_URL", "https://api.deepseek.com/v1/chat/completions"),
		DeepSeekModel:  getEnv("DEEPSEEK_MODEL", "deepseek-chat"),

		OpenAIAPIKey: getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:  getEnv("OPENAI_MODEL", "gpt-4o-mini"),

		FalAPIKey: getEnv("FAL_API_KEY", ""),

		AITimeout: parseDuration(getEnv("AI_TIMEOUT", "60s")),

		AdminEmails:  getEnv("ADMIN_EMAILS", ""),
		AdminUserIDs: getEnv("ADMIN_USER_IDS", ""),
		AdminToken:   getEnv("ADMIN_TOKEN", ""),

		Port:        getEnv("PORT", "8080"),
		CORSOrigins: getEnv("CORS_ORIGINS", ""),

		AppleTeamID:    getEnv("APPLE_TEAM_ID", ""),
		AppleKeyID:     getEnv("APPLE_KEY_ID", ""),
		ApplePrivateKey: getEnv("APPLE_PRIVATE_KEY", ""),

		AppsConfigPath: getEnv("APPS_CONFIG_PATH", "apps.json"),

		UploadsRoot: getEnv("UPLOADS_ROOT", "./uploads"),

		EmotionSenseMLURL: getEnv("EMOTION_SENSE_ML_URL", "http://esg8o8k08cgw4o44g8kkkc8g.89.47.113.196.sslip.io"),
	}
}

func (c *Config) DSN() string {
	return "host=" + c.DBHost +
		" user=" + c.DBUser +
		" password=" + c.DBPassword +
		" dbname=" + c.DBName +
		" port=" + c.DBPort +
		" sslmode=" + c.DBSSLMode +
		" TimeZone=UTC"
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 15 * time.Minute
	}
	return d
}
