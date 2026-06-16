package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// TestOrderExecutor_Run_AfterHoursAONLimit exercises additional Run branches:
// the after-hours fill delay, AON fill quantity, and a limit buy filling below
// its committed price (over-reservation refund).
func TestOrderExecutor_Run_AfterHoursAONLimit(t *testing.T) {
	db := openDivTestDB(t, "oe_extra")
	assetID := seedAsset(t, db, "EXX", 50, "USD") // ask 51
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, raspolozivo_stanje, stanje) VALUES (10, 2, 'aktivan', 1, 1000000, 1000000)`)

	orderRepo := repository.NewOrderRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	rates := &mockRateProv{rates: map[string]float64{"USD:RSD": 100}}
	psvc := service.NewPortfolioService(repository.NewPortfolioRepository(db),
		service.NewTaxService(repository.NewTaxRepository(db), marketRepo, rates), marketRepo, orderRepo)
	exec := service.NewOrderExecutor(orderRepo, marketRepo, psvc, rates)

	old := time.Now().UTC().Add(-time.Hour)
	recent := time.Now().UTC()
	limit := 60.0

	// After-hours order modified just now -> blocked by the 30-min fill delay.
	if err := orderRepo.CreateOrder(&models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "market", Direction: "buy",
		Quantity: 2, ContractSize: 1, PricePerUnit: 60, CurrencyRate: 1, AfterHours: true,
		Status: "approved", RemainingPortions: 2, AccountID: 10, LastModification: recent, CreatedAt: old,
	}); err != nil {
		t.Fatalf("seed after-hours: %v", err)
	}
	// AON limit buy that fills (at ask 51 < limit 60 -> over-reservation refund).
	if err := orderRepo.CreateOrder(&models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "limit", Direction: "buy",
		Quantity: 2, ContractSize: 1, PricePerUnit: 60, LimitValue: &limit, CurrencyRate: 1, IsAON: true,
		Status: "approved", RemainingPortions: 2, AccountID: 10, LastModification: old, CreatedAt: old,
	}); err != nil {
		t.Fatalf("seed AON limit: %v", err)
	}

	exec.Run()

	// The AON limit order should have filled fully (its 2 units in one go).
	var aon models.OrderRecord
	db.Where("asset_id = ? AND order_type = ?", assetID, "limit").First(&aon)
	if aon.RemainingPortions != 0 {
		t.Errorf("AON order remaining=%d, want 0", aon.RemainingPortions)
	}
}
