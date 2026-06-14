package service

import (
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

const afterHoursDelay = 30 * time.Minute

// OrderExecutor fills active orders on each cron tick.
type OrderExecutor struct {
	orderRepo    *repository.OrderRepository
	marketRepo   *repository.MarketRepository
	portfolioSvc *PortfolioService
	rateProvider RateProviderInterface
	notifier     *notify.Client
}

func NewOrderExecutor(orderRepo *repository.OrderRepository, marketRepo *repository.MarketRepository, portfolioSvc *PortfolioService, rateProvider RateProviderInterface) *OrderExecutor {
	return &OrderExecutor{orderRepo: orderRepo, marketRepo: marketRepo, portfolioSvc: portfolioSvc, rateProvider: rateProvider}
}

// WithNotifier wires the optional best-effort notification client.
func (e *OrderExecutor) WithNotifier(n *notify.Client) *OrderExecutor {
	e.notifier = n
	return e
}

// Run processes all approved, not-done orders and partially or fully fills them
// when their execution conditions are met.
func (e *OrderExecutor) Run() {
	orders, err := e.orderRepo.ListPendingActiveOrders()
	if err != nil {
		slog.Error("order executor: failed to load active orders", "error", err)
		return
	}

	for _, order := range orders {
		// Only execute approved orders; pending ones wait for supervisor action.
		if order.Status != "approved" {
			continue
		}

		listing := &order.Asset // preloaded by ListPendingActiveOrders

		// Evaluate the order's execution conditions for this tick.
		eval := evaluateOrder(&order, listing)

		// stop_limit: persist the stop-triggered latch the first tick we cross the stop.
		// This keeps the order armed as a pure limit order on subsequent ticks even if
		// the price moves back through the stop value.
		if eval.justTriggered {
			if err := e.orderRepo.SetStopTriggered(order.ID); err != nil {
				slog.Error("order executor: failed to latch stop_triggered", "orderID", order.ID, "error", err)
			} else {
				order.StopTriggered = true
			}
		}

		if !eval.fillNow {
			continue
		}

		// After-hours orders: enforce a 30-minute gap between consecutive fills.
		if order.AfterHours && time.Since(order.LastModification) < afterHoursDelay {
			continue
		}

		// Determine fill quantity and price.
		fillQty := fillQuantity(&order)
		price := executeFillPrice(&order, listing)

		// Persist the fill event.
		txRecord := &models.OrderTransactionRecord{
			OrderID:      order.ID,
			Quantity:     fillQty,
			PricePerUnit: price,
			ExecutedAt:   time.Now().UTC(),
		}
		if err := e.orderRepo.CreateOrderTransaction(txRecord); err != nil {
			slog.Error("order executor: failed to create transaction", "orderID", order.ID, "error", err)
			continue
		}

		// Decrement remaining portions; marks order done if it reaches 0.
		if err := e.orderRepo.DecrementRemainingPortions(order.ID, fillQty); err != nil {
			slog.Error("order executor: failed to decrement portions", "orderID", order.ID, "error", err)
			continue
		}

		// Update portfolio holdings to reflect the fill.
		if err := e.portfolioSvc.RecordFill(&order, fillQty, price); err != nil {
			slog.Error("order executor: failed to update portfolio", "orderID", order.ID, "error", err)
			// Non-fatal: order fill is committed; portfolio can be reconciled later.
		}

		// Refund the over-reservation for buy fills that came in below the
		// order's committed price-per-unit (limit / stop_limit where the actual
		// fill price beats the limit). The client was debited the worst case
		// upfront; we hand back the price gap per fill so total_paid / balance_after
		// settle to the real net spend.
		if order.Direction == "buy" && !order.IsMargin && price < order.PricePerUnit {
			overReservation := (order.PricePerUnit - price) * float64(fillQty) * float64(order.ContractSize)
			refund := round2(overReservation * order.CurrencyRate)
			if refund > 0 {
				if err := e.orderRepo.RefundBuyOverReservation(order.ID, order.AccountID, refund); err != nil {
					slog.Error("order executor: failed to refund buy over-reservation",
						"orderID", order.ID, "amount", refund, "error", err)
				}
			}
		}

		// Credit account for sell fills, converting to account currency if needed.
		// Commission is deducted from the proceeds and credited to the bank account.
		if order.Direction == "sell" {
			fillAmount := round2(float64(fillQty) * float64(order.ContractSize) * price)
			assetCurrency := order.Asset.Exchange.Currency

			// Proportional commission for this partial fill (in asset trading currency).
			fillCommission := round2(order.Commission * float64(fillQty) / float64(order.Quantity))

			// Credit bank account with the commission in the asset's trading currency.
			if fillCommission > 0 {
				bankAccountID, err := e.orderRepo.GetBankAccountByCurrency(assetCurrency)
				if err != nil {
					slog.Error("order executor: failed to find bank account for sell commission", "orderID", order.ID, "currency", assetCurrency, "error", err)
				} else if bankAccountID > 0 {
					if err := e.orderRepo.CreditAccount(bankAccountID, fillCommission); err != nil {
						slog.Error("order executor: failed to credit bank sell commission", "orderID", order.ID, "currency", assetCurrency, "amount", fillCommission, "error", err)
					}
				} else {
					slog.Warn("order executor: no bank account found for currency, sell commission not credited", "orderID", order.ID, "currency", assetCurrency)
				}
			}

			// Net proceeds after commission, converted to account currency if needed.
			netAmount := round2(fillAmount - fillCommission)
			_, accountCurrency, err := e.orderRepo.GetAccountBalance(order.AccountID)
			if err != nil {
				slog.Error("order executor: failed to get account currency on sell", "orderID", order.ID, "error", err)
			} else if assetCurrency != accountCurrency {
				rate, err := e.rateProvider.GetRate(assetCurrency, accountCurrency)
				if err != nil || rate == 0 {
					slog.Error("order executor: no forex rate for sell proceeds", "orderID", order.ID, "from", assetCurrency, "to", accountCurrency)
				} else {
					netAmount = round2(netAmount * rate)
				}
			}

			// Apply sell proceeds toward outstanding margin loans on the same asset
			// (FIFO by buy order creation date) before crediting the user. Loans were
			// recorded in the account's currency, so netAmount must already be converted.
			netAmount = e.settleMarginLoansFromProceeds(&order, netAmount)

			if netAmount > 0 {
				if err := e.orderRepo.CreditAccount(order.AccountID, netAmount); err != nil {
					slog.Error("order executor: failed to credit account on sell", "orderID", order.ID, "error", err)
				}
			}
		}

		// Credit the bank's account with the proportional commission for buy fills.
		// Commission was already deducted from the client's account at order creation
		// for non-margin orders. For margin orders the commission is part of the
		// bank's loan disbursement at create (totalCost includes commission), and
		// gets recovered when the client sells and the loan is repaid — so no
		// separate commission credit is needed here, or the bank would gain it
		// back twice and the "bank cash drop" wouldn't equal the loan amount.
		// The commission is in the asset's trading currency (exchange currency), not the user's account currency.
		if order.Direction == "buy" && order.Commission > 0 && !order.IsMargin {
			fillCommission := round2(order.Commission * float64(fillQty) / float64(order.Quantity))
			if fillCommission > 0 {
				currencyKod := order.Asset.Exchange.Currency
				bankAccountID, err := e.orderRepo.GetBankAccountByCurrency(currencyKod)
				if err != nil {
					slog.Error("order executor: failed to find bank account for commission", "orderID", order.ID, "currency", currencyKod, "error", err)
				} else if bankAccountID > 0 {
					if err := e.orderRepo.CreditAccount(bankAccountID, fillCommission); err != nil {
						slog.Error("order executor: failed to credit bank commission", "orderID", order.ID, "currency", currencyKod, "amount", fillCommission, "error", err)
					}
				} else {
					slog.Warn("order executor: no bank account found for currency, commission not credited", "orderID", order.ID, "currency", currencyKod)
				}
			}
		}

		remaining := order.RemainingPortions - fillQty

		slog.Info("order executor: filled",
			"orderID", order.ID,
			"type", order.OrderType,
			"direction", order.Direction,
			"filled", fillQty,
			"price", price,
			"remaining", remaining,
		)

		// Notify the order owner about this fill — fully executed vs partial.
		if remaining <= 0 {
			emitOrderNotification(e.notifier, &order, "ORDER_DONE",
				"Order izvršen",
				fmt.Sprintf("Vaš %s order za %s je u potpunosti izvršen (%d @ %.2f).",
					order.Direction, order.Asset.Ticker, order.Quantity, price))
		} else {
			emitOrderNotification(e.notifier, &order, "ORDER_PARTIAL",
				"Delimično izvršenje",
				fmt.Sprintf("Vaš %s order za %s je delimično izvršen (%d izvršeno, %d preostalo).",
					order.Direction, order.Asset.Ticker, fillQty, remaining))
		}
	}
}

// orderEval is the per-tick verdict for an order.
type orderEval struct {
	fillNow       bool // the order should fill this tick at executeFillPrice
	justTriggered bool // stop_limit: stop crossed for the first time this tick; persist the latch
}

// evaluateOrder reports whether current market prices satisfy the order's
// execution trigger, and (for stop_limit) whether the stop just crossed.
//
//	market:     always executable
//	limit buy:  ask <= limit_value
//	limit sell: bid >= limit_value
//	stop buy:   ask > stop_value  (then executes at market)
//	stop sell:  bid < stop_value  (then executes at market)
//
// Stop-limit is two-phase. The stop acts as a one-way trigger:
//   Buy:  arms when Ask >= stop_value (price rising into stop).
//   Sell: arms when Bid <= stop_value (price falling into stop).
//
// Once armed, the latch is persisted and the order behaves as a limit order on
// every subsequent tick — even if the price moves back through the stop value:
//   Buy:  fill while Ask <= limit_value
//   Sell: fill while Bid >= limit_value
func evaluateOrder(order *models.OrderRecord, listing *models.MarketListingRecord) orderEval {
	switch order.OrderType {
	case "market":
		return orderEval{fillNow: true}

	case "limit":
		if order.Direction == "buy" {
			return orderEval{fillNow: listing.Ask <= *order.LimitValue}
		}
		return orderEval{fillNow: listing.Bid >= *order.LimitValue}

	case "stop":
		if order.Direction == "buy" {
			return orderEval{fillNow: listing.Ask > *order.StopValue}
		}
		return orderEval{fillNow: listing.Bid < *order.StopValue}

	case "stop_limit":
		// Phase 1: arm the stop the first time the trigger price is crossed.
		armed := order.StopTriggered
		justArmed := false
		if !armed {
			if order.Direction == "buy" && listing.Ask >= *order.StopValue {
				armed = true
				justArmed = true
			} else if order.Direction == "sell" && listing.Bid <= *order.StopValue {
				armed = true
				justArmed = true
			}
		}
		if !armed {
			return orderEval{}
		}
		// Phase 2: once armed, behave as a pure limit order.
		var fillNow bool
		if order.Direction == "buy" {
			fillNow = listing.Ask <= *order.LimitValue
		} else {
			fillNow = listing.Bid >= *order.LimitValue
		}
		return orderEval{fillNow: fillNow, justTriggered: justArmed}
	}
	return orderEval{}
}

// settleMarginLoansFromProceeds applies sell proceeds (in the account's currency)
// against the user's outstanding margin loans on the same asset, FIFO by buy order
// creation date. Returns the remaining proceeds available to credit to the user.
func (e *OrderExecutor) settleMarginLoansFromProceeds(sellOrder *models.OrderRecord, proceeds float64) float64 {
	if proceeds <= 0 {
		return proceeds
	}
	loans, err := e.orderRepo.ListOutstandingMarginLoansForUserAsset(sellOrder.UserID, sellOrder.UserType, sellOrder.AssetID)
	if err != nil {
		slog.Error("order executor: failed to list margin loans for sell settlement",
			"orderID", sellOrder.ID, "error", err)
		return proceeds
	}
	remaining := proceeds
	for _, loan := range loans {
		if remaining <= 0 {
			break
		}
		toRepay := loan.MarginLoan
		if toRepay > remaining {
			toRepay = remaining
		}
		applied, err := e.orderRepo.ReduceMarginLoan(loan.ID, toRepay)
		if err != nil {
			slog.Error("order executor: failed to reduce margin loan",
				"sellOrderID", sellOrder.ID, "buyOrderID", loan.ID, "error", err)
			continue
		}
		// Credit the bank account in the loan's currency: this is the cash the
		// bank originally fronted at order creation, now coming back from the
		// sell proceeds. Without this, the loan number on the order zeroes out
		// but the bank's account never recovers the funds.
		if applied > 0 {
			_, loanCurrency, balErr := e.orderRepo.GetAccountBalance(loan.AccountID)
			if balErr != nil {
				slog.Error("order executor: failed to read account currency for margin repayment", "buyOrderID", loan.ID, "error", balErr)
			} else if loanCurrency != "" {
				bankID, err := e.orderRepo.GetBankAccountByCurrency(loanCurrency)
				if err != nil {
					slog.Error("order executor: bank lookup failed for margin repayment", "currency", loanCurrency, "buyOrderID", loan.ID, "error", err)
				} else if bankID > 0 {
					if err := e.orderRepo.CreditAccount(bankID, applied); err != nil {
						slog.Error("order executor: failed to credit bank for margin repayment", "bankID", bankID, "amount", applied, "buyOrderID", loan.ID, "error", err)
					}
				} else {
					slog.Warn("order executor: no bank account for currency, margin repayment not credited", "currency", loanCurrency, "buyOrderID", loan.ID)
				}
			}
		}
		remaining = round2(remaining - applied)
		slog.Info("order executor: applied sell proceeds to margin loan",
			"sellOrderID", sellOrder.ID, "buyOrderID", loan.ID, "applied", applied, "remaining", remaining)
	}
	if remaining < 0 {
		return 0
	}
	return remaining
}

// fillQuantity returns how many units to fill in this cycle.
// AON orders must fill the entire remaining quantity at once or not at all.
// Regular orders fill a random portion: rand[1, remaining_portions].
func fillQuantity(order *models.OrderRecord) int64 {
	if order.IsAON {
		return order.RemainingPortions
	}
	if order.RemainingPortions == 1 {
		return 1
	}
	return rand.Int63n(order.RemainingPortions) + 1
}

// executeFillPrice returns the actual per-unit price for a fill.
//
//	market / stop:      current ask (buy) or bid (sell)
//	limit / stop_limit: min(limit_value, ask) for buy; max(limit_value, bid) for sell
func executeFillPrice(order *models.OrderRecord, listing *models.MarketListingRecord) float64 {
	switch order.OrderType {
	case "limit", "stop_limit":
		if order.Direction == "buy" {
			return math.Min(*order.LimitValue, listing.Ask)
		}
		return math.Max(*order.LimitValue, listing.Bid)
	default: // market, stop
		if order.Direction == "buy" {
			return listing.Ask
		}
		return listing.Bid
	}
}
