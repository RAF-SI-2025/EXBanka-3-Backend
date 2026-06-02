package repository

import (
	"errors"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

var (
	// ErrTickerNotFound is returned when a ticker doesn't exist in market_listings.
	ErrTickerNotFound = errors.New("ticker not found in market listings")
	// ErrDuplicateItem is returned when the ticker is already on the watchlist.
	ErrDuplicateItem = errors.New("ticker already on this watchlist")
)

// WatchlistItemView is the enriched read model returned by GetItems.
// It joins market_listings and the most recent daily price info.
type WatchlistItemView struct {
	ID      uint      `json:"id"`
	Ticker  string    `json:"ticker"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Price   float64   `json:"price"`
	Change  float64   `json:"change"`
	Volume  int64     `json:"volume"`
	AddedAt time.Time `json:"added_at"`
}

type WatchlistRepository struct {
	db *gorm.DB
}

func NewWatchlistRepository(db *gorm.DB) *WatchlistRepository {
	return &WatchlistRepository{db: db}
}

// Create inserts a new watchlist for a user.
func (r *WatchlistRepository) Create(w *models.Watchlist) error {
	return r.db.Create(w).Error
}

// ListByUser returns all watchlists owned by the given user.
func (r *WatchlistRepository) ListByUser(userID uint, userType string) ([]models.Watchlist, error) {
	var list []models.Watchlist
	err := r.db.Where("user_id = ? AND user_type = ?", userID, userType).Find(&list).Error
	return list, err
}

// GetByID fetches a watchlist by primary key (without items).
func (r *WatchlistRepository) GetByID(id uint) (*models.Watchlist, error) {
	var w models.Watchlist
	if err := r.db.First(&w, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &w, nil
}

// Delete removes a watchlist and all its items.
// Items are deleted explicitly (before the parent) for SQLite FK cascade compatibility.
func (r *WatchlistRepository) Delete(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("watchlist_id = ?", id).Delete(&models.WatchlistItem{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Watchlist{}, id).Error
	})
}

// AddItem adds a ticker to a watchlist.
// Returns ErrTickerNotFound if the ticker is not in market_listings.
// Returns ErrDuplicateItem if the ticker is already on the watchlist.
func (r *WatchlistRepository) AddItem(watchlistID uint, ticker string) (*models.WatchlistItem, error) {
	var count int64
	if err := r.db.Model(&models.MarketListingRecord{}).
		Where("ticker = ?", ticker).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrTickerNotFound
	}

	var existing models.WatchlistItem
	err := r.db.Where("watchlist_id = ? AND ticker = ?", watchlistID, ticker).
		First(&existing).Error
	if err == nil {
		return nil, ErrDuplicateItem
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	item := &models.WatchlistItem{
		WatchlistID: watchlistID,
		Ticker:      ticker,
		AddedAt:     time.Now().UTC(),
	}
	if err := r.db.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

// RemoveItem deletes a ticker from a watchlist. It is idempotent (no error if not found).
func (r *WatchlistRepository) RemoveItem(watchlistID uint, ticker string) error {
	return r.db.Where("watchlist_id = ? AND ticker = ?", watchlistID, ticker).
		Delete(&models.WatchlistItem{}).Error
}

// GetItems returns enriched items for a watchlist in a single query.
// Each item is joined with market_listings and the most recent daily price info.
func (r *WatchlistRepository) GetItems(watchlistID uint) ([]WatchlistItemView, error) {
	const query = `
		SELECT
			wi.id,
			wi.ticker,
			ml.name,
			ml.type,
			ml.price,
			COALESCE(h.change, 0) AS change,
			ml.volume,
			wi.added_at
		FROM watchlist_items wi
		INNER JOIN market_listings ml ON ml.ticker = wi.ticker
		LEFT JOIN market_listing_daily_price_infos h
			ON h.listing_id = ml.id
			AND h.date = (
				SELECT MAX(h2.date)
				FROM market_listing_daily_price_infos h2
				WHERE h2.listing_id = ml.id
			)
		WHERE wi.watchlist_id = ?
		ORDER BY wi.added_at ASC
	`
	var rows []WatchlistItemView
	if err := r.db.Raw(query, watchlistID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
