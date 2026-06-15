package repository

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

func TestOrderRepository_MarginStopAndRefund(t *testing.T) {
	db := openRepoTestDB(t, "ord_margin")
	r := NewOrderRepository(db)

	o := &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: 5, Direction: "buy",
		IsMargin: true, MarginLoan: 1000, Quantity: 1, ContractSize: 1, PricePerUnit: 1,
		Status: "done", AccountID: 50, RemainingPortions: 0,
		LastModification: time.Now().UTC(), CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(o).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	if id, err := r.FindLatestBuyAccountID(1, "client", 5); err != nil || id != 50 {
		t.Fatalf("FindLatestBuyAccountID: %d err=%v", id, err)
	}
	if err := r.SetStopTriggered(o.ID); err != nil {
		t.Fatalf("SetStopTriggered: %v", err)
	}
	if err := r.SetMarginLoan(o.ID, 800); err != nil {
		t.Fatalf("SetMarginLoan: %v", err)
	}
	if loans, err := r.ListOutstandingMarginLoansForUserAsset(1, "client", 5); err != nil || len(loans) != 1 {
		t.Fatalf("ListOutstandingMarginLoans: %d err=%v", len(loans), err)
	}
	if applied, err := r.ReduceMarginLoan(o.ID, 500); err != nil || applied != 500 {
		t.Fatalf("ReduceMarginLoan partial: applied=%v err=%v", applied, err)
	}
	if applied, _ := r.ReduceMarginLoan(o.ID, 9999); applied != 300 {
		t.Errorf("ReduceMarginLoan clamp: applied=%v want 300", applied)
	}
	if applied, _ := r.ReduceMarginLoan(o.ID, 0); applied != 0 {
		t.Errorf("ReduceMarginLoan zero: applied=%v want 0", applied)
	}

	db.Exec(`INSERT INTO accounts (id, currency_id, status, raspolozivo_stanje, stanje) VALUES (50, 1, 'aktivan', 100, 100)`)
	if err := r.RefundBuyOverReservation(o.ID, 50, 10); err != nil {
		t.Fatalf("RefundBuyOverReservation: %v", err)
	}
	if err := r.RefundBuyOverReservation(o.ID, 50, 0); err != nil {
		t.Fatalf("RefundBuyOverReservation no-op: %v", err)
	}
}

func TestPortfolioRepository_SetAccountAndBuyFillTx(t *testing.T) {
	db := openRepoTestDB(t, "port_extra")
	pr := NewPortfolioRepository(db)

	if err := pr.RecordBuyFill(1, "client", 7, 50, 10, 100); err != nil {
		t.Fatalf("RecordBuyFill: %v", err)
	}
	var h models.PortfolioHoldingRecord
	if err := db.Where("user_id = 1 AND asset_id = 7").First(&h).Error; err != nil {
		t.Fatalf("load holding: %v", err)
	}
	if err := pr.SetHoldingAccountID(h.ID, 99); err != nil {
		t.Fatalf("SetHoldingAccountID: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return RecordBuyFillTx(tx, 1, "client", 7, 50, 5, 110)
	}); err != nil {
		t.Fatalf("RecordBuyFillTx existing: %v", err)
	}
	// New holding via the tx variant.
	if err := db.Transaction(func(tx *gorm.DB) error {
		return RecordBuyFillTx(tx, 2, "client", 8, 51, 3, 50)
	}); err != nil {
		t.Fatalf("RecordBuyFillTx new: %v", err)
	}
}

func TestFundRepository_CreditDebitTx(t *testing.T) {
	db := openRepoTestDB(t, "fund_txhelpers")
	db.Exec(`INSERT INTO accounts (id, currency_id, status, raspolozivo_stanje, stanje) VALUES (60, 1, 'aktivan', 100, 100)`)

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := CreditAccountTx(tx, 60, 50); err != nil {
			return err
		}
		return DebitAccountTx(tx, 60, 30)
	}); err != nil {
		t.Fatalf("credit/debit tx: %v", err)
	}
	// Insufficient funds -> error.
	if err := db.Transaction(func(tx *gorm.DB) error {
		return DebitAccountTx(tx, 60, 999999)
	}); err == nil {
		t.Error("expected insufficient-funds error")
	}
}
