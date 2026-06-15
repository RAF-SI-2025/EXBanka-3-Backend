package service

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/notify"
)

func TestCardNotifyHelpers(t *testing.T) {
	n := notify.NewClient("", "")

	// WithNotifier setters chain.
	cs := (&CardService{}).WithNotifier(n)
	if cs == nil {
		t.Fatal("CardService.WithNotifier returned nil")
	}
	if (&AccountService{}).WithNotifier(n) == nil {
		t.Fatal("AccountService.WithNotifier returned nil")
	}

	card := &models.Card{ClientID: 5, BrojKartice: "1234567890123456"}
	// All status branches (no-op client, so no network).
	for _, st := range []string{"blokirana", "aktivna", "deaktivirana", "nepoznato"} {
		cs.emitCardStatusInApp(card, st)
	}
	// Guard branches: nil notifier, and ClientID 0.
	(&CardService{}).emitCardStatusInApp(card, "blokirana")
	cs.emitCardStatusInApp(&models.Card{ClientID: 0, BrojKartice: "x"}, "blokirana")

	// maskCardNumber: short (returned as-is) and long (masked).
	if got := maskCardNumber("12"); got != "12" {
		t.Errorf("maskCardNumber short: got %q", got)
	}
	if got := maskCardNumber("1234567890123456"); got != "•••• 3456" {
		t.Errorf("maskCardNumber long: got %q", got)
	}
}
