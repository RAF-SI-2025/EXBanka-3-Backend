package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"gorm.io/gorm"
)

// openDivTestDB returns a migrated DB plus the reference currencies/accounts/
// firmas tables the dividend + fund-dividend money moves read from.
func openDivTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db := openTestDB(t, name)
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS currencies (
			id INTEGER PRIMARY KEY AUTOINCREMENT, kod TEXT, naziv TEXT, simbol TEXT, drzava TEXT,
			aktivan BOOLEAN, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			broj_racuna TEXT, currency_id INTEGER, tip TEXT, vrsta TEXT, podvrsta TEXT,
			stanje REAL DEFAULT 0, raspolozivo_stanje REAL DEFAULT 0,
			dnevni_limit REAL, mesecni_limit REAL,
			dnevna_potrosnja REAL DEFAULT 0, mesecna_potrosnja REAL DEFAULT 0,
			datum_isteka DATETIME, odrzavanje_racuna REAL DEFAULT 0,
			naziv TEXT, status TEXT, created_at DATETIME, updated_at DATETIME,
			client_id INTEGER, firma_id INTEGER, zaposleni_id INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS firmas (id INTEGER PRIMARY KEY AUTOINCREMENT, naziv TEXT, is_state BOOLEAN DEFAULT 0)`,
		`INSERT INTO currencies (id, kod, aktivan) VALUES (1,'RSD',1),(2,'USD',1)`,
		`INSERT INTO firmas (id, naziv, is_state) VALUES (1,'EXBanka',0)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed ref tables: %v", err)
		}
	}
	return db
}

// seedDivStock creates an exchange + stock listing (with a dividend yield) and
// returns the listing id.
func seedDivStock(t *testing.T, db *gorm.DB, ticker, currency string, price, yield float64) uint {
	t.Helper()
	exch := models.MarketExchangeRecord{
		Acronym: "DX-" + ticker, Name: "X", MICCode: "MX-" + ticker, Polity: "X",
		Currency: currency, Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatalf("exchange: %v", err)
	}
	listing := models.MarketListingRecord{
		Ticker: ticker, Name: ticker, Type: "stock", ExchangeID: exch.ID,
		Price: price, Ask: price, Bid: price, Volume: 100, LastRefresh: time.Now().UTC(),
	}
	if err := db.Create(&listing).Error; err != nil {
		t.Fatalf("listing: %v", err)
	}
	if err := db.Create(&models.StockRecord{ListingID: listing.ID, OutstandingShares: 1000, DividendYield: yield}).Error; err != nil {
		t.Fatalf("stock: %v", err)
	}
	return listing.ID
}

func seedHolding(t *testing.T, db *gorm.DB, userID uint, userType string, assetID uint, qty float64, accountID uint) {
	t.Helper()
	if err := db.Create(&models.PortfolioHoldingRecord{
		UserID: userID, UserType: userType, AssetID: assetID, Quantity: qty,
		AvgBuyPrice: 1, AccountID: accountID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("holding: %v", err)
	}
}

func acctBalance(t *testing.T, db *gorm.DB, id uint) float64 {
	t.Helper()
	var bal float64
	if err := db.Table("accounts").Select("raspolozivo_stanje").Where("id = ?", id).Scan(&bal).Error; err != nil {
		t.Fatalf("acct balance: %v", err)
	}
	return bal
}

func newDividendSvc(db *gorm.DB) *service.DividendService {
	rates := &mockRateProv{rates: map[string]float64{"USD:RSD": 100}}
	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), rates)
	return service.NewDividendService(repository.NewDividendRepository(db), repository.NewOrderRepository(db), taxSvc, rates)
}

func TestDividendService_DistributeForDate_ClientPayoutTaxAndIdempotent(t *testing.T) {
	db := openDivTestDB(t, "div_dist_client")
	assetID := seedDivStock(t, db, "DVA", "USD", 200, 0.04) // 4% yield
	// Client 7 holds 100 shares bought from USD account 10.
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id) VALUES (10, 2, 'aktivan', 7)`)
	seedHolding(t, db, 7, "client", assetID, 100, 10)

	before := acctBalance(t, db, 10)
	svc := newDividendSvc(db)
	res, err := svc.DistributeForDate(time.Now().UTC())
	if err != nil {
		t.Fatalf("DistributeForDate: %v", err)
	}
	if res.PaidOut != 1 || res.Failed != 0 {
		t.Fatalf("expected 1 paid 0 failed, got %+v", res)
	}

	// gross = 100 * 200 * 0.04/4 = 200 USD, credited to the buy account (USD match).
	if got := acctBalance(t, db, 10) - before; got != 200 {
		t.Errorf("expected account credited by 200, got %v", got)
	}
	// Tax recorded for the client: 200 USD -> 20000 RSD * 15% = 3000.
	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{rates: map[string]float64{"USD:RSD": 100}})
	taxes, _ := taxSvc.ListTaxRecords(7, "client", "")
	if len(taxes) != 1 || taxes[0].TaxRSD != 3000 {
		t.Errorf("expected 1 tax record of 3000, got %+v", taxes)
	}
	// History endpoint returns the payout.
	payouts, _ := svc.ListPayoutsForUser(7, "client", 0)
	if len(payouts) != 1 || payouts[0].GrossAmount != 200 {
		t.Errorf("expected 1 payout of 200, got %+v", payouts)
	}

	// Idempotent: a second run pays nothing more.
	res2, _ := svc.DistributeForDate(time.Now().UTC())
	if res2.PaidOut != 0 || res2.Skipped != 1 {
		t.Errorf("expected idempotent re-run (0 paid, 1 skipped), got %+v", res2)
	}
}

func TestDividendService_DistributeForDate_BankUntaxedAndFundExcluded(t *testing.T) {
	db := openDivTestDB(t, "div_dist_bank")
	assetID := seedDivStock(t, db, "DVB", "USD", 100, 0.04)
	// Bank holding (user 0) bought from a bank USD account.
	db.Exec(`INSERT INTO accounts (id, currency_id, status, firma_id) VALUES (20, 2, 'aktivan', 1)`)
	seedHolding(t, db, 0, "bank", assetID, 100, 20)
	// A fund holding on the same stock must be ignored by the client/bank path.
	seedHolding(t, db, 1, "fund", assetID, 100, 20)

	svc := newDividendSvc(db)
	res, err := svc.DistributeForDate(time.Now().UTC())
	if err != nil {
		t.Fatalf("DistributeForDate: %v", err)
	}
	if res.PaidOut != 1 {
		t.Fatalf("expected only the bank holding paid (fund excluded), got %+v", res)
	}
	// Bank holders are not taxed.
	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	if taxes, _ := taxSvc.ListTaxRecords(0, "bank", ""); len(taxes) != 0 {
		t.Errorf("bank holdings must be untaxed, got %d tax records", len(taxes))
	}
}

// TestDividendService_DistributeForDate_SkipBranches covers the negligible-gross
// skip and the no-usable-account skip branches.
func TestDividendService_DistributeForDate_SkipBranches(t *testing.T) {
	db := openDivTestDB(t, "div_dist_skips")
	assetID := seedDivStock(t, db, "DVS", "USD", 100, 0.04)

	// Negligible gross: 0.0001 * 100 * 0.01 ≈ tiny -> skipped (<=0.005).
	tiny := seedDivStock(t, db, "DTN", "USD", 0.01, 0.0001)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id) VALUES (60, 2, 'aktivan', 9)`)
	seedHolding(t, db, 9, "client", tiny, 0.01, 60)

	// Holder with no resolvable account -> skipped (account_id 0, no accounts row).
	seedHolding(t, db, 8, "client", assetID, 100, 0)

	svc := newDividendSvc(db)
	res, err := svc.DistributeForDate(time.Now().UTC())
	if err != nil {
		t.Fatalf("DistributeForDate: %v", err)
	}
	if res.PaidOut != 0 {
		t.Errorf("expected nothing paid out, got %+v", res)
	}
	if res.Skipped < 2 {
		t.Errorf("expected at least 2 skips (tiny + no-account), got %+v", res)
	}
}

// TestDividendService_WithNotifier exercises the WithNotifier wiring and the
// emit branch of notifyHolder. The notifier points at a dead endpoint; emits are
// fire-and-forget best-effort, so the distribution must still succeed.
func TestDividendService_WithNotifier(t *testing.T) {
	db := openDivTestDB(t, "div_notify")
	assetID := seedDivStock(t, db, "DVN", "USD", 100, 0.04)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id) VALUES (40, 2, 'aktivan', 7)`)
	seedHolding(t, db, 7, "client", assetID, 100, 40)

	svc := newDividendSvc(db).WithNotifier(notify.NewClient("http://127.0.0.1:0", "k"))
	res, err := svc.DistributeForDate(time.Now().UTC())
	if err != nil || res.PaidOut != 1 {
		t.Fatalf("expected 1 paid with notifier wired, got %+v err=%v", res, err)
	}
}

func TestDividendService_DistributeForDate_RSDFallbackAccount(t *testing.T) {
	db := openDivTestDB(t, "div_dist_fallback")
	assetID := seedDivStock(t, db, "DVC", "USD", 100, 0.04)
	// Buy account is RSD (currency mismatch) -> primary path skipped. The holder
	// has no USD account, so the payout converts to RSD and credits the RSD one.
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id) VALUES (30, 1, 'aktivan', 8)`)
	seedHolding(t, db, 8, "client", assetID, 100, 30)

	before := acctBalance(t, db, 30)
	svc := newDividendSvc(db)
	if _, err := svc.DistributeForDate(time.Now().UTC()); err != nil {
		t.Fatalf("DistributeForDate: %v", err)
	}
	// gross 100 USD -> 10000 RSD credited to the RSD account.
	if got := acctBalance(t, db, 30) - before; got != 10000 {
		t.Errorf("expected RSD account credited by 10000, got %v", got)
	}
}
