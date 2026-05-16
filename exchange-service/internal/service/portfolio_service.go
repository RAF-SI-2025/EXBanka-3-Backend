package service

import (
	"fmt"
	"math"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// optionContractSize is the number of underlying shares per option contract.
const optionContractSize = 100

// PortfolioService maintains PortfolioHoldingRecords in response to order fills
// and exposes P&L views over the live holdings.
type PortfolioService struct {
	portfolioRepo *repository.PortfolioRepository
	taxSvc        *TaxService
	marketRepo    *repository.MarketRepository
	orderRepo     *repository.OrderRepository
}

func NewPortfolioService(
	portfolioRepo *repository.PortfolioRepository,
	taxSvc *TaxService,
	marketRepo *repository.MarketRepository,
	orderRepo *repository.OrderRepository,
) *PortfolioService {
	return &PortfolioService{
		portfolioRepo: portfolioRepo,
		taxSvc:        taxSvc,
		marketRepo:    marketRepo,
		orderRepo:     orderRepo,
	}
}

// HoldingWithPnL enriches a PortfolioHoldingRecord with live unrealized P&L
// computed from the holding's preloaded Asset.Price.
type HoldingWithPnL struct {
	Holding          *models.PortfolioHoldingRecord
	CurrentPrice     float64
	UnrealizedPnL    float64 // (currentPrice - avgBuyPrice) * quantity
	UnrealizedPnLPct float64 // as a percentage of cost basis
	MarketValue      float64 // currentPrice * quantity
}

// RecordFill updates the portfolio after an order fill and creates a TaxRecord
// for any realised profit on a sell.
//
// Buy fill: upsert holding — increase quantity, recalculate weighted avg buy price.
// Sell fill: decrease quantity, accumulate realized profit, create TaxRecord if profit > 0.
//
// Effective quantity = fillQty × contractSize.
func (s *PortfolioService) RecordFill(order *models.OrderRecord, fillQty int64, fillPrice float64) error {
	effectiveQty := float64(fillQty) * float64(order.ContractSize)

	if order.Direction == "buy" {
		if err := s.portfolioRepo.RecordBuyFill(
			order.UserID, order.UserType, order.AssetID, order.AccountID,
			effectiveQty, fillPrice,
		); err != nil {
			return fmt.Errorf("portfolio: failed to record buy fill for order %d: %w", order.ID, err)
		}
		return nil
	}

	// Sell: decrease holding and accumulate realized profit.
	realizedProfit, err := s.portfolioRepo.RecordSellFill(
		order.UserID, order.UserType, order.AssetID, effectiveQty, fillPrice,
	)
	if err != nil {
		return fmt.Errorf("portfolio: failed to record sell fill for order %d: %w", order.ID, err)
	}

	// Delegate tax recording to TaxService which handles RSD conversion.
	// Non-fatal: tax failure does not block the fill.
	_ = s.taxSvc.RecordCapitalGainTax(
		order.UserID,
		order.UserType,
		order.AssetID,
		realizedProfit,
		order.Asset.Type,
		order.Asset.Exchange.Currency,
	)

	return nil
}

// ListHoldingsWithPnL returns all active holdings for a user enriched with
// live unrealized P&L computed from the preloaded Asset.Price.
func (s *PortfolioService) ListHoldingsWithPnL(userID uint, userType string) ([]HoldingWithPnL, error) {
	holdings, err := s.portfolioRepo.ListHoldingsForUser(userID, userType)
	if err != nil {
		return nil, err
	}

	result := make([]HoldingWithPnL, 0, len(holdings))
	for i := range holdings {
		s.ensureBuyAccountID(&holdings[i])
		result = append(result, computePnL(&holdings[i]))
	}
	return result, nil
}

// GetHoldingWithPnL returns a single holding enriched with live P&L.
func (s *PortfolioService) GetHoldingWithPnL(id uint) (*HoldingWithPnL, error) {
	h, err := s.portfolioRepo.GetHoldingByID(id)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, fmt.Errorf("holding not found")
	}
	s.ensureBuyAccountID(h)
	pnl := computePnL(h)
	return &pnl, nil
}

// ensureBuyAccountID backfills holding.AccountID from the most recent buy order
// when the column is empty (legacy rows created before account_id was tracked).
// Persists the backfill so future reads are cheap and the sell flow locks to a
// real account instead of falling back to the user's first listed account.
func (s *PortfolioService) ensureBuyAccountID(h *models.PortfolioHoldingRecord) {
	if h == nil || h.AccountID != 0 || s.orderRepo == nil {
		return
	}
	accountID, err := s.orderRepo.FindLatestBuyAccountID(h.UserID, h.UserType, h.AssetID)
	if err != nil || accountID == 0 {
		return
	}
	h.AccountID = accountID
	_ = s.portfolioRepo.SetHoldingAccountID(h.ID, accountID)
}

// GetHoldingByID returns a single holding by ID.
func (s *PortfolioService) GetHoldingByID(id uint) (*models.PortfolioHoldingRecord, error) {
	h, err := s.portfolioRepo.GetHoldingByID(id)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, fmt.Errorf("holding not found")
	}
	return h, nil
}

// ListHoldings returns all active holdings for a user.
func (s *PortfolioService) ListHoldings(userID uint, userType string) ([]models.PortfolioHoldingRecord, error) {
	return s.portfolioRepo.ListHoldingsForUser(userID, userType)
}

// SetPublic toggles the is_public flag on a holding.
func (s *PortfolioService) SetPublic(holdingID uint, isPublic bool) error {
	return s.portfolioRepo.SetHoldingPublic(holdingID, isPublic)
}

func (s *PortfolioService) SetPublicQuantity(holdingID uint, publicQuantity float64) error {
	return s.portfolioRepo.SetHoldingPublicQuantity(holdingID, publicQuantity)
}

func (s *PortfolioService) ReserveOTCQuantity(holdingID uint, quantity float64) error {
	return s.portfolioRepo.ReserveHoldingQuantity(holdingID, quantity)
}

func (s *PortfolioService) ReleaseOTCReservedQuantity(holdingID uint, quantity float64) error {
	return s.portfolioRepo.ReleaseHoldingReservedQuantity(holdingID, quantity)
}

// ExerciseOption exercises an in-the-money option held in the portfolio.
//
// Validations:
//   - Holding must be an option (listing type == "option")
//   - Settlement date must not have passed
//   - Option must be in-the-money (CALL: market > strike; PUT: market < strike)
//
// On exercise:
//   - Creates a synthetic market order + transaction for the underlying stock
//   - Updates the underlying stock holding (buy for CALL, sell for PUT) at strike price
//   - Zeroes out the option holding quantity, adds exercise profit to realized_profit
//   - Creates a TaxRecord if exercise profit > 0
func (s *PortfolioService) ExerciseOption(holdingID, actuaryID uint) error {
	// 1. Load the option holding.
	holding, err := s.portfolioRepo.GetHoldingByID(holdingID)
	if err != nil || holding == nil {
		return fmt.Errorf("holding not found")
	}
	if holding.Asset.Type != "option" {
		return fmt.Errorf("holding %d is not an option contract", holdingID)
	}
	if holding.Quantity <= 0 {
		return fmt.Errorf("option holding has no remaining quantity")
	}

	// 2. Load the OptionRecord for strike price and underlying.
	opt, err := s.marketRepo.GetOptionByListingID(holding.AssetID)
	if err != nil || opt == nil {
		return fmt.Errorf("option contract data not found for asset %d", holding.AssetID)
	}

	// 3. Validate settlement date.
	if time.Now().After(opt.SettlementDate) {
		return fmt.Errorf("option has expired (settlement: %s)", opt.SettlementDate.Format("2006-01-02"))
	}

	// 4. Load the underlying stock listing for the current market price.
	underlying, err := s.marketRepo.GetListingRecordByID(opt.StockListingID)
	if err != nil || underlying == nil {
		return fmt.Errorf("underlying stock listing not found")
	}

	marketPrice := underlying.Price
	strikePrice := opt.StrikePrice

	// 5. Validate in-the-money and determine direction.
	var direction string
	var intrinsicValue float64 // per share

	switch opt.OptionType {
	case "call":
		if marketPrice <= strikePrice {
			return fmt.Errorf("CALL option is not in-the-money (market: %.2f, strike: %.2f)", marketPrice, strikePrice)
		}
		direction = "buy"
		intrinsicValue = marketPrice - strikePrice
	case "put":
		if marketPrice >= strikePrice {
			return fmt.Errorf("PUT option is not in-the-money (market: %.2f, strike: %.2f)", marketPrice, strikePrice)
		}
		direction = "sell"
		intrinsicValue = strikePrice - marketPrice
	default:
		return fmt.Errorf("unknown option type: %s", opt.OptionType)
	}

	// Each option contract covers optionContractSize shares.
	totalShares := int64(holding.Quantity) * optionContractSize
	exerciseProfit := roundPnL(intrinsicValue * float64(totalShares))

	// 6. Create a synthetic exercise order for the underlying stock.
	now := time.Now().UTC()
	exerciseOrder := &models.OrderRecord{
		UserID:            holding.UserID,
		UserType:          holding.UserType,
		AssetID:           underlying.ID,
		OrderType:         "market",
		Direction:         direction,
		Quantity:          totalShares,
		ContractSize:      1,
		PricePerUnit:      strikePrice,
		Status:            "done",
		IsDone:            true,
		RemainingPortions: 0,
		AccountID:         holding.AccountID,
		LastModification:  now,
		CreatedAt:         now,
	}
	if err := s.orderRepo.CreateOrder(exerciseOrder); err != nil {
		return fmt.Errorf("failed to create exercise order: %w", err)
	}

	// 7. Record the fill transaction.
	txRecord := &models.OrderTransactionRecord{
		OrderID:      exerciseOrder.ID,
		Quantity:     totalShares,
		PricePerUnit: strikePrice,
		ExecutedAt:   now,
	}
	if err := s.orderRepo.CreateOrderTransaction(txRecord); err != nil {
		return fmt.Errorf("failed to record exercise transaction: %w", err)
	}

	// 8. Update the underlying stock portfolio holding.
	if direction == "buy" {
		if err := s.portfolioRepo.RecordBuyFill(
			holding.UserID, holding.UserType, underlying.ID, holding.AccountID,
			float64(totalShares), strikePrice,
		); err != nil {
			return fmt.Errorf("failed to update underlying stock holding: %w", err)
		}
	} else {
		if _, err := s.portfolioRepo.RecordSellFill(
			holding.UserID, holding.UserType, underlying.ID,
			float64(totalShares), strikePrice,
		); err != nil {
			return fmt.Errorf("failed to update underlying stock holding: %w", err)
		}
	}

	// 9. Zero out the option holding and record exercise profit.
	if err := s.portfolioRepo.ExerciseOptionHolding(holdingID, exerciseProfit); err != nil {
		return fmt.Errorf("failed to close option holding: %w", err)
	}

	// 10. Create a TaxRecord for the exercise profit via TaxService (handles RSD conversion).
	_ = s.taxSvc.RecordCapitalGainTax(
		holding.UserID,
		holding.UserType,
		holding.AssetID,
		exerciseProfit,
		"option",
		underlying.Exchange.Currency,
	)

	return nil
}

// --- helpers ---

// computePnL enriches a holding with unrealized P&L using the preloaded asset price.
func computePnL(h *models.PortfolioHoldingRecord) HoldingWithPnL {
	currentPrice := h.Asset.Price
	unrealized := roundPnL((currentPrice - h.AvgBuyPrice) * h.Quantity)
	marketValue := roundPnL(currentPrice * h.Quantity)

	pct := 0.0
	if h.AvgBuyPrice > 0 {
		pct = roundPnL(((currentPrice - h.AvgBuyPrice) / h.AvgBuyPrice) * 100)
	}

	return HoldingWithPnL{
		Holding:          h,
		CurrentPrice:     currentPrice,
		UnrealizedPnL:    unrealized,
		UnrealizedPnLPct: pct,
		MarketValue:      marketValue,
	}
}

func roundPnL(v float64) float64 {
	return math.Round(v*100) / 100
}
