package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// TestReconcile_Payments_TerminalAndPending drives the payment half of Run():
// committed/failed outbound rows get their terminal message resent (and are
// finalised), while a stuck pending row is retried. The partner stub ACKs
// terminal messages with 204; NEW_TX gets no vote body, exercising the
// pending-retry transient path.
func TestReconcile_Payments_TerminalAndPending(t *testing.T) {
	db := openSagaTestDB(t, "ib_recon_payments")
	db.Exec(`INSERT INTO accounts (id, currency_id, status, stanje, raspolozivo_stanje, client_id) VALUES (1, 1, 'aktivan', 1000, 1000, 9)`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	partnersJSON := fmt.Sprintf(
		`[{"code":444,"baseUrl":"%s","outboundKey":"out-key","inboundKey":"in-key","displayName":"Partner"}]`,
		srv.URL)
	registry, err := interbank.NewRegistryFromJSON(333, partnersJSON)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	paymentRepo := repository.NewInterbankPaymentRepository(db)

	acct := uint(1)
	seed := func(txid, status string) {
		p := &models.InterbankPayment{
			TxRoutingNumber: 333, TxID: txid, Direction: "outbound", PartnerRoutingNumber: 444,
			SenderAccountNumber: "s", RecipientAccountNumber: "rr", Currency: "RSD", Amount: 10,
			LocalAccountID: &acct, Status: status,
		}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("seed %s: %v", txid, err)
		}
		// Backdate updated_at so the staleness filter selects it.
		db.Model(&models.InterbankPayment{}).Where("id = ?", p.ID).
			Update("updated_at", time.Now().UTC().Add(-time.Hour))
	}
	seed("commit-term", models.InterbankPaymentStatusCommitted)
	seed("failed-term", models.InterbankPaymentStatusFailed)
	seed("stuck-pending", models.InterbankPaymentStatusPending)

	r := NewInterbankReconcileRunner(
		db, registry, interbank.NewClient(registry),
		paymentRepo,
		repository.NewInterbankPaymentWalletRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewInterbankWalletRepository(db),
		repository.NewInterbankOtcRepository(db),
	).WithStaleness(time.Minute)

	r.Run()

	// Both terminal rows should now be finalised (partner ACKed).
	for _, id := range []string{"commit-term", "failed-term"} {
		row, err := paymentRepo.GetByTxID(333, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if row == nil || row.PartnerFinalisedAt == nil {
			t.Errorf("terminal row %s not finalised", id)
		}
	}
}
