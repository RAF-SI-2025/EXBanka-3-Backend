package handler

// SAGA test suite (files/SAGA.md). These drive the OTC option-exercise saga
// end-to-end over HTTP: POST /api/v1/otc/contracts/{id}/exercise, then poll
// GET /api/v1/otc/saga/{id}, asserting the saga log, participant state, and the
// spec invariants I1-I6.
//
// Status-name mapping (our schema -> spec): in_progress=Running,
// completed=Completed, rolling_back=Compensating, rolled_back=Compensated.
//
// SG-01..SG-04 need no product changes. SG-05..SG-08 (forced-failure paths) are
// added once the X-Saga-* fault hooks land.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"gorm.io/gorm"
)

// =====================================================================
// Harness
// =====================================================================

// setupSagaExerciseHandler builds an OTC handler with the saga orchestrator and
// saga querier wired, mirroring cmd/server/main.go. Returns the saga repo too so
// tests can drive the retry cron (RetryCompensations) directly.
func setupSagaExerciseHandler(t *testing.T, db *gorm.DB) (*OtcHTTPHandler, *repository.SagaRepository) {
	t.Helper()
	cfg := &config.Config{JWTSecret: testJWTSecret, SagaFaultHooks: true}
	sagaRepo := repository.NewSagaRepository(db)
	orch := service.NewSagaOrchestrator(sagaRepo, db)
	otcSvc := service.NewOtcService(
		repository.NewPortfolioRepository(db),
		repository.NewOtcRepository(db),
	).WithOrchestrator(orch)
	h := NewOtcHTTPHandler(cfg, otcSvc).WithSagaQuerier(sagaRepo)
	return h, sagaRepo
}

// seedSagaAccountsSchema creates the reference tables the saga steps touch but
// which are not gorm-migrated models in exchange-service (they live in other
// services' migrations). Mirrors seedOtcHandlerAccounts.
func seedSagaAccountsSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS currencies (id integer primary key, kod text not null unique)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id integer primary key,
			client_id integer, firma_id integer, zaposleni_id integer,
			currency_id integer not null,
			stanje real not null default 0,
			raspolozivo_stanje real not null default 0,
			dnevna_potrosnja real not null default 0,
			mesecna_potrosnja real not null default 0,
			status text not null default 'aktivan'
		)`,
		`INSERT OR IGNORE INTO currencies (id, kod) VALUES (1, 'USD')`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed accounts schema: %v", err)
		}
	}
}

// exerciseSeed parameterises a ready-to-exercise contract. Defaults match the
// SG-01 happy path (qty=10, strike=300, buyer 5000, seller 10 reserved).
type exerciseSeed struct {
	amount         float64
	strike         float64
	buyerStanje    float64
	buyerAvail     float64
	sellerQty      float64
	sellerReserved float64
	status         string
	settlement     time.Time
}

func defaultExerciseSeed() exerciseSeed {
	return exerciseSeed{
		amount: 10, strike: 300,
		buyerStanje: 5000, buyerAvail: 5000,
		sellerQty: 10, sellerReserved: 10,
		status:     models.OtcContractStatusValid,
		settlement: time.Now().UTC().AddDate(0, 0, 1),
	}
}

// seedExercisableContract creates a listing, buyer account (id=2, client 100 —
// matches clientToken) and seller account (id=1, client 200), a reserved seller
// holding, and a contract owned by buyer client-100. Returns the contract.
func seedExercisableContract(t *testing.T, db *gorm.DB, s exerciseSeed) *models.OtcContractRecord {
	t.Helper()
	seedSagaAccountsSchema(t, db)
	_, listingID := seedExchangeAndListing(t, db, "SAGA")

	if err := db.Exec(`INSERT INTO accounts (id, client_id, currency_id, stanje, raspolozivo_stanje, status) VALUES
		(1, 200, 1, ?, ?, 'aktivan'),
		(2, 100, 1, ?, ?, 'aktivan')`,
		0.0, 0.0, s.buyerStanje, s.buyerAvail).Error; err != nil {
		t.Fatalf("seed accounts: %v", err)
	}

	now := time.Now().UTC()
	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: listingID,
		Quantity: s.sellerQty, ReservedQuantity: s.sellerReserved,
		AvgBuyPrice: 90, AccountID: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatalf("seed seller holding: %v", err)
	}

	contract := models.OtcContractRecord{
		StockListingID:  listingID,
		SellerHoldingID: holding.ID,
		Amount:          s.amount,
		StrikePrice:     s.strike,
		Premium:         25,
		SettlementDate:  s.settlement,
		BuyerID:         100, BuyerType: "client", BuyerAccountID: 2,
		SellerID: 200, SellerType: "client", SellerAccountID: 1,
		Status:    s.status,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&contract).Error; err != nil {
		t.Fatalf("seed contract: %v", err)
	}
	return &contract
}

// =====================================================================
// HTTP + assertion helpers
// =====================================================================

func exerciseContractHTTP(t *testing.T, h *OtcHTTPHandler, contractID uint, token string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/otc/contracts/%d/exercise", contractID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	return rec
}

func decodeSagaID(t *testing.T, rec *httptest.ResponseRecorder) uint {
	t.Helper()
	var body struct {
		SagaID uint `json:"sagaId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode exercise response: %v body=%s", err, rec.Body.String())
	}
	return body.SagaID
}

type sagaStepView struct {
	StepNumber   int    `json:"stepNumber"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage"`
}

type sagaAttemptView struct {
	Phase   string `json:"phase"`
	Outcome string `json:"outcome"`
	Error   string `json:"error"`
}

type sagaView struct {
	ID          uint              `json:"id"`
	Status      string            `json:"status"`
	CurrentStep int               `json:"currentStep"`
	RetryCount  int               `json:"retryCount"`
	Steps       []sagaStepView    `json:"steps"`
	Log         []sagaAttemptView `json:"log"`
}

func getSagaHTTP(t *testing.T, h *OtcHTTPHandler, sagaID uint, token string) sagaView {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/otc/saga/%d", sagaID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get saga: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Saga sagaView `json:"saga"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode saga: %v body=%s", err, rec.Body.String())
	}
	return body.Saga
}

func stepsByNumber(saga sagaView) map[int]sagaStepView {
	m := make(map[int]sagaStepView, len(saga.Steps))
	for _, s := range saga.Steps {
		m[s.StepNumber] = s
	}
	return m
}

// logShape renders the append-only saga log as ordered "F1 ok" / "C2 err"
// strings, for spec-literal comparison (files/SAGA.md).
func logShape(saga sagaView) []string {
	out := make([]string, 0, len(saga.Log))
	for _, a := range saga.Log {
		out = append(out, a.Phase+" "+a.Outcome)
	}
	return out
}

func assertLog(t *testing.T, saga sagaView, want []string) {
	t.Helper()
	got := logShape(saga)
	if len(got) != len(want) {
		t.Fatalf("saga log = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("saga log[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func accountField(t *testing.T, db *gorm.DB, accountID uint, column string) float64 {
	t.Helper()
	var v float64
	if err := db.Table("accounts").Select(column).Where("id = ?", accountID).Scan(&v).Error; err != nil {
		t.Fatalf("read account %d.%s: %v", accountID, column, err)
	}
	return v
}

func assertAccount(t *testing.T, db *gorm.DB, accountID uint, wantStanje, wantAvail float64) {
	t.Helper()
	if got := accountField(t, db, accountID, "stanje"); got != wantStanje {
		t.Fatalf("account %d stanje = %v, want %v", accountID, got, wantStanje)
	}
	if got := accountField(t, db, accountID, "raspolozivo_stanje"); got != wantAvail {
		t.Fatalf("account %d raspolozivo = %v, want %v", accountID, got, wantAvail)
	}
}

func countSagas(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&models.SagaTransactionRecord{}).Count(&n).Error; err != nil {
		t.Fatalf("count sagas: %v", err)
	}
	return n
}

// =====================================================================
// SG-01: happy path
// =====================================================================

func TestSaga_SG01_HappyPath(t *testing.T) {
	db := newTestDB(t, "saga_sg01")
	h, _ := setupSagaExerciseHandler(t, db)
	contract := seedExercisableContract(t, db, defaultExerciseSeed())

	rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("exercise: status=%d body=%s", rec.Code, rec.Body.String())
	}
	sagaID := decodeSagaID(t, rec)
	if sagaID == 0 {
		t.Fatal("expected non-zero sagaId")
	}

	saga := getSagaHTTP(t, h, sagaID, clientToken(t))
	if saga.Status != models.SagaStatusCompleted {
		t.Fatalf("expected saga completed, got %q", saga.Status)
	}
	if saga.CurrentStep != 5 {
		t.Fatalf("expected current_step 5, got %d", saga.CurrentStep)
	}
	if len(saga.Steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(saga.Steps))
	}
	for _, s := range saga.Steps {
		if s.Status != models.SagaStepStatusCompleted {
			t.Fatalf("step %d (%s) not completed: %s", s.StepNumber, s.Name, s.Status)
		}
	}
	// I4: the log records every phase attempted, in order — five clean forwards.
	assertLog(t, saga, []string{"F1 ok", "F2 ok", "F3 ok", "F4 ok", "F5 ok"})

	// I1 (money conserved) + balances: buyer 5000-3000=2000, seller 0+3000=3000.
	// I3: no stranded reservation -> buyer raspolozivo == stanje.
	assertAccount(t, db, 2, 2000, 2000) // buyer
	assertAccount(t, db, 1, 3000, 3000) // seller

	// I2 (shares conserved): buyer now holds 10, seller drained to 0/0.
	var buyerHolding models.PortfolioHoldingRecord
	if err := db.Where("user_id = ? AND user_type = ? AND asset_id = ?", 100, "client", contract.StockListingID).First(&buyerHolding).Error; err != nil {
		t.Fatalf("buyer holding: %v", err)
	}
	if buyerHolding.Quantity != 10 {
		t.Fatalf("expected buyer 10 shares, got %v", buyerHolding.Quantity)
	}
	var sellerHolding models.PortfolioHoldingRecord
	if err := db.First(&sellerHolding, contract.SellerHoldingID).Error; err != nil {
		t.Fatalf("seller holding: %v", err)
	}
	if sellerHolding.Quantity != 0 || sellerHolding.ReservedQuantity != 0 {
		t.Fatalf("expected seller 0/0, got %v/%v", sellerHolding.Quantity, sellerHolding.ReservedQuantity)
	}

	// I6: contract consumed only because the saga completed.
	var updated models.OtcContractRecord
	if err := db.First(&updated, contract.ID).Error; err != nil {
		t.Fatalf("contract: %v", err)
	}
	if updated.Status != models.OtcContractStatusExercised {
		t.Fatalf("expected contract exercised, got %q", updated.Status)
	}
}

// =====================================================================
// SG-02: pre-saga validation (a-d) — 4xx, no saga log, no side effects
// =====================================================================

func TestSaga_SG02_PreSagaValidation(t *testing.T) {
	t.Run("a_caller_not_buyer", func(t *testing.T) {
		db := newTestDB(t, "saga_sg02a")
		h, _ := setupSagaExerciseHandler(t, db)
		contract := seedExercisableContract(t, db, defaultExerciseSeed())
		// A different client (999) holding a valid trading token.
		rec := exerciseContractHTTP(t, h, contract.ID, clientTradingToken(t, 999), nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
		}
		if n := countSagas(t, db); n != 0 {
			t.Fatalf("expected no saga, got %d", n)
		}
		assertAccount(t, db, 2, 5000, 5000) // buyer untouched
	})

	t.Run("b_contract_missing", func(t *testing.T) {
		db := newTestDB(t, "saga_sg02b")
		h, _ := setupSagaExerciseHandler(t, db)
		seedSagaAccountsSchema(t, db)
		rec := exerciseContractHTTP(t, h, 99999, clientToken(t), nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
		}
		if n := countSagas(t, db); n != 0 {
			t.Fatalf("expected no saga, got %d", n)
		}
	})

	t.Run("c_status_not_valid", func(t *testing.T) {
		db := newTestDB(t, "saga_sg02c")
		h, _ := setupSagaExerciseHandler(t, db)
		s := defaultExerciseSeed()
		s.status = models.OtcContractStatusExercised
		contract := seedExercisableContract(t, db, s)
		rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
		}
		if n := countSagas(t, db); n != 0 {
			t.Fatalf("expected no saga, got %d", n)
		}
		assertAccount(t, db, 2, 5000, 5000)
	})

	t.Run("d_settlement_passed", func(t *testing.T) {
		db := newTestDB(t, "saga_sg02d")
		h, _ := setupSagaExerciseHandler(t, db)
		s := defaultExerciseSeed()
		s.settlement = time.Now().UTC().AddDate(0, 0, -2)
		contract := seedExercisableContract(t, db, s)
		rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
		}
		if n := countSagas(t, db); n != 0 {
			t.Fatalf("expected no saga, got %d", n)
		}
		assertAccount(t, db, 2, 5000, 5000)
	})
}

// =====================================================================
// SG-03: insufficient funds -> F1 fails, no prior steps to compensate
// =====================================================================

func TestSaga_SG03_InsufficientFunds_F1(t *testing.T) {
	db := newTestDB(t, "saga_sg03")
	h, _ := setupSagaExerciseHandler(t, db)
	s := defaultExerciseSeed()
	s.buyerStanje, s.buyerAvail = 500, 500 // cost is 3000
	contract := seedExercisableContract(t, db, s)

	rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	sagaID := decodeSagaID(t, rec)
	if sagaID == 0 {
		t.Fatal("expected sagaId even on failure")
	}

	saga := getSagaHTTP(t, h, sagaID, clientToken(t))
	if saga.Status != models.SagaStatusRolledBack {
		t.Fatalf("expected rolled_back (Compensated), got %q", saga.Status)
	}
	if saga.CurrentStep != 1 {
		t.Fatalf("expected current_step 1, got %d", saga.CurrentStep)
	}
	steps := stepsByNumber(saga)
	if steps[1].Status != models.SagaStepStatusFailed {
		t.Fatalf("step1 expected failed, got %s", steps[1].Status)
	}
	for n := 2; n <= 5; n++ {
		if steps[n].Status != models.SagaStepStatusPending {
			t.Fatalf("step%d expected pending (never attempted), got %s", n, steps[n].Status)
		}
	}
	// Log contains only F1 with an error.
	assertLog(t, saga, []string{"F1 err"})
	// No side effects: balances unchanged.
	assertAccount(t, db, 2, 500, 500)
}

// =====================================================================
// SG-04: insufficient shares -> F2 fails, C1 releases the F1 reservation
// =====================================================================

func TestSaga_SG04_InsufficientShares_F2(t *testing.T) {
	db := newTestDB(t, "saga_sg04")
	h, _ := setupSagaExerciseHandler(t, db)
	s := defaultExerciseSeed()
	s.sellerQty, s.sellerReserved = 3, 3 // contract wants 10
	contract := seedExercisableContract(t, db, s)

	rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	sagaID := decodeSagaID(t, rec)

	saga := getSagaHTTP(t, h, sagaID, clientToken(t))
	if saga.Status != models.SagaStatusRolledBack {
		t.Fatalf("expected rolled_back (Compensated), got %q", saga.Status)
	}
	if saga.CurrentStep != 2 {
		t.Fatalf("expected current_step 2, got %d", saga.CurrentStep)
	}
	steps := stepsByNumber(saga)
	if steps[1].Status != models.SagaStepStatusCompensated {
		t.Fatalf("step1 expected compensated (C1 ran), got %s", steps[1].Status)
	}
	if steps[2].Status != models.SagaStepStatusFailed {
		t.Fatalf("step2 expected failed, got %s", steps[2].Status)
	}
	// Log: F1 ok, F2 err, C1 ok.
	assertLog(t, saga, []string{"F1 ok", "F2 err", "C1 ok"})
	// C1 released F1's reservation: buyer fully restored, no stranded reservation.
	assertAccount(t, db, 2, 5000, 5000)
}

// assertExerciseFullyRolledBack checks the spec invariants common to SG-05/06/07
// (and SG-08): money and shares are conserved back to the seeded pre-exercise
// state, no reservations are stranded (I1/I3), shares are intact (I2), and the
// contract is still valid because the saga did not complete (I6).
func assertExerciseFullyRolledBack(t *testing.T, db *gorm.DB, contract *models.OtcContractRecord) {
	t.Helper()
	assertAccount(t, db, 2, 5000, 5000) // buyer restored
	assertAccount(t, db, 1, 0, 0)       // seller restored

	var sellerHolding models.PortfolioHoldingRecord
	if err := db.First(&sellerHolding, contract.SellerHoldingID).Error; err != nil {
		t.Fatalf("seller holding: %v", err)
	}
	if sellerHolding.Quantity != 10 || sellerHolding.ReservedQuantity != 10 {
		t.Fatalf("seller holding not restored: qty=%v reserved=%v", sellerHolding.Quantity, sellerHolding.ReservedQuantity)
	}

	var buyerHolding models.PortfolioHoldingRecord
	err := db.Where("user_id = ? AND user_type = ? AND asset_id = ?", 100, "client", contract.StockListingID).First(&buyerHolding).Error
	if err == nil && buyerHolding.Quantity != 0 {
		t.Fatalf("buyer should hold no net shares, got %v", buyerHolding.Quantity)
	}

	var updated models.OtcContractRecord
	if err := db.First(&updated, contract.ID).Error; err != nil {
		t.Fatalf("contract: %v", err)
	}
	if updated.Status != models.OtcContractStatusValid {
		t.Fatalf("contract should remain valid (I6), got %q", updated.Status)
	}
}

// =====================================================================
// SG-05/06/07: forced forward-phase failures with full compensation
// =====================================================================

func TestSaga_SG05_06_07_ForcedForwardFailures(t *testing.T) {
	cases := []struct {
		name        string
		forceFail   string // X-Saga-Force-Fail value
		currentStep int
		compensated []int // steps whose compensators must have run
		failedStep  int
		expectedLog []string
	}{
		{"SG05_F3", "F3", 3, []int{1, 2}, 3,
			[]string{"F1 ok", "F2 ok", "F3 err", "C2 ok", "C1 ok"}},
		{"SG06_F4", "F4", 4, []int{1, 2, 3}, 4,
			[]string{"F1 ok", "F2 ok", "F3 ok", "F4 err", "C3 ok", "C2 ok", "C1 ok"}},
		{"SG07_F5", "F5", 5, []int{1, 2, 3, 4}, 5,
			[]string{"F1 ok", "F2 ok", "F3 ok", "F4 ok", "F5 err", "C4 ok", "C3 ok", "C2 ok", "C1 ok"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, "saga_"+tc.name)
			h, _ := setupSagaExerciseHandler(t, db)
			contract := seedExercisableContract(t, db, defaultExerciseSeed())

			rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), map[string]string{
				"X-Saga-Force-Fail": tc.forceFail,
			})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			sagaID := decodeSagaID(t, rec)

			saga := getSagaHTTP(t, h, sagaID, clientToken(t))
			if saga.Status != models.SagaStatusRolledBack {
				t.Fatalf("expected rolled_back (Compensated), got %q", saga.Status)
			}
			if saga.CurrentStep != tc.currentStep {
				t.Fatalf("expected current_step %d, got %d", tc.currentStep, saga.CurrentStep)
			}
			steps := stepsByNumber(saga)
			for _, n := range tc.compensated {
				if steps[n].Status != models.SagaStepStatusCompensated {
					t.Fatalf("step%d expected compensated, got %s", n, steps[n].Status)
				}
			}
			if steps[tc.failedStep].Status != models.SagaStepStatusFailed {
				t.Fatalf("step%d expected failed, got %s", tc.failedStep, steps[tc.failedStep].Status)
			}
			assertLog(t, saga, tc.expectedLog)
			assertExerciseFullyRolledBack(t, db, contract)
		})
	}
}

// =====================================================================
// SG-08: a compensator fails once, then succeeds
// =====================================================================
//
// X-Saga-Compensate-Fail makes C2 fail its first attempt. Bounded inline
// compensation retry re-runs C2, which now succeeds, so the saga converges to
// rolled_back within the single exercise request. The append-only log shows C2
// twice (err then ok), matching the spec's six-entry example exactly:
// F1 ok, F2 ok, F3 err, C2 err, C2 ok, C1 ok (files/SAGA.md).
func TestSaga_SG08_CompensatorFailsThenSucceeds(t *testing.T) {
	db := newTestDB(t, "saga_sg08")
	h, _ := setupSagaExerciseHandler(t, db)
	contract := seedExercisableContract(t, db, defaultExerciseSeed())

	rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), map[string]string{
		"X-Saga-Force-Fail":            "F3",
		"X-Saga-Compensate-Fail":       "C2",
		"X-Saga-Compensate-Fail-Times": "1",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	sagaID := decodeSagaID(t, rec)

	saga := getSagaHTTP(t, h, sagaID, clientToken(t))
	if saga.Status != models.SagaStatusRolledBack {
		t.Fatalf("expected rolled_back after inline compensation retry, got %q", saga.Status)
	}
	if saga.CurrentStep != 3 {
		t.Fatalf("expected current_step 3, got %d", saga.CurrentStep)
	}
	steps := stepsByNumber(saga)
	if steps[1].Status != models.SagaStepStatusCompensated {
		t.Fatalf("step1 (C1) expected compensated, got %s", steps[1].Status)
	}
	if steps[2].Status != models.SagaStepStatusCompensated {
		t.Fatalf("step2 (C2) expected compensated after retry, got %s", steps[2].Status)
	}
	// The spec's six-entry log: C2 appears twice (err then ok).
	assertLog(t, saga, []string{"F1 ok", "F2 ok", "F3 err", "C2 err", "C2 ok", "C1 ok"})
	assertExerciseFullyRolledBack(t, db, contract)
}
