package service

import (
	"fmt"
	"math"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

const (
	// Market order commission: min(14% * price, $7)
	marketCommissionRate = 0.14
	marketCommissionCap  = 7.0

	// Limit order commission: min(24% * price, $12)
	limitCommissionRate = 0.24
	limitCommissionCap  = 12.0
)

// OrderService handles order creation, approval, and cancellation.
type OrderService struct {
	orderRepo    *repository.OrderRepository
	marketRepo   *repository.MarketRepository
	rateProvider RateProviderInterface
}

func NewOrderService(orderRepo *repository.OrderRepository, marketRepo *repository.MarketRepository, rateProvider RateProviderInterface) *OrderService {
	return &OrderService{
		orderRepo:    orderRepo,
		marketRepo:   marketRepo,
		rateProvider: rateProvider,
	}
}

// CreateOrderInput holds all fields required to place an order.
type CreateOrderInput struct {
	UserID       uint
	UserType     string // "client" or "bank"
	// ActorID is the employee placing the order on behalf of the bank.
	// Used for per-agent daily-limit tracking and approval flow.
	// Zero for client orders.
	ActorID      uint
	AssetTicker  string
	OrderType    string   // "market", "limit", "stop", "stop_limit"
	Direction    string   // "buy" or "sell"
	Quantity     int64
	ContractSize int64    // defaults to 1 if 0
	LimitValue   *float64 // required for limit / stop_limit
	StopValue    *float64 // required for stop / stop_limit
	IsAON        bool
	IsMargin     bool
	AccountID    uint
	AfterHours   bool
}

// CreateOrderResult is the full order record returned after creation.
type CreateOrderResult struct {
	Order      *models.OrderRecord
	Commission float64
	TotalPrice float64
}

// CreateOrder validates input and persists a new order.
// Market orders use current ask/bid. Limit orders use limit_value (or better if
// conditions are already met). Stop/stop-limit conditions are evaluated by the
// cron executor (Phase 3).
func (s *OrderService) CreateOrder(input CreateOrderInput) (*CreateOrderResult, error) {
	if err := validateOrderInput(input); err != nil {
		return nil, err
	}

	contractSize := input.ContractSize
	if contractSize <= 0 {
		contractSize = 1
	}

	// Fetch the listing to get current prices.
	listing, err := s.marketRepo.GetListingRecordByTicker(input.AssetTicker)
	if err != nil || listing == nil {
		return nil, fmt.Errorf("asset not found: %s", input.AssetTicker)
	}

	// Determine price per unit based on order type and current market.
	pricePerUnit := orderPricePerUnit(listing, input)

	// Total order value (before commission).
	totalPrice := round2(float64(contractSize) * pricePerUnit * float64(input.Quantity))

	// Commission depends on order type.
	commission := calcCommission(input.OrderType, totalPrice)

	// For buy orders, resolve the exchange rate between the asset's trading currency and
	// the account's currency. The rate is stored on the order so refunds use the same rate.
	// Sell orders don't debit at creation time — proceeds are converted at fill time.
	currencyRate := 1.0
	if input.Direction == "buy" {
		assetCurrency := listing.Exchange.Currency
		_, accountCurrency, err := s.orderRepo.GetAccountBalance(input.AccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to read account currency: %w", err)
		}
		if assetCurrency != accountCurrency {
			rate, err := s.rateProvider.GetRate(assetCurrency, accountCurrency)
			if err != nil || rate == 0 {
				return nil, fmt.Errorf("no exchange rate available for %s/%s", assetCurrency, accountCurrency)
			}
			currencyRate = rate
		}
	}

	// Margin orders: verify the account has enough available balance to cover
	// the Initial Margin Cost (MaintenanceMargin × 1.1) before accepting the order.
	if input.IsMargin {
		if err := s.validateMargin(input.AccountID, listing, input.Quantity, contractSize, pricePerUnit, currencyRate); err != nil {
			return nil, err
		}
	}

	// Convert totalPrice to RSD for agent limit checks (limits are always in RSD).
	totalPriceRSD := s.toRSD(totalPrice, listing.Exchange.Currency)

	// Determine initial order status.
	status, needsApproval, err := s.resolveStatus(input, totalPriceRSD)
	if err != nil {
		return nil, err
	}

	// For agent orders that don't need approval, increment their usedLimit in RSD.
	if input.UserType == "bank" && !needsApproval {
		if err := s.orderRepo.IncrementUsedLimit(input.ActorID, totalPriceRSD); err != nil {
			return nil, fmt.Errorf("failed to update actuary used limit: %w", err)
		}
	}

	// For buy orders, debit the full order value + commission from the account upfront,
	// converted to the account's currency if it differs from the asset's currency.
	if input.Direction == "buy" {
		totalDebit := round2((totalPrice + commission) * currencyRate)
		if err := s.orderRepo.DebitAccount(input.AccountID, totalDebit); err != nil {
			return nil, fmt.Errorf("insufficient funds: %w", err)
		}
	}

	now := time.Now().UTC()
	var placedBy *uint
	if input.UserType == "bank" && input.ActorID != 0 {
		placedBy = &input.ActorID
	}
	order := &models.OrderRecord{
		UserID:            input.UserID,
		UserType:          input.UserType,
		PlacedBy:          placedBy,
		AssetID:           listing.ID,
		OrderType:         input.OrderType,
		Direction:         input.Direction,
		Quantity:          input.Quantity,
		ContractSize:      contractSize,
		PricePerUnit:      pricePerUnit,
		LimitValue:        input.LimitValue,
		StopValue:         input.StopValue,
		IsAON:             input.IsAON,
		IsMargin:          input.IsMargin,
		Status:            status,
		IsDone:            false,
		RemainingPortions: input.Quantity,
		Commission:        commission,
		CurrencyRate:      currencyRate,
		AfterHours:        input.AfterHours,
		AccountID:         input.AccountID,
		LastModification:  now,
		CreatedAt:         now,
	}

	if err := s.orderRepo.CreateOrder(order); err != nil {
		return nil, fmt.Errorf("failed to persist order: %w", err)
	}

	return &CreateOrderResult{
		Order:      order,
		Commission: commission,
		TotalPrice: totalPrice,
	}, nil
}

// resolveStatus determines the initial status and whether supervisor approval is needed.
func (s *OrderService) resolveStatus(input CreateOrderInput, totalPrice float64) (status string, needsApproval bool, err error) {
	// Client orders are always auto-approved.
	if input.UserType == "client" {
		return "approved", false, nil
	}
	// Fund orders are placed by a supervisor on behalf of a fund and are
	// always auto-approved — the supervisor has already passed the fund
	// authorization checks before reaching here.
	if input.UserType == "fund" {
		return "approved", false, nil
	}

	// Bank orders: check the placing agent's actuary profile.
	profile, err := s.orderRepo.GetActuaryProfile(input.ActorID)
	if err != nil {
		return "", false, fmt.Errorf("failed to read actuary profile: %w", err)
	}

	// No actuary profile means basic employee — cannot place orders.
	if profile == nil {
		return "", false, fmt.Errorf("employee does not have trading permissions")
	}

	// Supervisor: no limit, always approved without supervisor review of self.
	if profile.Limit == nil {
		return "approved", false, nil
	}

	// Agent: daily limit already exhausted — hard block, no new orders allowed.
	if profile.UsedLimit >= *profile.Limit {
		return "", false, fmt.Errorf("daily trading limit exhausted (used: %.2f RSD, limit: %.2f RSD)", profile.UsedLimit, *profile.Limit)
	}

	// Agent: order would exceed remaining daily limit — reject outright. The
	// agent can resubmit a smaller order; no supervisor approval is requested.
	remaining := *profile.Limit - profile.UsedLimit
	if totalPrice > remaining {
		return "", false, fmt.Errorf("order exceeds remaining daily limit (cost: %.2f RSD, remaining: %.2f RSD)", totalPrice, remaining)
	}

	// Agent flagged as always-needs-approval still routes to supervisor.
	if profile.NeedApproval {
		return "pending", true, nil
	}

	return "approved", false, nil
}

// ApproveOrder approves a pending order on behalf of a supervisor.
// If the order's asset has a past settlement date it is auto-declined instead.
// For employee orders the agent's usedLimit is incremented on approval since it
// was withheld while the order was pending.
func (s *OrderService) ApproveOrder(orderID, supervisorID uint) error {
	order, err := s.orderRepo.GetOrderByID(orderID)
	if err != nil {
		return fmt.Errorf("failed to load order: %w", err)
	}
	if order == nil {
		return fmt.Errorf("order not found")
	}
	if order.Status != "pending" {
		return fmt.Errorf("order is not pending (status: %s)", order.Status)
	}

	// Auto-decline if the underlying asset's settlement date has passed.
	expired, err := s.isSettlementExpired(order.AssetID)
	if err != nil {
		return fmt.Errorf("failed to check settlement date: %w", err)
	}
	if expired {
		return s.orderRepo.UpdateOrderStatus(orderID, "declined", &supervisorID)
	}

	// Increment the placing agent's usedLimit in RSD now that the order is officially approved.
	if order.UserType == "bank" && order.PlacedBy != nil {
		totalPrice := round2(float64(order.ContractSize) * order.PricePerUnit * float64(order.Quantity))
		totalPriceRSD := s.toRSD(totalPrice, order.Asset.Exchange.Currency)
		if err := s.orderRepo.IncrementUsedLimit(*order.PlacedBy, totalPriceRSD); err != nil {
			return fmt.Errorf("failed to update actuary used limit: %w", err)
		}
	}

	return s.orderRepo.UpdateOrderStatus(orderID, "approved", &supervisorID)
}

// DeclineOrder declines a pending order on behalf of a supervisor.
// For buy orders the full debit (order value + commission) is refunded.
func (s *OrderService) DeclineOrder(orderID, supervisorID uint) error {
	order, err := s.orderRepo.GetOrderByID(orderID)
	if err != nil {
		return fmt.Errorf("failed to load order: %w", err)
	}
	if order == nil {
		return fmt.Errorf("order not found")
	}
	if order.Status != "pending" {
		return fmt.Errorf("order is not pending (status: %s)", order.Status)
	}

	// Refund the full debit that was taken on creation, converting back to account currency.
	if order.Direction == "buy" {
		totalPrice := round2(float64(order.Quantity) * float64(order.ContractSize) * order.PricePerUnit)
		refund := round2((totalPrice + order.Commission) * order.CurrencyRate)
		if err := s.orderRepo.RefundToAccount(order.AccountID, refund); err != nil {
			return fmt.Errorf("failed to refund account on decline: %w", err)
		}
	}

	return s.orderRepo.UpdateOrderStatus(orderID, "declined", &supervisorID)
}

// CancelOrder cancels the unfilled portion of an active order.
//
// newRemaining == 0  → full cancel: is_done=true, status="cancelled"
// newRemaining  > 0  → partial cancel: reduce remaining_portions, keep status
//
// The value of the cancelled quantity is refunded to the order's account.
func (s *OrderService) CancelOrder(orderID, requesterID uint, newRemaining int64) error {
	order, err := s.orderRepo.GetOrderByID(orderID)
	if err != nil {
		return fmt.Errorf("failed to load order: %w", err)
	}
	if order == nil {
		return fmt.Errorf("order not found")
	}
	if order.IsDone {
		return fmt.Errorf("order is already done and cannot be cancelled")
	}
	if newRemaining < 0 || newRemaining >= order.RemainingPortions {
		return fmt.Errorf("newRemaining must be between 0 and %d (exclusive)", order.RemainingPortions)
	}

	cancelledQty := order.RemainingPortions - newRemaining
	refundAmount := round2(float64(cancelledQty) * float64(order.ContractSize) * order.PricePerUnit * order.CurrencyRate)

	if newRemaining == 0 {
		// Full cancel.
		if err := s.orderRepo.FullCancelOrder(orderID); err != nil {
			return fmt.Errorf("failed to cancel order: %w", err)
		}
	} else {
		// Partial cancel: just trim remaining_portions.
		if err := s.orderRepo.SetRemainingPortions(orderID, newRemaining); err != nil {
			return fmt.Errorf("failed to update remaining portions: %w", err)
		}
	}

	// Refund the cancelled portion back to the account.
	if err := s.orderRepo.RefundToAccount(order.AccountID, refundAmount); err != nil {
		return fmt.Errorf("failed to refund account: %w", err)
	}

	return nil
}

// isSettlementExpired returns true when the listing is a futures or options
// contract whose settlement date has already passed.
func (s *OrderService) isSettlementExpired(assetID uint) (bool, error) {
	settlDate, err := s.orderRepo.GetSettlementDate(assetID)
	if err != nil {
		return false, err
	}
	if settlDate == nil {
		return false, nil // not a dated instrument
	}
	return time.Now().After(*settlDate), nil
}

// GetOrder returns a single order by ID.
func (s *OrderService) GetOrder(id uint) (*models.OrderRecord, error) {
	order, err := s.orderRepo.GetOrderByID(id)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, fmt.Errorf("order not found")
	}
	return order, nil
}

// ListOrdersForUser returns orders for a user with optional status filter.
func (s *OrderService) ListOrdersForUser(userID uint, userType, statusFilter string) ([]models.OrderRecord, error) {
	return s.orderRepo.ListOrdersForUser(userID, userType, statusFilter)
}

// ListAllOrders returns all orders across all users, used by supervisors.
func (s *OrderService) ListAllOrders(statusFilter string) ([]models.OrderRecord, error) {
	return s.orderRepo.ListAllOrders(statusFilter)
}

// ListTransactionsForOrder returns all fill events for an order.
func (s *OrderService) ListTransactionsForOrder(orderID uint) ([]models.OrderTransactionRecord, error) {
	return s.orderRepo.ListTransactionsForOrder(orderID)
}

// --- Helpers ---

func validateOrderInput(input CreateOrderInput) error {
	if input.UserType != "client" && input.UserType != "bank" && input.UserType != "fund" {
		return fmt.Errorf("user type must be 'client', 'bank', or 'fund'")
	}
	if input.UserType == "client" && input.UserID == 0 {
		return fmt.Errorf("user ID is required")
	}
	if input.UserType == "fund" && input.UserID == 0 {
		return fmt.Errorf("fund ID is required")
	}
	if input.AssetTicker == "" {
		return fmt.Errorf("asset ticker is required")
	}
	if input.Direction != "buy" && input.Direction != "sell" {
		return fmt.Errorf("direction must be 'buy' or 'sell'")
	}
	if input.Quantity <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	if input.AccountID == 0 {
		return fmt.Errorf("account ID is required")
	}

	switch input.OrderType {
	case "market":
		// no extra fields required
	case "limit":
		if input.LimitValue == nil {
			return fmt.Errorf("limit_value is required for limit orders")
		}
	case "stop":
		if input.StopValue == nil {
			return fmt.Errorf("stop_value is required for stop orders")
		}
	case "stop_limit":
		if input.StopValue == nil || input.LimitValue == nil {
			return fmt.Errorf("stop_value and limit_value are required for stop_limit orders")
		}
	default:
		return fmt.Errorf("order type must be one of: market, limit, stop, stop_limit")
	}

	return nil
}

// orderPricePerUnit determines the price per unit to record on the order.
//
// Market: buy fills at ask, sell fills at bid.
//
// Limit: use current market price if the condition is already satisfied,
// otherwise fall back to limit_value (worst-case committed price).
//   - Buy limit:  condition = ask <= limit_value  → fill price = ask
//   - Sell limit: condition = bid >= limit_value  → fill price = bid
//
// Stop: becomes a market order when the stop triggers. If the condition is
// already met at placement, use current ask/bid; otherwise use stop_value.
//   - Buy stop:  condition = ask > stop_value  (buy into a rising market)
//   - Sell stop: condition = bid < stop_value  (sell into a falling market)
//
// Stop-limit: stop triggers the order, which then executes as a limit at
// limit_value. Use limit_value as the committed reference price.
func orderPricePerUnit(listing *models.MarketListingRecord, input CreateOrderInput) float64 {
	switch input.OrderType {
	case "limit":
		if input.Direction == "buy" && listing.Ask <= *input.LimitValue {
			return listing.Ask
		}
		if input.Direction == "sell" && listing.Bid >= *input.LimitValue {
			return listing.Bid
		}
		return *input.LimitValue

	case "stop":
		if input.Direction == "buy" && listing.Ask > *input.StopValue {
			return listing.Ask // stop already triggered — market fill
		}
		if input.Direction == "sell" && listing.Bid < *input.StopValue {
			return listing.Bid // stop already triggered — market fill
		}
		return *input.StopValue // not yet triggered; use stop as reference

	case "stop_limit":
		// When the stop triggers the order executes as a limit at limit_value.
		return *input.LimitValue

	default: // "market"
		if input.Direction == "buy" {
			return listing.Ask
		}
		return listing.Bid
	}
}

// validateMargin checks that the account's available balance covers the
// Initial Margin Cost for a margin order.
//
//	MaintenanceMargin = contractSize * pricePerUnit * 10%
//	InitialMarginCost = MaintenanceMargin * 1.1
// validateMargin checks that the account's available balance covers the Initial Margin Cost.
// Margin rates per asset type:
//   stock:   50% of total position value  (quantity × contractSize × price × 50%)
//   option:  100 shares/contract × 50% × underlying stock price  (quantity × contractSize × 100 × stockPrice × 50%)
//   forex / futures: 10% of nominal value (quantity × contractSize × price × 10%)
// InitialMarginCost = MaintenanceMargin × 1.1
func (s *OrderService) validateMargin(accountID uint, listing *models.MarketListingRecord, quantity, contractSize int64, pricePerUnit, currencyRate float64) error {
	balance, _, err := s.orderRepo.GetAccountBalance(accountID)
	if err != nil {
		return fmt.Errorf("failed to read account balance: %w", err)
	}

	qty := float64(quantity) * float64(contractSize)
	var maintenanceMargin float64

	switch listing.Type {
	case "stock":
		maintenanceMargin = qty * pricePerUnit * 0.50
	case "option":
		opt, err := s.marketRepo.GetOptionByListingID(listing.ID)
		if err != nil || opt == nil {
			return fmt.Errorf("option contract data not found for margin calculation")
		}
		underlying, err := s.marketRepo.GetListingRecordByID(opt.StockListingID)
		if err != nil || underlying == nil {
			return fmt.Errorf("underlying stock not found for option margin calculation")
		}
		// Each option contract covers 100 shares; margin = 50% of underlying position value.
		maintenanceMargin = qty * 100 * underlying.Price * 0.50
	default: // forex, futures
		maintenanceMargin = qty * pricePerUnit * 0.10
	}

	imc := round2(maintenanceMargin * 1.1 * currencyRate)
	if balance < imc {
		return fmt.Errorf("insufficient balance for margin order: need %.2f, have %.2f", imc, balance)
	}
	return nil
}

// toRSD converts an amount from the given currency to RSD using the rate provider.
// Returns the original amount unchanged if already RSD or no rate is found.
func (s *OrderService) toRSD(amount float64, currency string) float64 {
	if currency == "RSD" || currency == "" {
		return amount
	}
	rate, err := s.rateProvider.GetRate(currency, "RSD")
	if err != nil || rate == 0 {
		return amount
	}
	return round2(amount * rate)
}

// calcCommission computes the commission for an order.
// Market + Stop (stop becomes a market order on trigger): min(14% * price, $7)
// Limit + Stop-Limit (stop-limit becomes a limit order on trigger): min(24% * price, $12)
func calcCommission(orderType string, totalPrice float64) float64 {
	if orderType == "market" || orderType == "stop" {
		return math.Min(marketCommissionRate*totalPrice, marketCommissionCap)
	}
	return math.Min(limitCommissionRate*totalPrice, limitCommissionCap)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
