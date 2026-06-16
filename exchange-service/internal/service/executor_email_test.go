package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestSMTPEmailService_SendError(t *testing.T) {
	// A dead SMTP endpoint makes SendMail fail fast, covering the error path.
	svc := service.NewSMTPEmailService("127.0.0.1", 1, "from@bank.com")
	if err := svc.Send("to@bank.com", "subj", "body"); err == nil {
		t.Error("expected SMTP send error to a dead endpoint")
	}
}

func TestOrderExecutor_Run_FillsBuyAndSell(t *testing.T) {
	db := openDivTestDB(t, "oe_run")
	assetID := seedAsset(t, db, "EXE", 50, "USD") // price 50, ask 51, bid 49
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

	now := time.Now().UTC().Add(-time.Hour) // old enough to dodge after-hours delay
	// Approved market BUY with a high committed price -> fill (at ask) below it
	// triggers the over-reservation refund branch.
	buy := &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "market", Direction: "buy",
		Quantity: 2, ContractSize: 1, PricePerUnit: 60, CurrencyRate: 1, Commission: 4,
		Status: "approved", RemainingPortions: 2, AccountID: 10,
		LastModification: now, CreatedAt: now,
	}
	if err := orderRepo.CreateOrder(buy); err != nil {
		t.Fatalf("seed buy: %v", err)
	}

	// A holding so the SELL order can execute and credit proceeds + commission.
	if err := db.Create(&models.PortfolioHoldingRecord{
		UserID: 1, UserType: "client", AssetID: assetID, Quantity: 10, AvgBuyPrice: 40,
		AccountID: 10, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed holding: %v", err)
	}
	sell := &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "market", Direction: "sell",
		Quantity: 2, ContractSize: 1, PricePerUnit: 49, CurrencyRate: 1, Commission: 2,
		Status: "approved", RemainingPortions: 2, AccountID: 10,
		LastModification: now, CreatedAt: now,
	}
	if err := orderRepo.CreateOrder(sell); err != nil {
		t.Fatalf("seed sell: %v", err)
	}

	exec.Run()

	// Both orders should have recorded at least one transaction (market fills now).
	for _, id := range []uint{buy.ID, sell.ID} {
		txs, err := orderRepo.ListTransactionsForOrder(id)
		if err != nil {
			t.Fatalf("ListTransactionsForOrder(%d): %v", id, err)
		}
		if len(txs) == 0 {
			t.Errorf("order %d should have a fill transaction after Run", id)
		}
	}
}
