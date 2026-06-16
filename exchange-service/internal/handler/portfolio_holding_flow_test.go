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

// TestPortfolioHTTP_HoldingReadsAndPublic covers getHolding (owner/forbidden/
// not-found), setPublic (isPublic + publicQuantity + bad body + forbidden), and
// the dividend list route.
func TestPortfolioHTTP_HoldingReadsAndPublic(t *testing.T) {
	db := newTestDB(t, "h_portfolio_holding_flow")
	now := time.Now().UTC()
	exch := models.MarketExchangeRecord{Acronym: "PHX", Name: "X", MICCode: "PHX1", Polity: "X", Currency: "USD", Timezone: "UTC", WorkingHours: "09-17"}
	db.Create(&exch)
	listing := models.MarketListingRecord{Ticker: "PHS", Name: "PHS", Type: "stock", ExchangeID: exch.ID, Price: 50, Ask: 51, Bid: 49, Volume: 1, LastRefresh: now}
	db.Create(&listing)
	holding := models.PortfolioHoldingRecord{UserID: 100, UserType: "client", AssetID: listing.ID, Quantity: 10, AvgBuyPrice: 40, AccountID: 1, CreatedAt: now}
	db.Create(&holding)

	cfg := &config.Config{JWTSecret: testJWTSecret}
	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &fakeRates{})
	psvc := service.NewPortfolioService(repository.NewPortfolioRepository(db), taxSvc, repository.NewMarketRepository(db), repository.NewOrderRepository(db))
	divSvc := service.NewDividendService(repository.NewDividendRepository(db), repository.NewOrderRepository(db), taxSvc, &fakeRates{})
	h := NewPortfolioHTTPHandler(cfg, psvc).WithDividendService(divSvc)

	tok := clientToken(t)
	hp := fmt.Sprintf("/api/v1/portfolio/holdings/%d", holding.ID)

	// Owner reads the holding.
	if rec := do(t, h.PortfolioRoutes, http.MethodGet, hp, tok, ""); rec.Code != http.StatusOK {
		t.Errorf("owner getHolding: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Different client is forbidden.
	if rec := do(t, h.PortfolioRoutes, http.MethodGet, hp, client2Token(t), ""); rec.Code != http.StatusForbidden {
		t.Errorf("non-owner getHolding: want 403, got %d", rec.Code)
	}
	// Missing holding.
	if rec := do(t, h.PortfolioRoutes, http.MethodGet, "/api/v1/portfolio/holdings/99999", tok, ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing getHolding: want 404, got %d", rec.Code)
	}
	// Invalid holding id.
	if rec := do(t, h.PortfolioRoutes, http.MethodGet, "/api/v1/portfolio/holdings/abc", tok, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: want 400, got %d", rec.Code)
	}

	// setPublic via isPublic.
	if rec := do(t, h.PortfolioRoutes, http.MethodPut, hp+"/public", tok, `{"isPublic":true}`); rec.Code != http.StatusOK {
		t.Errorf("setPublic isPublic: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// setPublic via publicQuantity.
	if rec := do(t, h.PortfolioRoutes, http.MethodPut, hp+"/public", tok, `{"publicQuantity":3}`); rec.Code != http.StatusOK {
		t.Errorf("setPublic publicQuantity: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Empty body -> 400.
	if rec := do(t, h.PortfolioRoutes, http.MethodPut, hp+"/public", tok, `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("setPublic empty: want 400, got %d", rec.Code)
	}
	// Non-owner setPublic -> 403.
	if rec := do(t, h.PortfolioRoutes, http.MethodPut, hp+"/public", client2Token(t), `{"isPublic":true}`); rec.Code != http.StatusForbidden {
		t.Errorf("non-owner setPublic: want 403, got %d", rec.Code)
	}

	// Dividend history list (empty but exercises the route + handler).
	if rec := do(t, h.PortfolioRoutes, http.MethodGet, "/api/v1/portfolio/dividends", tok, ""); rec.Code != http.StatusOK {
		t.Errorf("listDividends: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
