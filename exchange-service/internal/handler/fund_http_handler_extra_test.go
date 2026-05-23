package handler

import (
	"bytes"
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
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
	"gorm.io/gorm"
)

// newFundTestDB returns a DB with the exchange-service migrations PLUS the
// reference accounts/currencies tables that fund operations read.
func newFundTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db := newTestDB(t, name)
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS currencies (
		id INTEGER PRIMARY KEY AUTOINCREMENT, kod TEXT, naziv TEXT, simbol TEXT, drzava TEXT,
		aktivan BOOLEAN, created_at DATETIME, updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("currencies: %v", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		broj_racuna TEXT, currency_id INTEGER, tip TEXT, vrsta TEXT, podvrsta TEXT,
		stanje REAL DEFAULT 0, raspolozivo_stanje REAL DEFAULT 0,
		dnevni_limit REAL, mesecni_limit REAL,
		dnevna_potrosnja REAL DEFAULT 0, mesecna_potrosnja REAL DEFAULT 0,
		datum_isteka DATETIME, odrzavanje_racuna REAL DEFAULT 0,
		naziv TEXT, status TEXT,
		created_at DATETIME, updated_at DATETIME,
		client_id INTEGER, firma_id INTEGER, zaposleni_id INTEGER
	)`).Error; err != nil {
		t.Fatalf("accounts: %v", err)
	}
	// Fund account ownership checks LEFT JOIN firmas to read is_state, so the
	// table must exist even when empty (SQLite errors on a missing table).
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS firmas (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		is_state BOOLEAN DEFAULT false,
		created_at DATETIME, updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("firmas: %v", err)
	}
	return db
}

type fundRatesProv struct{}

func (fundRatesProv) GetRate(from, to string) (float64, error) {
	if from == to {
		return 1, nil
	}
	return 110, nil
}
func (fundRatesProv) GetAllRates() []service.ExchangeRate { return nil }

func setupFundHandler(t *testing.T, db *gorm.DB) (*FundHTTPHandler, *service.FundService) {
	cfg := &config.Config{JWTSecret: testJWTSecret}
	rates := fundRatesProv{}
	svc := service.NewFundService(
		repository.NewFundRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
		repository.NewOrderRepository(db),
		rates,
	)
	return NewFundHTTPHandler(cfg, svc), svc
}

func TestFundHTTP_ListFunds_Empty(t *testing.T) {
	db := newFundTestDB(t, "fh_list_empty")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/funds", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFundHTTP_ListFunds_Unauthorized(t *testing.T) {
	db := newFundTestDB(t, "fh_list_unauth")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/funds", nil)
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestFundHTTP_CreateFund_RequiresSupervisor(t *testing.T) {
	db := newFundTestDB(t, "fh_create_role")
	h, _ := setupFundHandler(t, db)
	body, _ := json.Marshal(map[string]interface{}{"naziv": "X", "minimalniUlog": 1000})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/funds", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected forbidden/unauthorized, got %d", rec.Code)
	}
}

func TestFundHTTP_CreateFund_AsSupervisor_BadBody(t *testing.T) {
	db := newFundTestDB(t, "fh_create_badbody")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/funds", bytes.NewBufferString("not-json"))
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFundHTTP_CreateFund_AsSupervisor_OK(t *testing.T) {
	db := newFundTestDB(t, "fh_create_ok")
	h, _ := setupFundHandler(t, db)
	body, _ := json.Marshal(map[string]interface{}{"naziv": "Alpha", "minimalniUlog": 1000})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/funds", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFundHTTP_GetFund_NotFound(t *testing.T) {
	db := newFundTestDB(t, "fh_get_notfound")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/funds/9999", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestFundHTTP_GetFund_OK(t *testing.T) {
	db := newFundTestDB(t, "fh_get_ok")
	h, svc := setupFundHandler(t, db)
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "Beta", MinimalniUlog: 1000, ManagerID: 6})
	url := fmt.Sprintf("/api/v1/funds/%d", fund.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFundHTTP_GetPerformance_BadGranularity(t *testing.T) {
	db := newFundTestDB(t, "fh_perf_badgran")
	h, svc := setupFundHandler(t, db)
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "C", MinimalniUlog: 1000, ManagerID: 6})
	url := fmt.Sprintf("/api/v1/funds/%d/performance?granularity=blah", fund.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFundHTTP_ListHoldings(t *testing.T) {
	db := newFundTestDB(t, "fh_holdings")
	h, svc := setupFundHandler(t, db)
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "D", MinimalniUlog: 1000, ManagerID: 6})
	url := fmt.Sprintf("/api/v1/funds/%d/holdings", fund.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFundHTTP_InvestInFund_Validation(t *testing.T) {
	db := newFundTestDB(t, "fh_invest_val")
	h, svc := setupFundHandler(t, db)
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "E", MinimalniUlog: 1000, ManagerID: 6})

	// Empty body → 400
	url := fmt.Sprintf("/api/v1/funds/%d/invest", fund.ID)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString("not-json"))
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Valid body but no source account → service error → 400
	body, _ := json.Marshal(map[string]interface{}{"amount": 500, "sourceAccountId": 99999})
	req2 := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req2.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec2 := httptest.NewRecorder()
	h.FundRoutes(rec2, req2)
	if rec2.Code == http.StatusOK {
		t.Fatalf("expected non-2xx for missing source, got %d", rec2.Code)
	}
}

func TestFundHTTP_WithdrawFromFund_Validation(t *testing.T) {
	db := newFundTestDB(t, "fh_withdraw_val")
	h, svc := setupFundHandler(t, db)
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "F", MinimalniUlog: 1000, ManagerID: 6})
	url := fmt.Sprintf("/api/v1/funds/%d/withdraw", fund.ID)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString("not-json"))
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFundHTTP_ListMyPositions_Empty(t *testing.T) {
	db := newFundTestDB(t, "fh_positions_empty")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/funds/positions/mine", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFundHTTP_ValidateOrder_OK(t *testing.T) {
	db := newFundTestDB(t, "fh_validate_ok")
	h, svc := setupFundHandler(t, db)
	// Token claims employee ID 6 — match that to manager ID.
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "G", MinimalniUlog: 1000, ManagerID: 6})
	url := fmt.Sprintf("/api/v1/funds/%d/validate-order", fund.ID)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(""))
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFundHTTP_ValidateOrder_NotManager(t *testing.T) {
	db := newFundTestDB(t, "fh_validate_notmgr")
	h, svc := setupFundHandler(t, db)
	// Fund managed by 99 — supervisor token (employee 6) is not the manager.
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "H", MinimalniUlog: 1000, ManagerID: 99})
	url := fmt.Sprintf("/api/v1/funds/%d/validate-order", fund.ID)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(""))
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestFundHTTP_Routes_InvalidPath_ReturnsBadRequestOrNotFound(t *testing.T) {
	db := newFundTestDB(t, "fh_routes_invalid")
	h, _ := setupFundHandler(t, db)
	// Invalid fund id
	req := httptest.NewRequest(http.MethodGet, "/api/v1/funds/abc", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Unknown sub-route
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/funds/1/unknown", nil)
	req2.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec2 := httptest.NewRecorder()
	h.FundRoutes(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec2.Code)
	}

	// Method-not-allowed on root
	req3 := httptest.NewRequest(http.MethodDelete, "/api/v1/funds", nil)
	req3.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec3 := httptest.NewRecorder()
	h.FundRoutes(rec3, req3)
	if rec3.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec3.Code)
	}
}

func TestFundHTTP_HelperFunctions(t *testing.T) {
	// fundParticipantIdentity covers client/employee/bank cases
	clientClaims := &util.Claims{ClientID: 42, TokenSource: "client", Permissions: []string{models.PermClientTrading}}
	id, typ, err := fundParticipantIdentity(clientClaims, false)
	if err != nil || id != 42 || typ != "client" {
		t.Fatalf("client identity: %v %v %v", id, typ, err)
	}

	// Client without trading permission → error
	if _, _, err := fundParticipantIdentity(&util.Claims{ClientID: 42, TokenSource: "client"}, false); err == nil {
		t.Fatal("expected canTrade error")
	}

	supClaims := &util.Claims{EmployeeID: 7, TokenSource: "employee", Permissions: []string{models.PermEmployeeSupervisor}}
	_, typ, err = fundParticipantIdentity(supClaims, true)
	if err != nil || typ != "bank" {
		t.Fatalf("bank identity: %v %v", typ, err)
	}

	// Non-employee asBank → error
	if _, _, err := fundParticipantIdentity(&util.Claims{TokenSource: "client"}, true); err == nil {
		t.Fatal("expected employee-required error")
	}
	// Employee without supervisor perm asBank → error
	if _, _, err := fundParticipantIdentity(&util.Claims{TokenSource: "employee"}, true); err == nil {
		t.Fatal("expected supervisor-required error")
	}

	// summariseToJSON
	now := time.Now().UTC()
	summary := &service.FundSummary{
		Fund:              &models.InvestmentFundRecord{ID: 1, Naziv: "N", DatumKreiranja: now},
		FundValueRSD:      1000,
		LiquidCashRSD:     500,
		HoldingsValueRSD:  500,
		TotalInvestedRSD:  800,
		ProfitRSD:         200,
		ManagerID:         5,
		ParticipantsCount: 3,
	}
	out := summariseToJSON(summary)
	if out["id"].(uint) != 1 {
		t.Fatalf("expected id=1, got %+v", out["id"])
	}
}
