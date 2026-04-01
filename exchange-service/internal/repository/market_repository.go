package repository

import (
	"errors"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
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
