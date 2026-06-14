package models

import "time"

type MarketExchangeRecord struct {
	ID             uint                       `gorm:"primaryKey"`
	Name           string                     `gorm:"not null"`
	Acronym        string                     `gorm:"not null;uniqueIndex"`
	MICCode        string                     `gorm:"column:mic_code;not null;uniqueIndex"`
	Polity         string                     `gorm:"not null"`
	Currency       string                     `gorm:"not null"`
	Timezone       string                     `gorm:"not null"`
	WorkingHours   string                     `gorm:"column:working_hours;not null"`
	UseManualTime  bool                       `gorm:"column:use_manual_time;not null;default:false"`
	ManualTimeOpen bool                       `gorm:"column:manual_time_open;not null;default:false"`
	Enabled        bool                       `gorm:"not null;default:true"`
	Listings       []MarketListingRecord      `gorm:"foreignKey:ExchangeID"`
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
	UserType          string                   `gorm:"column:user_type;not null"` // "client" or "bank" (all employees act on behalf of the bank)
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
	MarginLoan        float64                  `gorm:"column:margin_loan;not null;default:0"` // outstanding bank-fronted balance for margin buys, in the account's currency
	StopTriggered     bool                     `gorm:"column:stop_triggered;not null;default:false"` // stop_limit latch: true once the stop value has been crossed
	Status            string                   `gorm:"not null;default:'pending'"` // pending, approved, declined, done
	ApprovedBy        *uint                    `gorm:"column:approved_by"`
	PlacedBy          *uint                    `gorm:"column:placed_by"` // employee who placed the order on the bank's behalf (nil for client orders)
	IsDone            bool                     `gorm:"column:is_done;not null;default:false"`
	RemainingPortions int64                    `gorm:"column:remaining_portions;not null"`
	Commission        float64                  `gorm:"column:commission;not null;default:0"`
	CurrencyRate      float64                  `gorm:"column:currency_rate;not null;default:1"` // rate from asset currency to account currency at order creation
	AfterHours        bool                     `gorm:"column:after_hours;not null;default:false"`
	AccountID         uint                     `gorm:"column:account_id;not null"`
	// TotalPaid is the amount actually debited from the user's account at order
	// creation, in the account's currency. Non-margin: (totalPrice + commission) × rate.
	// Margin: just the Initial Margin Cost (the rest is bank-fronted loan).
	// Zero on sell orders (no upfront debit).
	TotalPaid float64 `gorm:"column:total_paid;not null;default:0"`
	// BalanceBefore / BalanceAfter snapshot the account balance immediately
	// before and after the buy debit. Both zero for sell orders.
	BalanceBefore    float64                  `gorm:"column:balance_before;not null;default:0"`
	BalanceAfter     float64                  `gorm:"column:balance_after;not null;default:0"`
	LastModification time.Time                `gorm:"column:last_modification;not null"`
	CreatedAt        time.Time                `gorm:"not null"`
	Transactions     []OrderTransactionRecord `gorm:"foreignKey:OrderID"`
	// Notify* capture the human who placed the order (client, or the employee
	// acting for the bank/fund) so order-lifecycle notifications reach a real
	// inbox and in-app bell. Set once at creation from the caller's JWT.
	NotifyUserID   uint   `gorm:"column:notify_user_id"`
	NotifyUserType string `gorm:"column:notify_user_type"` // "client" | "employee"
	NotifyEmail    string `gorm:"column:notify_email"`
}

func (OrderRecord) TableName() string { return "orders" }

type OrderTransactionRecord struct {
	ID           uint        `gorm:"primaryKey"`
	OrderID      uint        `gorm:"column:order_id;not null;index"`
	Order        OrderRecord `gorm:"foreignKey:OrderID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Quantity     int64       `gorm:"not null"`
	PricePerUnit float64     `gorm:"column:price_per_unit;not null"`
	ExecutedAt   time.Time   `gorm:"column:executed_at;not null"`
}

func (OrderTransactionRecord) TableName() string { return "order_transactions" }

// Portfolio holdings — persistent, built from executed order transactions.

type PortfolioHoldingRecord struct {
	ID               uint                `gorm:"primaryKey"`
	UserID           uint                `gorm:"column:user_id;not null;index:idx_portfolio_user_asset"`
	UserType         string              `gorm:"column:user_type;not null"` // "client" or "bank" (all employees share the bank-owned portfolio)
	AssetID          uint                `gorm:"column:asset_id;not null;index:idx_portfolio_user_asset"`
	Asset            MarketListingRecord `gorm:"foreignKey:AssetID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	Quantity         float64             `gorm:"not null;default:0"`
	AvgBuyPrice      float64             `gorm:"column:avg_buy_price;not null;default:0"`
	IsPublic         bool                `gorm:"column:is_public;not null;default:false"`
	PublicQuantity   float64             `gorm:"column:public_quantity;not null;default:0"`
	ReservedQuantity float64             `gorm:"column:reserved_quantity;not null;default:0"`
	AccountID        uint                `gorm:"column:account_id;not null"`
	RealizedProfit   float64             `gorm:"column:realized_profit;not null;default:0"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (PortfolioHoldingRecord) TableName() string { return "portfolio_holdings" }

func (h PortfolioHoldingRecord) EffectivePublicQuantity() float64 {
	if h.PublicQuantity > 0 {
		return h.PublicQuantity
	}
	if h.IsPublic {
		return h.Quantity
	}
	return 0
}

func (h PortfolioHoldingRecord) AvailableForOTC() float64 {
	available := h.EffectivePublicQuantity() - h.ReservedQuantity
	if available < 0 {
		return 0
	}
	return available
}

const (
	OtcOfferStatusPending   = "pending"
	OtcOfferStatusAccepted  = "accepted"
	OtcOfferStatusDeclined  = "declined"
	OtcOfferStatusCancelled = "cancelled"
)

type OtcOfferRecord struct {
	ID              uint                   `gorm:"primaryKey"`
	StockListingID  uint                   `gorm:"column:stock_listing_id;not null;index"`
	StockListing    MarketListingRecord    `gorm:"foreignKey:StockListingID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	SellerHoldingID uint                   `gorm:"column:seller_holding_id;not null;index"`
	SellerHolding   PortfolioHoldingRecord `gorm:"foreignKey:SellerHoldingID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	Amount          float64                `gorm:"not null"`
	PricePerStock   float64                `gorm:"column:price_per_stock;not null"`
	SettlementDate  time.Time              `gorm:"column:settlement_date;type:date;not null"`
	Premium         float64                `gorm:"not null"`
	LastModified    time.Time              `gorm:"column:last_modified;not null"`
	ModifiedByID    uint                   `gorm:"column:modified_by_id;not null"`
	ModifiedByType  string                 `gorm:"column:modified_by_type;not null"`
	Status          string                 `gorm:"not null;default:'pending';index"`
	BuyerID         uint                   `gorm:"column:buyer_id;not null;index"`
	BuyerType       string                 `gorm:"column:buyer_type;not null"`
	BuyerAccountID  uint                   `gorm:"column:buyer_account_id;not null;index"`
	SellerID        uint                   `gorm:"column:seller_id;not null;index"`
	SellerType      string                 `gorm:"column:seller_type;not null"`
	SellerAccountID uint                   `gorm:"column:seller_account_id;not null;index"`
	BankID          *uint                  `gorm:"column:bank_id;index"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (OtcOfferRecord) TableName() string { return "otc_offers" }

const (
	OtcContractStatusValid     = "valid"
	OtcContractStatusExercised = "exercised"
	OtcContractStatusExpired   = "expired"
)

type OtcContractRecord struct {
	ID              uint                   `gorm:"primaryKey"`
	OfferID         *uint                  `gorm:"column:offer_id;index"`
	StockListingID  uint                   `gorm:"column:stock_listing_id;not null;index"`
	StockListing    MarketListingRecord    `gorm:"foreignKey:StockListingID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	SellerHoldingID uint                   `gorm:"column:seller_holding_id;not null;index"`
	SellerHolding   PortfolioHoldingRecord `gorm:"foreignKey:SellerHoldingID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	Amount          float64                `gorm:"not null"`
	StrikePrice     float64                `gorm:"column:strike_price;not null"`
	Premium         float64                `gorm:"not null"`
	SettlementDate  time.Time              `gorm:"column:settlement_date;type:date;not null;index"`
	BuyerID         uint                   `gorm:"column:buyer_id;not null;index"`
	BuyerType       string                 `gorm:"column:buyer_type;not null"`
	BuyerAccountID  uint                   `gorm:"column:buyer_account_id;not null;index"`
	SellerID        uint                   `gorm:"column:seller_id;not null;index"`
	SellerType      string                 `gorm:"column:seller_type;not null"`
	SellerAccountID uint                   `gorm:"column:seller_account_id;not null;index"`
	Status          string                 `gorm:"not null;default:'valid';index"`
	BankID          *uint                  `gorm:"column:bank_id;index"`
	// Snapshotted at exercise (or expiry) so historical P&L doesn't drift with the market.
	// Zero on still-valid contracts.
	ExercisedAtPrice float64 `gorm:"column:exercised_at_price;not null;default:0"`
	BuyerProfit      float64 `gorm:"column:buyer_profit;not null;default:0"`
	SellerProfit     float64 `gorm:"column:seller_profit;not null;default:0"`
	// ExpiryReminderSent dedups the "expires in N days" notification so the
	// daily/hourly reminder cron fires at most once per contract.
	ExpiryReminderSent bool `gorm:"column:expiry_reminder_sent;not null;default:false"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (OtcContractRecord) TableName() string { return "otc_contracts" }

// TaxRecord tracks capital gains tax (15%) owed per user per month.

type TaxRecord struct {
	ID        uint    `gorm:"primaryKey"`
	UserID    uint    `gorm:"column:user_id;not null;index:idx_tax_user_period"`
	UserType  string  `gorm:"column:user_type;not null"`
	AssetID   uint    `gorm:"column:asset_id;not null"`
	Period    string  `gorm:"not null;index:idx_tax_user_period"` // "YYYY-MM", e.g. "2026-04"
	ProfitRSD float64 `gorm:"column:profit_rsd;not null"`
	TaxRSD    float64 `gorm:"column:tax_rsd;not null"`   // 15% of profit_rsd
	Status    string  `gorm:"not null;default:'unpaid'"` // unpaid, paid
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (TaxRecord) TableName() string { return "tax_records" }
