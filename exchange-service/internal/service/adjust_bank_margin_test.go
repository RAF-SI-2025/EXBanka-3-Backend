package service

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// TestAdjustBankForMarginLoan exercises the debit, credit, no-account, and
// no-op branches of adjustBankForMarginLoan directly.
func TestAdjustBankForMarginLoan(t *testing.T) {
	db := openSagaTestDB(t, "adjust_bank_margin")
	// A non-state firma + an active RSD bank account it owns.
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS firmas (id INTEGER PRIMARY KEY AUTOINCREMENT, naziv TEXT, is_state BOOLEAN DEFAULT 0)`).Error; err != nil {
		t.Fatalf("firmas: %v", err)
	}
	db.Exec(`INSERT INTO firmas (id, naziv, is_state) VALUES (1, 'EXBanka', 0)`)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, firma_id, stanje, raspolozivo_stanje) VALUES (50, 1, 'aktivan', 1, 1000, 1000)`)

	svc := NewOrderService(repository.NewOrderRepository(db), repository.NewMarketRepository(db), cronRateProv{})

	// No-op branches.
	svc.adjustBankForMarginLoan("RSD", 0, 1, "disburse")
	svc.adjustBankForMarginLoan("", 100, 1, "disburse")

	// Debit (loan disbursed).
	svc.adjustBankForMarginLoan("RSD", -200, 1, "disburse")
	// Credit (loan repaid).
	svc.adjustBankForMarginLoan("RSD", 150, 1, "repay")
	// Unknown currency -> bankAccountID == 0, warn + skip.
	svc.adjustBankForMarginLoan("XXX", 50, 1, "repay")

	var bal float64
	db.Table("accounts").Select("raspolozivo_stanje").Where("id = 50").Scan(&bal)
	if bal != 1000-200+150 {
		t.Errorf("bank balance=%v, want %v", bal, 1000-200+150)
	}
}
