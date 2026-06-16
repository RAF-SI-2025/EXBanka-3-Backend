package service

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// TestCronOtcExpiry_WithData drives the count>0 logging branches of the OTC
// cron helpers (reminders + expiry) plus the OtcService bodies they call.
func TestCronOtcExpiry_WithData(t *testing.T) {
	db := openSagaTestDB(t, "cron_otc_expiry")
	now := time.Now().UTC()

	exch := models.MarketExchangeRecord{Acronym: "CEX", Name: "X", MICCode: "CEX1", Polity: "X", Currency: "RSD", Timezone: "UTC", WorkingHours: "09-17"}
	db.Create(&exch)
	listing := models.MarketListingRecord{Ticker: "CET", Name: "CET", Type: "stock", ExchangeID: exch.ID, Price: 50, Ask: 50, Bid: 50, Volume: 1, LastRefresh: now}
	db.Create(&listing)

	// A valid contract whose settlement is within the reminder window.
	db.Create(&models.OtcContractRecord{
		StockListingID: listing.ID, Amount: 1, StrikePrice: 50,
		SettlementDate: now.Add(24 * time.Hour),
		BuyerID:        100, BuyerType: "client", SellerID: 200, SellerType: "client",
		Status:         models.OtcContractStatusValid, ExpiryReminderSent: false,
		CreatedAt:      now, UpdatedAt: now,
	})
	// A valid contract already past settlement — due to expire. Needs a seller
	// holding whose reserved quantity gets released on expiry.
	holding := models.PortfolioHoldingRecord{UserID: 201, UserType: "client", AssetID: listing.ID, Quantity: 5, ReservedQuantity: 1, AvgBuyPrice: 40, AccountID: 2, CreatedAt: now}
	db.Create(&holding)
	db.Create(&models.OtcContractRecord{
		StockListingID: listing.ID, SellerHoldingID: holding.ID, Amount: 1, StrikePrice: 50,
		SettlementDate: now.Add(-48 * time.Hour),
		BuyerID:        101, BuyerType: "client", SellerID: 201, SellerType: "client",
		Status:         models.OtcContractStatusValid, ExpiryReminderSent: true,
		CreatedAt:      now, UpdatedAt: now,
	})

	otcSvc := NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db)).
		WithNotifier(notify.NewClient("", ""))

	remindExpiringOtcContracts(otcSvc)
	expireDueOtcContracts(otcSvc)
	expireDueInterbankReservations(repository.NewInterbankOtcRepository(db))

	// The past-settlement contract should now be expired.
	var expired int64
	db.Model(&models.OtcContractRecord{}).Where("status = ?", models.OtcContractStatusExpired).Count(&expired)
	if expired == 0 {
		t.Error("expected at least one expired contract")
	}
	// The reminder should have been marked sent on the in-window contract.
	var reminded int64
	db.Model(&models.OtcContractRecord{}).Where("expiry_reminder_sent = ? AND buyer_id = 100", true).Count(&reminded)
	if reminded == 0 {
		t.Error("expected the in-window contract reminder to be marked sent")
	}
}
