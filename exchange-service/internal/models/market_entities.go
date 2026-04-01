package models

import "time"

type MarketExchangeRecord struct {
	ID             uint                  `gorm:"primaryKey"`
	Name           string                `gorm:"not null"`
	Acronym        string                `gorm:"not null;uniqueIndex"`
	MICCode        string                `gorm:"column:mic_code;not null;uniqueIndex"`
	Polity         string                `gorm:"not null"`
	Currency       string                `gorm:"not null"`
	Timezone       string                `gorm:"not null"`
	WorkingHours   string                `gorm:"column:working_hours;not null"`
	UseManualTime  bool                  `gorm:"column:use_manual_time;not null;default:false"`
	ManualTimeOpen bool                  `gorm:"column:manual_time_open;not null;default:false"`
	Enabled        bool                  `gorm:"not null;default:true"`
	Listings       []MarketListingRecord `gorm:"foreignKey:ExchangeID"`
	WorkingDays    []ExchangeWorkingDayRecord `gorm:"foreignKey:ExchangeID"`
}

func (MarketExchangeRecord) TableName() string {
	return "market_exchanges"
}

func (r MarketExchangeRecord) ToDomain() Exchange {
	return Exchange{
		ID:             r.ID,
		Name:           r.Name,
		Acronym:        r.Acronym,
		MICCode:        r.MICCode,
		Polity:         r.Polity,
		Currency:       r.Currency,
		Timezone:       r.Timezone,
		WorkingHours:   r.WorkingHours,
		UseManualTime:  r.UseManualTime,
		ManualTimeOpen: r.ManualTimeOpen,
		Enabled:        r.Enabled,
	}
}

func (r MarketExchangeRecord) ToSummary() ExchangeSummary {
	return ExchangeSummary{
		Name:     r.Name,
		Acronym:  r.Acronym,
		MICCode:  r.MICCode,
		Currency: r.Currency,
	}
}

type MarketListingRecord struct {
	ID          uint                                `gorm:"primaryKey"`
	Ticker      string                              `gorm:"not null;uniqueIndex"`
	Name        string                              `gorm:"not null"`
	ExchangeID  uint                                `gorm:"column:exchange_id;not null;index"`
	Exchange    MarketExchangeRecord                `gorm:"foreignKey:ExchangeID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	LastRefresh time.Time                           `gorm:"column:last_refresh;not null"`
	Price       float64                             `gorm:"not null"`
	Ask         float64                             `gorm:"not null"`
	Bid         float64                             `gorm:"not null"`
	Volume      int64                               `gorm:"not null"`
	Type        string                              `gorm:"not null"`
	History     []MarketListingDailyPriceInfoRecord `gorm:"foreignKey:ListingID"`
}

func (MarketListingRecord) TableName() string {
	return "market_listings"
}

func (r MarketListingRecord) ToDomain() Listing {
	return Listing{
		Ticker:      r.Ticker,
		Name:        r.Name,
		Exchange:    r.Exchange.ToSummary(),
		LastRefresh: r.LastRefresh,
		Price:       r.Price,
		Ask:         r.Ask,
		Bid:         r.Bid,
		Volume:      r.Volume,
		Type:        ListingType(r.Type),
	}
}

type MarketListingDailyPriceInfoRecord struct {
	ID        uint                `gorm:"primaryKey"`
	ListingID uint                `gorm:"column:listing_id;not null;index;uniqueIndex:idx_market_listing_date"`
	Listing   MarketListingRecord `gorm:"foreignKey:ListingID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Date      time.Time           `gorm:"type:date;not null;uniqueIndex:idx_market_listing_date"`
	Price     float64             `gorm:"not null"`
	High      float64             `gorm:"not null"`
	Low       float64             `gorm:"not null"`
	Change    float64             `gorm:"not null"`
	Volume    int64               `gorm:"not null"`
}

func (MarketListingDailyPriceInfoRecord) TableName() string {
	return "market_listing_daily_price_infos"
}

func (r MarketListingDailyPriceInfoRecord) ToDomain() ListingDailyPriceInfo {
	return ListingDailyPriceInfo{
		Date:   r.Date,
		Price:  r.Price,
		High:   r.High,
		Low:    r.Low,
		Change: r.Change,
		Volume: r.Volume,
	}
}

// ExchangeWorkingDayRecord stores specific working days for each exchange.
type ExchangeWorkingDayRecord struct {
	ID         uint                 `gorm:"primaryKey"`
	ExchangeID uint                 `gorm:"column:exchange_id;not null;index;uniqueIndex:idx_exchange_date"`
	Exchange   MarketExchangeRecord `gorm:"foreignKey:ExchangeID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Date       time.Time            `gorm:"type:date;not null;uniqueIndex:idx_exchange_date"`
}

func (ExchangeWorkingDayRecord) TableName() string {
	return "exchange_working_days"
}

// Listing subtype records

type StockRecord struct {
	ID                uint                `gorm:"primaryKey"`
	ListingID         uint                `gorm:"column:listing_id;not null;uniqueIndex"`
	Listing           MarketListingRecord `gorm:"foreignKey:ListingID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	OutstandingShares int64               `gorm:"column:outstanding_shares;not null;default:0"`
	DividendYield     float64             `gorm:"column:dividend_yield;not null;default:0"`
}

func (StockRecord) TableName() string { return "stocks" }

type ForexPairRecord struct {
	ID            uint                `gorm:"primaryKey"`
	ListingID     uint                `gorm:"column:listing_id;not null;uniqueIndex"`
	Listing       MarketListingRecord `gorm:"foreignKey:ListingID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	BaseCurrency  string              `gorm:"column:base_currency;not null"`
	QuoteCurrency string              `gorm:"column:quote_currency;not null"`
	Liquidity     string              `gorm:"not null;default:'Medium'"`
}

func (ForexPairRecord) TableName() string { return "forex_pairs" }

type FuturesContractRecord struct {
	ID             uint                `gorm:"primaryKey"`
	ListingID      uint                `gorm:"column:listing_id;not null;uniqueIndex"`
	Listing        MarketListingRecord `gorm:"foreignKey:ListingID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ContractSize   int64               `gorm:"column:contract_size;not null"`
	ContractUnit   string              `gorm:"column:contract_unit;not null"`
	SettlementDate time.Time           `gorm:"column:settlement_date;type:date;not null"`
}

func (FuturesContractRecord) TableName() string { return "futures_contracts" }

type OptionRecord struct {
	ID                uint                `gorm:"primaryKey"`
	ListingID         uint                `gorm:"column:listing_id;not null;uniqueIndex"`
	Listing           MarketListingRecord `gorm:"foreignKey:ListingID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	StockListingID    uint                `gorm:"column:stock_listing_id;not null;index"`
	StockListing      MarketListingRecord `gorm:"foreignKey:StockListingID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	OptionType        string              `gorm:"column:option_type;not null"` // "call" or "put"
	StrikePrice       float64             `gorm:"column:strike_price;not null"`
	ImpliedVolatility float64             `gorm:"column:implied_volatility;not null;default:1"`
	OpenInterest      int64               `gorm:"column:open_interest;not null;default:0"`
	SettlementDate    time.Time           `gorm:"column:settlement_date;type:date;not null"`
}

func (OptionRecord) TableName() string { return "options" }

// Order + OrderTransaction records

type OrderRecord struct {
	ID                uint                     `gorm:"primaryKey"`
	UserID            uint                     `gorm:"column:user_id;not null;index"`
	UserType          string                   `gorm:"column:user_type;not null"` // "client" or "employee"
	AssetID           uint                     `gorm:"column:asset_id;not null;index"`
	Asset             MarketListingRecord      `gorm:"foreignKey:AssetID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	OrderType         string                   `gorm:"column:order_type;not null"` // market, limit, stop, stop_limit
	Direction         string                   `gorm:"not null"`                   // buy, sell
	Quantity          int64                    `gorm:"not null"`
	ContractSize      int64                    `gorm:"column:contract_size;not null;default:1"`
	PricePerUnit      float64                  `gorm:"column:price_per_unit;not null"`
	LimitValue        *float64                 `gorm:"column:limit_value"`
	StopValue         *float64                 `gorm:"column:stop_value"`
	IsAON             bool                     `gorm:"column:is_aon;not null;default:false"`
	IsMargin          bool                     `gorm:"column:is_margin;not null;default:false"`
	Status            string                   `gorm:"not null;default:'pending'"` // pending, approved, declined, done
	ApprovedBy        *uint                    `gorm:"column:approved_by"`
	IsDone            bool                     `gorm:"column:is_done;not null;default:false"`
	RemainingPortions int64                    `gorm:"column:remaining_portions;not null"`
	AfterHours        bool                     `gorm:"column:after_hours;not null;default:false"`
	AccountID         uint                     `gorm:"column:account_id;not null"`
	LastModification  time.Time                `gorm:"column:last_modification;not null"`
	CreatedAt         time.Time                `gorm:"not null"`
	Transactions      []OrderTransactionRecord `gorm:"foreignKey:OrderID"`
}

func (OrderRecord) TableName() string { return "orders" }

type OrderTransactionRecord struct {
	ID           uint      `gorm:"primaryKey"`
	OrderID      uint      `gorm:"column:order_id;not null;index"`
	Order        OrderRecord `gorm:"foreignKey:OrderID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Quantity     int64     `gorm:"not null"`
	PricePerUnit float64   `gorm:"column:price_per_unit;not null"`
	ExecutedAt   time.Time `gorm:"column:executed_at;not null"`
}

func (OrderTransactionRecord) TableName() string { return "order_transactions" }
