package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestPortfolioService_ExerciseOption_CallInTheMoney(t *testing.T) {
	db := openDivTestDB(t, "exercise_call")
	now := time.Now().UTC()

	exch := models.MarketExchangeRecord{
		Acronym: "OPX", Name: "X", MICCode: "OPX1", Polity: "X",
		Currency: "USD", Timezone: "UTC", WorkingHours: "09-17",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatalf("exchange: %v", err)
	}
	underlying := models.MarketListingRecord{
		Ticker: "UND", Name: "UND", Type: "stock", ExchangeID: exch.ID,
		Price: 100, Ask: 101, Bid: 99, Volume: 1, LastRefresh: now,
	}
	optListing := models.MarketListingRecord{
		Ticker: "UNDC", Name: "opt", Type: "option", ExchangeID: exch.ID,
		Price: 5, Ask: 5, Bid: 5, Volume: 1, LastRefresh: now,
	}
	if err := db.Create(&underlying).Error; err != nil {
		t.Fatalf("underlying: %v", err)
	}
	if err := db.Create(&optListing).Error; err != nil {
		t.Fatalf("option listing: %v", err)
	}
	if err := db.Create(&models.OptionRecord{
		ListingID: optListing.ID, StockListingID: underlying.ID, OptionType: "call",
		StrikePrice: 50, ImpliedVolatility: 1, SettlementDate: now.AddDate(0, 0, 5),
	}).Error; err != nil {
		t.Fatalf("option record: %v", err)
	}
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, raspolozivo_stanje, stanje) VALUES (10, 2, 'aktivan', 1, 1000000, 1000000)`)
	holding := models.PortfolioHoldingRecord{
		UserID: 1, UserType: "client", AssetID: optListing.ID, Quantity: 1,
		AccountID: 10, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatalf("holding: %v", err)
	}

	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db),
		service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{rates: map[string]float64{"USD:RSD": 100}}),
		repository.NewMarketRepository(db),
		repository.NewOrderRepository(db),
	)

	// CALL is in-the-money (market 100 > strike 50) -> exercises to a stock buy.
	if err := psvc.ExerciseOption(holding.ID, 5); err != nil {
		t.Fatalf("ExerciseOption: %v", err)
	}
	// The option holding is closed and an underlying stock holding now exists.
	var stockHolding models.PortfolioHoldingRecord
	if err := db.Where("user_id = 1 AND asset_id = ?", underlying.ID).First(&stockHolding).Error; err != nil {
		t.Fatalf("expected underlying holding created: %v", err)
	}
	if stockHolding.Quantity != float64(optionContractSizeForTest()) {
		t.Errorf("underlying qty=%v", stockHolding.Quantity)
	}
}

// optionContractSizeForTest mirrors the package's optionContractSize (100).
func optionContractSizeForTest() int64 { return 100 }
