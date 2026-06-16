package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// TestOrderService_CreateOrder_MarginOption covers computeInitialMarginCost's
// "option" branch (which loads the option contract + underlying for the margin
// maintenance calculation).
func TestOrderService_CreateOrder_MarginOption(t *testing.T) {
	db := openDivTestDB(t, "oc_margin_opt")
	now := time.Now().UTC()

	exch := models.MarketExchangeRecord{Acronym: "MOX", Name: "X", MICCode: "MOX1", Polity: "X", Currency: "USD", Timezone: "UTC", WorkingHours: "09-17"}
	db.Create(&exch)
	underlying := models.MarketListingRecord{Ticker: "MUN", Name: "u", Type: "stock", ExchangeID: exch.ID, Price: 100, Ask: 101, Bid: 99, Volume: 1, LastRefresh: now}
	optListing := models.MarketListingRecord{Ticker: "MUNC", Name: "o", Type: "option", ExchangeID: exch.ID, Price: 5, Ask: 5, Bid: 5, Volume: 1, LastRefresh: now}
	db.Create(&underlying)
	db.Create(&optListing)
	db.Create(&models.OptionRecord{ListingID: optListing.ID, StockListingID: underlying.ID, OptionType: "call", StrikePrice: 50, ImpliedVolatility: 1, SettlementDate: now.AddDate(0, 0, 5)})
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, raspolozivo_stanje, stanje) VALUES (10, 2, 'aktivan', 1, 10000000, 10000000)`)

	svc := service.NewOrderService(
		repository.NewOrderRepository(db),
		repository.NewMarketRepository(db),
		&mockRateProv{rates: map[string]float64{"USD:RSD": 100}},
	)

	res, err := svc.CreateOrder(service.CreateOrderInput{
		UserID: 1, UserType: "client", AssetTicker: "MUNC", OrderType: "market",
		Direction: "buy", Quantity: 1, ContractSize: 1, AccountID: 10, IsMargin: true,
	})
	if err != nil {
		t.Fatalf("margin option order: %v", err)
	}
	if res.Order.Status != "approved" {
		t.Errorf("status=%s", res.Order.Status)
	}
}
