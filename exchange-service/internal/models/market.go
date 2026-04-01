package models

import "time"

type ListingType string

const (
	ListingTypeStock   ListingType = "stock"
	ListingTypeForex   ListingType = "forex"
	ListingTypeFutures ListingType = "futures"
	ListingTypeOption  ListingType = "option"
)

type Exchange struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Acronym        string `json:"acronym"`
	MICCode        string `json:"micCode"`
	Polity         string `json:"polity"`
	Currency       string `json:"currency"`
	Timezone       string `json:"timezone"`
	WorkingHours   string `json:"workingHours"`
	UseManualTime  bool   `json:"useManualTime"`
	ManualTimeOpen bool   `json:"manualTimeOpen"`
	Enabled        bool   `json:"enabled"`
}

type ExchangeSummary struct {
	Name     string `json:"name"`
	Acronym  string `json:"acronym"`
	MICCode  string `json:"micCode"`
	Currency string `json:"currency"`
}

type Listing struct {
	Ticker      string          `json:"ticker"`
	Name        string          `json:"name"`
	Exchange    ExchangeSummary `json:"exchange"`
	LastRefresh time.Time       `json:"lastRefresh"`
	Price       float64         `json:"price"`
	Ask         float64         `json:"ask"`
	Bid         float64         `json:"bid"`
	Volume      int64           `json:"volume"`
	Type        ListingType     `json:"type"`
}

type ListingDailyPriceInfo struct {
	Date   time.Time `json:"date"`
	Price  float64   `json:"price"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Change float64   `json:"change"`
	Volume int64     `json:"volume"`
}

type PortfolioOwnerType string

const (
	PortfolioOwnerTypeClient   PortfolioOwnerType = "client"
	PortfolioOwnerTypeEmployee PortfolioOwnerType = "employee"
)

type PortfolioModelType string

const (
	PortfolioModelTypeSprint4SeededReadOnly PortfolioModelType = "sprint4_seeded_read_model"
)

const (
	PortfolioPositionSourceDeterministicSeed = "deterministic_seed"
	PortfolioPricingSourceListingSnapshot    = "listing_snapshot"
	PortfolioValuationCurrencyMixed          = "MIXED"
)

type PortfolioItem struct {
	Ticker       string  `json:"ticker"`
	Name         string  `json:"name"`
	Exchange     string  `json:"exchange"`
	Currency     string  `json:"currency"`
	Quantity     float64 `json:"quantity"`
	AveragePrice float64 `json:"averagePrice"`
	CurrentPrice float64 `json:"currentPrice"`
	MarketValue  float64 `json:"marketValue"`
	PnL          float64 `json:"pnl"`
	PnLPercent   float64 `json:"pnlPercent"`
}

type Portfolio struct {
	OwnerID           uint               `json:"ownerId"`
	OwnerType         PortfolioOwnerType `json:"ownerType"`
	GeneratedAt       time.Time          `json:"generatedAt"`
	ValuationAsOf     time.Time          `json:"valuationAsOf"`
	ValuationCurrency string             `json:"valuationCurrency"`
	EstimatedValue    float64            `json:"estimatedValue"`
	UnrealizedPnL     float64            `json:"unrealizedPnL"`
	PositionCount     int                `json:"positionCount"`
	ReadOnly          bool               `json:"readOnly"`
	ModelType         PortfolioModelType `json:"modelType"`
	PositionSource    string             `json:"positionSource"`
	PricingSource     string             `json:"pricingSource"`
	Items             []PortfolioItem    `json:"items"`
}
