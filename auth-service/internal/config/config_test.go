package config

import (
	"os"
	"testing"
)

func TestGetEnv_ReturnsValueWhenSet(t *testing.T) {
	t.Setenv("TEST_KEY_X", "value")
	if got := getEnv("TEST_KEY_X", "fallback"); got != "value" {
		t.Errorf("getEnv = %q, want %q", got, "value")
	}
}

func TestGetEnv_ReturnsDefaultWhenUnset(t *testing.T) {
	os.Unsetenv("TEST_KEY_Y")
	if got := getEnv("TEST_KEY_Y", "fallback"); got != "fallback" {
		t.Errorf("getEnv = %q, want %q", got, "fallback")
	}
}

func TestGetEnv_ReturnsDefaultWhenEmpty(t *testing.T) {
	t.Setenv("TEST_KEY_Z", "")
	if got := getEnv("TEST_KEY_Z", "fallback"); got != "fallback" {
		t.Errorf("getEnv with empty string = %q, want %q", got, "fallback")
	}
}

func TestLoad_ReturnsDefaults(t *testing.T) {
	// Unset everything our code reads
	for _, k := range []string{
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSL_MODE",
		"GRPC_PORT", "HTTP_PORT", "JWT_SECRET",
		"JWT_ACCESS_DURATION_MINUTES", "JWT_REFRESH_DURATION_HOURS",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASSWORD", "SMTP_FROM",
		"FRONTEND_URL",
	} {
		os.Unsetenv(k)
	}

	cfg := Load()
	if cfg == nil {
		t.Fatal("Load returned nil")
	}
	if cfg.DBHost != "localhost" {
		t.Errorf("DBHost = %q, want localhost", cfg.DBHost)
	}
	if cfg.GRPCPort != "9090" {
		t.Errorf("GRPCPort = %q, want 9090", cfg.GRPCPort)
	}
	if cfg.JWTAccessDuration != 15 {
		t.Errorf("JWTAccessDuration = %d, want 15", cfg.JWTAccessDuration)
	}
	if cfg.JWTRefreshDuration != 24 {
		t.Errorf("JWTRefreshDuration = %d, want 24", cfg.JWTRefreshDuration)
	}
	if cfg.SMTPPort != 1025 {
		t.Errorf("SMTPPort = %d, want 1025", cfg.SMTPPort)
	}
}

func TestLoad_ReadsEnvOverrides(t *testing.T) {
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("HTTP_PORT", "9999")
	t.Setenv("JWT_ACCESS_DURATION_MINUTES", "30")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("FRONTEND_URL", "https://app.example.com")

	cfg := Load()
	if cfg.DBHost != "db.example.com" {
		t.Errorf("DBHost = %q", cfg.DBHost)
	}
	if cfg.HTTPPort != "9999" {
		t.Errorf("HTTPPort = %q", cfg.HTTPPort)
	}
	if cfg.JWTAccessDuration != 30 {
		t.Errorf("JWTAccessDuration = %d", cfg.JWTAccessDuration)
	}
	if cfg.SMTPPort != 2525 {
		t.Errorf("SMTPPort = %d", cfg.SMTPPort)
	}
	if cfg.FrontendURL != "https://app.example.com" {
		t.Errorf("FrontendURL = %q", cfg.FrontendURL)
	}
}
