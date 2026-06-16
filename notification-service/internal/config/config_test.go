package config

import (
	"os"
	"testing"
)

func TestGetEnv(t *testing.T) {
	t.Setenv("NOTIF_CFG_X", "value")
	if got := getEnv("NOTIF_CFG_X", "fallback"); got != "value" {
		t.Errorf("set: getEnv = %q, want value", got)
	}
	os.Unsetenv("NOTIF_CFG_Y")
	if got := getEnv("NOTIF_CFG_Y", "fallback"); got != "fallback" {
		t.Errorf("unset: getEnv = %q, want fallback", got)
	}
	t.Setenv("NOTIF_CFG_Z", "")
	if got := getEnv("NOTIF_CFG_Z", "fallback"); got != "fallback" {
		t.Errorf("empty: getEnv = %q, want fallback", got)
	}
}

func TestLoad_Defaults(t *testing.T) {
	for _, k := range []string{
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSL_MODE",
		"HTTP_PORT", "SMTP_HOST", "SMTP_PORT", "SMTP_FROM", "INTERNAL_API_KEY", "FRONTEND_URL",
	} {
		os.Unsetenv(k)
	}
	t.Setenv("JWT_SECRET", "test-secret")

	cfg := Load()
	if cfg == nil {
		t.Fatal("Load returned nil")
	}
	if cfg.DBHost != "localhost" {
		t.Errorf("DBHost = %q, want localhost", cfg.DBHost)
	}
	if cfg.HTTPPort != "8090" {
		t.Errorf("HTTPPort = %q, want 8090", cfg.HTTPPort)
	}
	if cfg.SMTPPort != 1025 {
		t.Errorf("SMTPPort = %d, want 1025", cfg.SMTPPort)
	}
	if cfg.SMTPHost != "mailhog" {
		t.Errorf("SMTPHost = %q, want mailhog", cfg.SMTPHost)
	}
	if cfg.JWTSecret != "test-secret" {
		t.Errorf("JWTSecret = %q", cfg.JWTSecret)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("JWT_SECRET", "s")
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("HTTP_PORT", "9999")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("INTERNAL_API_KEY", "k123")
	t.Setenv("FRONTEND_URL", "https://app.example.com")

	cfg := Load()
	if cfg.DBHost != "db.example.com" {
		t.Errorf("DBHost = %q", cfg.DBHost)
	}
	if cfg.HTTPPort != "9999" {
		t.Errorf("HTTPPort = %q", cfg.HTTPPort)
	}
	if cfg.SMTPPort != 2525 {
		t.Errorf("SMTPPort = %d", cfg.SMTPPort)
	}
	if cfg.InternalAPIKey != "k123" {
		t.Errorf("InternalAPIKey = %q", cfg.InternalAPIKey)
	}
	if cfg.FrontendURL != "https://app.example.com" {
		t.Errorf("FrontendURL = %q", cfg.FrontendURL)
	}
}
