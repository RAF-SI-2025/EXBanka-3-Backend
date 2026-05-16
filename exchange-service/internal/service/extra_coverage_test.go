package service_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"gorm.io/gorm"
)

func seedFundReferenceTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS currencies (id INTEGER PRIMARY KEY AUTOINCREMENT, kod TEXT, naziv TEXT)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			broj_racuna TEXT, currency_id INTEGER, tip TEXT, vrsta TEXT, podvrsta TEXT,
			stanje REAL DEFAULT 0, raspolozivo_stanje REAL DEFAULT 0,
			dnevni_limit REAL, mesecni_limit REAL,
			dnevna_potrosnja REAL DEFAULT 0, mesecna_potrosnja REAL DEFAULT 0,
			datum_isteka DATETIME, odrzavanje_racuna REAL DEFAULT 0,
			naziv TEXT, status TEXT,
			created_at DATETIME, updated_at DATETIME,
			client_id INTEGER, firma_id INTEGER, zaposleni_id INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS firmas (
			id INTEGER PRIMARY KEY AUTOINCREMENT, naziv TEXT, is_state BOOLEAN DEFAULT 0
		)`,
		`INSERT OR IGNORE INTO currencies (id, kod) VALUES (1, 'RSD')`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed ref table: %v (%s)", err, s)
		}
	}
}

func TestStartCronJobs_Constructs(t *testing.T) {
	db := openTestDB(t, "ex_cron_ctor")
	rates := &mockRateProv{}
	portfolioRepo := repository.NewPortfolioRepository(db)
	taxRepo := repository.NewTaxRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	taxSvc := service.NewTaxService(taxRepo, marketRepo, rates)
	psvc := service.NewPortfolioService(portfolioRepo, taxSvc, marketRepo, orderRepo)
	fundSvc := service.NewFundService(repository.NewFundRepository(db), portfolioRepo, marketRepo, orderRepo, rates)

	c := service.StartCronJobs(db, psvc, rates, nil, fundSvc, nil, nil)
	if c == nil {
		t.Fatal("expected non-nil cron")
	}
	c.Stop()
}

// --- Fund service tests ---

func TestFundService_CreateFund_Validation(t *testing.T) {
	db := openTestDB(t, "ex_fund_create_val")
	rates := &mockRateProv{}
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)

	if _, err := svc.CreateFund(service.CreateFundInput{}); err == nil {
		t.Fatal("expected naziv-required error")
	}
	if _, err := svc.CreateFund(service.CreateFundInput{Naziv: "x", MinimalniUlog: -1, ManagerID: 1}); err == nil {
		t.Fatal("expected positive-ulog error")
	}
	if _, err := svc.CreateFund(service.CreateFundInput{Naziv: "x", MinimalniUlog: 100}); err == nil {
		t.Fatal("expected manager-required error")
	}
}

func TestFundService_GetFund_NotFound(t *testing.T) {
	db := openTestDB(t, "ex_fund_get_notfound")
	rates := &mockRateProv{}
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)
	if _, err := svc.GetFund(99999); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestFundService_ListFunds_Empty(t *testing.T) {
	db := openTestDB(t, "ex_fund_list_empty")
	rates := &mockRateProv{}
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)
	if funds, err := svc.ListFunds(); err != nil || len(funds) != 0 {
		t.Fatalf("expected empty list, got %d funds err=%v", len(funds), err)
	}
	if funds, err := svc.ListFundsByManager(1); err != nil || len(funds) != 0 {
		t.Fatalf("expected empty list by manager, got %d err=%v", len(funds), err)
	}
}

func TestFundService_InvestInFund_Validation(t *testing.T) {
	db := openTestDB(t, "ex_fund_invest_val")
	rates := &mockRateProv{}
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)

	if _, err := svc.InvestInFund(service.InvestInFundInput{}); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if _, err := svc.InvestInFund(service.InvestInFundInput{Amount: 100}); err == nil {
		t.Fatal("expected error for missing source account")
	}
	if _, err := svc.InvestInFund(service.InvestInFundInput{Amount: 100, SourceAccountID: 1, ClientType: "unknown"}); err == nil {
		t.Fatal("expected error for bad client type")
	}
	if _, err := svc.InvestInFund(service.InvestInFundInput{Amount: 100, SourceAccountID: 1, ClientType: "client", FundID: 99999}); err == nil {
		t.Fatal("expected fund-not-found error")
	}
}

func TestFundService_WithdrawFromFund_Validation(t *testing.T) {
	db := openTestDB(t, "ex_fund_withdraw_val")
	rates := &mockRateProv{}
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)

	if _, err := svc.WithdrawFromFund(service.WithdrawFromFundInput{ClientType: "bogus"}); err == nil {
		t.Fatal("expected client-type error")
	}
	if _, err := svc.WithdrawFromFund(service.WithdrawFromFundInput{ClientType: "client"}); err == nil {
		t.Fatal("expected destination-required error")
	}
	if _, err := svc.WithdrawFromFund(service.WithdrawFromFundInput{ClientType: "client", DestinationAccountID: 1, FundID: 99999}); err == nil {
		t.Fatal("expected fund-not-found error")
	}
}

func TestFundService_CreateFund_Success_AndSummariseFund(t *testing.T) {
	db := openTestDB(t, "ex_fund_create_ok")
	seedFundReferenceTables(t, db)
	rates := &mockRateProv{}
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)

	fund, err := svc.CreateFund(service.CreateFundInput{
		Naziv: "Test Fund", Opis: "x", MinimalniUlog: 1000, ManagerID: 1,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if fund.AccountID == 0 {
		t.Fatal("expected fund account to be created")
	}

	got, err := svc.GetFund(fund.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Naziv != "Test Fund" {
		t.Fatalf("expected name preserved, got %s", got.Naziv)
	}

	summary, err := svc.SummariseFund(fund)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Fund.ID != fund.ID {
		t.Fatalf("expected summary fund.ID match, got %d", summary.Fund.ID)
	}

	funds, err := svc.ListFunds()
	if err != nil || len(funds) != 1 {
		t.Fatalf("list: %d funds err=%v", len(funds), err)
	}
}

func TestFundService_ValidateFundBuyOrder(t *testing.T) {
	db := openTestDB(t, "ex_fund_validate_buy")
	rates := &mockRateProv{}
	seedFundReferenceTables(t, db)
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)

	fund, err := svc.CreateFund(service.CreateFundInput{Naziv: "F1", MinimalniUlog: 1000, ManagerID: 1})
	if err != nil {
		t.Fatalf("create fund: %v", err)
	}
	// Wrong actor.
	if _, err := svc.ValidateFundBuyOrder(fund.ID, 99, fund.AccountID); err == nil {
		t.Fatal("expected wrong-supervisor error")
	}
	// Wrong account.
	if _, err := svc.ValidateFundBuyOrder(fund.ID, 1, 99999); err == nil {
		t.Fatal("expected wrong-account error")
	}
	// Correct.
	if _, err := svc.ValidateFundBuyOrder(fund.ID, 1, fund.AccountID); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestFundService_GetClientPosition_None(t *testing.T) {
	db := openTestDB(t, "ex_fund_pos_none")
	seedFundReferenceTables(t, db)
	rates := &mockRateProv{}
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "F1", MinimalniUlog: 1000, ManagerID: 1})
	if _, err := svc.GetClientPosition(99, "client", fund.ID); err == nil {
		t.Fatal("expected no-position error")
	}
}

func TestFundService_ListClientFundPositions_Empty(t *testing.T) {
	db := openTestDB(t, "ex_fund_list_pos_empty")
	rates := &mockRateProv{}
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)
	if list, err := svc.ListClientFundPositions(99, "client"); err != nil || len(list) != 0 {
		t.Fatalf("expected empty list, got %d err=%v", len(list), err)
	}
}

func TestFundService_ListFundHoldings_Empty(t *testing.T) {
	db := openTestDB(t, "ex_fund_holdings_empty")
	seedFundReferenceTables(t, db)
	rates := &mockRateProv{}
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "F1", MinimalniUlog: 1000, ManagerID: 1})
	if h, err := svc.ListFundHoldings(fund.ID); err != nil || len(h) != 0 {
		t.Fatalf("expected empty, got %d err=%v", len(h), err)
	}
}

func TestFundService_RecordDailyPerformance_NoFunds(t *testing.T) {
	db := openTestDB(t, "ex_fund_perf_none")
	rates := &mockRateProv{}
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)
	if err := svc.RecordDailyPerformance(time.Now().UTC()); err != nil {
		t.Fatalf("expected nil with no funds, got %v", err)
	}
}

func TestFundService_GetPerformance_NoData(t *testing.T) {
	db := openTestDB(t, "ex_fund_perf_get")
	seedFundReferenceTables(t, db)
	rates := &mockRateProv{}
	svc := service.NewFundService(repository.NewFundRepository(db), repository.NewPortfolioRepository(db), repository.NewMarketRepository(db), repository.NewOrderRepository(db), rates)
	fund, _ := svc.CreateFund(service.CreateFundInput{Naziv: "F1", MinimalniUlog: 1000, ManagerID: 1})
	if _, err := svc.GetPerformance(fund.ID, "7d"); err == nil {
		t.Fatal("expected unknown-granularity error")
	}
}

// PreviousMonthPeriod is exposed; ensure it returns a sensible string.
func TestPreviousMonthPeriod_Format(t *testing.T) {
	period := service.PreviousMonthPeriod()
	if len(period) < 4 {
		t.Fatalf("unexpected period: %q", period)
	}
}

// ---- Helpers ----

// suppress unused-import warnings when adjusting tests.
var _ = fmt.Sprintf
var _ = repository.NewMarketRepository
var _ models.MarketListingRecord
