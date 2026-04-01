package database

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

type listingSeed struct {
	Ticker          string
	Name            string
	ExchangeAcronym string
	Price           float64
	Volume          int64
	Type            models.ListingType
	// Stock-specific
	OutstandingShares int64
	DividendYield     float64
	// Forex-specific
	BaseCurrency  string
	QuoteCurrency string
	Liquidity     string
	// Futures-specific
	ContractSize   int64
	ContractUnit   string
	SettlementDate time.Time
}

func SeedMarketData(db *gorm.DB) error {
	exchangeIDs := make(map[string]uint, len(seedExchanges()))

	for _, exchange := range seedExchanges() {
		record := models.MarketExchangeRecord{
			Name:         exchange.Name,
			Acronym:      exchange.Acronym,
			MICCode:      exchange.MICCode,
			Polity:       exchange.Polity,
			Currency:     exchange.Currency,
			Timezone:     exchange.Timezone,
			WorkingHours: exchange.WorkingHours,
			Enabled:      exchange.Enabled,
		}

		if err := db.Where("acronym = ?", exchange.Acronym).
			Assign(record).
			FirstOrCreate(&record).Error; err != nil {
			return err
		}
		exchangeIDs[exchange.Acronym] = record.ID
	}

	referenceTime := seedReferenceTime()
	allListings := append(seedListings(), seedForexListings()...)
	allListings = append(allListings, seedFuturesListings()...)

	for _, listing := range allListings {
		exchangeID, ok := exchangeIDs[listing.ExchangeAcronym]
		if !ok {
			continue
		}

		record := models.MarketListingRecord{
			Ticker:      listing.Ticker,
			Name:        listing.Name,
			ExchangeID:  exchangeID,
			LastRefresh: referenceTime,
			Price:       round2(listing.Price),
			Ask:         round2(listing.Price * 1.002),
			Bid:         round2(listing.Price * 0.998),
			Volume:      listing.Volume,
			Type:        string(listing.Type),
		}

		if err := db.Where("ticker = ?", listing.Ticker).
			Assign(record).
			FirstOrCreate(&record).Error; err != nil {
			return err
		}

		// Create subtype records
		switch listing.Type {
		case models.ListingTypeStock:
			stockRec := models.StockRecord{
				ListingID:         record.ID,
				OutstandingShares: listing.OutstandingShares,
				DividendYield:     listing.DividendYield,
			}
			if err := db.Where("listing_id = ?", record.ID).Assign(stockRec).FirstOrCreate(&stockRec).Error; err != nil {
				return err
			}
		case models.ListingTypeForex:
			forexRec := models.ForexPairRecord{
				ListingID:     record.ID,
				BaseCurrency:  listing.BaseCurrency,
				QuoteCurrency: listing.QuoteCurrency,
				Liquidity:     listing.Liquidity,
			}
			if err := db.Where("listing_id = ?", record.ID).Assign(forexRec).FirstOrCreate(&forexRec).Error; err != nil {
				return err
			}
		case models.ListingTypeFutures:
			futuresRec := models.FuturesContractRecord{
				ListingID:      record.ID,
				ContractSize:   listing.ContractSize,
				ContractUnit:   listing.ContractUnit,
				SettlementDate: listing.SettlementDate,
			}
			if err := db.Where("listing_id = ?", record.ID).Assign(futuresRec).FirstOrCreate(&futuresRec).Error; err != nil {
				return err
			}
		}

		history := buildSeedHistory(listing.Ticker, listing.Price, listing.Volume)
		for _, item := range history {
			historyRecord := models.MarketListingDailyPriceInfoRecord{
				ListingID: record.ID,
				Date:      item.Date,
				Price:     item.Price,
				High:      item.High,
				Low:       item.Low,
				Change:    item.Change,
				Volume:    item.Volume,
			}

			if err := db.Where("listing_id = ? AND date = ?", record.ID, item.Date).
				Assign(historyRecord).
				FirstOrCreate(&historyRecord).Error; err != nil {
				return err
			}
		}
	}

	// Generate options for all stock listings
	if err := seedOptions(db, referenceTime); err != nil {
		return err
	}

	slog.Info("Market seed complete",
		"exchanges", len(seedExchanges()),
		"listings", len(allListings),
		"history_days", 30,
	)
	return nil
}

func seedReferenceTime() time.Time {
	return time.Date(2026, 3, 30, 19, 0, 0, 0, time.UTC)
}

func seedExchanges() []models.Exchange {
	return []models.Exchange{
		{Name: "New York Stock Exchange", Acronym: "NYSE", MICCode: "XNYS", Polity: "United States", Currency: "USD", Timezone: "America/New_York", WorkingHours: "09:30-16:00", Enabled: true},
		{Name: "NASDAQ", Acronym: "NASDAQ", MICCode: "XNAS", Polity: "United States", Currency: "USD", Timezone: "America/New_York", WorkingHours: "09:30-16:00", Enabled: true},
		{Name: "London Stock Exchange", Acronym: "LSE", MICCode: "XLON", Polity: "United Kingdom", Currency: "GBP", Timezone: "Europe/London", WorkingHours: "08:00-16:30", Enabled: true},
		{Name: "Xetra", Acronym: "XETRA", MICCode: "XETR", Polity: "Germany", Currency: "EUR", Timezone: "Europe/Berlin", WorkingHours: "09:00-17:30", Enabled: true},
		{Name: "Euronext Paris", Acronym: "EPA", MICCode: "XPAR", Polity: "France", Currency: "EUR", Timezone: "Europe/Paris", WorkingHours: "09:00-17:30", Enabled: true},
		{Name: "Tokyo Stock Exchange", Acronym: "TSE", MICCode: "XTKS", Polity: "Japan", Currency: "JPY", Timezone: "Asia/Tokyo", WorkingHours: "09:00-15:00", Enabled: true},
	}
}

func seedListings() []listingSeed {
	return []listingSeed{
		{Ticker: "AAPL", Name: "Apple Inc.", ExchangeAcronym: "NASDAQ", Price: 214.33, Volume: 68123412, Type: models.ListingTypeStock, OutstandingShares: 15460000000, DividendYield: 0.0055},
		{Ticker: "MSFT", Name: "Microsoft Corp.", ExchangeAcronym: "NASDAQ", Price: 421.84, Volume: 35219873, Type: models.ListingTypeStock, OutstandingShares: 7430000000, DividendYield: 0.0072},
		{Ticker: "NVDA", Name: "NVIDIA Corp.", ExchangeAcronym: "NASDAQ", Price: 905.18, Volume: 58741239, Type: models.ListingTypeStock, OutstandingShares: 24600000000, DividendYield: 0.0002},
		{Ticker: "GOOGL", Name: "Alphabet Inc. Class A", ExchangeAcronym: "NASDAQ", Price: 172.56, Volume: 24117893, Type: models.ListingTypeStock, OutstandingShares: 12200000000, DividendYield: 0.0050},
		{Ticker: "AMZN", Name: "Amazon.com Inc.", ExchangeAcronym: "NASDAQ", Price: 188.14, Volume: 42631518, Type: models.ListingTypeStock, OutstandingShares: 10300000000, DividendYield: 0},
		{Ticker: "JPM", Name: "JPMorgan Chase & Co.", ExchangeAcronym: "NYSE", Price: 197.41, Volume: 14328761, Type: models.ListingTypeStock, OutstandingShares: 2870000000, DividendYield: 0.0230},
		{Ticker: "KO", Name: "Coca-Cola Co.", ExchangeAcronym: "NYSE", Price: 63.74, Volume: 11873491, Type: models.ListingTypeStock, OutstandingShares: 4320000000, DividendYield: 0.0310},
		{Ticker: "SAP", Name: "SAP SE", ExchangeAcronym: "XETRA", Price: 178.26, Volume: 3124876, Type: models.ListingTypeStock, OutstandingShares: 1229000000, DividendYield: 0.0120},
		{Ticker: "BMW", Name: "BMW AG", ExchangeAcronym: "XETRA", Price: 108.44, Volume: 2987461, Type: models.ListingTypeStock, OutstandingShares: 601000000, DividendYield: 0.0540},
		{Ticker: "AIR", Name: "Airbus SE", ExchangeAcronym: "EPA", Price: 161.58, Volume: 2251874, Type: models.ListingTypeStock, OutstandingShares: 783000000, DividendYield: 0.0130},
		{Ticker: "VOD", Name: "Vodafone Group Plc", ExchangeAcronym: "LSE", Price: 71.18, Volume: 18743122, Type: models.ListingTypeStock, OutstandingShares: 26700000000, DividendYield: 0.0560},
		{Ticker: "SONY", Name: "Sony Group Corp.", ExchangeAcronym: "TSE", Price: 13180.0, Volume: 4287193, Type: models.ListingTypeStock, OutstandingShares: 1261000000, DividendYield: 0.0060},
	}
}

func seedForexListings() []listingSeed {
	return []listingSeed{
		{Ticker: "EUR/USD", Name: "Euro / US Dollar", ExchangeAcronym: "NASDAQ", Price: 1.0821, Volume: 450000000, Type: models.ListingTypeForex, BaseCurrency: "EUR", QuoteCurrency: "USD", Liquidity: "High"},
		{Ticker: "GBP/USD", Name: "British Pound / US Dollar", ExchangeAcronym: "LSE", Price: 1.2634, Volume: 350000000, Type: models.ListingTypeForex, BaseCurrency: "GBP", QuoteCurrency: "USD", Liquidity: "High"},
		{Ticker: "USD/JPY", Name: "US Dollar / Japanese Yen", ExchangeAcronym: "TSE", Price: 151.42, Volume: 300000000, Type: models.ListingTypeForex, BaseCurrency: "USD", QuoteCurrency: "JPY", Liquidity: "High"},
		{Ticker: "USD/CHF", Name: "US Dollar / Swiss Franc", ExchangeAcronym: "XETRA", Price: 0.8843, Volume: 120000000, Type: models.ListingTypeForex, BaseCurrency: "USD", QuoteCurrency: "CHF", Liquidity: "Medium"},
		{Ticker: "AUD/USD", Name: "Australian Dollar / US Dollar", ExchangeAcronym: "NASDAQ", Price: 0.6532, Volume: 150000000, Type: models.ListingTypeForex, BaseCurrency: "AUD", QuoteCurrency: "USD", Liquidity: "Medium"},
		{Ticker: "EUR/GBP", Name: "Euro / British Pound", ExchangeAcronym: "LSE", Price: 0.8566, Volume: 90000000, Type: models.ListingTypeForex, BaseCurrency: "EUR", QuoteCurrency: "GBP", Liquidity: "Medium"},
	}
}

func seedFuturesListings() []listingSeed {
	return []listingSeed{
		{Ticker: "CLN26", Name: "Crude Oil Jul 2026", ExchangeAcronym: "NYSE", Price: 78.45, Volume: 450000, Type: models.ListingTypeFutures, ContractSize: 1000, ContractUnit: "Barrel", SettlementDate: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)},
		{Ticker: "CLQ26", Name: "Crude Oil Aug 2026", ExchangeAcronym: "NYSE", Price: 77.92, Volume: 320000, Type: models.ListingTypeFutures, ContractSize: 1000, ContractUnit: "Barrel", SettlementDate: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)},
		{Ticker: "GCQ26", Name: "Gold Aug 2026", ExchangeAcronym: "NYSE", Price: 2342.10, Volume: 180000, Type: models.ListingTypeFutures, ContractSize: 100, ContractUnit: "Troy Ounce", SettlementDate: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)},
		{Ticker: "SIN26", Name: "Silver Jul 2026", ExchangeAcronym: "NYSE", Price: 29.85, Volume: 75000, Type: models.ListingTypeFutures, ContractSize: 5000, ContractUnit: "Troy Ounce", SettlementDate: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)},
		{Ticker: "NGN26", Name: "Natural Gas Jul 2026", ExchangeAcronym: "NYSE", Price: 2.68, Volume: 250000, Type: models.ListingTypeFutures, ContractSize: 10000, ContractUnit: "MMBtu", SettlementDate: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)},
		{Ticker: "ZWN26", Name: "Wheat Jul 2026", ExchangeAcronym: "NYSE", Price: 5.72, Volume: 95000, Type: models.ListingTypeFutures, ContractSize: 5000, ContractUnit: "Bushel", SettlementDate: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)},
	}
}

func seedOptions(db *gorm.DB, referenceTime time.Time) error {
	// Find all stock listings
	var stocks []models.MarketListingRecord
	if err := db.Where("type = ?", "stock").Find(&stocks).Error; err != nil {
		return err
	}

	// Generate expiration dates per spec:
	// First date = 6 days from reference, then every 6 days until 30 days,
	// then 6 more dates with 30-day spacing
	expirationDates := generateExpirationDates(referenceTime)

	optionCount := 0
	for _, stock := range stocks {
		// Round price to nearest integer for strike prices
		baseStrike := math.Round(stock.Price)

		// 2 strikes above and below, plus the center
		strikes := make([]float64, 0, 5)
		for i := -2; i <= 2; i++ {
			strikes = append(strikes, baseStrike+float64(i))
		}

		for _, expDate := range expirationDates {
			for _, strike := range strikes {
				for _, optionType := range []string{"call", "put"} {
					// Build ticker: TICKER + YYMMDD + C/P + strike in cents (8 digits)
					typeChar := "C"
					if optionType == "put" {
						typeChar = "P"
					}
					strikeCents := int64(strike * 100)
					optionTicker := fmt.Sprintf("%s%s%s%08d",
						stock.Ticker,
						expDate.Format("060102"),
						typeChar,
						strikeCents,
					)

					optionName := fmt.Sprintf("%s %s %.0f %s",
						stock.Ticker,
						expDate.Format("Jan 02 2006"),
						strike,
						strings.ToUpper(optionType),
					)

					// Deterministic pricing from seed
					seed := float64(hashTicker(optionTicker)%100) / 100.0
					premium := math.Abs(stock.Price-strike)*0.3 + stock.Price*0.02*(1+seed*0.5)
					premium = round2(premium)
					if premium < 0.01 {
						premium = 0.01
					}

					optionListing := models.MarketListingRecord{
						Ticker:      optionTicker,
						Name:        optionName,
						ExchangeID:  stock.ExchangeID,
						LastRefresh: referenceTime,
						Price:       premium,
						Ask:         round2(premium * 1.05),
						Bid:         round2(premium * 0.95),
						Volume:      int64(100 + hashTicker(optionTicker)%5000),
						Type:        string(models.ListingTypeOption),
					}

					if err := db.Where("ticker = ?", optionTicker).
						Assign(optionListing).
						FirstOrCreate(&optionListing).Error; err != nil {
						return err
					}

					optionRecord := models.OptionRecord{
						ListingID:         optionListing.ID,
						StockListingID:    stock.ID,
						OptionType:        optionType,
						StrikePrice:       strike,
						ImpliedVolatility: 1.0,
						OpenInterest:      int64(100 + hashTicker(optionTicker)%10000),
						SettlementDate:    expDate,
					}

					if err := db.Where("listing_id = ?", optionListing.ID).
						Assign(optionRecord).
						FirstOrCreate(&optionRecord).Error; err != nil {
						return err
					}

					optionCount++
				}
			}
		}
	}

	slog.Info("Options seed complete", "options", optionCount, "stocks", len(stocks))
	return nil
}

func generateExpirationDates(reference time.Time) []time.Time {
	// 3 expiry dates: ~1 week, ~1 month, ~2 months out
	return []time.Time{
		reference.AddDate(0, 0, 7),
		reference.AddDate(0, 1, 0),
		reference.AddDate(0, 2, 0),
	}
}

func buildSeedHistory(ticker string, currentPrice float64, volume int64) []models.ListingDailyPriceInfo {
	seed := float64(hashTicker(ticker)%17 + 3)
	referenceDate := seedReferenceTime()
	history := make([]models.ListingDailyPriceInfo, 0, 30)

	var previous float64
	for dayOffset := 29; dayOffset >= 0; dayOffset-- {
		date := time.Date(referenceDate.Year(), referenceDate.Month(), referenceDate.Day(), 0, 0, 0, 0, time.UTC).
			AddDate(0, 0, -dayOffset)
		drift := math.Sin(float64(dayOffset+1)/4.0+seed/10.0) * 0.024
		step := (float64(29-dayOffset) * 0.0012) - 0.018
		price := round2(currentPrice * (1 + drift + step))
		high := round2(price * (1 + 0.008 + seed/1000))
		low := round2(price * (1 - 0.008 - seed/1200))
		dayVolume := int64(math.Round(float64(volume) * (0.78 + math.Mod(seed, 5)/10 + float64(dayOffset%4)/40)))
		change := 0.0
		if previous > 0 {
			change = round2(price - previous)
		}
		previous = price

		history = append(history, models.ListingDailyPriceInfo{
			Date:   date,
			Price:  price,
			High:   high,
			Low:    low,
			Change: change,
			Volume: dayVolume,
		})
	}

	return history
}

func hashTicker(ticker string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(ticker))
	return h.Sum32()
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}
