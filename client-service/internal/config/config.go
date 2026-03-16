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

	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string

	FrontendURL string
}

func Load() *Config {
	_ = godotenv.Load()

	smtpPort, _ := strconv.Atoi(getEnv("SMTP_PORT", "1025"))
	jwtAccessDur, _ := strconv.Atoi(getEnv("JWT_ACCESS_DURATION_MINUTES", "15"))
	jwtRefreshDur, _ := strconv.Atoi(getEnv("JWT_REFRESH_DURATION_HOURS", "24"))

	cfg := &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", "postgres"),
		DBName:     getEnv("DB_NAME", "bankdb"),
		DBSSLMode:  getEnv("DB_SSL_MODE", "disable"),
		GRPCPort:   getEnv("GRPC_PORT", "9090"),
		HTTPPort:   getEnv("HTTP_PORT", "8080"),

		JWTSecret:          getEnv("JWT_SECRET", "super-secret-jwt-key-change-in-production"),
		JWTAccessDuration:  jwtAccessDur,
		JWTRefreshDuration: jwtRefreshDur,

		SMTPHost:     getEnv("SMTP_HOST", "localhost"),
		SMTPPort:     smtpPort,
		SMTPUser:     getEnv("SMTP_USER", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:     getEnv("SMTP_FROM", "noreply@bank.com"),
		FrontendURL:  getEnv("FRONTEND_URL", "http://localhost:5173"),
	}

	slog.Info("Client config loaded",
		"db_host", cfg.DBHost,
		"db_port", cfg.DBPort,
		"db_name", cfg.DBName,
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
