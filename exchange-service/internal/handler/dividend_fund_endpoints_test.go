package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"gorm.io/gorm"
)

func do(t *testing.T, h http.HandlerFunc, method, target, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h(rec, r)
	return rec
}

// --- Portfolio dividend endpoints ---

func setupPortfolioWithDividends(t *testing.T, db *gorm.DB) *PortfolioHTTPHandler {
	t.Helper()
	cfg := &config.Config{JWTSecret: testJWTSecret}
	rates := fundRatesProv{}
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db),
		service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), rates),
		repository.NewMarketRepository(db),
		repository.NewOrderRepository(db),
	)
	divSvc := service.NewDividendService(
		repository.NewDividendRepository(db),
		repository.NewOrderRepository(db),
		service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), rates),
		rates,
	)
	return NewPortfolioHTTPHandler(cfg, psvc).WithDividendService(divSvc)
}

func TestPortfolioDividends_ListEmpty(t *testing.T) {
	db := newFundTestDB(t, "pf_div_list")
	h := setupPortfolioWithDividends(t, db)
	rec := do(t, h.PortfolioRoutes, http.MethodGet, "/api/v1/portfolio/dividends", clientToken(t), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Count != 0 {
		t.Errorf("expected 0 dividends, got %d", resp.Count)
	}
}

func TestPortfolioDividends_Unauthorized(t *testing.T) {
	db := newFundTestDB(t, "pf_div_unauth")
	h := setupPortfolioWithDividends(t, db)
	rec := do(t, h.PortfolioRoutes, http.MethodGet, "/api/v1/portfolio/dividends", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestPortfolioDividends_RunRequiresSupervisor(t *testing.T) {
	db := newFundTestDB(t, "pf_div_run")
	h := setupPortfolioWithDividends(t, db)
	// Client cannot trigger.
	if rec := do(t, h.PortfolioRoutes, http.MethodPost, "/api/v1/portfolio/dividends/run", clientToken(t), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for client, got %d", rec.Code)
	}
	// Supervisor can: no holdings -> 0 paid, 200.
	rec := do(t, h.PortfolioRoutes, http.MethodPost, "/api/v1/portfolio/dividends/run", supervisorToken(t), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for supervisor, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// --- Fund statistics / dividends / policy / benchmark / run ---

func setupFundWithDividends(t *testing.T, db *gorm.DB) *FundHTTPHandler {
	t.Helper()
	cfg := &config.Config{JWTSecret: testJWTSecret}
	rates := fundRatesProv{}
	svc := service.NewFundService(
		repository.NewFundRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
		repository.NewOrderRepository(db),
		rates,
	).WithDividendRepo(repository.NewDividendRepository(db))
	return NewFundHTTPHandler(cfg, svc)
}

// createFundViaAPI creates a fund as the supervisor (EmployeeID 6 -> manager 6).
func createFundViaAPI(t *testing.T, h *FundHTTPHandler) uint {
	t.Helper()
	rec := do(t, h.FundRoutes, http.MethodPost, "/api/v1/funds", supervisorToken(t), `{"naziv":"F1","opis":"d","minimalniUlog":1000}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create fund: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Fund struct {
			ID uint `json:"id"`
		} `json:"fund"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode fund: %v", err)
	}
	return resp.Fund.ID
}

func TestFundStatistics_InsufficientData(t *testing.T) {
	db := newFundTestDB(t, "fh_stats")
	h := setupFundWithDividends(t, db)
	id := createFundViaAPI(t, h)
	rec := do(t, h.FundRoutes, http.MethodGet, "/api/v1/funds/"+itoa(id)+"/statistics", clientToken(t), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Statistics struct {
			Available bool `json:"available"`
		} `json:"statistics"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Statistics.Available {
		t.Errorf("expected available=false for a fund with no snapshots")
	}
}

func TestFundDividends_ListAndBenchmark(t *testing.T) {
	db := newFundTestDB(t, "fh_fdiv")
	h := setupFundWithDividends(t, db)
	id := createFundViaAPI(t, h)

	if rec := do(t, h.FundRoutes, http.MethodGet, "/api/v1/funds/"+itoa(id)+"/dividends", clientToken(t), ""); rec.Code != http.StatusOK {
		t.Fatalf("dividends: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h.FundRoutes, http.MethodGet, "/api/v1/funds/benchmark", clientToken(t), ""); rec.Code != http.StatusOK {
		t.Fatalf("benchmark: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFundDividendPolicy_Guards(t *testing.T) {
	db := newFundTestDB(t, "fh_policy")
	h := setupFundWithDividends(t, db)
	id := createFundViaAPI(t, h)
	path := "/api/v1/funds/" + itoa(id) + "/dividend-policy"

	// Client cannot set policy.
	if rec := do(t, h.FundRoutes, http.MethodPut, path, clientToken(t), `{"policy":"payout"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for client, got %d", rec.Code)
	}
	// Invalid policy value.
	if rec := do(t, h.FundRoutes, http.MethodPut, path, supervisorToken(t), `{"policy":"bogus"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid policy, got %d", rec.Code)
	}
	// Supervisor-manager sets it.
	rec := do(t, h.FundRoutes, http.MethodPut, path, supervisorToken(t), `{"policy":"payout"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		DividendPolicy string `json:"dividendPolicy"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.DividendPolicy != "payout" {
		t.Errorf("expected payout, got %q", resp.DividendPolicy)
	}
}

func TestFundDividends_RunRequiresSupervisor(t *testing.T) {
	db := newFundTestDB(t, "fh_run")
	h := setupFundWithDividends(t, db)
	if rec := do(t, h.FundRoutes, http.MethodPost, "/api/v1/funds/dividends/run", clientToken(t), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for client, got %d", rec.Code)
	}
	rec := do(t, h.FundRoutes, http.MethodPost, "/api/v1/funds/dividends/run", supervisorToken(t), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for supervisor, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// --- OTC negotiation endpoints ---

func TestOtcNegotiations_ListAndHistory(t *testing.T) {
	db := newTestDB(t, "otc_neg_http")
	h := setupOtcHandler(t, db)
	// Empty negotiations list -> 200.
	if rec := do(t, h.OtcRoutes, http.MethodGet, "/api/v1/otc/negotiations", clientToken(t), ""); rec.Code != http.StatusOK {
		t.Fatalf("negotiations list: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// History for a nonexistent offer -> 404.
	if rec := do(t, h.OtcRoutes, http.MethodGet, "/api/v1/otc/offers/9999/history", clientToken(t), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown offer history, got %d", rec.Code)
	}
	// Unauthenticated -> 401.
	if rec := do(t, h.OtcRoutes, http.MethodGet, "/api/v1/otc/negotiations", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func itoa(v uint) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
