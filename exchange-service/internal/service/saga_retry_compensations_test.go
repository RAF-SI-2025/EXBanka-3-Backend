package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// TestSagaRetryRunner_RetryCompensations seeds a saga stuck in rolling_back with
// completed step records + a buildable contract, then runs the retry — driving
// SagaOrchestrator.RetryCompensations and the OTC-exercise compensation funcs.
func TestSagaRetryRunner_RetryCompensations(t *testing.T) {
	db := openSagaTestDB(t, "saga_retry_comp")
	now := time.Now().UTC()

	exch := models.MarketExchangeRecord{Acronym: "SRC", Name: "X", MICCode: "SRC1", Polity: "X", Currency: "RSD", Timezone: "UTC", WorkingHours: "09-17"}
	db.Create(&exch)
	listing := models.MarketListingRecord{Ticker: "SRT", Name: "SRT", Type: "stock", ExchangeID: exch.ID, Price: 50, Ask: 50, Bid: 50, Volume: 1, LastRefresh: now}
	db.Create(&listing)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, stanje, raspolozivo_stanje) VALUES (1, 1, 'aktivan', 100, 5000, 5000), (2, 1, 'aktivan', 200, 5000, 5000)`)
	holding := models.PortfolioHoldingRecord{UserID: 200, UserType: "client", AssetID: listing.ID, Quantity: 10, ReservedQuantity: 3, AvgBuyPrice: 40, AccountID: 2, CreatedAt: now}
	db.Create(&holding)
	ct := models.OtcContractRecord{
		StockListingID: listing.ID, SellerHoldingID: holding.ID, Amount: 3, StrikePrice: 50,
		SettlementDate: now.AddDate(0, 0, 1),
		BuyerID:        100, BuyerType: "client", BuyerAccountID: 1,
		SellerID:       200, SellerType: "client", SellerAccountID: 2,
		Status:         models.OtcContractStatusValid, CreatedAt: now, UpdatedAt: now,
	}
	db.Create(&ct)

	saga := models.SagaTransactionRecord{
		Type: models.SagaTypeOtcExercise, Status: models.SagaStatusRollingBack,
		Payload: fmt.Sprintf(`{"contractId":%d}`, ct.ID), CurrentStep: 3, RetryCount: 0,
		CreatedAt: now, UpdatedAt: now,
	}
	db.Create(&saga)
	db.Model(&models.SagaTransactionRecord{}).Where("id = ?", saga.ID).UpdateColumn("updated_at", now.Add(-time.Hour))
	for i := 1; i <= 3; i++ {
		db.Create(&models.SagaStepRecord{
			SagaID: saga.ID, StepNumber: i, StepName: fmt.Sprintf("step%d", i),
			Status: models.SagaStepStatusCompleted, CreatedAt: now, UpdatedAt: now,
		})
	}

	sagaRepo := repository.NewSagaRepository(db)
	r := NewSagaRetryRunner(sagaRepo, repository.NewOtcRepository(db), NewSagaOrchestrator(sagaRepo, db))
	r.Run()

	// The saga should have progressed out of rolling_back (rolled_back on success,
	// or requires_manual_intervention if a compensation failed) — either way
	// RetryCompensations ran.
	var got models.SagaTransactionRecord
	if err := db.First(&got, saga.ID).Error; err != nil {
		t.Fatalf("reload saga: %v", err)
	}
	if got.Status == models.SagaStatusRollingBack && got.RetryCount == 0 {
		t.Errorf("expected the retry to advance the saga, status=%s retry=%d", got.Status, got.RetryCount)
	}
}
