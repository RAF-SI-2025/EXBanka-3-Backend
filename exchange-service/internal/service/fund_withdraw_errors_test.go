package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestFundService_WithdrawErrorBranches(t *testing.T) {
	e := newFundEnv(t, "fund_withdraw_err")
	fund, err := e.svc.CreateFund(service.CreateFundInput{Naziv: "WE", MinimalniUlog: 100, ManagerID: 5})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	acct := e.seedClientAccount(t, 5000)
	if _, err := e.svc.InvestInFund(service.InvestInFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client", SourceAccountID: acct, Amount: 1000,
	}); err != nil {
		t.Fatalf("invest: %v", err)
	}

	// Missing destination account.
	if _, err := e.svc.WithdrawFromFund(service.WithdrawFromFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client", DestinationAccountID: 0,
	}); err == nil {
		t.Error("missing destination should error")
	}
	// Unknown participant type.
	if _, err := e.svc.WithdrawFromFund(service.WithdrawFromFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "alien", DestinationAccountID: acct,
	}); err == nil {
		t.Error("unknown type should error")
	}
	// Requested exceeds the client's share.
	if _, err := e.svc.WithdrawFromFund(service.WithdrawFromFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client", DestinationAccountID: acct, Amount: 999999,
	}); err == nil {
		t.Error("over-share withdrawal should error")
	}
	// Destination account owned by a different client.
	now := time.Now().UTC()
	e.db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, stanje, raspolozivo_stanje, status, client_id, created_at, updated_at) VALUES (?, 1, 0, 0, 'aktivan', 777, ?, ?)`,
		"OTHER-CLI", now, now)
	var otherID uint
	e.db.Table("accounts").Select("id").Where("broj_racuna = ?", "OTHER-CLI").Scan(&otherID)
	if _, err := e.svc.WithdrawFromFund(service.WithdrawFromFundInput{
		FundID: fund.ID, ClientID: e.clientID, ClientType: "client", DestinationAccountID: otherID, Amount: 100,
	}); err == nil {
		t.Error("destination not owned by client should error")
	}
	// No position in a different fund.
	other, _ := e.svc.CreateFund(service.CreateFundInput{Naziv: "WE2", MinimalniUlog: 100, ManagerID: 5})
	if _, err := e.svc.WithdrawFromFund(service.WithdrawFromFundInput{
		FundID: other.ID, ClientID: e.clientID, ClientType: "client", DestinationAccountID: acct, WithdrawAll: true,
	}); err == nil {
		t.Error("withdrawing from a fund with no position should error")
	}
}
