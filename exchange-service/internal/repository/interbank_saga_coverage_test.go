package repository

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

func TestInterbankAndSagaListHelpers(t *testing.T) {
	db := openRepoTestDB(t, "ib_saga_lists")
	now := time.Now().UTC()

	ex := NewInterbankExerciseRepository(db)
	if _, err := ex.ListStuckPendingOutbound(now, 10); err != nil {
		t.Fatalf("ListStuckPendingOutbound: %v", err)
	}
	if _, err := ex.ListUndispatchedTerminalOutbound(now, 10); err != nil {
		t.Fatalf("ListUndispatchedTerminalOutbound: %v", err)
	}
	if _, err := NewInterbankOtcRepository(db).ListUndispatchedAcceptCommits(now, 10); err != nil {
		t.Fatalf("ListUndispatchedAcceptCommits: %v", err)
	}

	// saga AppendAttempt against a real saga row.
	saga := &models.SagaTransactionRecord{Type: "otc_exercise", Status: "in_progress", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(saga).Error; err != nil {
		t.Fatalf("seed saga: %v", err)
	}
	if err := NewSagaRepository(db).AppendAttempt(saga.ID, "F1", "ok", ""); err != nil {
		t.Fatalf("AppendAttempt: %v", err)
	}
}

func TestOtcRepository_ExpiryReminderHelpers(t *testing.T) {
	db := openRepoTestDB(t, "otc_reminder")
	now := time.Now().UTC()

	c := &models.OtcContractRecord{
		StockListingID: 1, SellerHoldingID: 1, Amount: 1, StrikePrice: 1,
		SettlementDate: now.Add(48 * time.Hour),
		BuyerID:        1, BuyerType: "client", BuyerAccountID: 1,
		SellerID:       2, SellerType: "client", SellerAccountID: 2,
		Status:         models.OtcContractStatusValid, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(c).Error; err != nil {
		t.Fatalf("seed contract: %v", err)
	}

	r := NewOtcRepository(db)
	if _, err := r.ListContractsNeedingExpiryReminder(now, 3); err != nil {
		t.Fatalf("ListContractsNeedingExpiryReminder: %v", err)
	}
	if err := r.MarkExpiryReminderSent(c.ID); err != nil {
		t.Fatalf("MarkExpiryReminderSent: %v", err)
	}
}
