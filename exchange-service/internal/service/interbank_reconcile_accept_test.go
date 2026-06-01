package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// TestReconcileAcceptCommit_ResendsAndCredits verifies the OTC-accept
// crash-recovery path: a negotiation that got a YES vote but never
// confirmed its COMMIT_TX gets the COMMIT_TX resent, the seller credited,
// and the row finalised so it isn't retried again.
func TestReconcileAcceptCommit_ResendsAndCredits(t *testing.T) {
	db := openSagaTestDB(t, "ib_recon_accept_commit")

	// Seller client-9 has an RSD account, empty, to receive the premium.
	if err := db.Exec(`INSERT INTO accounts (id, broj_racuna, currency_id, status, stanje, raspolozivo_stanje, client_id)
		VALUES (1, '333000000000000009', 1, 'aktivan', 0, 0, 9)`).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}

	// Stub buyer's bank: any POST /interbank (the COMMIT_TX) → 204.
	var commitHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&commitHits, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	partnersJSON := fmt.Sprintf(
		`[{"code":444,"baseUrl":"%s","outboundKey":"out-key","inboundKey":"in-key","displayName":"Buyer Bank"}]`,
		srv.URL)
	registry, err := interbank.NewRegistryFromJSON(333, partnersJSON)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	client := interbank.NewClient(registry)
	negRepo := repository.NewInterbankOtcRepository(db)

	// Accepted negotiation: YES vote recorded (accept_tx_id set), commit
	// never finalised.
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber:    333,
		NegotiationID:               "neg-acc-1",
		LocalRole:                   models.InterbankNegotiationRoleSeller,
		CounterpartyRoutingNumber:   444,
		BuyerRoutingNumber:          444,
		BuyerID:                     "client-1",
		SellerRoutingNumber:         333,
		SellerID:                    "client-9",
		StockTicker:                 "ACME",
		Amount:                      3,
		PricePerUnitCurrency:        "RSD",
		PricePerUnitAmount:          10,
		PremiumCurrency:             "RSD",
		PremiumAmount:               50,
		SettlementDate:              time.Now().UTC().Format(time.RFC3339),
		LastModifiedByRoutingNumber: 444,
		LastModifiedByID:            "client-1",
		IsOngoing:                   false,
		AcceptTxRoutingNumber:       333,
		AcceptTxID:                  "acc-tx-1",
	}
	if err := negRepo.Create(neg); err != nil {
		t.Fatalf("create negotiation: %v", err)
	}
	// Backdate updated_at so the staleness filter picks it up.
	past := time.Now().UTC().Add(-time.Hour)
	if err := db.Model(&models.InterbankOtcNegotiation{}).Where("id = ?", neg.ID).
		Update("updated_at", past).Error; err != nil {
		t.Fatal(err)
	}

	r := NewInterbankReconcileRunner(
		db, registry, client,
		repository.NewInterbankPaymentRepository(db),
		repository.NewInterbankPaymentWalletRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewInterbankWalletRepository(db),
		negRepo,
	).WithStaleness(time.Minute)

	r.Run()

	if got := atomic.LoadInt32(&commitHits); got != 1 {
		t.Fatalf("COMMIT_TX sent %d times, want 1", got)
	}

	// Negotiation finalised.
	got, _ := negRepo.Get(333, "neg-acc-1")
	if got.AcceptCommitFinalisedAt == nil {
		t.Fatal("accept_commit_finalised_at not stamped")
	}

	// Seller credited the 50 premium (both stanje and raspolozivo).
	var acc struct {
		Stanje      float64 `gorm:"column:stanje"`
		Raspolozivo float64 `gorm:"column:raspolozivo_stanje"`
	}
	if err := db.Table("accounts").Select("stanje, raspolozivo_stanje").Where("id = 1").Scan(&acc).Error; err != nil {
		t.Fatal(err)
	}
	if acc.Stanje != 50 || acc.Raspolozivo != 50 {
		t.Fatalf("seller account = (stanje %v, raspolozivo %v), want (50, 50)", acc.Stanje, acc.Raspolozivo)
	}

	// Second run is a no-op: already finalised, no further COMMIT_TX.
	r.Run()
	if got := atomic.LoadInt32(&commitHits); got != 1 {
		t.Fatalf("COMMIT_TX re-sent after finalise; total %d, want 1", got)
	}
}
