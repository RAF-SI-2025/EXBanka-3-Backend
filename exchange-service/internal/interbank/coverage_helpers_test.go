package interbank

import (
	"fmt"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// openInterbankTestDB returns an in-memory sqlite DB with the exchange-service
// migrations plus the reference currencies/accounts tables that the wallet
// repos read. Mirrors repository.openRepoTestDB, which lives in the
// repository package's test files and isn't importable here.
func openInterbankTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
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
			naziv TEXT, status TEXT,
			created_at DATETIME, updated_at DATETIME,
			client_id INTEGER, firma_id INTEGER, zaposleni_id INTEGER
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed schema: %v", err)
		}
	}
	return db
}

// seedCurrency inserts a currency row and returns its id.
func seedCurrency(t *testing.T, db *gorm.DB, kod string) uint {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO currencies (kod, naziv, aktivan, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`,
		kod, kod, time.Now(), time.Now(),
	).Error; err != nil {
		t.Fatalf("seed currency: %v", err)
	}
	var id uint
	if err := db.Raw(`SELECT id FROM currencies WHERE kod = ? ORDER BY id DESC LIMIT 1`, kod).Scan(&id).Error; err != nil {
		t.Fatalf("read currency id: %v", err)
	}
	return id
}

// seedClientAccount inserts an active account for a client in a currency with
// the given balances and returns its id and account number.
func seedClientAccount(t *testing.T, db *gorm.DB, clientID uint, currencyID uint, brojRacuna string, stanje, raspolozivo float64) uint {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO accounts (broj_racuna, currency_id, status, stanje, raspolozivo_stanje, client_id, created_at, updated_at)
		 VALUES (?, ?, 'aktivan', ?, ?, ?, ?, ?)`,
		brojRacuna, currencyID, stanje, raspolozivo, clientID, time.Now(), time.Now(),
	).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}
	var id uint
	if err := db.Raw(`SELECT id FROM accounts WHERE broj_racuna = ? ORDER BY id DESC LIMIT 1`, brojRacuna).Scan(&id).Error; err != nil {
		t.Fatalf("read account id: %v", err)
	}
	return id
}

// seedListing inserts an exchange + stock listing and returns the listing id.
func seedListing(t *testing.T, db *gorm.DB, ticker string) uint {
	t.Helper()
	exchange := models.MarketExchangeRecord{
		Acronym: "X" + ticker, Name: "Exchange " + ticker, MICCode: "M" + ticker,
		Polity: "US", Currency: "USD", Timezone: "UTC", WorkingHours: "09:30-16:00", Enabled: true,
	}
	if err := db.Create(&exchange).Error; err != nil {
		t.Fatalf("seed exchange: %v", err)
	}
	listing := models.MarketListingRecord{
		Ticker: ticker, Name: ticker + " Corp", ExchangeID: exchange.ID,
		LastRefresh: time.Now().UTC(), Price: 100, Ask: 101, Bid: 99, Volume: 1000,
		Type: string(models.ListingTypeStock),
	}
	if err := db.Create(&listing).Error; err != nil {
		t.Fatalf("seed listing: %v", err)
	}
	return listing.ID
}

// testRegistry builds a registry with our own routing number and one partner.
func testRegistry(t *testing.T, own RoutingNumber, partnerCode RoutingNumber, baseURL string) *Registry {
	t.Helper()
	partnersJSON := fmt.Sprintf(
		`[{"code":%d,"baseUrl":%q,"outboundKey":"out-key","inboundKey":"in-key","displayName":"Partner %d"}]`,
		partnerCode, baseURL, partnerCode,
	)
	reg, err := NewRegistryFromJSON(own, partnersJSON)
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}
	return reg
}
