package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// TestReconcilePending_YesVoteCommits drives reconcilePending's happy path: the
// retried NEW_TX gets a YES vote, so the row is committed locally (debit) and
// COMMIT_TX is dispatched + finalised.
func TestReconcilePending_YesVoteCommits(t *testing.T) {
	db := openSagaTestDB(t, "ib_recon_yes")
	db.Exec(`INSERT INTO accounts (id, currency_id, status, stanje, raspolozivo_stanje, client_id) VALUES (1, 1, 'aktivan', 1000, 1000, 9)`)

	// Partner stub: a YES vote for NEW_TX; the same 200+body is harmless for COMMIT_TX.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(interbank.TransactionVote{Vote: interbank.VoteYes})
	}))
	defer srv.Close()

	registry, err := interbank.NewRegistryFromJSON(333, fmt.Sprintf(
		`[{"code":444,"baseUrl":"%s","outboundKey":"o","inboundKey":"i","displayName":"P"}]`, srv.URL))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	paymentRepo := repository.NewInterbankPaymentRepository(db)

	acct := uint(1)
	p := &models.InterbankPayment{
		TxRoutingNumber: 333, TxID: "pending-yes", Direction: "outbound", PartnerRoutingNumber: 444,
		SenderAccountNumber: "s", RecipientAccountNumber: "rr", Currency: "RSD", Amount: 25,
		LocalAccountID: &acct, Status: models.InterbankPaymentStatusPending,
	}
	if err := db.Create(p).Error; err != nil {
		t.Fatalf("seed payment: %v", err)
	}
	db.Model(&models.InterbankPayment{}).Where("id = ?", p.ID).
		Update("updated_at", time.Now().UTC().Add(-time.Hour))

	r := NewInterbankReconcileRunner(
		db, registry, interbank.NewClient(registry),
		paymentRepo,
		repository.NewInterbankPaymentWalletRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewInterbankWalletRepository(db),
		repository.NewInterbankOtcRepository(db),
	).WithStaleness(time.Minute)

	r.Run()

	got, err := paymentRepo.GetByTxID(333, "pending-yes")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != models.InterbankPaymentStatusCommitted {
		t.Errorf("status=%s, want committed after YES vote", got.Status)
	}
}
