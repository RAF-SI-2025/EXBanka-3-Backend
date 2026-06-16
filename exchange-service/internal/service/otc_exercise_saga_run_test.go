package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// TestOtcService_ExerciseContract_RunsSaga drives the full 5-step OTC exercise
// SAGA (build steps -> reserve cash/shares -> transfer cash/ownership ->
// finalize) through the orchestrator, covering otc_exercise_saga.go and the
// orchestrator's forward path.
func TestOtcService_ExerciseContract_RunsSaga(t *testing.T) {
	db := openTestDB(t, "otc_exercise_run")
	seedOtcAccountTables(t, db) // accounts: 1 (client 200), 2 (client 100), USD, balance 1000
	assetID := seedAsset(t, db, "SGE", 100, "USD")
	now := time.Now().UTC()

	holding := &models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID,
		Quantity: 10, ReservedQuantity: 3, PublicQuantity: 10, AvgBuyPrice: 90,
		AccountID: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(holding).Error; err != nil {
		t.Fatalf("seed holding: %v", err)
	}

	contract := &models.OtcContractRecord{
		StockListingID: assetID, SellerHoldingID: holding.ID, Amount: 3, StrikePrice: 50,
		SettlementDate: now.AddDate(0, 0, 1),
		BuyerID:        100, BuyerType: "client", BuyerAccountID: 2,
		SellerID:       200, SellerType: "client", SellerAccountID: 1,
		Status:         models.OtcContractStatusValid, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(contract).Error; err != nil {
		t.Fatalf("seed contract: %v", err)
	}

	otcRepo := repository.NewOtcRepository(db)
	svc := service.NewOtcService(repository.NewPortfolioRepository(db), otcRepo).
		WithOrchestrator(service.NewSagaOrchestrator(repository.NewSagaRepository(db), db))

	res, err := svc.ExerciseContract(contract.ID, 100, "client")
	if err != nil {
		t.Fatalf("ExerciseContract: %v", err)
	}
	if res.SagaID == 0 {
		t.Error("expected a saga id")
	}

	got, err := otcRepo.GetContractByID(contract.ID)
	if err != nil {
		t.Fatalf("GetContractByID: %v", err)
	}
	if got.Status != models.OtcContractStatusExercised {
		t.Errorf("contract status=%s, want exercised", got.Status)
	}
}

// TestOtcService_ExerciseContract_FaultRollsBack injects a forced failure mid-saga
// so the orchestrator runs the compensation chain — covering the rollback path
// and the fault-injection hooks (forceFailAfter / shouldFailCompensation).
func TestOtcService_ExerciseContract_FaultRollsBack(t *testing.T) {
	db := openTestDB(t, "otc_exercise_fault")
	seedOtcAccountTables(t, db)
	assetID := seedAsset(t, db, "SGF", 100, "USD")
	now := time.Now().UTC()

	holding := &models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID,
		Quantity: 10, ReservedQuantity: 3, PublicQuantity: 10, AvgBuyPrice: 90,
		AccountID: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(holding).Error; err != nil {
		t.Fatalf("seed holding: %v", err)
	}
	contract := &models.OtcContractRecord{
		StockListingID: assetID, SellerHoldingID: holding.ID, Amount: 3, StrikePrice: 50,
		SettlementDate: now.AddDate(0, 0, 1),
		BuyerID:        100, BuyerType: "client", BuyerAccountID: 2,
		SellerID:       200, SellerType: "client", SellerAccountID: 1,
		Status:         models.OtcContractStatusValid, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(contract).Error; err != nil {
		t.Fatalf("seed contract: %v", err)
	}

	svc := service.NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db)).
		WithOrchestrator(service.NewSagaOrchestrator(repository.NewSagaRepository(db), db))

	// Force step 3 to fail (and fail compensation of step 1 once) -> rollback.
	faults := &service.SagaFaultConfig{
		ForceFailStep: 3, ForceFailAfter: true,
		CompensateFailStep: 1, CompensateFailTimes: 1,
	}
	res, _ := svc.ExerciseContractWithFaults(contract.ID, 100, "client", faults)
	if res == nil || res.SagaID == 0 {
		t.Fatal("expected a saga id even on rollback")
	}
	// The contract should NOT be exercised (the saga failed and rolled back).
	got, _ := repository.NewOtcRepository(db).GetContractByID(contract.ID)
	if got != nil && got.Status == models.OtcContractStatusExercised {
		t.Error("contract should not be exercised after a forced mid-saga failure")
	}
}

// TestOtcService_ExerciseContract_FailAtLastStep runs all forward steps then
// fails the final step, so every prior step gets compensated — covering the
// transfer/reverse step functions.
func TestOtcService_ExerciseContract_FailAtLastStep(t *testing.T) {
	db := openTestDB(t, "otc_exercise_fail5")
	seedOtcAccountTables(t, db)
	assetID := seedAsset(t, db, "SG5", 100, "USD")
	now := time.Now().UTC()

	holding := &models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID,
		Quantity: 10, ReservedQuantity: 3, PublicQuantity: 10, AvgBuyPrice: 90,
		AccountID: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(holding).Error; err != nil {
		t.Fatalf("seed holding: %v", err)
	}
	contract := &models.OtcContractRecord{
		StockListingID: assetID, SellerHoldingID: holding.ID, Amount: 3, StrikePrice: 50,
		SettlementDate: now.AddDate(0, 0, 1),
		BuyerID:        100, BuyerType: "client", BuyerAccountID: 2,
		SellerID:       200, SellerType: "client", SellerAccountID: 1,
		Status:         models.OtcContractStatusValid, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(contract).Error; err != nil {
		t.Fatalf("seed contract: %v", err)
	}

	svc := service.NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db)).
		WithOrchestrator(service.NewSagaOrchestrator(repository.NewSagaRepository(db), db))

	res, _ := svc.ExerciseContractWithFaults(contract.ID, 100, "client", &service.SagaFaultConfig{ForceFailStep: 5})
	if res == nil || res.SagaID == 0 {
		t.Fatal("expected a saga id")
	}
	got, _ := repository.NewOtcRepository(db).GetContractByID(contract.ID)
	if got != nil && got.Status == models.OtcContractStatusExercised {
		t.Error("contract should not be exercised after a step-5 failure + rollback")
	}
}
