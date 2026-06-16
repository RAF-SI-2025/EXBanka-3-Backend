package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestTaxCollector_CollectsAndRecordsDebt(t *testing.T) {
	db := openDivTestDB(t, "tax_collect") // currency 1 = RSD
	now := time.Now().UTC()
	period := "2026-04"

	// State treasury (RSD) + a paying client (RSD, funded) + a broke client (RSD).
	db.Exec(`INSERT INTO accounts (id, currency_id, status, naziv, raspolozivo_stanje, stanje) VALUES (1, 1, 'aktivan', 'Republika Srbija', 0, 0)`)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, raspolozivo_stanje, stanje) VALUES (10, 1, 'aktivan', 1, 1000, 1000)`)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, raspolozivo_stanje, stanje) VALUES (11, 1, 'aktivan', 2, 10, 10)`)

	taxRepo := repository.NewTaxRepository(db)
	for _, r := range []*models.TaxRecord{
		{UserID: 1, UserType: "client", AssetID: 1, Period: period, ProfitRSD: 1000, TaxRSD: 150, Status: "unpaid", CreatedAt: now, UpdatedAt: now},
		{UserID: 2, UserType: "client", AssetID: 1, Period: period, ProfitRSD: 2000, TaxRSD: 300, Status: "unpaid", CreatedAt: now, UpdatedAt: now},
	} {
		if err := taxRepo.CreateTaxRecord(r); err != nil {
			t.Fatalf("seed tax: %v", err)
		}
	}

	taxSvc := service.NewTaxService(taxRepo, repository.NewMarketRepository(db), &mockRateProv{})
	collector := service.NewTaxCollector(taxSvc, repository.NewOrderRepository(db), taxRepo)

	res := collector.CollectForPeriod(period)

	// Client 1 pays 150; client 2 (balance 10 < 300) becomes a debt.
	if res.TotalCollected != 150 {
		t.Errorf("expected 150 collected, got %v", res.TotalCollected)
	}
	if len(res.Debts) != 1 || res.Debts[0].UserID != 2 {
		t.Errorf("expected client 2 recorded as a debt, got %+v", res.Debts)
	}
}
