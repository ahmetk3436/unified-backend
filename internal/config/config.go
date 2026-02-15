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

	AITimeout time.Duration

	// Admin
	AdminEmails  string
	AdminUserIDs string
	AdminToken   string

	// Server
	Port        string
	CORSOrigins string

	// App registry
	AppsConfigPath string
}

func Load() *Config {
	return &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "unified_db"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

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

		AITimeout: parseDuration(getEnv("AI_TIMEOUT", "60s")),

		AdminEmails:  getEnv("ADMIN_EMAILS", ""),
		AdminUserIDs: getEnv("ADMIN_USER_IDS", ""),
		AdminToken:   getEnv("ADMIN_TOKEN", ""),

		Port:        getEnv("PORT", "8080"),
		CORSOrigins: getEnv("CORS_ORIGINS", "*"),

		AppsConfigPath: getEnv("APPS_CONFIG_PATH", "apps.json"),
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
