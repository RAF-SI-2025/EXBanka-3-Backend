package handler

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestFundHTTP_DividendPolicyAndStats(t *testing.T) {
	db := newFundTestDB(t, "fh_div_policy")
	h, svc := setupFundHandler(t, db)
	// Manager = supervisor (employee 6, supervisorToken).
	fund, err := svc.CreateFund(service.CreateFundInput{Naziv: "DP", MinimalniUlog: 100, ManagerID: 6})
	if err != nil {
		t.Fatalf("create fund: %v", err)
	}
	policy := fmt.Sprintf("/api/v1/funds/%d/dividend-policy", fund.ID)

	// Supervisor-manager sets the policy.
	if rec := do(t, h.FundRoutes, http.MethodPut, policy, supervisorToken(t), `{"policy":"payout"}`); rec.Code != http.StatusOK {
		t.Errorf("set policy status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Invalid policy value.
	if rec := do(t, h.FundRoutes, http.MethodPut, policy, supervisorToken(t), `{"policy":"bogus"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid policy status=%d", rec.Code)
	}
	// Client cannot set policy.
	if rec := do(t, h.FundRoutes, http.MethodPut, policy, clientToken(t), `{"policy":"payout"}`); rec.Code != http.StatusForbidden {
		t.Errorf("client policy status=%d", rec.Code)
	}
	// Statistics on a missing fund -> 404.
	if rec := do(t, h.FundRoutes, http.MethodGet, "/api/v1/funds/99999/statistics", clientToken(t), ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing-fund stats status=%d", rec.Code)
	}
	// Dividend history list.
	if rec := do(t, h.FundRoutes, http.MethodGet, fmt.Sprintf("/api/v1/funds/%d/dividends", fund.ID), clientToken(t), ""); rec.Code != http.StatusOK {
		t.Errorf("dividends list status=%d", rec.Code)
	}
}

func TestFundHTTP_RunDividends(t *testing.T) {
	db := newFundTestDB(t, "fh_run_dividends")
	cfg := &config.Config{JWTSecret: testJWTSecret}
	rates := fundRatesProv{}
	svc := service.NewFundService(
		repository.NewFundRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
		repository.NewOrderRepository(db),
		rates,
	).WithDividendRepo(repository.NewDividendRepository(db))
	h := NewFundHTTPHandler(cfg, svc)

	// Client cannot trigger.
	if rec := do(t, h.FundRoutes, http.MethodPost, "/api/v1/funds/dividends/run", clientToken(t), ""); rec.Code != http.StatusForbidden {
		t.Errorf("client run status=%d", rec.Code)
	}
	// Supervisor can: no fund holdings -> 0 processed, 200.
	if rec := do(t, h.FundRoutes, http.MethodPost, "/api/v1/funds/dividends/run", supervisorToken(t), ""); rec.Code != http.StatusOK {
		t.Errorf("supervisor run status=%d body=%s", rec.Code, rec.Body.String())
	}
}
