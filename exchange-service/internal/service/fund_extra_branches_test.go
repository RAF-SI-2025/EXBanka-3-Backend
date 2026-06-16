package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestFundService_WithdrawPartial_NoLiquidation(t *testing.T) {
	e := newFundEnv(t, "fund_withdraw_partial")
	fund, err := e.svc.CreateFund(service.CreateFundInput{Naziv: "Part", MinimalniUlog: 100, ManagerID: 5})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	acct := e.seedClientAccount(t, 5000)
	if _, err := e.svc.InvestInFund(service.InvestInFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client", SourceAccountID: acct, Amount: 1000,
	}); err != nil {
		t.Fatalf("invest: %v", err)
	}

	// Fund holds 1000 cash, no securities -> withdrawing 400 needs no liquidation.
	res, err := e.svc.WithdrawFromFund(service.WithdrawFromFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client", DestinationAccountID: acct, Amount: 400,
	})
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if res.Liquidated {
		t.Error("no liquidation expected for a cash-covered withdrawal")
	}
	if res.Commission <= 0 {
		t.Error("client withdrawal should incur a commission")
	}
}

func TestFundService_InvestInFund_ErrorBranches(t *testing.T) {
	e := newFundEnv(t, "fund_invest_errors")
	fund, err := e.svc.CreateFund(service.CreateFundInput{Naziv: "Err", MinimalniUlog: 100, ManagerID: 5})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Non-positive amount.
	if _, err := e.svc.InvestInFund(service.InvestInFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client", SourceAccountID: 1, Amount: 0,
	}); err == nil {
		t.Error("zero amount should error")
	}
	// Unknown participant type.
	if _, err := e.svc.InvestInFund(service.InvestInFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "alien", SourceAccountID: 1, Amount: 200,
	}); err == nil {
		t.Error("unknown client type should error")
	}
	// Missing source account.
	if _, err := e.svc.InvestInFund(service.InvestInFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client", SourceAccountID: 0, Amount: 200,
	}); err == nil {
		t.Error("missing source account should error")
	}
	// Insufficient balance.
	low := e.seedClientAccount(t, 10)
	if _, err := e.svc.InvestInFund(service.InvestInFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client", SourceAccountID: low, Amount: 500,
	}); err == nil {
		t.Error("insufficient balance should error")
	}
	// Non-RSD source account.
	e.db.Exec(`INSERT INTO currencies (id, kod) VALUES (2, 'USD')`)
	now := time.Now().UTC()
	e.db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, stanje, raspolozivo_stanje, status, client_id, created_at, updated_at) VALUES (?, 2, ?, ?, 'aktivan', ?, ?, ?)`,
		"USD-FUND", 5000.0, 5000.0, e.clientID, now, now)
	var usdID uint
	e.db.Table("accounts").Select("id").Where("broj_racuna = ?", "USD-FUND").Scan(&usdID)
	if _, err := e.svc.InvestInFund(service.InvestInFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client", SourceAccountID: usdID, Amount: 200,
	}); err == nil {
		t.Error("non-RSD source account should error")
	}
}
