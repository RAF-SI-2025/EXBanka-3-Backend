package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestFundService_WithdrawWithLiquidation(t *testing.T) {
	e := newFundEnv(t, "fund_withdraw_liq")
	fund, err := e.svc.CreateFund(service.CreateFundInput{Naziv: "Liq", MinimalniUlog: 100, ManagerID: 5})
	if err != nil {
		t.Fatalf("create fund: %v", err)
	}
	acct := e.seedClientAccount(t, 5000)

	// Invest 1000 RSD -> fund cash 1000, client position 1000.
	if _, err := e.svc.InvestInFund(service.InvestInFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client",
		SourceAccountID: acct, Amount: 1000,
	}); err != nil {
		t.Fatalf("invest: %v", err)
	}

	// Give the fund a stock holding worth ~1000 RSD (no USD:RSD rate -> price used as-is).
	assetID := seedAsset(t, e.db, "FLQ", 50, "USD")
	if err := e.db.Create(&models.PortfolioHoldingRecord{
		UserID: fund.ID, UserType: "fund", AssetID: assetID, Quantity: 20,
		AvgBuyPrice: 40, AccountID: fund.AccountID, CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("seed fund holding: %v", err)
	}

	// Withdraw the whole position -> requested (~2000) exceeds liquid cash (1000),
	// so the fund auto-liquidates holdings to cover the shortfall.
	res, err := e.svc.WithdrawFromFund(service.WithdrawFromFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client",
		DestinationAccountID: acct, WithdrawAll: true,
	})
	if err != nil {
		t.Fatalf("WithdrawFromFund: %v", err)
	}
	if !res.Liquidated || len(res.LiquidatedItems) == 0 {
		t.Errorf("expected liquidation, got %+v", res)
	}
	if res.NetToAccount <= 0 {
		t.Errorf("expected positive net to account, got %v", res.NetToAccount)
	}
}

func TestFundService_RecordDailyPerformanceAndValidateBuy(t *testing.T) {
	e := newFundEnv(t, "fund_perf_validate")
	fund, err := e.svc.CreateFund(service.CreateFundInput{Naziv: "Perf", MinimalniUlog: 100, ManagerID: 5})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Daily snapshot writer over all funds.
	if err := e.svc.RecordDailyPerformance(time.Now().UTC()); err != nil {
		t.Fatalf("RecordDailyPerformance: %v", err)
	}

	// ValidateFundBuyOrder: manager + fund account ok; wrong manager / wrong account rejected.
	if _, err := e.svc.ValidateFundBuyOrder(fund.ID, 5, fund.AccountID); err != nil {
		t.Errorf("valid buy-order preflight failed: %v", err)
	}
	if _, err := e.svc.ValidateFundBuyOrder(fund.ID, 999, fund.AccountID); err == nil {
		t.Error("non-manager should be rejected")
	}
	if _, err := e.svc.ValidateFundBuyOrder(fund.ID, 5, 99999); err == nil {
		t.Error("wrong account should be rejected")
	}
}
