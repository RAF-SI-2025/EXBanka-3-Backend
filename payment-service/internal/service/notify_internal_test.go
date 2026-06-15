package service

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
)

func TestNotifyHelpers(t *testing.T) {
	n := NewAppNotifier(&config.Config{NotificationServiceURL: "", InternalAPIKey: ""})
	if n == nil {
		t.Fatal("NewAppNotifier returned nil")
	}
	if (&PaymentService{}).WithAppNotifier(n) == nil {
		t.Error("PaymentService.WithAppNotifier returned nil")
	}
	if (&PrenosService{}).WithAppNotifier(n) == nil {
		t.Error("PrenosService.WithAppNotifier returned nil")
	}

	p := &models.Payment{Iznos: 100, RacunPrimaocaBroj: "111"}
	sender := uint(1)
	receiver := uint(2)

	// nil client -> early return.
	emitPaymentSettled(nil, p, &sender, &receiver, "payment")
	// payment kind, both sender + receiver.
	emitPaymentSettled(n, p, &sender, &receiver, "payment")
	// prenos kind, sender only (receiver not our client).
	emitPaymentSettled(n, p, &sender, nil, "prenos")
	// receiver only (e.g. incoming to our client from elsewhere).
	emitPaymentSettled(n, p, nil, &receiver, "payment")
}
