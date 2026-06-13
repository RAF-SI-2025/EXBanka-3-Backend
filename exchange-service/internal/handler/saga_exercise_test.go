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
// Step-by-step narration (visible with `go test -v`)
// =====================================================================
//
// These print a readable account of each scenario — setup, action, the
// resulting saga timeline, and the invariant checks — so a reviewer following
// along on the console sees WHAT every test exercised, not just PASS/ok. Run:
//   go test ./internal/handler/ -run TestSaga -v

// say prints one narration line, attributed to the calling test.
func say(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Logf(format, args...)
}

// specStatusName maps our internal saga status to the spec's status vocabulary
// (files/SAGA.md): in_progress=Running, completed=Completed,
// rolling_back=Compensating, rolled_back=Compensated.
func specStatusName(status string) string {
	switch status {
	case models.SagaStatusInProgress:
		return "Running"
	case models.SagaStatusCompleted:
		return "Completed"
	case models.SagaStatusRollingBack:
		return "Compensating"
	case models.SagaStatusRolledBack:
		return "Compensated"
	default:
		return status
	}
}

// sagaPhaseDesc gives each forward/compensation phase a one-line description so
// the printed timeline reads in plain English.
var sagaPhaseDesc = map[string]string{
	"F1": "reserve buyer funds (qty x strike)",
	"F2": "verify & reserve seller shares",
	"F3": "transfer funds buyer -> seller",
	"F4": "transfer share ownership seller -> buyer",
	"F5": "finalize; contract no longer 'valid'",
	"C1": "release buyer fund reservation",
	"C2": "release seller share reservation",
	"C3": "return funds to buyer",
	"C4": "return shares to seller",
	"C5": "restore contract to 'valid'",
}

// dumpSagaLog prints the saga header and its append-only log as a numbered
// timeline, marking each attempt ok/ERR and echoing the error on failures.
func dumpSagaLog(t *testing.T, saga sagaView) {
	t.Helper()
	t.Logf("RESULT: saga #%d  status=%s (%s)  current_step=%d", saga.ID, saga.Status, specStatusName(saga.Status), saga.CurrentStep)
	t.Logf("        append-only log (%d entries):", len(saga.Log))
	for i, a := range saga.Log {
		desc := sagaPhaseDesc[a.Phase]
		if a.Outcome == models.SagaAttemptOutcomeOK {
			t.Logf("          %2d. %-2s ok    %s", i+1, a.Phase, desc)
		} else {
			t.Logf("          %2d. %-2s ERR   %s  <- %s", i+1, a.Phase, desc, a.Error)
		}
	}
}

// =====================================================================
// SG-01: happy path
// =====================================================================

func TestSaga_SG01_HappyPath(t *testing.T) {
	db := newTestDB(t, "saga_sg01")
	h, _ := setupSagaExerciseHandler(t, db)
	contract := seedExercisableContract(t, db, defaultExerciseSeed())

	say(t, "SG-01 Happy path — exercise a fully-funded OTC option to completion")
	say(t, "SETUP: contract qty=10 strike=300 (cost 3000 USD); buyer acct#2 available=5000; seller acct#1 holds 10 shares reserved")
	say(t, "ACTION: POST /options/%d/exercise  (no fault headers)", contract.ID)

	rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("exercise: status=%d body=%s", rec.Code, rec.Body.String())
	}
	say(t, "        -> HTTP %d", rec.Code)
	sagaID := decodeSagaID(t, rec)
	if sagaID == 0 {
		t.Fatal("expected non-zero sagaId")
	}

	saga := getSagaHTTP(t, h, sagaID, clientToken(t))
	dumpSagaLog(t, saga)
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
	say(t, "ASSERT I4: log has all 5 forward phases in order, every one ok")

	// I1 (money conserved) + balances: buyer 5000-3000=2000, seller 0+3000=3000.
	// I3: no stranded reservation -> buyer raspolozivo == stanje.
	assertAccount(t, db, 2, 2000, 2000) // buyer
	assertAccount(t, db, 1, 3000, 3000) // seller
	say(t, "ASSERT I1: buyer 5000->2000, seller 0->3000 (3000 USD moved, sum conserved); I3: no stranded reservation")

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
	say(t, "ASSERT I2: buyer now holds 10 shares, seller 0/0; I6: contract consumed ('exercised') because the saga Completed")
	say(t, "PASS: happy path settled and the contract was consumed exactly once")
}

// =====================================================================
// SG-02: pre-saga validation (a-d) — 4xx, no saga log, no side effects
// =====================================================================

func TestSaga_SG02_PreSagaValidation(t *testing.T) {
	t.Run("a_caller_not_buyer", func(t *testing.T) {
		db := newTestDB(t, "saga_sg02a")
		h, _ := setupSagaExerciseHandler(t, db)
		contract := seedExercisableContract(t, db, defaultExerciseSeed())
		say(t, "SG-02a Pre-saga validation: caller is NOT the buyer")
		say(t, "ACTION: POST /exercise as client #999 (a stranger to the contract)")
		// A different client (999) holding a valid trading token.
		rec := exerciseContractHTTP(t, h, contract.ID, clientTradingToken(t, 999), nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
		}
		if n := countSagas(t, db); n != 0 {
			t.Fatalf("expected no saga, got %d", n)
		}
		assertAccount(t, db, 2, 5000, 5000) // buyer untouched
		say(t, "RESULT: HTTP %d, 0 sagas created; buyer balance untouched (rejected before any log/side effect)", rec.Code)
	})

	t.Run("b_contract_missing", func(t *testing.T) {
		db := newTestDB(t, "saga_sg02b")
		h, _ := setupSagaExerciseHandler(t, db)
		seedSagaAccountsSchema(t, db)
		say(t, "SG-02b Pre-saga validation: contract id does not exist")
		say(t, "ACTION: POST /options/99999/exercise")
		rec := exerciseContractHTTP(t, h, 99999, clientToken(t), nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
		}
		if n := countSagas(t, db); n != 0 {
			t.Fatalf("expected no saga, got %d", n)
		}
		say(t, "RESULT: HTTP %d, 0 sagas created", rec.Code)
	})

	t.Run("c_status_not_valid", func(t *testing.T) {
		db := newTestDB(t, "saga_sg02c")
		h, _ := setupSagaExerciseHandler(t, db)
		s := defaultExerciseSeed()
		s.status = models.OtcContractStatusExercised
		contract := seedExercisableContract(t, db, s)
		say(t, "SG-02c Pre-saga validation: contract already 'exercised' (not in 'valid' state)")
		say(t, "ACTION: POST /exercise on an already-consumed contract")
		rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
		}
		if n := countSagas(t, db); n != 0 {
			t.Fatalf("expected no saga, got %d", n)
		}
		assertAccount(t, db, 2, 5000, 5000)
		say(t, "RESULT: HTTP %d, 0 sagas created; buyer balance untouched", rec.Code)
	})

	t.Run("d_settlement_passed", func(t *testing.T) {
		db := newTestDB(t, "saga_sg02d")
		h, _ := setupSagaExerciseHandler(t, db)
		s := defaultExerciseSeed()
		s.settlement = time.Now().UTC().AddDate(0, 0, -2)
		contract := seedExercisableContract(t, db, s)
		say(t, "SG-02d Pre-saga validation: settlement date is in the past")
		say(t, "ACTION: POST /exercise on a contract whose settlement window has closed")
		rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
		}
		if n := countSagas(t, db); n != 0 {
			t.Fatalf("expected no saga, got %d", n)
		}
		assertAccount(t, db, 2, 5000, 5000)
		say(t, "RESULT: HTTP %d, 0 sagas created; buyer balance untouched", rec.Code)
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

	say(t, "SG-03 Insufficient funds — F1 fails, nothing to compensate")
	say(t, "SETUP: exercise costs 3000 USD but buyer acct#2 has only 500")
	say(t, "ACTION: POST /exercise  (no fault headers; F1 fails on the balance check)")

	rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	say(t, "        -> HTTP %d", rec.Code)
	sagaID := decodeSagaID(t, rec)
	if sagaID == 0 {
		t.Fatal("expected sagaId even on failure")
	}

	saga := getSagaHTTP(t, h, sagaID, clientToken(t))
	dumpSagaLog(t, saga)
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
	say(t, "ASSERT: Compensated at current_step=1; steps 2-5 never attempted; log = [F1 err]; buyer still 500/500 (no side effects)")
	say(t, "PASS: a first-step failure terminates cleanly with no compensators needed")
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

	say(t, "SG-04 Insufficient shares — F2 fails, C1 releases the F1 reservation")
	say(t, "SETUP: contract wants 10 shares but seller holds only 3")
	say(t, "ACTION: POST /exercise  (F1 reserves funds, then F2 fails on the share check)")

	rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	say(t, "        -> HTTP %d", rec.Code)
	sagaID := decodeSagaID(t, rec)

	saga := getSagaHTTP(t, h, sagaID, clientToken(t))
	dumpSagaLog(t, saga)
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
	say(t, "ASSERT: Compensated at current_step=2; log = [F1 ok, F2 err, C1 ok]; C1 released F1's reservation so buyer back to 5000/5000 (I1/I3)")
	say(t, "PASS: F2 failure rolled back the one prior side effect")
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

			say(t, "%s Forced failure at %s, then full compensation back to step 1", tc.name, tc.forceFail)
			say(t, "SETUP: ready contract (qty=10 strike=300); buyer 5000, seller 10 shares reserved")
			say(t, "ACTION: POST /exercise  with header X-Saga-Force-Fail: %s", tc.forceFail)

			rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), map[string]string{
				"X-Saga-Force-Fail": tc.forceFail,
			})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			say(t, "        -> HTTP %d", rec.Code)
			sagaID := decodeSagaID(t, rec)

			saga := getSagaHTTP(t, h, sagaID, clientToken(t))
			dumpSagaLog(t, saga)
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
			say(t, "ASSERT: Compensated at current_step=%d; compensators for steps %v all ran in reverse; state identical to pre-exercise (I1/I2/I3); contract still 'valid' (I6)", tc.currentStep, tc.compensated)
			say(t, "PASS: %s rolled the saga fully back", tc.forceFail)
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

	say(t, "SG-08 Compensator fails once then succeeds (inline retry heals it in-request)")
	say(t, "SETUP: ready contract (qty=10 strike=300)")
	say(t, "ACTION: POST /exercise  with X-Saga-Force-Fail: F3, X-Saga-Compensate-Fail: C2, X-Saga-Compensate-Fail-Times: 1")

	rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), map[string]string{
		"X-Saga-Force-Fail":            "F3",
		"X-Saga-Compensate-Fail":       "C2",
		"X-Saga-Compensate-Fail-Times": "1",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	say(t, "        -> HTTP %d", rec.Code)
	sagaID := decodeSagaID(t, rec)

	saga := getSagaHTTP(t, h, sagaID, clientToken(t))
	dumpSagaLog(t, saga)
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
	say(t, "ASSERT: log shows C2 twice (err then ok) — the bounded inline retry re-ran it and it succeeded; Compensated within the one request; state restored")
	say(t, "PASS: a transient compensator failure self-heals without the retry cron")
}

// =====================================================================
// SG-09: infrastructure failure at the first phase (parametrized a/b/c)
// =====================================================================
//
// Spec mechanisms: pause the bank service (a), Toxiproxy latency past the RPC
// timeout (b), or a network partition (c) — all so F1 fails with an
// infrastructure error before any side effect. In a split trading/bank
// deployment those are three distinct injectors; in our single-DB monolith F1
// ("reserve buyer funds") is a local transaction, not an RPC to a remote bank,
// so the three mechanisms collapse to the same observable: F1 errors before
// applying side effects. We reproduce that with the F1 force-fail hook and
// assert the spec's expected outcome for every variant — the saga goes straight
// to Compensated (no earlier phase to compensate), current_step=1, the log holds
// only F1 with an error, and no account/holding state changed.
func TestSaga_SG09_InfraFailure_F1(t *testing.T) {
	cases := []struct {
		name     string
		kindNote string // which spec mechanism this variant stands in for
	}{
		{"a_service_paused", "docker compose pause bank -> Unavailable"},
		{"b_latency_over_timeout", "Toxiproxy latency > RPC timeout -> DeadlineExceeded"},
		{"c_network_partition", "Toxiproxy down/bandwidth=0 -> Unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			say(t, "SG-09 %s — infrastructure failure at the first phase (F1)", tc.name)
			say(t, "       models %q; in our single-DB build this collapses to an F1 infra failure", tc.kindNote)
			db := newTestDB(t, "saga_sg09_"+tc.name)
			h, _ := setupSagaExerciseHandler(t, db)
			contract := seedExercisableContract(t, db, defaultExerciseSeed())

			say(t, "SETUP: ready contract; simulate the bank being unreachable at F1")
			say(t, "ACTION: POST /exercise  with X-Saga-Force-Fail: F1 (stands in for the infra fault)")

			rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), map[string]string{
				"X-Saga-Force-Fail": "F1",
			})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			say(t, "        -> HTTP %d", rec.Code)
			sagaID := decodeSagaID(t, rec)
			if sagaID == 0 {
				t.Fatal("expected sagaId even on F1 failure")
			}

			saga := getSagaHTTP(t, h, sagaID, clientToken(t))
			dumpSagaLog(t, saga)
			// Compensated immediately: nothing ran before F1, nothing to compensate.
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
			// Log: only F1 with an error.
			assertLog(t, saga, []string{"F1 err"})
			// No side effects anywhere.
			assertAccount(t, db, 2, 5000, 5000) // buyer untouched
			assertAccount(t, db, 1, 0, 0)       // seller untouched
			var sellerHolding models.PortfolioHoldingRecord
			if err := db.First(&sellerHolding, contract.SellerHoldingID).Error; err != nil {
				t.Fatalf("seller holding: %v", err)
			}
			if sellerHolding.Quantity != 10 || sellerHolding.ReservedQuantity != 10 {
				t.Fatalf("seller holding changed: qty=%v reserved=%v", sellerHolding.Quantity, sellerHolding.ReservedQuantity)
			}
			say(t, "ASSERT: Compensated at current_step=1; steps 2-5 never attempted; log = [F1 err]; buyer/seller balances and holdings all unchanged")
			say(t, "PASS: an infra failure at F1 fails closed — straight to Compensated with zero side effects")
		})
	}
}

// latestSagaSnapshot reads the most recent saga row and its log length without
// failing the test, so it is safe to poll from the test goroutine while an
// exercise request runs concurrently. (SG-10 pins the pool to one connection so
// access is serialized, but the tolerant signature keeps the poll loop simple.)
func latestSagaSnapshot(db *gorm.DB) (status string, currentStep, logLen int, ok bool) {
	var sagas []models.SagaTransactionRecord
	if err := db.Order("id DESC").Limit(1).Find(&sagas).Error; err != nil || len(sagas) == 0 {
		return "", 0, 0, false
	}
	var n int64
	if err := db.Model(&models.SagaAttemptRecord{}).Where("saga_id = ?", sagas[0].ID).Count(&n).Error; err != nil {
		return "", 0, 0, false
	}
	return sagas[0].Status, sagas[0].CurrentStep, int(n), true
}

// =====================================================================
// SG-10: service paused mid-saga, then resumed before compensation
// =====================================================================
//
// Spec: X-Saga-Inject-Delay: F3:5000 opens a window between F2 and F3; during it
// the bank is paused so F3 fails (Unavailable), then the bank is unpaused before
// C1 runs so compensation succeeds. We model the pause-induced F3 failure with
// the F3 force-fail hook and keep the injected delay (shortened) so the window is
// real: the request runs on a background goroutine while we observe the saga
// parked at current_step=3, still Running, with F1/F2 already logged — the
// "paused mid-flight" state — before it converges. Outcome matches the spec:
// Compensated, current_step=3, log F1 ok, F2 ok, F3 err, C2 ok, C1 ok, state
// restored.
func TestSaga_SG10_ServicePausedMidSaga(t *testing.T) {
	db := newTestDB(t, "saga_sg10")
	// Serialize DB access on one connection so the concurrent poll below never
	// races the orchestrator's commits on the shared in-memory database.
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	h, _ := setupSagaExerciseHandler(t, db)
	contract := seedExercisableContract(t, db, defaultExerciseSeed())

	const windowMs = 1000
	token := clientToken(t)

	say(t, "SG-10 Service paused mid-saga, resumed before compensation")
	say(t, "SETUP: ready contract; open a %dms window at F3 (X-Saga-Inject-Delay: F3:%d) then fail F3 (the 'pause')", windowMs, windowMs)
	say(t, "ACTION: POST /exercise on a background goroutine; meanwhile we watch the saga sit mid-flight")

	done := make(chan *httptest.ResponseRecorder, 1)
	start := time.Now()
	go func() {
		done <- exerciseContractHTTP(t, h, contract.ID, token, map[string]string{
			"X-Saga-Inject-Delay": fmt.Sprintf("F3:%d", windowMs),
			"X-Saga-Force-Fail":   "F3",
		})
	}()

	// Observe the open window: F1 and F2 have committed and the orchestrator is
	// parked at F3 (current_step=3, status still Running, two log entries).
	observed := false
	deadline := time.Now().Add(windowMs * time.Millisecond)
	for time.Now().Before(deadline) {
		if status, step, logLen, ok := latestSagaSnapshot(db); ok &&
			step == 3 && status == models.SagaStatusInProgress && logLen == 2 {
			observed = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !observed {
		t.Fatal("never observed the saga parked mid-flight at F3 (Running, current_step=3)")
	}
	say(t, "OBSERVED: mid-flight the saga is Running at current_step=3 with F1+F2 already logged (the 'service paused' window)")

	rec := <-done
	elapsed := time.Since(start)
	if elapsed < windowMs*time.Millisecond {
		t.Fatalf("expected the F3 delay window (%dms) to be honored, took %v", windowMs, elapsed)
	}
	say(t, "        request returned after %v (the injected F3 window was honored) -> HTTP %d", elapsed.Round(time.Millisecond), rec.Code)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	sagaID := decodeSagaID(t, rec)

	saga := getSagaHTTP(t, h, sagaID, token)
	dumpSagaLog(t, saga)
	if saga.Status != models.SagaStatusRolledBack {
		t.Fatalf("expected rolled_back (Compensated), got %q", saga.Status)
	}
	if saga.CurrentStep != 3 {
		t.Fatalf("expected current_step 3, got %d", saga.CurrentStep)
	}
	assertLog(t, saga, []string{"F1 ok", "F2 ok", "F3 err", "C2 ok", "C1 ok"})
	assertExerciseFullyRolledBack(t, db, contract)
	say(t, "ASSERT: after the window F3 failed, then C2 and C1 succeeded (service back); Compensated at current_step=3; state restored")
	say(t, "PASS: a pause that times out F3 still compensates cleanly once the service returns")
}

// =====================================================================
// SG-11: coordinator dies mid-flight; restart resumes from the persisted log
// =====================================================================
//
// Spec: SIGKILL the trading coordinator mid-saga, restart it, and let it read the
// persisted log and drive the saga to a terminal state with no dangling
// reservations. We have no separate process to kill, but our crash-recovery
// mechanism IS that read-the-log-and-resume path: a saga left in rolling_back is
// rediscovered by SagaRetryRunner (the 5-min cron), which rebuilds the steps from
// the persisted payload and finishes compensation.
//
// To stage a half-finished saga we force F3 to fail and force C1 (release buyer
// funds) to fail more times than the inline retry bound, so the request gives up
// with the buyer's reservation still stranded and the saga stuck in Compensating
// — the "coordinator died before C1 completed" snapshot. Then we simulate the
// restart: backdate the saga past the staleness cutoff and run the retry runner,
// which resumes from the log, releases the reservation, and reaches Compensated.
func TestSaga_SG11_CoordinatorCrash_ResumeFromLog(t *testing.T) {
	db := newTestDB(t, "saga_sg11")
	h, _ := setupSagaExerciseHandler(t, db)
	contract := seedExercisableContract(t, db, defaultExerciseSeed())

	say(t, "SG-11 Coordinator dies mid-flight; restart resumes from the persisted log")
	say(t, "SETUP: ready contract; force F3 to fail and force C1 to fail 5x (> the inline retry bound of 3)")
	say(t, "ACTION 1 (crash): POST /exercise  with X-Saga-Force-Fail: F3, X-Saga-Compensate-Fail: C1, X-Saga-Compensate-Fail-Times: 5")

	// Stage the crash: F3 fails, and C1 fails every inline attempt (5 > the
	// orchestrator's inline bound of 3), so compensation cannot finish in-request.
	rec := exerciseContractHTTP(t, h, contract.ID, clientToken(t), map[string]string{
		"X-Saga-Force-Fail":            "F3",
		"X-Saga-Compensate-Fail":       "C1",
		"X-Saga-Compensate-Fail-Times": "5",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	say(t, "        -> HTTP %d", rec.Code)
	sagaID := decodeSagaID(t, rec)

	// Crash snapshot: stuck in Compensating, C2 ran (no-op), C1 never completed,
	// so the buyer's F1 reservation is still held (raspolozivo 5000-3000=2000).
	stuck := getSagaHTTP(t, h, sagaID, clientToken(t))
	say(t, "--- crash snapshot (coordinator died before C1 finished) ---")
	dumpSagaLog(t, stuck)
	if stuck.Status != models.SagaStatusRollingBack {
		t.Fatalf("expected rolling_back (Compensating) after crash, got %q", stuck.Status)
	}
	if stuck.CurrentStep != 3 {
		t.Fatalf("expected current_step 3, got %d", stuck.CurrentStep)
	}
	stuckSteps := stepsByNumber(stuck)
	if stuckSteps[1].Status != models.SagaStepStatusFailed {
		t.Fatalf("step1 (C1) expected failed after exhausting inline retries, got %s", stuckSteps[1].Status)
	}
	if stuckSteps[2].Status != models.SagaStepStatusCompensated {
		t.Fatalf("step2 (C2) expected compensated, got %s", stuckSteps[2].Status)
	}
	assertAccount(t, db, 2, 5000, 2000) // buyer reservation still stranded
	// Three inline C1 failures (maxInlineCompensateAttempts) before giving up.
	assertLog(t, stuck, []string{"F1 ok", "F2 ok", "F3 err", "C2 ok", "C1 err", "C1 err", "C1 err"})
	say(t, "ASSERT crash: stuck in Compensating at current_step=3; C1 errored 3x (inline bound); buyer reservation STRANDED -> acct#2 raspolozivo=2000 (of 5000)")

	// Restart the coordinator: backdate past the staleness cutoff so the retry
	// cron picks the saga up, then run it. RetryCompensations re-reads the log and
	// resumes C1 with no fault (the "service" is healthy again).
	say(t, "ACTION 2 (restart): backdate the saga past the 5-min staleness cutoff and run SagaRetryRunner")
	say(t, "         the runner rebuilds the steps from the persisted payload and resumes compensation from the log")
	if err := db.Model(&models.SagaTransactionRecord{}).
		Where("id = ?", sagaID).
		Update("updated_at", time.Now().UTC().Add(-10*time.Minute)).Error; err != nil {
		t.Fatalf("backdate saga: %v", err)
	}
	runner := service.NewSagaRetryRunner(
		repository.NewSagaRepository(db),
		repository.NewOtcRepository(db),
		service.NewSagaOrchestrator(repository.NewSagaRepository(db), db),
	)
	runner.Run()

	// After restart: terminal Compensated, reservation released, state fully
	// restored, and the log records the recovery C1 that finally succeeded (I4).
	recovered := getSagaHTTP(t, h, sagaID, clientToken(t))
	say(t, "--- after restart (coordinator read the log and resumed) ---")
	dumpSagaLog(t, recovered)
	if recovered.Status != models.SagaStatusRolledBack {
		t.Fatalf("expected rolled_back (Compensated) after restart, got %q", recovered.Status)
	}
	if recoveredSteps := stepsByNumber(recovered); recoveredSteps[1].Status != models.SagaStepStatusCompensated {
		t.Fatalf("step1 (C1) expected compensated after resume, got %s", recoveredSteps[1].Status)
	}
	assertLog(t, recovered, []string{"F1 ok", "F2 ok", "F3 err", "C2 ok", "C1 err", "C1 err", "C1 err", "C1 ok"})
	assertExerciseFullyRolledBack(t, db, contract) // no dangling reservation
	say(t, "ASSERT recovery: log appends the final 'C1 ok' (I4); now Compensated; reservation released -> acct#2 back to 5000/5000; no dangling reservations")
	say(t, "PASS: a mid-flight crash recovers from the persisted log to a clean terminal state")
}
