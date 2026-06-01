package service

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// TestReconcileExercisePending_RollsBackAndReleases verifies the
// crash-recovery path for an outbound option exercise stuck in `pending`:
// the reconcile marks it failed and refunds the buyer's reserved strike
// cash, so a ROLLBACK_TX can follow and the buyer can re-exercise.
func TestReconcileExercisePending_RollsBackAndReleases(t *testing.T) {
	db := openSagaTestDB(t, "ib_recon_exercise_pending")

	// Buyer client-7 holds an RSD account: stanje 100, raspolozivo 60 —
	// i.e. 40 is reserved against the in-flight exercise below.
	if err := db.Exec(`INSERT INTO accounts (id, broj_racuna, currency_id, status, stanje, raspolozivo_stanje, client_id)
		VALUES (1, '333000000000000001', 1, 'aktivan', 100, 60, 7)`).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}

	past := time.Now().UTC().Add(-time.Hour)
	row := &models.InterbankPendingExercise{
		TxRoutingNumber:          333,
		TxID:                     "ex-stuck-1",
		Direction:                models.InterbankExerciseDirectionOutbound,
		PartnerRoutingNumber:     444,
		NegotiationRoutingNumber: 444,
		NegotiationID:            "neg-1",
		StockTicker:              "ACME",
		StockAmount:              4,
		PricePerUnitCurrency:     "RSD",
		PricePerUnitAmount:       10,
		CashAmount:               40,
		BuyerRoutingNumber:       333,
		BuyerID:                  "client-7",
		SellerRoutingNumber:      444,
		SellerID:                 "client-9",
		Status:                   models.InterbankExerciseStatusPending,
		CreatedAt:                past,
		UpdatedAt:                past,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("create exercise row: %v", err)
	}

	r := NewInterbankReconcileRunner(
		db, nil, nil,
		repository.NewInterbankPaymentRepository(db),
		repository.NewInterbankPaymentWalletRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewInterbankWalletRepository(db),
		repository.NewInterbankOtcRepository(db),
	).WithStaleness(time.Minute)

	r.Run()

	var got models.InterbankPendingExercise
	if err := db.First(&got, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != models.InterbankExerciseStatusFailed {
		t.Fatalf("status = %q, want %q", got.Status, models.InterbankExerciseStatusFailed)
	}

	var acc struct {
		Raspolozivo float64 `gorm:"column:raspolozivo_stanje"`
	}
	if err := db.Table("accounts").Select("raspolozivo_stanje").Where("id = 1").Scan(&acc).Error; err != nil {
		t.Fatal(err)
	}
	if acc.Raspolozivo != 100 {
		t.Fatalf("raspolozivo_stanje = %v, want 100 (40 released back)", acc.Raspolozivo)
	}
}
