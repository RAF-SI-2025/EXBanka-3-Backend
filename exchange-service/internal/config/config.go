package config

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	GRPCPort  string
	HTTPPort  string
	JWTSecret string
}

func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		GRPCPort:  getEnv("GRPC_PORT", "9098"),
		HTTPPort:  getEnv("HTTP_PORT", "8088"),
		JWTSecret: getEnv("JWT_SECRET", "super-secret-jwt-key-change-in-production"),
	}

	slog.Info("Exchange-service config loaded",
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
