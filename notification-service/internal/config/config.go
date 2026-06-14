package config

import (
	"log/slog"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	HTTPPort string

	JWTSecret string

	SMTPHost string
	SMTPPort int
	SMTPFrom string

	// InternalAPIKey guards the service-to-service emit endpoint. Other
	// services send it in the X-Internal-Key header.
	InternalAPIKey string
	FrontendURL    string
}

func Load() *Config {
	_ = godotenv.Load()

	if os.Getenv("JWT_SECRET") == "" {
		slog.Error("JWT_SECRET is required and must not be empty", "service", "notification-service")
		os.Exit(1)
	}

	smtpPort, _ := strconv.Atoi(getEnv("SMTP_PORT", "1025"))

	cfg := &Config{
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "postgres"),
		DBPassword:     getEnv("DB_PASSWORD", "postgres"),
		DBName:         getEnv("DB_NAME", "bankdb"),
		DBSSLMode:      getEnv("DB_SSL_MODE", "disable"),
		HTTPPort:       getEnv("HTTP_PORT", "8090"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		SMTPHost:       getEnv("SMTP_HOST", "mailhog"),
		SMTPPort:       smtpPort,
		SMTPFrom:       getEnv("SMTP_FROM", "noreply@bank.com"),
		InternalAPIKey: getEnv("INTERNAL_API_KEY", ""),
		FrontendURL:    getEnv("FRONTEND_URL", "http://localhost:5173"),
	}

	slog.Info("Notification-service config loaded",
		"db_host", cfg.DBHost,
		"http_port", cfg.HTTPPort,
	)

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
