package service_test

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestOrderService_CreateOrder_BankAndFundStatus(t *testing.T) {
	db := openDivTestDB(t, "oc_status")
	seedAsset(t, db, "STS", 50, "USD")
	db.Exec(`CREATE TABLE IF NOT EXISTS actuary_profiles (
		id INTEGER PRIMARY KEY AUTOINCREMENT, employee_id INTEGER,
		trading_limit REAL, used_limit REAL DEFAULT 0, need_approval BOOLEAN DEFAULT 0)`)
	// Bank USD account for buy debits.
	db.Exec(`INSERT INTO accounts (id, currency_id, status, firma_id, raspolozivo_stanje, stanje) VALUES (30, 2, 'aktivan', 1, 10000000, 10000000)`)

	svc := service.NewOrderService(
		repository.NewOrderRepository(db),
		repository.NewMarketRepository(db),
		&mockRateProv{rates: map[string]float64{"USD:RSD": 100}},
	)
	order := func(userType string, userID, actorID uint, qty int64) (*service.CreateOrderResult, error) {
		return svc.CreateOrder(service.CreateOrderInput{
			UserID: userID, UserType: userType, ActorID: actorID, AssetTicker: "STS",
			OrderType: "market", Direction: "buy", Quantity: qty, ContractSize: 1, AccountID: 30,
		})
	}

	// Fund order -> auto-approved.
	if res, err := order("fund", 1, 0, 2); err != nil || res.Order.Status != "approved" {
		t.Fatalf("fund order: status=%v err=%v", res, err)
	}
	// Bank order, no actuary profile -> rejected.
	if _, err := order("bank", 0, 7, 2); err == nil {
		t.Error("bank order without profile should error")
	}
	// Supervisor (NULL trading_limit) -> approved.
	db.Exec(`INSERT INTO actuary_profiles (employee_id, trading_limit) VALUES (8, NULL)`)
	if res, err := order("bank", 0, 8, 2); err != nil || res.Order.Status != "approved" {
		t.Errorf("supervisor order: status=%v err=%v", res, err)
	}
	// Agent within limit -> approved.
	db.Exec(`INSERT INTO actuary_profiles (employee_id, trading_limit, used_limit, need_approval) VALUES (9, 1000000, 0, 0)`)
	if res, err := order("bank", 0, 9, 2); err != nil || res.Order.Status != "approved" {
		t.Errorf("agent order: status=%v err=%v", res, err)
	}
	// Agent flagged need_approval -> pending.
	db.Exec(`INSERT INTO actuary_profiles (employee_id, trading_limit, used_limit, need_approval) VALUES (10, 1000000, 0, 1)`)
	if res, err := order("bank", 0, 10, 2); err != nil || res.Order.Status != "pending" {
		t.Errorf("need-approval order: status=%v err=%v", res, err)
	}
	// Agent daily limit exhausted -> error.
	db.Exec(`INSERT INTO actuary_profiles (employee_id, trading_limit, used_limit) VALUES (11, 100, 100)`)
	if _, err := order("bank", 0, 11, 2); err == nil {
		t.Error("exhausted-limit order should error")
	}
	// Agent order exceeds remaining limit -> error.
	db.Exec(`INSERT INTO actuary_profiles (employee_id, trading_limit, used_limit) VALUES (12, 10, 0)`)
	if _, err := order("bank", 0, 12, 100); err == nil {
		t.Error("over-limit order should error")
	}
}
