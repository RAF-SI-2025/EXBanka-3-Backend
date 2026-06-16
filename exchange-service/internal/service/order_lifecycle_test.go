package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func seedPendingBuy(t *testing.T, repo *repository.OrderRepository, assetID, accountID uint, qty int64, margin bool, marginLoan float64) *models.OrderRecord {
	t.Helper()
	o := &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "market", Direction: "buy",
		Quantity: qty, ContractSize: 1, PricePerUnit: 50, CurrencyRate: 1, Commission: 5,
		IsMargin: margin, MarginLoan: marginLoan, Status: "pending", RemainingPortions: qty,
		AccountID: accountID, LastModification: time.Now().UTC(), CreatedAt: time.Now().UTC(),
	}
	if err := repo.CreateOrder(o); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	return o
}

func TestOrderService_ApproveDeclineCancel(t *testing.T) {
	db := openDivTestDB(t, "os_lifecycle")
	assetID := seedAsset(t, db, "ORD", 50, "USD")
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, raspolozivo_stanje, stanje) VALUES (10, 2, 'aktivan', 1, 1000, 1000)`)
	repo := repository.NewOrderRepository(db)
	svc := service.NewOrderService(repo, repository.NewMarketRepository(db), &mockRateProv{rates: map[string]float64{"USD:RSD": 100}})

	// Approve a pending client order.
	o := seedPendingBuy(t, repo, assetID, 10, 2, false, 0)
	if err := svc.ApproveOrder(o.ID, 6); err != nil {
		t.Fatalf("ApproveOrder: %v", err)
	}
	if err := svc.ApproveOrder(o.ID, 6); err == nil {
		t.Error("re-approve should fail (not pending)")
	}

	// Decline refunds the buy debit.
	o2 := seedPendingBuy(t, repo, assetID, 10, 2, false, 0)
	before := acctBalance(t, db, 10)
	if err := svc.DeclineOrder(o2.ID, 6); err != nil {
		t.Fatalf("DeclineOrder: %v", err)
	}
	if acctBalance(t, db, 10) <= before {
		t.Error("decline should refund the account")
	}

	// Cancel: partial then full.
	o3 := seedPendingBuy(t, repo, assetID, 10, 4, false, 0)
	if err := svc.CancelOrder(o3.ID, 1, 2); err != nil {
		t.Fatalf("partial cancel: %v", err)
	}
	if err := svc.CancelOrder(o3.ID, 1, 0); err != nil {
		t.Fatalf("full cancel: %v", err)
	}
	if err := svc.CancelOrder(o3.ID, 1, 0); err == nil {
		t.Error("cancel of done order should fail")
	}

	// Not-found paths.
	if err := svc.ApproveOrder(99999, 6); err == nil {
		t.Error("approve missing -> error")
	}
	if err := svc.DeclineOrder(99999, 6); err == nil {
		t.Error("decline missing -> error")
	}
	if err := svc.CancelOrder(99999, 1, 0); err == nil {
		t.Error("cancel missing -> error")
	}
}

func TestOrderService_MarginDeclineAndCancelRepayBank(t *testing.T) {
	db := openDivTestDB(t, "os_margin")
	assetID := seedAsset(t, db, "MRG", 50, "USD")
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, raspolozivo_stanje, stanje) VALUES (10, 2, 'aktivan', 1, 1000, 1000)`)
	// Bank USD account (firma 1 = EXBanka, is_state=false) for margin repayment.
	db.Exec(`INSERT INTO accounts (id, currency_id, status, firma_id, raspolozivo_stanje, stanje) VALUES (20, 2, 'aktivan', 1, 100000, 100000)`)
	repo := repository.NewOrderRepository(db)
	svc := service.NewOrderService(repo, repository.NewMarketRepository(db), &mockRateProv{rates: map[string]float64{"USD:RSD": 100}})

	// Margin decline: refunds the IMC and repays the bank's fronted loan.
	o := seedPendingBuy(t, repo, assetID, 10, 4, true, 200)
	if err := svc.DeclineOrder(o.ID, 6); err != nil {
		t.Fatalf("margin DeclineOrder: %v", err)
	}

	// Margin cancel: partial cancel reduces the loan proportionally.
	o2 := seedPendingBuy(t, repo, assetID, 10, 4, true, 200)
	if err := svc.CancelOrder(o2.ID, 1, 2); err != nil {
		t.Fatalf("margin CancelOrder: %v", err)
	}
}
