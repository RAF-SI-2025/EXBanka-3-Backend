package service_test

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func newOrderCreateSvc(t *testing.T, name string) (*service.OrderService, uint) {
	t.Helper()
	db := openDivTestDB(t, name)
	assetID := seedAsset(t, db, "CRT", 50, "USD") // price 50, ask 51, bid 49
	_ = assetID
	// Client USD account (currency 2 = USD) + a bank USD account for margin.
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, raspolozivo_stanje, stanje) VALUES (10, 2, 'aktivan', 1, 1000000, 1000000)`)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, firma_id, raspolozivo_stanje, stanje) VALUES (20, 2, 'aktivan', 1, 1000000, 1000000)`)
	svc := service.NewOrderService(
		repository.NewOrderRepository(db),
		repository.NewMarketRepository(db),
		&mockRateProv{rates: map[string]float64{"USD:RSD": 100}},
	)
	return svc, 10
}

func TestOrderService_CreateOrder_ClientMarketBuy(t *testing.T) {
	svc, acct := newOrderCreateSvc(t, "oc_market_buy")
	res, err := svc.CreateOrder(service.CreateOrderInput{
		UserID: 1, UserType: "client", AssetTicker: "CRT", OrderType: "market",
		Direction: "buy", Quantity: 3, ContractSize: 1, AccountID: acct,
	})
	if err != nil {
		t.Fatalf("CreateOrder market buy: %v", err)
	}
	// Client orders auto-approve; a buy debits the account (TotalPaid > 0).
	if res.Order.Status != "approved" || res.Order.TotalPaid <= 0 {
		t.Errorf("unexpected order: status=%s totalPaid=%v", res.Order.Status, res.Order.TotalPaid)
	}
}

func TestOrderService_CreateOrder_ClientLimitBuyAndSell(t *testing.T) {
	svc, acct := newOrderCreateSvc(t, "oc_limit_sell")
	limit := 48.0
	if _, err := svc.CreateOrder(service.CreateOrderInput{
		UserID: 1, UserType: "client", AssetTicker: "CRT", OrderType: "limit",
		Direction: "buy", Quantity: 2, ContractSize: 1, AccountID: acct, LimitValue: &limit,
	}); err != nil {
		t.Fatalf("CreateOrder limit buy: %v", err)
	}
	// Sell orders don't debit at creation.
	res, err := svc.CreateOrder(service.CreateOrderInput{
		UserID: 1, UserType: "client", AssetTicker: "CRT", OrderType: "market",
		Direction: "sell", Quantity: 2, ContractSize: 1, AccountID: acct,
	})
	if err != nil {
		t.Fatalf("CreateOrder sell: %v", err)
	}
	if res.Order.TotalPaid != 0 {
		t.Errorf("sell order should not debit, got totalPaid=%v", res.Order.TotalPaid)
	}
}

func TestOrderService_CreateOrder_ClientMarginBuy(t *testing.T) {
	svc, acct := newOrderCreateSvc(t, "oc_margin_buy")
	res, err := svc.CreateOrder(service.CreateOrderInput{
		UserID: 1, UserType: "client", AssetTicker: "CRT", OrderType: "market",
		Direction: "buy", Quantity: 4, ContractSize: 1, AccountID: acct, IsMargin: true,
	})
	if err != nil {
		t.Fatalf("CreateOrder margin buy: %v", err)
	}
	// A margin buy fronts part of the cost as a bank loan.
	if res.Order.MarginLoan <= 0 {
		t.Errorf("expected a margin loan, got %v", res.Order.MarginLoan)
	}
}

func TestOrderService_CreateOrder_AssetNotFoundAndInvalid(t *testing.T) {
	svc, acct := newOrderCreateSvc(t, "oc_errors")
	if _, err := svc.CreateOrder(service.CreateOrderInput{
		UserID: 1, UserType: "client", AssetTicker: "NOPE", OrderType: "market",
		Direction: "buy", Quantity: 1, ContractSize: 1, AccountID: acct,
	}); err == nil {
		t.Error("expected asset-not-found error")
	}
	if _, err := svc.CreateOrder(service.CreateOrderInput{}); err == nil {
		t.Error("expected validation error for empty input")
	}
}
