package repository

import (
	"errors"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MarketRepository struct {
	db *gorm.DB
}

func NewMarketRepository(db *gorm.DB) *MarketRepository {
	return &MarketRepository{db: db}
}

func (r *MarketRepository) ListExchanges() ([]models.Exchange, error) {
	var records []models.MarketExchangeRecord
	if err := r.db.Order("acronym ASC").Find(&records).Error; err != nil {
		return nil, err
	}

	items := make([]models.Exchange, 0, len(records))
	for _, record := range records {
		items = append(items, record.ToDomain())
	}
	return items, nil
}

func (r *MarketRepository) ListListings() ([]models.Listing, error) {
	var records []models.MarketListingRecord
	if err := r.db.Preload("Exchange").Order("ticker ASC").Find(&records).Error; err != nil {
		return nil, err
	}

	items := make([]models.Listing, 0, len(records))
	for _, record := range records {
		items = append(items, record.ToDomain())
	}
	return items, nil
}

func (r *MarketRepository) GetListing(ticker string) (*models.Listing, error) {
	var record models.MarketListingRecord
	if err := r.db.Preload("Exchange").Where("ticker = ?", ticker).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	listing := record.ToDomain()
	return &listing, nil
}

func (r *MarketRepository) GetListingsByTickers(tickers []string) (map[string]models.Listing, error) {
	if len(tickers) == 0 {
		return map[string]models.Listing{}, nil
	}

	var records []models.MarketListingRecord
	if err := r.db.Preload("Exchange").Where("ticker IN ?", tickers).Find(&records).Error; err != nil {
		return nil, err
	}

	items := make(map[string]models.Listing, len(records))
	for _, record := range records {
		items[record.Ticker] = record.ToDomain()
	}
	return items, nil
}

func (r *MarketRepository) GetExchangeByAcronym(acronym string) (*models.MarketExchangeRecord, error) {
	var record models.MarketExchangeRecord
	if err := r.db.Where("acronym = ?", acronym).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *MarketRepository) ToggleExchangeManualTime(acronym string, useManual bool, manualOpen bool) error {
	return r.db.Model(&models.MarketExchangeRecord{}).
		Where("acronym = ?", acronym).
		Updates(map[string]interface{}{
			"use_manual_time":  useManual,
			"manual_time_open": manualOpen,
		}).Error
}

func (r *MarketRepository) ListListingsByType(listingType string) ([]models.MarketListingRecord, error) {
	var records []models.MarketListingRecord
	if err := r.db.Preload("Exchange").Where("type = ?", listingType).Order("ticker ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *MarketRepository) GetStockByListingID(listingID uint) (*models.StockRecord, error) {
	var record models.StockRecord
	if err := r.db.Where("listing_id = ?", listingID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *MarketRepository) GetForexByListingID(listingID uint) (*models.ForexPairRecord, error) {
	var record models.ForexPairRecord
	if err := r.db.Where("listing_id = ?", listingID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *MarketRepository) GetFuturesByListingID(listingID uint) (*models.FuturesContractRecord, error) {
	var record models.FuturesContractRecord
	if err := r.db.Where("listing_id = ?", listingID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *MarketRepository) GetOptionsByStockListingID(stockListingID uint) ([]models.OptionRecord, error) {
	var records []models.OptionRecord
	if err := r.db.Preload("Listing").Where("stock_listing_id = ?", stockListingID).
		Order("settlement_date ASC, strike_price ASC, option_type ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// GetListingRecordByID returns a market listing by primary key.
func (r *MarketRepository) GetListingRecordByID(id uint) (*models.MarketListingRecord, error) {
	var record models.MarketListingRecord
	if err := r.db.Preload("Exchange").First(&record, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// GetOptionByListingID returns the OptionRecord whose own listing_id matches
// (i.e. the option contract's listing, not its underlying stock listing).
func (r *MarketRepository) GetOptionByListingID(listingID uint) (*models.OptionRecord, error) {
	var record models.OptionRecord
	if err := r.db.Preload("Listing").Where("listing_id = ?", listingID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// GetForexRate returns the current exchange rate from baseCurrency to quoteCurrency
// by looking up the corresponding forex pair listing price.
// Returns 0, nil when no matching pair exists in the database.
func (r *MarketRepository) GetForexRate(baseCurrency, quoteCurrency string) (float64, error) {
	// Direct pair: base→quote
	row := r.db.Table("forex_pairs").
		Select("market_listings.price").
		Joins("JOIN market_listings ON market_listings.id = forex_pairs.listing_id").
		Where("forex_pairs.base_currency = ? AND forex_pairs.quote_currency = ?", baseCurrency, quoteCurrency).
		Limit(1).Row()
	var price float64
	if err := row.Scan(&price); err == nil && price > 0 {
		return price, nil
	}
	// Inverse pair: quote→base (1/price)
	row2 := r.db.Table("forex_pairs").
		Select("market_listings.price").
		Joins("JOIN market_listings ON market_listings.id = forex_pairs.listing_id").
		Where("forex_pairs.base_currency = ? AND forex_pairs.quote_currency = ?", quoteCurrency, baseCurrency).
		Limit(1).Row()
	var invPrice float64
	if err := row2.Scan(&invPrice); err == nil && invPrice > 0 {
		return 1.0 / invPrice, nil
	}
	return 0, nil // no pair found
}

func (r *MarketRepository) GetListingRecordByTicker(ticker string) (*models.MarketListingRecord, error) {
	var record models.MarketListingRecord
	if err := r.db.Preload("Exchange").Where("ticker = ?", ticker).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// EnsureForeignListing returns the local listing for ticker, creating a
// synthetic one if it does not exist. Cross-bank option contracts can
// reference tickers we don't list locally (the §public-stock feed
// carries tickers only, no prices), so when a buyer exercises such a
// contract we still need a market_listings row — portfolio_holdings.
// asset_id FKs to it. Price/ask/bid are seeded from the caller-supplied
// value (the option strike), the exchange is chosen by currency, and
// volume is left 0. The insert is idempotent on ticker so concurrent
// exercises of the same foreign asset don't collide.
func (r *MarketRepository) EnsureForeignListing(ticker, currency string, seedPrice float64) (*models.MarketListingRecord, error) {
	if existing, err := r.GetListingRecordByTicker(ticker); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	exchangeID, err := r.pickExchangeForCurrency(currency)
	if err != nil {
		return nil, err
	}

	row := &models.MarketListingRecord{
		Ticker:      ticker,
		Name:        ticker,
		ExchangeID:  exchangeID,
		LastRefresh: time.Now().UTC(),
		Price:       seedPrice,
		Ask:         seedPrice,
		Bid:         seedPrice,
		Volume:      0,
		Type:        "stock",
	}
	if err := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "ticker"}},
		DoNothing: true,
	}).Create(row).Error; err != nil {
		return nil, err
	}
	if row.ID == 0 {
		// A concurrent exercise inserted it first; return that row.
		return r.GetListingRecordByTicker(ticker)
	}
	return row, nil
}

// pickExchangeForCurrency returns the id of an enabled exchange whose
// currency matches, falling back to any enabled exchange. Used to anchor
// synthetic foreign listings to a plausible venue.
func (r *MarketRepository) pickExchangeForCurrency(currency string) (uint, error) {
	var ex models.MarketExchangeRecord
	err := r.db.Where("enabled = ? AND currency = ?", true, currency).Order("id ASC").First(&ex).Error
	if err == nil {
		return ex.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	if err := r.db.Where("enabled = ?", true).Order("id ASC").First(&ex).Error; err != nil {
		return 0, err
	}
	return ex.ID, nil
}

func (r *MarketRepository) GetHistory(ticker string) ([]models.ListingDailyPriceInfo, error) {
	var listing models.MarketListingRecord
	if err := r.db.Where("ticker = ?", ticker).First(&listing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var records []models.MarketListingDailyPriceInfoRecord
	if err := r.db.Where("listing_id = ?", listing.ID).Order("date ASC").Find(&records).Error; err != nil {
		return nil, err
	}

	items := make([]models.ListingDailyPriceInfo, 0, len(records))
	for _, record := range records {
		items = append(items, record.ToDomain())
	}
	return items, nil
}
