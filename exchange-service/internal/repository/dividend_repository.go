package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

type DividendRepository struct {
	db *gorm.DB
}

func NewDividendRepository(db *gorm.DB) *DividendRepository {
	return &DividendRepository{db: db}
}

// DividendEligibleHolding is one stock position that earns a dividend this
// quarter: the holder, the account the stock was bought from, the held
// quantity, plus the current price, currency and dividend yield needed to
// compute the payout.
type DividendEligibleHolding struct {
	HoldingID     uint    `gorm:"column:holding_id"`
	UserID        uint    `gorm:"column:user_id"`
	UserType      string  `gorm:"column:user_type"`
	AssetID       uint    `gorm:"column:asset_id"`
	AccountID     uint    `gorm:"column:account_id"`
	Quantity      float64 `gorm:"column:quantity"`
	Ticker        string  `gorm:"column:ticker"`
	Price         float64 `gorm:"column:price"`
	Currency      string  `gorm:"column:currency"`
	DividendYield float64 `gorm:"column:dividend_yield"`
}

// ListDividendEligibleHoldings returns every positive stock holding whose
// underlying listing pays a dividend (dividend_yield > 0). Joins the price and
// currency from the listing/exchange and the yield from the stock subtype.
//
// Fund-owned holdings (user_type = "fund") are excluded: dividend inflow and
// reinvestment for funds is a separate Celina 4 mechanism with its own rules.
// This payout covers client holders and bank-owned (actuary) holdings.
func (r *DividendRepository) ListDividendEligibleHoldings() ([]DividendEligibleHolding, error) {
	var rows []DividendEligibleHolding
	err := r.db.Table("portfolio_holdings AS ph").
		Select(`ph.id AS holding_id, ph.user_id, ph.user_type, ph.asset_id,
			ph.account_id, ph.quantity, ml.ticker, ml.price,
			me.currency, s.dividend_yield`).
		Joins("JOIN market_listings ml ON ml.id = ph.asset_id AND ml.type = 'stock'").
		Joins("JOIN market_exchanges me ON me.id = ml.exchange_id").
		Joins("JOIN stocks s ON s.listing_id = ml.id AND s.dividend_yield > 0").
		Where("ph.quantity > 0 AND ph.user_type <> 'fund'").
		Order("ph.id ASC").
		Scan(&rows).Error
	return rows, err
}

// PayoutExists reports whether a payout has already been recorded for this
// (asset, holder, period) — the idempotency guard for re-runs of one quarter.
func (r *DividendRepository) PayoutExists(assetID, userID uint, userType, period string) (bool, error) {
	var count int64
	err := r.db.Model(&models.DividendPayoutRecord{}).
		Where("asset_id = ? AND user_id = ? AND user_type = ? AND period = ?",
			assetID, userID, userType, period).
		Count(&count).Error
	return count > 0, err
}

func (r *DividendRepository) CreatePayout(payout *models.DividendPayoutRecord) error {
	return r.db.Create(payout).Error
}

// ListPayoutsForUser returns a holder's dividend history, newest first.
// assetID is optional (0 = all assets) so the portfolio page can show either the
// whole history or just one position's dividends.
func (r *DividendRepository) ListPayoutsForUser(userID uint, userType string, assetID uint) ([]models.DividendPayoutRecord, error) {
	q := r.db.Where("user_id = ? AND user_type = ?", userID, userType)
	if assetID != 0 {
		q = q.Where("asset_id = ?", assetID)
	}
	var payouts []models.DividendPayoutRecord
	if err := q.Order("paid_at DESC, id DESC").Find(&payouts).Error; err != nil {
		return nil, err
	}
	return payouts, nil
}

// FindActiveAccountByCurrency returns an active account for the holder in the
// given currency, or 0 if none. Clients match on client_id; bank-owned holdings
// match a non-state firm account (EXBanka itself).
func (r *DividendRepository) FindActiveAccountByCurrency(userID uint, userType, currencyKod string) (uint, error) {
	q := r.db.Table("accounts").
		Select("accounts.id").
		Joins("JOIN currencies ON currencies.id = accounts.currency_id").
		Where("currencies.kod = ? AND accounts.status = 'aktivan'", currencyKod)
	if userType == "client" {
		q = q.Where("accounts.client_id = ?", userID)
	} else {
		q = q.Joins("JOIN firmas ON firmas.id = accounts.firma_id").
			Where("firmas.is_state = false")
	}
	var id uint
	err := q.Limit(1).Scan(&id).Error
	return id, err
}
