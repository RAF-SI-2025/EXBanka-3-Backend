package service

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// TestGetHoldingWithPnL drives the not-found path, the account-id backfill path
// (legacy holding with AccountID==0 + a prior buy order), and the already-set
// path — covering GetHoldingWithPnL and ensureBuyAccountID.
func TestGetHoldingWithPnL(t *testing.T) {
	db := openSagaTestDB(t, "holding_pnl")
	rates := cronRateProv{}
	portfolioRepo := repository.NewPortfolioRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	taxRepo := repository.NewTaxRepository(db)
	taxSvc := NewTaxService(taxRepo, marketRepo, rates)
	svc := NewPortfolioService(portfolioRepo, taxSvc, marketRepo, orderRepo)
	now := time.Now().UTC()

	exch := models.MarketExchangeRecord{Acronym: "HPX", Name: "X", MICCode: "HPX1", Polity: "X", Currency: "RSD", Timezone: "UTC", WorkingHours: "09-17"}
	db.Create(&exch)
	listing := models.MarketListingRecord{Ticker: "HPT", Name: "HPT", Type: "stock", ExchangeID: exch.ID, Price: 120, Ask: 120, Bid: 120, Volume: 1, LastRefresh: now}
	db.Create(&listing)

	// Not found.
	if _, err := svc.GetHoldingWithPnL(99999); err == nil {
		t.Error("expected error for missing holding")
	}

	// Legacy holding with no AccountID + a prior buy order on account 7.
	legacy := models.PortfolioHoldingRecord{UserID: 100, UserType: "client", AssetID: listing.ID, Quantity: 10, AvgBuyPrice: 100, AccountID: 0, CreatedAt: now}
	db.Create(&legacy)
	db.Create(&models.OrderRecord{
		UserID: 100, UserType: "client", AssetID: listing.ID, Direction: "buy",
		AccountID: 7, Quantity: 10, Status: "executed", CreatedAt: now,
	})
	got, err := svc.GetHoldingWithPnL(legacy.ID)
	if err != nil {
		t.Fatalf("legacy holding: %v", err)
	}
	if got.CurrentPrice != 120 {
		t.Errorf("CurrentPrice=%v, want 120", got.CurrentPrice)
	}
	var reloaded models.PortfolioHoldingRecord
	db.First(&reloaded, legacy.ID)
	if reloaded.AccountID != 7 {
		t.Errorf("expected AccountID backfilled to 7, got %d", reloaded.AccountID)
	}

	// Holding with AccountID already set — no backfill attempted.
	set := models.PortfolioHoldingRecord{UserID: 200, UserType: "client", AssetID: listing.ID, Quantity: 5, AvgBuyPrice: 90, AccountID: 3, CreatedAt: now}
	db.Create(&set)
	if _, err := svc.GetHoldingWithPnL(set.ID); err != nil {
		t.Fatalf("set holding: %v", err)
	}
}
