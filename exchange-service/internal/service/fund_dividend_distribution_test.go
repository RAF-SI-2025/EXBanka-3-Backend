package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"gorm.io/gorm"
)

func newFundSvc(db *gorm.DB) *service.FundService {
	rates := &mockRateProv{rates: map[string]float64{"USD:RSD": 100}}
	return service.NewFundService(
		repository.NewFundRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
		repository.NewOrderRepository(db),
		rates,
	).WithDividendRepo(repository.NewDividendRepository(db))
}

func holdingQty(t *testing.T, db *gorm.DB, fundID, assetID uint) float64 {
	t.Helper()
	var qty float64
	db.Table("portfolio_holdings").Select("quantity").
		Where("user_id = ? AND user_type = 'fund' AND asset_id = ?", fundID, assetID).Scan(&qty)
	return qty
}

func TestFundService_DistributeFundDividends_Reinvest(t *testing.T) {
	db := openDivTestDB(t, "fund_div_reinvest")
	svc := newFundSvc(db)

	fund, err := svc.CreateFund(service.CreateFundInput{Naziv: "Reinvest Fund", Opis: "x", MinimalniUlog: 1000, ManagerID: 6})
	if err != nil {
		t.Fatalf("CreateFund: %v", err)
	}
	// 400 shares @ 50 USD, 4% yield. gross = 400*50*0.01 = 200 USD -> 20000 RSD.
	// priceRSD = 5000 -> 4 whole shares reinvested.
	assetID := seedDivStock(t, db, "FRE", "USD", 50, 0.04)
	seedHolding(t, db, fund.ID, "fund", assetID, 400, fund.AccountID)

	res, err := svc.DistributeFundDividends(time.Now().UTC())
	if err != nil {
		t.Fatalf("DistributeFundDividends: %v", err)
	}
	if res.Processed != 1 || res.Failed != 0 {
		t.Fatalf("expected 1 processed, got %+v", res)
	}
	if got := holdingQty(t, db, fund.ID, assetID); got != 404 {
		t.Errorf("expected holding 400 -> 404 after reinvest, got %v", got)
	}
	hist, _ := svc.ListFundDividends(fund.ID)
	if len(hist) != 1 || hist[0].ReinvestedShares != 4 || hist[0].Policy != models.FundDividendPolicyReinvest {
		t.Errorf("unexpected dividend record: %+v", hist)
	}

	// Idempotent.
	res2, _ := svc.DistributeFundDividends(time.Now().UTC())
	if res2.Processed != 0 || res2.Skipped != 1 {
		t.Errorf("expected idempotent re-run, got %+v", res2)
	}
}

func TestFundService_DistributeFundDividends_Payout(t *testing.T) {
	db := openDivTestDB(t, "fund_div_payout")
	svc := newFundSvc(db)

	fund, err := svc.CreateFund(service.CreateFundInput{Naziv: "Payout Fund", Opis: "x", MinimalniUlog: 1000, ManagerID: 6})
	if err != nil {
		t.Fatalf("CreateFund: %v", err)
	}
	if _, err := svc.SetDividendPolicy(fund.ID, 6, models.FundDividendPolicyPayout); err != nil {
		t.Fatalf("SetDividendPolicy: %v", err)
	}

	assetID := seedDivStock(t, db, "FPA", "USD", 100, 0.04)
	seedHolding(t, db, fund.ID, "fund", assetID, 100, fund.AccountID) // gross 100 USD -> 10000 RSD

	// Sole participant: client 9 with a RSD payout account.
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id) VALUES (90, 1, 'aktivan', 9)`)
	db.Create(&models.ClientFundPositionRecord{
		ClientID: 9, ClientType: "client", FundID: fund.ID, UkupanUlozeniIznos: 1000,
		DatumPoslednjePromene: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})

	before := acctBalance(t, db, 90)
	res, err := svc.DistributeFundDividends(time.Now().UTC())
	if err != nil {
		t.Fatalf("DistributeFundDividends: %v", err)
	}
	if res.Processed != 1 {
		t.Fatalf("expected 1 processed, got %+v", res)
	}
	// 100% share -> the full 10000 RSD is paid to the client.
	if got := acctBalance(t, db, 90) - before; got != 10000 {
		t.Errorf("expected client paid 10000 RSD, got %v", got)
	}
	hist, _ := svc.ListFundDividends(fund.ID)
	if len(hist) != 1 || hist[0].DistributedRSD != 10000 || hist[0].Policy != models.FundDividendPolicyPayout {
		t.Errorf("unexpected dividend record: %+v", hist)
	}
}

// TestFundService_DistributeFundDividends_PayoutNotifies covers the
// notifyFundDividendPayout emit branch (best-effort, dead endpoint).
func TestFundService_DistributeFundDividends_PayoutNotifies(t *testing.T) {
	db := openDivTestDB(t, "fund_div_notify")
	svc := newFundSvc(db).WithNotifier(notify.NewClient("http://127.0.0.1:0", "k"))

	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "Notify Fund", Opis: "x", MinimalniUlog: 1000, ManagerID: 6})
	_, _ = svc.SetDividendPolicy(fund.ID, 6, models.FundDividendPolicyPayout)
	assetID := seedDivStock(t, db, "FPN", "USD", 100, 0.04)
	seedHolding(t, db, fund.ID, "fund", assetID, 100, fund.AccountID)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id) VALUES (95, 1, 'aktivan', 9)`)
	db.Create(&models.ClientFundPositionRecord{
		ClientID: 9, ClientType: "client", FundID: fund.ID, UkupanUlozeniIznos: 1000,
		DatumPoslednjePromene: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})

	res, err := svc.DistributeFundDividends(time.Now().UTC())
	if err != nil || res.Processed != 1 {
		t.Fatalf("expected 1 processed with notifier wired, got %+v err=%v", res, err)
	}
}

func TestFundService_SetDividendPolicy_Guards(t *testing.T) {
	db := openDivTestDB(t, "fund_policy_guards")
	svc := newFundSvc(db)
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "Guard Fund", Opis: "x", MinimalniUlog: 1000, ManagerID: 6})

	// Wrong manager.
	if _, err := svc.SetDividendPolicy(fund.ID, 999, models.FundDividendPolicyPayout); err == nil {
		t.Error("expected error for non-manager")
	}
	// Invalid policy.
	if _, err := svc.SetDividendPolicy(fund.ID, 6, "bogus"); err == nil {
		t.Error("expected error for invalid policy")
	}
	// Valid.
	updated, err := svc.SetDividendPolicy(fund.ID, 6, models.FundDividendPolicyPayout)
	if err != nil || updated.DividendPolicy != models.FundDividendPolicyPayout {
		t.Errorf("expected policy update to payout, got %v err=%v", updated, err)
	}
}

func TestFundService_StatisticsAndBenchmark(t *testing.T) {
	db := openDivTestDB(t, "fund_stats_db")
	svc := newFundSvc(db)
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "Stats Fund", Opis: "x", MinimalniUlog: 1000, ManagerID: 6})

	// Five monthly snapshots: 100 -> 110 -> 99 -> 120 -> 130.
	vals := []struct {
		d string
		v float64
	}{
		{"2026-02-28", 100000}, {"2026-03-31", 110000}, {"2026-04-30", 99000},
		{"2026-05-31", 120000}, {"2026-06-15", 130000},
	}
	for _, s := range vals {
		d, _ := time.Parse("2006-01-02", s.d)
		if err := db.Create(&models.FundPerformanceHistoryRecord{FundID: fund.ID, Date: d, FundValue: s.v, CreatedAt: time.Now().UTC()}).Error; err != nil {
			t.Fatalf("snapshot: %v", err)
		}
	}

	stats, err := svc.ComputeStatistics(fund.ID)
	if err != nil {
		t.Fatalf("ComputeStatistics: %v", err)
	}
	if !stats.Available || stats.MonthsOfData != 4 {
		t.Fatalf("expected available with 4 monthly returns, got %+v", stats)
	}
	// Max drawdown 110 -> 99 = 0.10.
	if stats.MaxDrawdown < 0.099 || stats.MaxDrawdown > 0.101 {
		t.Errorf("expected drawdown ~0.10, got %v", stats.MaxDrawdown)
	}

	bench, err := svc.AverageFundBenchmark()
	if err != nil {
		t.Fatalf("AverageFundBenchmark: %v", err)
	}
	if len(bench) != 5 || bench[0].IndexValue != 100 {
		t.Errorf("expected 5 benchmark points starting at 100, got %+v", bench)
	}
}
