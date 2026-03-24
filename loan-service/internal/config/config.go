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
}

func Load() *Config {
	_ = godotenv.Load()

	smtpPort, _ := strconv.Atoi(getEnv("SMTP_PORT", "1025"))

	cfg := &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", "postgres"),
		DBName:     getEnv("DB_NAME", "bankdb"),
		DBSSLMode:  getEnv("DB_SSL_MODE", "disable"),
		HTTPPort:   getEnv("HTTP_PORT", "8089"),
		JWTSecret:  getEnv("JWT_SECRET", "super-secret-jwt-key-change-in-production"),
		SMTPHost:   getEnv("SMTP_HOST", "mailhog"),
		SMTPPort:   smtpPort,
		SMTPFrom:   getEnv("SMTP_FROM", "noreply@bank.com"),
	}

	slog.Info("Loan-service config loaded",
		"db_host", cfg.DBHost,
		"db_name", cfg.DBName,
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
