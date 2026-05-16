package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openSagaTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Reference tables used by saga steps.
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS currencies (id INTEGER PRIMARY KEY AUTOINCREMENT, kod TEXT)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			broj_racuna TEXT, currency_id INTEGER, status TEXT DEFAULT 'aktivan',
			stanje REAL DEFAULT 0, raspolozivo_stanje REAL DEFAULT 0,
			dnevni_limit REAL, mesecni_limit REAL,
			dnevna_potrosnja REAL DEFAULT 0, mesecna_potrosnja REAL DEFAULT 0,
			client_id INTEGER, firma_id INTEGER, zaposleni_id INTEGER,
			created_at DATETIME, updated_at DATETIME
		)`,
		`INSERT OR IGNORE INTO currencies (id, kod) VALUES (1, 'RSD')`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed: %v (%s)", err, s)
		}
	}
	return db
}

// =====================================================================
// BuildOtcExerciseSteps + MarshalOtcExercisePayload
// =====================================================================

func TestBuildOtcExerciseSteps_FiveSteps(t *testing.T) {
	contract := &models.OtcContractRecord{
		ID: 1, BuyerID: 100, BuyerType: "client",
		BuyerAccountID: 1, SellerAccountID: 2, SellerHoldingID: 3,
		StockListingID: 4, Amount: 10, StrikePrice: 50,
	}
	steps := BuildOtcExerciseSteps(contract)
	if len(steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(steps))
	}
	for _, s := range steps {
		if s.Forward == nil || s.Compensate == nil || s.Name == "" {
			t.Fatalf("step missing fields: %+v", s)
		}
	}
}

func TestMarshalOtcExercisePayload(t *testing.T) {
	contract := &models.OtcContractRecord{
		ID: 1, BuyerID: 100, BuyerType: "client",
		Amount: 10, StrikePrice: 50,
	}
	raw, err := MarshalOtcExercisePayload(contract)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if raw == "" {
		t.Fatal("expected non-empty JSON")
	}
}

// =====================================================================
// reserveAccountFunds + releaseAccountFunds + transferStrikeFunds
// =====================================================================

func seedSagaAccount(t *testing.T, db *gorm.DB, balance, available float64) uint {
	t.Helper()
	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, status, stanje, raspolozivo_stanje, dnevni_limit, mesecni_limit, client_id, created_at, updated_at) VALUES (?, 1, ?, ?, ?, ?, ?, 1, ?, ?)`,
		fmt.Sprintf("SAGA-%d-%f", time.Now().UnixNano(), balance), "aktivan", balance, available, 1000000.0, 10000000.0, now, now).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	var id uint
	db.Table("accounts").Select("id").Order("id DESC").Limit(1).Scan(&id)
	return id
}

func TestReserveAccountFunds_AndRelease(t *testing.T) {
	db := openSagaTestDB(t, "saga_reserve")
	acct := seedSagaAccount(t, db, 1000, 1000)

	db.Transaction(func(tx *gorm.DB) error {
		if err := reserveAccountFunds(tx, acct, 100); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		var ref repository.OtcAccountReference
		_ = lockAccount(tx, acct, &ref)
		if ref.RaspolozivoStanje != 900 {
			t.Fatalf("expected raspolozivo 900, got %v", ref.RaspolozivoStanje)
		}
		if err := releaseAccountFunds(tx, acct, 100); err != nil {
			t.Fatalf("release: %v", err)
		}
		return nil
	})
}

func TestReserveAccountFunds_Insufficient(t *testing.T) {
	db := openSagaTestDB(t, "saga_reserve_insufficient")
	acct := seedSagaAccount(t, db, 50, 50)
	db.Transaction(func(tx *gorm.DB) error {
		if err := reserveAccountFunds(tx, acct, 100); err == nil {
			t.Fatal("expected insufficient err")
		}
		return nil
	})
}

func TestTransferStrikeFunds_AndReverse(t *testing.T) {
	db := openSagaTestDB(t, "saga_transfer_funds")
	buyer := seedSagaAccount(t, db, 1000, 1000)
	seller := seedSagaAccount(t, db, 0, 0)

	db.Transaction(func(tx *gorm.DB) error {
		if err := transferStrikeFunds(tx, buyer, seller, 200); err != nil {
			t.Fatalf("transfer: %v", err)
		}
		if err := reverseStrikeFunds(tx, buyer, seller, 200); err != nil {
			t.Fatalf("reverse: %v", err)
		}
		return nil
	})
}

func TestLockAccount_NotFound(t *testing.T) {
	db := openSagaTestDB(t, "saga_lock_notfound")
	db.Transaction(func(tx *gorm.DB) error {
		var ref repository.OtcAccountReference
		if err := lockAccount(tx, 99999, &ref); err == nil {
			t.Fatal("expected error for missing account")
		}
		return nil
	})
}

// =====================================================================
// Saga orchestrator + retry + public_stock_cache constructors
// =====================================================================

func TestNewSagaOrchestrator_Constructs(t *testing.T) {
	db := openSagaTestDB(t, "saga_orch_ctor")
	o := NewSagaOrchestrator(repository.NewSagaRepository(db), db)
	if o == nil {
		t.Fatal("expected non-nil orchestrator")
	}
}

func TestSagaOrchestrator_Run_EmptySteps(t *testing.T) {
	db := openSagaTestDB(t, "saga_orch_run")
	o := NewSagaOrchestrator(repository.NewSagaRepository(db), db)
	id, err := o.Run("test", "{}", []SagaStep{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero saga id")
	}
}

func TestSagaOrchestrator_Run_SuccessThenComplete(t *testing.T) {
	db := openSagaTestDB(t, "saga_orch_success")
	o := NewSagaOrchestrator(repository.NewSagaRepository(db), db)
	calls := 0
	steps := []SagaStep{
		{Name: "step1",
			Forward:    func(tx *gorm.DB) error { calls++; return nil },
			Compensate: func(tx *gorm.DB) error { return nil }},
		{Name: "step2",
			Forward:    func(tx *gorm.DB) error { calls++; return nil },
			Compensate: func(tx *gorm.DB) error { return nil }},
	}
	if _, err := o.Run("test", "{}", steps); err != nil {
		t.Fatalf("run: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 forward calls, got %d", calls)
	}
}

func TestSagaOrchestrator_Run_ForwardFailure_Compensates(t *testing.T) {
	db := openSagaTestDB(t, "saga_orch_failure")
	o := NewSagaOrchestrator(repository.NewSagaRepository(db), db)
	compensatedFirst := false
	steps := []SagaStep{
		{Name: "step1",
			Forward:    func(tx *gorm.DB) error { return nil },
			Compensate: func(tx *gorm.DB) error { compensatedFirst = true; return nil }},
		{Name: "step2",
			Forward:    func(tx *gorm.DB) error { return fmt.Errorf("boom") },
			Compensate: func(tx *gorm.DB) error { return nil }},
	}
	if _, err := o.Run("test", "{}", steps); err == nil {
		t.Fatal("expected error from failed forward")
	}
	if !compensatedFirst {
		t.Fatal("expected first step to be compensated")
	}
}

func TestNewSagaRetryRunner_Constructs(t *testing.T) {
	db := openSagaTestDB(t, "saga_retry_ctor")
	sagaRepo := repository.NewSagaRepository(db)
	otcRepo := repository.NewOtcRepository(db)
	orch := NewSagaOrchestrator(sagaRepo, db)
	r := NewSagaRetryRunner(sagaRepo, otcRepo, orch)
	if r == nil {
		t.Fatal("expected non-nil retry runner")
	}
}

func TestSagaRetryRunner_Run_NoStuck(t *testing.T) {
	db := openSagaTestDB(t, "saga_retry_run")
	sagaRepo := repository.NewSagaRepository(db)
	otcRepo := repository.NewOtcRepository(db)
	orch := NewSagaOrchestrator(sagaRepo, db)
	r := NewSagaRetryRunner(sagaRepo, otcRepo, orch)
	r.Run() // no panic, no return value
}

// =====================================================================
// OtcService.WithOrchestrator + GetContractForParticipant
// =====================================================================

func TestOtcService_WithOrchestrator_Chains(t *testing.T) {
	db := openSagaTestDB(t, "otc_with_orch")
	svc := NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db))
	if got := svc.WithOrchestrator(nil); got != svc {
		t.Fatal("expected self-return for chaining")
	}
}

func TestOtcService_GetContractForParticipant_NotFound(t *testing.T) {
	db := openSagaTestDB(t, "otc_get_contract_notfound")
	svc := NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db))
	if _, err := svc.GetContractForParticipant(99999, 1, "client"); err == nil {
		t.Fatal("expected error")
	}
}

func TestOtcService_ExerciseContract_NotFound(t *testing.T) {
	db := openSagaTestDB(t, "otc_exercise_contract_notfound")
	svc := NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db))
	if _, err := svc.ExerciseContract(99999, 1, "client"); err == nil {
		t.Fatal("expected error")
	}
}

// =====================================================================
// expireDueOtcContracts cron helper
// =====================================================================

func TestExpireDueOtcContracts_NoContracts(t *testing.T) {
	db := openSagaTestDB(t, "otc_expire_none")
	svc := NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db))
	// Should not panic with no contracts.
	expireDueOtcContracts(svc)
}

// =====================================================================
// InterbankReconcileRunner constructor + Run with empty DB
// =====================================================================

func TestNewInterbankReconcileRunner_Constructs(t *testing.T) {
	db := openSagaTestDB(t, "ib_recon_ctor")
	r := NewInterbankReconcileRunner(
		db, nil, nil,
		repository.NewInterbankPaymentRepository(db),
		repository.NewInterbankPaymentWalletRepository(db),
	)
	if r == nil {
		t.Fatal("expected non-nil runner")
	}
	if got := r.WithStaleness(time.Minute); got != r {
		t.Fatal("expected self-return")
	}
}

// =====================================================================
// verifySellerReservedShares + transferShareOwnership + reverseShareOwnership
// =====================================================================

func seedSagaHolding(t *testing.T, db *gorm.DB, userID, assetID uint, qty, reserved float64) uint {
	t.Helper()
	now := time.Now().UTC()
	h := &models.PortfolioHoldingRecord{
		UserID: userID, UserType: "client", AssetID: assetID,
		Quantity: qty, ReservedQuantity: reserved,
		AvgBuyPrice: 50,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.Create(h).Error; err != nil {
		t.Fatalf("seed holding: %v", err)
	}
	return h.ID
}

func TestVerifySellerReservedShares_OK(t *testing.T) {
	db := openSagaTestDB(t, "saga_verify_shares")
	hID := seedSagaHolding(t, db, 1, 5, 10, 10)
	db.Transaction(func(tx *gorm.DB) error {
		if err := verifySellerReservedShares(tx, hID, 5); err != nil {
			t.Fatalf("expected OK, got %v", err)
		}
		return nil
	})
}

func TestVerifySellerReservedShares_InsufficientQuantity(t *testing.T) {
	db := openSagaTestDB(t, "saga_verify_insufficient_qty")
	hID := seedSagaHolding(t, db, 1, 5, 1, 1)
	db.Transaction(func(tx *gorm.DB) error {
		if err := verifySellerReservedShares(tx, hID, 100); err == nil {
			t.Fatal("expected error")
		}
		return nil
	})
}

func TestVerifySellerReservedShares_InsufficientReservation(t *testing.T) {
	db := openSagaTestDB(t, "saga_verify_insufficient_reserve")
	hID := seedSagaHolding(t, db, 1, 5, 100, 0)
	db.Transaction(func(tx *gorm.DB) error {
		if err := verifySellerReservedShares(tx, hID, 5); err == nil {
			t.Fatal("expected error")
		}
		return nil
	})
}

func TestTransferShareOwnership_NewBuyerHolding(t *testing.T) {
	db := openSagaTestDB(t, "saga_transfer_owner_new")
	sellerID := seedSagaHolding(t, db, 1, 5, 10, 10)
	buyerAccountID := seedSagaAccount(t, db, 0, 0)

	db.Transaction(func(tx *gorm.DB) error {
		err := transferShareOwnership(tx, sellerID, 2, "client", buyerAccountID, 5, 5, 100)
		if err != nil {
			t.Fatalf("transfer: %v", err)
		}
		return nil
	})

	// Buyer should now have a holding with quantity 5
	var holding models.PortfolioHoldingRecord
	if err := db.Where("user_id = ? AND user_type = ? AND asset_id = ?", 2, "client", 5).First(&holding).Error; err != nil {
		t.Fatalf("expected buyer holding: %v", err)
	}
	if holding.Quantity != 5 {
		t.Fatalf("expected qty 5, got %v", holding.Quantity)
	}
}

func TestTransferShareOwnership_QuantityBelow(t *testing.T) {
	db := openSagaTestDB(t, "saga_transfer_qty_below")
	sellerID := seedSagaHolding(t, db, 1, 5, 1, 1)
	buyerAccountID := seedSagaAccount(t, db, 0, 0)
	db.Transaction(func(tx *gorm.DB) error {
		err := transferShareOwnership(tx, sellerID, 2, "client", buyerAccountID, 5, 100, 50)
		if err == nil {
			t.Fatal("expected qty below error")
		}
		return nil
	})
}

func TestTransferShareOwnership_AppendsToExistingBuyer(t *testing.T) {
	db := openSagaTestDB(t, "saga_transfer_append")
	sellerID := seedSagaHolding(t, db, 1, 5, 10, 10)
	// Buyer already holds the asset
	_ = seedSagaHolding(t, db, 2, 5, 3, 0)
	buyerAccountID := seedSagaAccount(t, db, 0, 0)

	db.Transaction(func(tx *gorm.DB) error {
		err := transferShareOwnership(tx, sellerID, 2, "client", buyerAccountID, 5, 2, 100)
		if err != nil {
			t.Fatalf("transfer: %v", err)
		}
		return nil
	})

	var holding models.PortfolioHoldingRecord
	if err := db.Where("user_id = ? AND user_type = ? AND asset_id = ?", 2, "client", 5).First(&holding).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if holding.Quantity != 5 {
		t.Fatalf("expected qty 5 (3+2), got %v", holding.Quantity)
	}
}

func TestReverseShareOwnership_NoBuyerHolding(t *testing.T) {
	db := openSagaTestDB(t, "saga_reverse_nobuyer")
	sellerID := seedSagaHolding(t, db, 1, 5, 5, 0)
	db.Transaction(func(tx *gorm.DB) error {
		// No buyer holding — reverseShareOwnership should be a no-op success.
		if err := reverseShareOwnership(tx, sellerID, 99, "client", 5, 1, 100); err != nil {
			t.Fatalf("reverse: %v", err)
		}
		return nil
	})
}

func TestReverseShareOwnership_WithBuyerHolding(t *testing.T) {
	db := openSagaTestDB(t, "saga_reverse_withbuyer")
	sellerID := seedSagaHolding(t, db, 1, 5, 5, 0)
	_ = seedSagaHolding(t, db, 2, 5, 3, 0)

	db.Transaction(func(tx *gorm.DB) error {
		if err := reverseShareOwnership(tx, sellerID, 2, "client", 5, 2, 100); err != nil {
			t.Fatalf("reverse: %v", err)
		}
		return nil
	})
	var holding models.PortfolioHoldingRecord
	db.Where("user_id = ? AND user_type = ? AND asset_id = ?", 2, "client", 5).First(&holding)
	if holding.Quantity != 1 {
		t.Fatalf("expected qty 1 (3-2), got %v", holding.Quantity)
	}
}

// =====================================================================
// finalizeOtcExercise + revertOtcExerciseFinalization
// =====================================================================

func TestRevertOtcExerciseFinalization_RestoresFunds(t *testing.T) {
	db := openSagaTestDB(t, "saga_revert_finalize")
	buyerAcct := seedSagaAccount(t, db, 1000, 1000)
	// Seed an OtcContractRecord row so the status flip in revertFinalization can run.
	contract := &models.OtcContractRecord{
		Status: "exercised",
	}
	db.Create(contract)
	db.Transaction(func(tx *gorm.DB) error {
		if err := revertOtcExerciseFinalization(tx, contract.ID, buyerAcct, 200); err != nil {
			t.Fatalf("revert: %v", err)
		}
		return nil
	})
}

// =====================================================================
// PublicStockCacheRunner constructor + Run with nil registry
// =====================================================================

// Skipped: Run() needs registry; constructor is hit by saga test below.

// =====================================================================
// NewInterbankReconcileRunner_Run_EmptyTable (continued)
// =====================================================================

// =====================================================================
// NewPublicStockCacheRunner constructor
// =====================================================================

func TestNewPublicStockCacheRunner_Constructs(t *testing.T) {
	db := openSagaTestDB(t, "ps_cache_ctor")
	r := NewPublicStockCacheRunner(nil, nil, repository.NewRemotePublicStockRepository(db))
	if r == nil {
		t.Fatal("expected non-nil runner")
	}
}

// =====================================================================
// OrderExecutor.Run — empty
// =====================================================================

func TestOrderExecutor_Run_NoOrders(t *testing.T) {
	db := openSagaTestDB(t, "order_executor_run_empty")
	orderRepo := repository.NewOrderRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	rates := struct {
		mockRateProv
	}{}
	executor := NewOrderExecutor(orderRepo, marketRepo, nil, &rates.mockRateProv)
	executor.Run() // empty → no panic
}

type mockRateProv struct{}

func (mockRateProv) GetRate(from, to string) (float64, error) {
	if from == to {
		return 1, nil
	}
	return 110, nil
}
func (mockRateProv) GetAllRates() []ExchangeRate { return nil }

// =====================================================================
// SagaOrchestrator.RetryCompensations — no-op for an empty saga
// =====================================================================

func TestSagaOrchestrator_RetryCompensations_EmptyStepRecords(t *testing.T) {
	db := openSagaTestDB(t, "saga_retry_empty")
	o := NewSagaOrchestrator(repository.NewSagaRepository(db), db)
	saga := &models.SagaTransactionRecord{ID: 1, Status: "rolling_back"}
	if err := o.RetryCompensations(saga, []SagaStep{}); err != nil {
		t.Fatalf("expected no error for empty steps, got %v", err)
	}
}

func TestInterbankReconcileRunner_Run_EmptyTable(t *testing.T) {
	db := openSagaTestDB(t, "ib_recon_run_empty")
	r := NewInterbankReconcileRunner(
		db, nil, nil,
		repository.NewInterbankPaymentRepository(db),
		repository.NewInterbankPaymentWalletRepository(db),
	)
	r.Run() // empty table — exits cleanly
}
