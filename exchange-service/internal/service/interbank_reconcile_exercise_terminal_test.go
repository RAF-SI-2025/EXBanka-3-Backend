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

// TestReconcile_ExerciseTerminal resends the terminal message for committed and
// failed outbound exercise rows and finalises them once the partner ACKs (204).
func TestReconcile_ExerciseTerminal(t *testing.T) {
	db := openSagaTestDB(t, "ib_recon_ex_terminal")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	registry, err := interbank.NewRegistryFromJSON(333, fmt.Sprintf(
		`[{"code":444,"baseUrl":"%s","outboundKey":"o","inboundKey":"i","displayName":"P"}]`, srv.URL))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	exRepo := repository.NewInterbankExerciseRepository(db)

	past := time.Now().UTC().Add(-time.Hour)
	seed := func(txid, status string) {
		row := &models.InterbankPendingExercise{
			TxRoutingNumber: 333, TxID: txid, Direction: models.InterbankExerciseDirectionOutbound,
			PartnerRoutingNumber: 444, NegotiationRoutingNumber: 444, NegotiationID: "neg-" + txid,
			StockTicker: "ACME", StockAmount: 1, PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10,
			CashAmount: 10, BuyerRoutingNumber: 333, BuyerID: "client-7",
			SellerRoutingNumber: 444, SellerID: "client-9",
			Status: status, CreatedAt: past, UpdatedAt: past,
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %s: %v", txid, err)
		}
	}
	seed("ex-commit", models.InterbankExerciseStatusCommitted)
	seed("ex-failed", models.InterbankExerciseStatusFailed)

	r := NewInterbankReconcileRunner(
		db, registry, interbank.NewClient(registry),
		repository.NewInterbankPaymentRepository(db),
		repository.NewInterbankPaymentWalletRepository(db),
		exRepo,
		repository.NewInterbankWalletRepository(db),
		repository.NewInterbankOtcRepository(db),
	).WithStaleness(time.Minute)

	r.Run()

	for _, id := range []string{"ex-commit", "ex-failed"} {
		var got models.InterbankPendingExercise
		if err := db.Where("tx_routing_number = ? AND tx_id = ?", 333, id).First(&got).Error; err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if got.PartnerFinalisedAt == nil {
			t.Errorf("exercise %s not finalised after terminal resend", id)
		}
	}
}
