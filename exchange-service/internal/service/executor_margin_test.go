package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// TestOrderExecutor_Run_SellRepaysMarginLoan covers settleMarginLoansFromProceeds:
// a sell fill on an asset the user holds a margin loan against repays the loan
// (FIFO) from the proceeds and credits the bank.
func TestOrderExecutor_Run_SellRepaysMarginLoan(t *testing.T) {
	db := openDivTestDB(t, "oe_margin_sell")
	assetID := seedAsset(t, db, "MGS", 50, "USD")
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, raspolozivo_stanje, stanje) VALUES (10, 2, 'aktivan', 1, 100000, 100000)`)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, firma_id, raspolozivo_stanje, stanje) VALUES (20, 2, 'aktivan', 1, 100000, 100000)`)

	orderRepo := repository.NewOrderRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	rates := &mockRateProv{rates: map[string]float64{"USD:RSD": 100}}
	portfolioSvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db),
		service.NewTaxService(repository.NewTaxRepository(db), marketRepo, rates),
		marketRepo, orderRepo,
	)
	exec := service.NewOrderExecutor(orderRepo, marketRepo, portfolioSvc, rates)

	now := time.Now().UTC().Add(-time.Hour)
	// An outstanding margin BUY loan on the asset.
	if err := orderRepo.CreateOrder(&models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "market", Direction: "buy",
		Quantity: 10, ContractSize: 1, PricePerUnit: 50, CurrencyRate: 1, IsMargin: true, MarginLoan: 500,
		Status: "done", IsDone: true, RemainingPortions: 0, AccountID: 10, LastModification: now, CreatedAt: now,
	}); err != nil {
		t.Fatalf("seed margin buy: %v", err)
	}
	// A holding to sell from.
	if err := db.Create(&models.PortfolioHoldingRecord{
		UserID: 1, UserType: "client", AssetID: assetID, Quantity: 10, AvgBuyPrice: 50,
		AccountID: 10, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed holding: %v", err)
	}
	// An approved sell order — its fill proceeds repay the loan.
	sell := &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "market", Direction: "sell",
		Quantity: 6, ContractSize: 1, PricePerUnit: 49, CurrencyRate: 1, Commission: 3,
		Status: "approved", RemainingPortions: 6, AccountID: 10, LastModification: now, CreatedAt: now,
	}
	if err := orderRepo.CreateOrder(sell); err != nil {
		t.Fatalf("seed sell: %v", err)
	}

	exec.Run()

	// The margin loan should have been (at least partially) repaid from proceeds.
	var loanOrder models.OrderRecord
	db.Where("asset_id = ? AND is_margin = ?", assetID, true).First(&loanOrder)
	if loanOrder.MarginLoan >= 500 {
		t.Errorf("expected margin loan to be reduced from 500, got %v", loanOrder.MarginLoan)
	}
}
