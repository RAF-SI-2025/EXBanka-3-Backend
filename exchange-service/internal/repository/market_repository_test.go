package repository

import (
	"fmt"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openMarketRepositoryTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		t.Fatalf("failed to migrate market tables: %v", err)
	}

	return db
}

func TestMarketRepository_SeededCatalogIsIdempotentAndQueryable(t *testing.T) {
	db := openMarketRepositoryTestDB(t, "market_repository_seed")

	if err := database.SeedMarketData(db); err != nil {
		t.Fatalf("first market seed failed: %v", err)
	}
	if err := database.SeedMarketData(db); err != nil {
		t.Fatalf("second market seed failed: %v", err)
	}

	var exchangeCount int64
	var listingCount int64
	var historyCount int64

	if err := db.Model(&models.MarketExchangeRecord{}).Count(&exchangeCount).Error; err != nil {
		t.Fatalf("count exchanges failed: %v", err)
	}
	if err := db.Model(&models.MarketListingRecord{}).Count(&listingCount).Error; err != nil {
		t.Fatalf("count listings failed: %v", err)
	}
	if err := db.Model(&models.MarketListingDailyPriceInfoRecord{}).Count(&historyCount).Error; err != nil {
		t.Fatalf("count history failed: %v", err)
	}

	if exchangeCount != 6 {
		t.Fatalf("expected 6 exchanges after idempotent seed, got %d", exchangeCount)
	}
	// 12 stocks + 6 forex + 6 futures + generated options
	if listingCount < 24 {
		t.Fatalf("expected at least 24 listings after idempotent seed, got %d", listingCount)
	}
	// 24 base listings * 30 days = 720 history rows
	if historyCount < 720 {
		t.Fatalf("expected at least 720 history rows after idempotent seed, got %d", historyCount)
	}

	repo := NewMarketRepository(db)

	listing, err := repo.GetListing("AAPL")
	if err != nil {
		t.Fatalf("GetListing(AAPL) returned error: %v", err)
	}
	if listing == nil {
		t.Fatal("expected seeded AAPL listing")
	}
	if listing.Ticker != "AAPL" {
		t.Fatalf("expected AAPL ticker, got %q", listing.Ticker)
	}
	if listing.Exchange.Acronym != "NASDAQ" {
		t.Fatalf("expected NASDAQ exchange, got %q", listing.Exchange.Acronym)
	}
	if listing.Type != models.ListingTypeStock {
		t.Fatalf("expected stock listing type, got %q", listing.Type)
	}

	history, err := repo.GetHistory("AAPL")
	if err != nil {
		t.Fatalf("GetHistory(AAPL) returned error: %v", err)
	}
	if len(history) != 30 {
		t.Fatalf("expected 30 seeded history rows, got %d", len(history))
	}
	for i := 1; i < len(history); i++ {
		if !history[i-1].Date.Before(history[i].Date) {
			t.Fatalf("expected ascending history dates, got %s then %s", history[i-1].Date, history[i].Date)
		}
	}
	last := history[len(history)-1]
	if last.Price <= 0 || last.High <= 0 || last.Low <= 0 || last.Volume <= 0 {
		t.Fatalf("expected positive latest history values, got %+v", last)
	}
	if last.High < last.Price || last.Low > last.Price {
		t.Fatalf("expected latest history high/low to bound price, got %+v", last)
	}
}

func TestMarketRepository_EnsureForeignListing(t *testing.T) {
	db := openMarketRepositoryTestDB(t, "market_repository_ensure_foreign")
	if err := database.SeedMarketData(db); err != nil {
		t.Fatalf("market seed failed: %v", err)
	}
	repo := NewMarketRepository(db)

	// Existing locally-listed ticker: returns the seeded row, no new insert.
	before, err := repo.GetListingRecordByTicker("AAPL")
	if err != nil || before == nil {
		t.Fatalf("expected seeded AAPL, got %+v err=%v", before, err)
	}
	got, err := repo.EnsureForeignListing("AAPL", "USD", 1.23)
	if err != nil {
		t.Fatalf("EnsureForeignListing(AAPL) error: %v", err)
	}
	if got.ID != before.ID {
		t.Fatalf("expected existing AAPL id %d, got %d", before.ID, got.ID)
	}
	if got.Price == 1.23 {
		t.Fatalf("EnsureForeignListing must not overwrite an existing listing's price")
	}

	// Unknown cross-bank ticker: synthesised, seeded from the strike,
	// anchored to a USD exchange.
	const strike = 41.5
	created, err := repo.EnsureForeignListing("BAC", "USD", strike)
	if err != nil {
		t.Fatalf("EnsureForeignListing(BAC) error: %v", err)
	}
	if created == nil || created.ID == 0 {
		t.Fatalf("expected a synthesised BAC listing, got %+v", created)
	}
	if created.Ticker != "BAC" || created.Type != "stock" {
		t.Fatalf("unexpected synthesised listing: %+v", created)
	}
	if created.Price != strike || created.Ask != strike || created.Bid != strike {
		t.Fatalf("expected price/ask/bid seeded to %v, got %+v", strike, created)
	}

	var ex models.MarketExchangeRecord
	if err := db.First(&ex, created.ExchangeID).Error; err != nil {
		t.Fatalf("loading anchored exchange: %v", err)
	}
	if ex.Currency != "USD" {
		t.Fatalf("expected USD exchange for USD contract, got %q", ex.Currency)
	}

	// Idempotent: a second call returns the same row, no duplicate.
	again, err := repo.EnsureForeignListing("BAC", "USD", 999)
	if err != nil {
		t.Fatalf("second EnsureForeignListing(BAC) error: %v", err)
	}
	if again.ID != created.ID {
		t.Fatalf("expected idempotent BAC id %d, got %d", created.ID, again.ID)
	}
	var count int64
	if err := db.Model(&models.MarketListingRecord{}).Where("ticker = ?", "BAC").Count(&count).Error; err != nil {
		t.Fatalf("count BAC listings: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 BAC listing, got %d", count)
	}
}
