package service

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
)

func TestNotifyHelpers(t *testing.T) {
	// NewAppNotifier builds a (no-op) client from config.
	n := NewAppNotifier(&config.Config{NotificationServiceURL: "", InternalAPIKey: ""})
	if n == nil {
		t.Fatal("NewAppNotifier returned nil")
	}
	// WithAppNotifier chains on the service.
	svc := (&TransferService{}).WithAppNotifier(n)
	if svc == nil {
		t.Fatal("WithAppNotifier returned nil")
	}

	// emitTransferSettled: nil guard branch + the emit branch (no-op client).
	emitTransferSettled(nil, &models.Transfer{}, nil)
	cid := uint(5)
	emitTransferSettled(n, &models.Transfer{Iznos: 100, ValutaIznosa: "RSD"}, &cid)
}
