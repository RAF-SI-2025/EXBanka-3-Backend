package service_test

import (
	"strings"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/service"
)

func TestNotificationService_New(t *testing.T) {
	s := service.NewNotificationService(&config.Config{})
	if s == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestSendTransferVerificationCode_EmptyEmail_ReturnsError(t *testing.T) {
	s := service.NewNotificationService(&config.Config{})
	tr := &models.Transfer{VerifikacioniKod: "123456", Iznos: 500, ValutaIznosa: "RSD"}
	err := s.SendTransferVerificationCode("", "Ana", tr)
	if err == nil {
		t.Fatal("expected error for empty email")
	}
	if !strings.Contains(err.Error(), "missing recipient email") {
		t.Errorf("unexpected error: %v", err)
	}
}

// SendTransferVerificationCode with a real recipient hits the SMTP dialer; we
// can't test that path without a fake SMTP server. We verify the validation
// branch fails fast and trust integration tests for the dialer path.

func TestSendTransferVerificationCode_UsesExpiresAtWhenSet(t *testing.T) {
	// This still attempts to send; we expect an SMTP failure but the function
	// should at least construct the body without panicking. To avoid waiting
	// on SMTP we skip this in -short and use an unreachable host via cfg.
	if testing.Short() {
		t.Skip("requires SMTP path; covered by integration")
	}
	cfg := &config.Config{
		SMTPHost: "127.0.0.1",
		SMTPPort: 1, // closed port — fails fast
		SMTPFrom: "noreply@exbanka.local",
	}
	s := service.NewNotificationService(cfg)
	expires := time.Now().Add(5 * time.Minute)
	tr := &models.Transfer{
		VerifikacioniKod:      "999",
		Iznos:                 100,
		ValutaIznosa:          "RSD",
		Svrha:                 "test",
		VerificationExpiresAt: &expires,
	}
	if err := s.SendTransferVerificationCode("a@b.com", "X", tr); err == nil {
		t.Fatal("expected SMTP failure on closed port")
	}
}
