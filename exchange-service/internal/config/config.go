package config

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	GRPCPort  string
	HTTPPort  string
	JWTSecret string

	AlphaVantageAPIKey string
}

func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", "postgres"),
		DBName:     getEnv("DB_NAME", "bankdb"),
		DBSSLMode:  getEnv("DB_SSL_MODE", "disable"),
		GRPCPort:   getEnv("GRPC_PORT", "9098"),
		HTTPPort:   getEnv("HTTP_PORT", "8088"),
		JWTSecret:          getEnv("JWT_SECRET", "super-secret-jwt-key-change-in-production"),
		AlphaVantageAPIKey: getEnv("ALPHA_VANTAGE_API_KEY", "demo"),
	}

	slog.Info("Exchange-service config loaded",
		"db_host", cfg.DBHost,
		"http_port", cfg.HTTPPort,
		"grpc_port", cfg.GRPCPort,
	)

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
