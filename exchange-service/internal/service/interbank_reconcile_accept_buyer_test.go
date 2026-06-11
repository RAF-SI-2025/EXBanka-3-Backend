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

// TestReconcileAcceptCommit_BuyerCoordinator covers the buyer-coordinated
// accept crash-recovery path added for spec §3.6 ("the party whose turn it
// is may accept" — that can be the buyer). A buyer-role negotiation that
// got a YES vote but never confirmed its COMMIT_TX must, on reconcile:
//   - resend COMMIT_TX to the SELLER's bank (the counterparty), and
//   - debit the buyer's reserved premium + materialise the local option
//     contract (the buyer holds the option), exactly once.
//
// It is the mirror of TestReconcileAcceptCommit_ResendsAndCredits, which
// covers the seller-coordinated direction.
func TestReconcileAcceptCommit_BuyerCoordinator(t *testing.T) {
	db := openSagaTestDB(t, "ib_recon_accept_commit_buyer")

	// Buyer client-1 (ours) has an RSD account holding 100, with 50 already
	// reserved (raspolozivo decremented) at accept time. COMMIT debits the
	// 50 premium from stanje, leaving (stanje 50, raspolozivo 50).
	if err := db.Exec(`INSERT INTO accounts (id, broj_racuna, currency_id, status, stanje, raspolozivo_stanje, client_id)
		VALUES (1, '333000000000000001', 1, 'aktivan', 100, 50, 1)`).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}

	// Stub seller's bank (the counterparty, routing 555): any POST
	// /interbank (the COMMIT_TX) → 204.
	var commitHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&commitHits, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	partnersJSON := fmt.Sprintf(
		`[{"code":555,"baseUrl":"%s","outboundKey":"out-key","inboundKey":"in-key","displayName":"Seller Bank"}]`,
		srv.URL)
	registry, err := interbank.NewRegistryFromJSON(333, partnersJSON)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	client := interbank.NewClient(registry)
	negRepo := repository.NewInterbankOtcRepository(db)

	// Buyer-role negotiation: we (333) are the buyer, partner 555 the seller.
	// The negotiation key is the seller's coordinates (555/neg-buy-1). YES
	// vote recorded, commit never finalised.
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber:    555,
		NegotiationID:               "neg-buy-1",
		LocalRole:                   models.InterbankNegotiationRoleBuyer,
		CounterpartyRoutingNumber:   555,
		BuyerRoutingNumber:          333,
		BuyerID:                     "client-1",
		SellerRoutingNumber:         555,
		SellerID:                    "client-7",
		StockTicker:                 "ACME",
		Amount:                      3,
		PricePerUnitCurrency:        "RSD",
		PricePerUnitAmount:          10,
		PremiumCurrency:             "RSD",
		PremiumAmount:               50,
		SettlementDate:              time.Now().UTC().Format(time.RFC3339),
		LastModifiedByRoutingNumber: 555,
		LastModifiedByID:            "client-7",
		IsOngoing:                   false,
		AcceptTxRoutingNumber:       333,
		AcceptTxID:                  "acc-tx-buy-1",
	}
	if err := negRepo.Create(neg); err != nil {
		t.Fatalf("create negotiation: %v", err)
	}
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

	// COMMIT_TX resent to the seller's bank exactly once.
	if got := atomic.LoadInt32(&commitHits); got != 1 {
		t.Fatalf("COMMIT_TX sent %d times, want 1", got)
	}

	// Negotiation finalised.
	got, _ := negRepo.Get(555, "neg-buy-1")
	if got.AcceptCommitFinalisedAt == nil {
		t.Fatal("accept_commit_finalised_at not stamped")
	}

	// Buyer debited the 50 premium from stanje only (raspolozivo was already
	// decremented at reserve time, so it is unchanged at 50).
	var acc struct {
		Stanje      float64 `gorm:"column:stanje"`
		Raspolozivo float64 `gorm:"column:raspolozivo_stanje"`
	}
	if err := db.Table("accounts").Select("stanje, raspolozivo_stanje").Where("id = 1").Scan(&acc).Error; err != nil {
		t.Fatal(err)
	}
	if acc.Stanje != 50 || acc.Raspolozivo != 50 {
		t.Fatalf("buyer account = (stanje %v, raspolozivo %v), want (50, 50)", acc.Stanje, acc.Raspolozivo)
	}

	// Local option contract materialised with us on the buyer side.
	var contract models.InterbankOptionContract
	if err := db.Where("negotiation_routing_number = ? AND negotiation_id = ?", 555, "neg-buy-1").
		First(&contract).Error; err != nil {
		t.Fatalf("option contract not created: %v", err)
	}
	if contract.BuyerLocalID != "client-1" {
		t.Fatalf("option contract BuyerLocalID = %q, want client-1", contract.BuyerLocalID)
	}
	if contract.Amount != 3 || contract.PremiumAmount != 50 || contract.Status != models.InterbankOptionContractStatusValid {
		t.Fatalf("option contract terms = (amount %v, premium %v, status %q), want (3, 50, valid)",
			contract.Amount, contract.PremiumAmount, contract.Status)
	}

	// Second run is a no-op: already finalised, no further COMMIT_TX and no
	// double-debit.
	r.Run()
	if got := atomic.LoadInt32(&commitHits); got != 1 {
		t.Fatalf("COMMIT_TX re-sent after finalise; total %d, want 1", got)
	}
	if err := db.Table("accounts").Select("stanje, raspolozivo_stanje").Where("id = 1").Scan(&acc).Error; err != nil {
		t.Fatal(err)
	}
	if acc.Stanje != 50 {
		t.Fatalf("buyer stanje double-debited on second run: %v, want 50", acc.Stanje)
	}
}
