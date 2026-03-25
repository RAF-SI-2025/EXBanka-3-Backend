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

	GRPCPort string
	HTTPPort string

	JWTSecret          string
	JWTAccessDuration  int
	JWTRefreshDuration int

	SMTPHost string
	SMTPPort int
	SMTPFrom string
}

func Load() *Config {
	_ = godotenv.Load()

	jwtAccessDur, _ := strconv.Atoi(getEnv("JWT_ACCESS_DURATION_MINUTES", "15"))
	jwtRefreshDur, _ := strconv.Atoi(getEnv("JWT_REFRESH_DURATION_HOURS", "24"))
	smtpPort, _ := strconv.Atoi(getEnv("SMTP_PORT", "1025"))

	cfg := &Config{
		DBHost:             getEnv("DB_HOST", "localhost"),
		DBPort:             getEnv("DB_PORT", "5432"),
		DBUser:             getEnv("DB_USER", "postgres"),
		DBPassword:         getEnv("DB_PASSWORD", "postgres"),
		DBName:             getEnv("DB_NAME", "bankdb"),
		DBSSLMode:          getEnv("DB_SSL_MODE", "disable"),
		GRPCPort:           getEnv("GRPC_PORT", "9096"),
		HTTPPort:           getEnv("HTTP_PORT", "8086"),
		JWTSecret:          getEnv("JWT_SECRET", "super-secret-jwt-key-change-in-production"),
		JWTAccessDuration:  jwtAccessDur,
		JWTRefreshDuration: jwtRefreshDur,
		SMTPHost:           getEnv("SMTP_HOST", "mailhog"),
		SMTPPort:           smtpPort,
		SMTPFrom:           getEnv("SMTP_FROM", "noreply@bank.com"),
	}

	slog.Info("Transfer-service config loaded",
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
