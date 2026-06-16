package handler

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// TestFundHTTP_ReadsWithData drives the populated read paths of the fund handler
// (getFund + summariseToJSON, performance, holdings loop, dividends loop).
func TestFundHTTP_ReadsWithData(t *testing.T) {
	db := newFundTestDB(t, "fh_reads_data")
	rates := fundRatesProv{}
	svc := service.NewFundService(
		repository.NewFundRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
		repository.NewOrderRepository(db),
		rates,
	).WithDividendRepo(repository.NewDividendRepository(db))
	h := NewFundHTTPHandler(&config.Config{JWTSecret: testJWTSecret}, svc)

	fund, err := svc.CreateFund(service.CreateFundInput{Naziv: "Reads Fund", MinimalniUlog: 100, ManagerID: 6})
	if err != nil {
		t.Fatalf("CreateFund: %v", err)
	}
	now := time.Now().UTC()

	exch := models.MarketExchangeRecord{Acronym: "FRX", Name: "X", MICCode: "FRX1", Polity: "X", Currency: "RSD", Timezone: "UTC", WorkingHours: "09-17"}
	db.Create(&exch)
	listing := models.MarketListingRecord{Ticker: "FRT", Name: "FRT", Type: "stock", ExchangeID: exch.ID, Price: 50, Ask: 50, Bid: 50, Volume: 10, LastRefresh: now}
	db.Create(&listing)

	db.Create(&models.PortfolioHoldingRecord{UserID: fund.ID, UserType: "fund", AssetID: listing.ID, Quantity: 8, AvgBuyPrice: 40, AccountID: fund.AccountID, CreatedAt: now})
	db.Create(&models.FundPerformanceHistoryRecord{FundID: fund.ID, Date: now, FundValue: 1234})
	db.Create(&models.FundDividendRecord{
		FundID: fund.ID, AssetID: listing.ID, Ticker: "FRT", Period: "2026-Q2",
		Quantity: 8, GrossRSD: 100, Policy: models.FundDividendPolicyReinvest,
		ReinvestedShares: 2, ReinvestedRSD: 100, PaidAt: now,
	})

	tok := clientToken(t)
	base := fmt.Sprintf("/api/v1/funds/%d", fund.ID)

	if rec := do(t, h.FundRoutes, http.MethodGet, base, tok, ""); rec.Code != http.StatusOK {
		t.Errorf("getFund: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h.FundRoutes, http.MethodGet, base+"/performance?granularity=monthly", tok, ""); rec.Code != http.StatusOK {
		t.Errorf("performance: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h.FundRoutes, http.MethodGet, base+"/holdings", tok, ""); rec.Code != http.StatusOK {
		t.Errorf("holdings: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h.FundRoutes, http.MethodGet, base+"/dividends", tok, ""); rec.Code != http.StatusOK {
		t.Errorf("dividends: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h.FundRoutes, http.MethodGet, base+"/statistics", tok, ""); rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Errorf("statistics: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// --- supervisor ops ---
	super := supervisorToken(t)

	// validate-order: supervisor (success or a domain 403), then non-supervisor 403.
	if rec := do(t, h.FundRoutes, http.MethodPost, base+"/validate-order", super, ""); rec.Code != http.StatusOK && rec.Code != http.StatusForbidden {
		t.Errorf("validate-order supervisor: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h.FundRoutes, http.MethodPost, base+"/validate-order", tok, ""); rec.Code != http.StatusForbidden {
		t.Errorf("validate-order client: want 403, got %d", rec.Code)
	}
	if rec := do(t, h.FundRoutes, http.MethodPost, "/api/v1/funds/99999/validate-order", super, ""); rec.Code != http.StatusNotFound {
		t.Errorf("validate-order missing fund: want 404, got %d", rec.Code)
	}

	// dividend-policy: set to payout (success), bad body (400), non-supervisor (403).
	if rec := do(t, h.FundRoutes, http.MethodPut, base+"/dividend-policy", super, `{"policy":"payout"}`); rec.Code != http.StatusOK {
		t.Errorf("set policy: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h.FundRoutes, http.MethodPut, base+"/dividend-policy", super, `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("set policy bad body: want 400, got %d", rec.Code)
	}
	if rec := do(t, h.FundRoutes, http.MethodPut, base+"/dividend-policy", tok, `{"policy":"payout"}`); rec.Code != http.StatusForbidden {
		t.Errorf("set policy client: want 403, got %d", rec.Code)
	}

	// run fund dividends (supervisor).
	if rec := do(t, h.FundRoutes, http.MethodPost, "/api/v1/funds/dividends/run", super, ""); rec.Code != http.StatusOK {
		t.Errorf("run fund dividends: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
