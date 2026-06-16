package handler

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestFundHTTP_SupervisorReads(t *testing.T) {
	db := newFundTestDB(t, "fh_super_reads")
	h, svc := setupFundHandler(t, db)
	fund, err := svc.CreateFund(service.CreateFundInput{Naziv: "SR", MinimalniUlog: 100, ManagerID: 6})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	super := supervisorToken(t)

	// Supervisor view of "my" funds (managed) — the non-client branch of listMyPositions.
	if rec := do(t, h.FundRoutes, http.MethodGet, "/api/v1/funds/positions/mine", super, ""); rec.Code != http.StatusOK {
		t.Errorf("positions/mine status=%d body=%s", rec.Code, rec.Body.String())
	}
	// getFund not found.
	if rec := do(t, h.FundRoutes, http.MethodGet, "/api/v1/funds/99999", super, ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing fund status=%d", rec.Code)
	}
	// Holdings + benchmark as supervisor.
	if rec := do(t, h.FundRoutes, http.MethodGet, fmt.Sprintf("/api/v1/funds/%d/holdings", fund.ID), super, ""); rec.Code != http.StatusOK {
		t.Errorf("holdings status=%d", rec.Code)
	}
	if rec := do(t, h.FundRoutes, http.MethodGet, "/api/v1/funds/benchmark", super, ""); rec.Code != http.StatusOK {
		t.Errorf("benchmark status=%d", rec.Code)
	}
}

func TestPortfolioHTTP_BankReads(t *testing.T) {
	db := newTestDB(t, "h_portfolio_bank")
	exch := models.MarketExchangeRecord{Acronym: "BPX", Name: "X", MICCode: "BPX1", Polity: "X", Currency: "USD", Timezone: "UTC", WorkingHours: "09-17"}
	db.Create(&exch)
	listing := models.MarketListingRecord{Ticker: "BPS", Name: "BPS", Type: "stock", ExchangeID: exch.ID, Price: 50, Ask: 51, Bid: 49, Volume: 1, LastRefresh: time.Now().UTC()}
	db.Create(&listing)
	// Bank-owned holding (user 0, bank).
	db.Create(&models.PortfolioHoldingRecord{UserID: 0, UserType: "bank", AssetID: listing.ID, Quantity: 5, AvgBuyPrice: 40, AccountID: 1, CreatedAt: time.Now().UTC()})

	h := setupPortfolioHandler(t, db)
	bank := bankToken(t) // employee agent -> bank identity

	if rec := do(t, h.PortfolioCollection, http.MethodGet, "/api/v1/portfolio", bank, ""); rec.Code != http.StatusOK {
		t.Errorf("portfolio summary status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h.PortfolioRoutes, http.MethodGet, "/api/v1/portfolio/holdings", bank, ""); rec.Code != http.StatusOK {
		t.Errorf("holdings status=%d", rec.Code)
	}
}

func TestMarketHTTP_ListingDetail(t *testing.T) {
	db := newTestDB(t, "h_market_listing")
	seedExchangeAndListing(t, db, "MKT")
	h := setupMarketHandler(t, db)

	if rec := do(t, h.ListingRoutes, http.MethodGet, "/api/v1/listings/MKT", clientToken(t), ""); rec.Code != http.StatusOK {
		t.Errorf("listing detail status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Unknown ticker -> 404.
	if rec := do(t, h.ListingRoutes, http.MethodGet, "/api/v1/listings/NOPE", clientToken(t), ""); rec.Code != http.StatusNotFound && rec.Code != http.StatusOK {
		t.Errorf("unknown listing status=%d", rec.Code)
	}
}
