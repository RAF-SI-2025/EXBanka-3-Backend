package service

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

func TestRefreshListingPrices(t *testing.T) {
	db := openSagaTestDB(t, "refresh_prices")
	now := time.Now().UTC()

	exch := models.MarketExchangeRecord{Acronym: "RX", Name: "X", MICCode: "RX1", Polity: "X", Currency: "USD", Timezone: "UTC", WorkingHours: "09-17"}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatalf("exchange: %v", err)
	}
	for _, tk := range []string{"RFP1", "RFP2"} {
		if err := db.Create(&models.MarketListingRecord{
			Ticker: tk, Name: tk, Type: "stock", ExchangeID: exch.ID,
			Price: 50, Ask: 51, Bid: 49, Volume: 100, LastRefresh: now.Add(-24 * time.Hour),
		}).Error; err != nil {
			t.Fatalf("listing %s: %v", tk, err)
		}
	}

	email := NewSMTPEmailService("127.0.0.1", 1, "f@x.com")

	// First pass creates today's daily snapshot; second pass updates it.
	refreshListingPrices(db, email)
	refreshListingPrices(db, email)

	var snaps int64
	db.Model(&models.MarketListingDailyPriceInfoRecord{}).Count(&snaps)
	if snaps == 0 {
		t.Error("expected daily price snapshots to be recorded")
	}
}
