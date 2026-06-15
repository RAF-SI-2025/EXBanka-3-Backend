package repository

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

// seedDividendStock creates an exchange + stock listing (with a dividend yield)
// and a positive holding for the given owner, returning the listing id.
func seedDividendStock(t *testing.T, db *gorm.DB, ticker, currency string, price, yield float64, ownerID uint, ownerType string, accountID uint) uint {
	t.Helper()
	exch := models.MarketExchangeRecord{
		Acronym: "DX-" + ticker, Name: "Div Exchange", MICCode: "MX-" + ticker, Polity: "X",
		Currency: currency, Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatalf("seed exchange: %v", err)
	}
	listing := models.MarketListingRecord{
		Ticker: ticker, Name: ticker, Type: "stock", ExchangeID: exch.ID,
		Price: price, Ask: price, Bid: price, Volume: 100, LastRefresh: time.Now().UTC(),
	}
	if err := db.Create(&listing).Error; err != nil {
		t.Fatalf("seed listing: %v", err)
	}
	if err := db.Create(&models.StockRecord{ListingID: listing.ID, OutstandingShares: 1000, DividendYield: yield}).Error; err != nil {
		t.Fatalf("seed stock: %v", err)
	}
	if err := db.Create(&models.PortfolioHoldingRecord{
		UserID: ownerID, UserType: ownerType, AssetID: listing.ID, Quantity: 10,
		AvgBuyPrice: price, AccountID: accountID,
	}).Error; err != nil {
		t.Fatalf("seed holding: %v", err)
	}
	return listing.ID
}

func TestDividendRepository_EligibleHoldingsAndPayouts(t *testing.T) {
	db := openRepoTestDB(t, "dividend_repo")
	r := NewDividendRepository(db)

	assetID := seedDividendStock(t, db, "DIV", "USD", 100, 0.04, 7, "client", 1)
	// A zero-yield stock must NOT be eligible.
	seedDividendStock(t, db, "NODIV", "USD", 50, 0, 7, "client", 1)
	// A fund-owned holding must NOT be eligible (Celina 4 handles funds).
	seedDividendStock(t, db, "FDIV", "USD", 100, 0.04, 1, "fund", 1)

	eligible, err := r.ListDividendEligibleHoldings()
	if err != nil {
		t.Fatalf("ListDividendEligibleHoldings: %v", err)
	}
	if len(eligible) != 1 {
		t.Fatalf("expected 1 eligible holding (zero-yield + fund excluded), got %d", len(eligible))
	}
	h := eligible[0]
	if h.AssetID != assetID || h.Ticker != "DIV" || h.Currency != "USD" || h.DividendYield != 0.04 || h.Quantity != 10 {
		t.Fatalf("unexpected eligible holding: %+v", h)
	}

	// No payout yet for this quarter.
	exists, err := r.PayoutExists(assetID, 7, "client", "2026-Q2")
	if err != nil || exists {
		t.Fatalf("expected no existing payout, got exists=%v err=%v", exists, err)
	}

	payout := &models.DividendPayoutRecord{
		AssetID: assetID, Ticker: "DIV", UserID: 7, UserType: "client", AccountID: 1,
		Quantity: 10, PricePerShare: 100, DividendYield: 0.04, Currency: "USD",
		GrossAmount: 10, CreditedAmount: 10, CreditedCurrency: "USD", TaxRSD: 180,
		Period: "2026-Q2", PaidAt: time.Now().UTC(),
	}
	if err := r.CreatePayout(payout); err != nil {
		t.Fatalf("CreatePayout: %v", err)
	}

	exists, err = r.PayoutExists(assetID, 7, "client", "2026-Q2")
	if err != nil || !exists {
		t.Fatalf("expected payout to exist after create, got exists=%v err=%v", exists, err)
	}

	list, err := r.ListPayoutsForUser(7, "client", 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 payout for user, got %d err=%v", len(list), err)
	}
	// Filter by a different asset → empty.
	other, _ := r.ListPayoutsForUser(7, "client", 9999)
	if len(other) != 0 {
		t.Errorf("expected 0 payouts for unrelated asset, got %d", len(other))
	}
}

func TestDividendRepository_FindActiveAccountByCurrency(t *testing.T) {
	db := openRepoTestDB(t, "dividend_acct")
	r := NewDividendRepository(db)

	// currencies
	db.Exec(`INSERT INTO currencies (id, kod, aktivan) VALUES (1,'USD',1),(2,'RSD',1)`)
	// a non-state firm (EXBanka)
	db.Exec(`INSERT INTO firmas (id, naziv, is_state) VALUES (1,'EXBanka',0),(2,'Drzava',1)`)
	// client USD account, and a bank (firm) RSD account
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id) VALUES (10, 1, 'aktivan', 7)`)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, firma_id) VALUES (20, 2, 'aktivan', 1)`)

	if id, err := r.FindActiveAccountByCurrency(7, "client", "USD"); err != nil || id != 10 {
		t.Errorf("expected client USD account 10, got %d err=%v", id, err)
	}
	if id, _ := r.FindActiveAccountByCurrency(7, "client", "EUR"); id != 0 {
		t.Errorf("expected 0 for missing currency, got %d", id)
	}
	if id, err := r.FindActiveAccountByCurrency(0, "bank", "RSD"); err != nil || id != 20 {
		t.Errorf("expected bank RSD account 20, got %d err=%v", id, err)
	}
}
