package service

import (
	"fmt"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// noopEmailSender satisfies EmailSender without dialing any SMTP server.
type noopEmailSender struct{}

func (noopEmailSender) Send(_, _, _ string) error { return nil }

func openCronTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestRefreshListingPrices_UpdatesNonOptionListings(t *testing.T) {
	db := openCronTestDB(t, "cron_refresh")

	exch := &models.MarketExchangeRecord{
		Acronym: "X", Name: "X", MICCode: "X1", Polity: "X", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(exch).Error; err != nil {
		t.Fatal(err)
	}

	stock := &models.MarketListingRecord{
		Ticker: "ST", Name: "Stock", Type: "stock",
		ExchangeID: exch.ID, Price: 100, Ask: 101, Bid: 99, Volume: 1000,
	}
	if err := db.Create(stock).Error; err != nil {
		t.Fatal(err)
	}

	option := &models.MarketListingRecord{
		Ticker: "OP", Name: "Option", Type: "option",
		ExchangeID: exch.ID, Price: 5, Ask: 5.1, Bid: 4.9, Volume: 50,
	}
	if err := db.Create(option).Error; err != nil {
		t.Fatal(err)
	}

	refreshListingPrices(db, noopEmailSender{})

	// Stock should have a daily price record.
	var stockHistory []models.MarketListingDailyPriceInfoRecord
	if err := db.Where("listing_id = ?", stock.ID).Find(&stockHistory).Error; err != nil {
		t.Fatal(err)
	}
	if len(stockHistory) != 1 {
		t.Errorf("expected 1 history row for stock, got %d", len(stockHistory))
	}

	// Options are skipped — no history.
	var optHistory []models.MarketListingDailyPriceInfoRecord
	db.Where("listing_id = ?", option.ID).Find(&optHistory)
	if len(optHistory) != 0 {
		t.Errorf("options should be skipped, got %d history rows", len(optHistory))
	}

	// Calling a second time should update (not duplicate) the existing day's row.
	refreshListingPrices(db, noopEmailSender{})
	var afterSecond []models.MarketListingDailyPriceInfoRecord
	db.Where("listing_id = ?", stock.ID).Find(&afterSecond)
	if len(afterSecond) != 1 {
		t.Errorf("expected still 1 history row after re-run, got %d", len(afterSecond))
	}
}
