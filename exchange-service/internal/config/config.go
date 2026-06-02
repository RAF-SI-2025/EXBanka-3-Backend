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

	GRPCPort  string
	HTTPPort  string
	JWTSecret string

	AlphaVantageAPIKey string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Inter-bank protocol (Celina 5). OwnRoutingNumber is the 3-digit prefix
	// used in our account numbers. PartnerBanksJSON is a JSON array of
	// {code,baseUrl,outboundKey,inboundKey,displayName} entries; parsed once
	// at startup via interbank.NewRegistryFromJSON.
	OwnRoutingNumber int
	PartnerBanksJSON string

	// SMTP — same env vars as account-service; targets Mailhog (host mailhog, port 1025)
	SMTPHost string
	SMTPPort int
	SMTPFrom string
}

func Load() *Config {
	_ = godotenv.Load()

	if os.Getenv("JWT_SECRET") == "" {
		slog.Error("JWT_SECRET is required and must not be empty", "service", "exchange-service")
		os.Exit(1)
	}

	smtpPort, _ := strconv.Atoi(getEnv("SMTP_PORT", "1025"))

	cfg := &Config{
		DBHost:             getEnv("DB_HOST", "localhost"),
		DBPort:             getEnv("DB_PORT", "5432"),
		DBUser:             getEnv("DB_USER", "postgres"),
		DBPassword:         getEnv("DB_PASSWORD", "postgres"),
		DBName:             getEnv("DB_NAME", "bankdb"),
		DBSSLMode:          getEnv("DB_SSL_MODE", "disable"),
		GRPCPort:           getEnv("GRPC_PORT", "9098"),
		HTTPPort:           getEnv("HTTP_PORT", "8088"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		AlphaVantageAPIKey: getEnv("ALPHA_VANTAGE_API_KEY", "demo"),
		RedisAddr:          getEnv("REDIS_ADDR", ""),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		RedisDB:            getEnvInt("REDIS_DB", 0),
		OwnRoutingNumber:   getEnvInt("BANK_ROUTING_NUMBER", 333),
		PartnerBanksJSON:   getEnv("PARTNER_BANKS_JSON", "[]"),
		SMTPHost:           getEnv("SMTP_HOST", "mailhog"),
		SMTPPort:           smtpPort,
		SMTPFrom:           getEnv("SMTP_FROM", "noreply@bank.com"),
	}

	slog.Info("Exchange-service config loaded",
		"db_host", cfg.DBHost,
		"http_port", cfg.HTTPPort,
		"grpc_port", cfg.GRPCPort,
		"own_routing", cfg.OwnRoutingNumber,
	)

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		slog.Warn("invalid int env var, using default", "key", key, "raw", v, "default", defaultVal)
	}
	return defaultVal
}
