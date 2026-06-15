package models

import "time"

// DividendPayoutRecord is one quarterly dividend paid to one holder of one
// stock. The (asset_id, user_id, user_type, period) unique index makes the
// payout cron idempotent — re-running for the same quarter never double-pays.
//
// GrossAmount is in the stock's listing currency. CreditedAmount/CreditedCurrency
// reflect what actually hit the account: normally the same as gross, but when the
// stock's account is gone the payout may be converted to RSD (§Celina 3). TaxRSD
// is the 15% capital-gains tax recorded for client holders (zero for bank-owned
// actuary holdings, which feed bank profit untaxed).
type DividendPayoutRecord struct {
	ID               uint      `gorm:"primaryKey"`
	AssetID          uint      `gorm:"column:asset_id;not null;index;uniqueIndex:idx_dividend_asset_user_period"`
	Ticker           string    `gorm:"not null"`
	UserID           uint      `gorm:"column:user_id;not null;index;uniqueIndex:idx_dividend_asset_user_period"`
	UserType         string    `gorm:"column:user_type;not null;uniqueIndex:idx_dividend_asset_user_period"` // "client" | "bank"
	AccountID        uint      `gorm:"column:account_id;not null"`
	Quantity         float64   `gorm:"not null"`
	PricePerShare    float64   `gorm:"column:price_per_share;not null"`
	DividendYield    float64   `gorm:"column:dividend_yield;not null"`
	Currency         string    `gorm:"not null"`
	GrossAmount      float64   `gorm:"column:gross_amount;not null"`
	CreditedAmount   float64   `gorm:"column:credited_amount;not null"`
	CreditedCurrency string    `gorm:"column:credited_currency;not null"`
	TaxRSD           float64   `gorm:"column:tax_rsd;not null;default:0"`
	Period           string    `gorm:"not null;index;uniqueIndex:idx_dividend_asset_user_period"` // "2026-Q2"
	PaidAt           time.Time `gorm:"column:paid_at;not null"`
	CreatedAt        time.Time
}

func (DividendPayoutRecord) TableName() string { return "dividend_payouts" }
