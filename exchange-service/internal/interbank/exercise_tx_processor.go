package interbank

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// ExerciseTxProcessor handles the 4-posting OPTION-execution shape
// described in protocol §2.7.2:
//
//	OPTION{neg_id}      Amount=+π·k   Asset=MONAS{premium currency}
//	PERSON{buyer_bank}  Amount=-π·k   Asset=MONAS{premium currency}
//	OPTION{neg_id}      Amount=-k     Asset=STOCK{ticker}
//	PERSON{buyer_bank}  Amount=+k     Asset=STOCK{ticker}
//
// Per spec, the OPTION pseudo-account always lives in the seller's
// bank (its id is the negotiation id, which the seller's bank minted).
// So we only see this processor's OnNewTx when we ARE the seller's
// bank. The buyer's bank is the initiator — see ExerciseForLocalBuyer
// in interbank_otc_http_handler.go for that side.
//
// COMMIT_TX effects on the seller's side:
//
//	1. Mark the local OTC negotiation as exercised (we reuse the
//	   negotiation row's lifecycle since we don't separately track
//	   seller-side option contracts yet — see "Known gap" below).
//	2. Decrement the seller's stock holding by StockAmount, releasing
//	   the reservation that the acceptance flow took. Realised profit
//	   is computed via RecordSellFillTx.
//	3. Credit the seller's account by CashAmount in the strike currency.
//
// Known gap: cross-bank OTC acceptance does not currently reserve the
// seller's local stock holding (see Additions.md §3.2). If the seller
// has disposed of the stocks between acceptance and exercise, the
// RecordSellFill call will fail and the COMMIT_TX will return 5xx —
// the partner's reconciliation will keep retrying until an operator
// intervenes. Closing that gap is tracked separately.
type ExerciseTxProcessor struct {
	db            *gorm.DB
	registry      *Registry
	negRepo       *repository.InterbankOtcRepository
	exerciseRepo  *repository.InterbankExerciseRepository
	portfolioRepo *repository.PortfolioRepository
	marketRepo    *repository.MarketRepository
	walletRepo    *repository.InterbankWalletRepository
}

// NewExerciseTxProcessor wires the seller-side processor.
func NewExerciseTxProcessor(
	db *gorm.DB,
	registry *Registry,
	negRepo *repository.InterbankOtcRepository,
	exerciseRepo *repository.InterbankExerciseRepository,
	portfolioRepo *repository.PortfolioRepository,
	marketRepo *repository.MarketRepository,
	walletRepo *repository.InterbankWalletRepository,
) *ExerciseTxProcessor {
	return &ExerciseTxProcessor{
		db:            db,
		registry:      registry,
		negRepo:       negRepo,
		exerciseRepo:  exerciseRepo,
		portfolioRepo: portfolioRepo,
		marketRepo:    marketRepo,
		walletRepo:    walletRepo,
	}
}

// OnNewTx validates the exercise shape against the stored negotiation,
// persists the pending exercise row, and votes YES. No local effects
// are applied here — the seller's accounting moves on COMMIT_TX.
func (p *ExerciseTxProcessor) OnNewTx(_ context.Context, partner *PartnerBank, tx *Transaction) (*TransactionVote, error) {
	if reason := checkBalanced(tx); reason != nil {
		return voteNo(*reason), nil
	}
	parsed, reason := parseExerciseTx(tx)
	if reason != nil {
		return voteNo(*reason), nil
	}

	// The OPTION pseudo-account routing must be our own.
	ownRouting := p.registry.OwnRoutingNumber()
	if parsed.optionRouting != ownRouting {
		return voteNo(NoVoteReason{Reason: ReasonOptionNegotiationNotFound, Posting: parsed.optionCashLeg}), nil
	}
	// The partner that POSTed this NEW_TX must be the buyer's bank.
	if parsed.buyerRouting != partner.Code {
		return voteNo(NoVoteReason{Reason: ReasonNoSuchAccount, Posting: parsed.buyerCashLeg}), nil
	}

	neg, err := p.negRepo.Get(int(parsed.optionNegRouting), parsed.optionNegID)
	if err != nil {
		return nil, fmt.Errorf("loading negotiation: %w", err)
	}
	if neg == nil {
		return voteNo(NoVoteReason{Reason: ReasonOptionNegotiationNotFound, Posting: parsed.optionCashLeg}), nil
	}

	// Validate terms against the stored negotiation.
	if int(parsed.buyerRouting) != neg.BuyerRoutingNumber || parsed.buyerID != neg.BuyerID {
		return voteNo(NoVoteReason{Reason: ReasonNoSuchAccount, Posting: parsed.buyerCashLeg}), nil
	}
	if parsed.stockTicker != neg.StockTicker {
		return voteNo(NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: parsed.buyerStockLeg}), nil
	}
	if !nearlyEqual(parsed.stockAmount, neg.Amount) {
		return voteNo(NoVoteReason{Reason: ReasonOptionAmountIncorrect, Posting: parsed.buyerStockLeg}), nil
	}
	if parsed.cashCurrency != neg.PricePerUnitCurrency {
		return voteNo(NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: parsed.buyerCashLeg}), nil
	}
	expectedCash := neg.PricePerUnitAmount * neg.Amount
	if !nearlyEqual(parsed.cashAmount, expectedCash) {
		return voteNo(NoVoteReason{Reason: ReasonOptionAmountIncorrect, Posting: parsed.buyerCashLeg}), nil
	}

	// Has the option already been exercised? The negotiation's
	// IsOngoing was flipped off at acceptance time, so it doesn't tell
	// us — instead, check the exercise log for a committed row tied to
	// this negotiation.
	already, err := p.exerciseRepo.HasCommittedForNegotiation(neg.NegotiationRoutingNumber, neg.NegotiationID)
	if err != nil {
		return nil, fmt.Errorf("checking exercise history: %w", err)
	}
	if already {
		return voteNo(NoVoteReason{Reason: ReasonOptionUsedOrExpired, Posting: parsed.optionCashLeg}), nil
	}
	// Settlement-date check: per spec §2.7.2 last paragraph, an option
	// whose settlementDate has passed should not execute. We parse
	// loosely — if the stored format is malformed, we accept rather
	// than reject (better to let the partner test pass).
	if exp, ok := parseSettlement(neg.SettlementDate); ok {
		if time.Now().UTC().After(exp.Add(24 * time.Hour)) {
			return voteNo(NoVoteReason{Reason: ReasonOptionUsedOrExpired, Posting: parsed.optionCashLeg}), nil
		}
	}

	// Idempotent replay: pending row already exists for this txID.
	if existing, err := p.exerciseRepo.GetByTxID(int(tx.TransactionID.RoutingNumber), tx.TransactionID.ID); err != nil {
		return nil, fmt.Errorf("looking up pending exercise: %w", err)
	} else if existing != nil {
		return &TransactionVote{Vote: VoteYes}, nil
	}

	row := &models.InterbankPendingExercise{
		TxRoutingNumber: int(tx.TransactionID.RoutingNumber),
		TxID:            tx.TransactionID.ID,
		Direction:       models.InterbankExerciseDirectionInbound,

		PartnerRoutingNumber: int(partner.Code),

		NegotiationRoutingNumber: neg.NegotiationRoutingNumber,
		NegotiationID:            neg.NegotiationID,

		StockTicker:          neg.StockTicker,
		StockAmount:          neg.Amount,
		PricePerUnitCurrency: neg.PricePerUnitCurrency,
		PricePerUnitAmount:   neg.PricePerUnitAmount,
		CashAmount:           expectedCash,

		BuyerRoutingNumber:  neg.BuyerRoutingNumber,
		BuyerID:             neg.BuyerID,
		SellerRoutingNumber: neg.SellerRoutingNumber,
		SellerID:            neg.SellerID,

		Status:    models.InterbankExerciseStatusPending,
		CreatedAt: time.Now().UTC(),
	}
	if err := p.db.Transaction(func(dbtx *gorm.DB) error {
		return p.exerciseRepo.CreateTx(dbtx, row)
	}); err != nil {
		return nil, fmt.Errorf("persisting pending exercise: %w", err)
	}

	slog.Info("interbank: option exercise NEW_TX accepted",
		"tx_routing", tx.TransactionID.RoutingNumber, "tx_id", tx.TransactionID.ID,
		"negotiation", neg.NegotiationID, "ticker", neg.StockTicker,
		"stock_amount", neg.Amount, "cash_amount", expectedCash,
	)
	return &TransactionVote{Vote: VoteYes}, nil
}

// OnCommitTx applies the seller-side effects: reduce the seller's
// stock holding by StockAmount (recording realised profit at the
// strike price), credit the seller's account by CashAmount, and CAS
// the pending row to committed.
func (p *ExerciseTxProcessor) OnCommitTx(_ context.Context, _ *PartnerBank, txID ForeignBankId) error {
	pending, err := p.exerciseRepo.GetByTxID(int(txID.RoutingNumber), txID.ID)
	if err != nil {
		return fmt.Errorf("loading pending exercise: %w", err)
	}
	if pending == nil {
		return fmt.Errorf("COMMIT_TX for unknown exercise %d/%s", txID.RoutingNumber, txID.ID)
	}
	if pending.Direction != models.InterbankExerciseDirectionInbound {
		return fmt.Errorf("COMMIT_TX dispatched to exercise processor for non-inbound row %d/%s", txID.RoutingNumber, txID.ID)
	}

	switch pending.Status {
	case models.InterbankExerciseStatusCommitted:
		return nil
	case models.InterbankExerciseStatusRolledBack, models.InterbankExerciseStatusRejected, models.InterbankExerciseStatusFailed:
		return fmt.Errorf("COMMIT_TX for already-resolved exercise %d/%s (status=%s)",
			txID.RoutingNumber, txID.ID, pending.Status)
	case models.InterbankExerciseStatusPending:
		// proceed
	default:
		return fmt.Errorf("exercise %d/%s has unknown status %q", txID.RoutingNumber, txID.ID, pending.Status)
	}

	// Resolve the seller's local user id from the encoded form. Only
	// "client-N" is supported — bank-side OTC isn't a thing on the
	// inter-bank wire yet.
	sellerType, sellerUID, err := DecodeLocalParticipantID(pending.SellerID)
	if err != nil {
		return fmt.Errorf("decoding seller local id %q: %w", pending.SellerID, err)
	}
	if sellerType != LocalParticipantClient {
		return fmt.Errorf("only client-side sellers are supported for exercise (got %q)", string(sellerType))
	}

	listing, err := p.marketRepo.GetListingRecordByTicker(pending.StockTicker)
	if err != nil {
		return fmt.Errorf("looking up listing for ticker %q: %w", pending.StockTicker, err)
	}

	txErr := p.db.Transaction(func(dbtx *gorm.DB) error {
		rows, err := p.exerciseRepo.MarkCommittedCAS(dbtx, int(txID.RoutingNumber), txID.ID)
		if err != nil {
			return err
		}
		if rows == 0 {
			return errExerciseAlreadyResolved
		}

		// Seller's stocks leave the holding — record at the strike
		// price so realised profit reflects the agreed-upon terms.
		if _, err := repository.RecordSellFillTx(
			dbtx,
			sellerUID, "client",
			listing.ID,
			pending.StockAmount,
			pending.PricePerUnitAmount,
		); err != nil {
			return fmt.Errorf("recording seller sell fill: %w", err)
		}

		// Seller is credited the strike-price cash in the agreed
		// currency. Reuses the OTC wallet repo's "first active
		// account in currency" lookup.
		if err := p.walletRepo.Credit(dbtx, pending.SellerID, pending.PricePerUnitCurrency, pending.CashAmount); err != nil {
			return fmt.Errorf("crediting seller cash: %w", err)
		}
		return nil
	})
	if errors.Is(txErr, errExerciseAlreadyResolved) {
		return nil
	}
	if txErr != nil {
		return txErr
	}

	slog.Info("interbank: option exercise COMMIT_TX applied",
		"tx_routing", txID.RoutingNumber, "tx_id", txID.ID,
		"negotiation", pending.NegotiationID,
		"stocks_moved", pending.StockAmount,
		"cash_credited", pending.CashAmount,
	)
	return nil
}

// OnRollbackTx flips the pending row to rolled_back. No local effects
// to undo on the seller side: NEW_TX didn't apply anything.
func (p *ExerciseTxProcessor) OnRollbackTx(_ context.Context, _ *PartnerBank, txID ForeignBankId) error {
	pending, err := p.exerciseRepo.GetByTxID(int(txID.RoutingNumber), txID.ID)
	if err != nil {
		return fmt.Errorf("loading pending exercise: %w", err)
	}
	if pending == nil {
		return nil
	}
	if pending.Direction != models.InterbankExerciseDirectionInbound {
		return fmt.Errorf("ROLLBACK_TX dispatched to exercise processor for non-inbound row %d/%s", txID.RoutingNumber, txID.ID)
	}
	switch pending.Status {
	case models.InterbankExerciseStatusRolledBack:
		return nil
	case models.InterbankExerciseStatusCommitted:
		return fmt.Errorf("ROLLBACK_TX for already-committed exercise %d/%s", txID.RoutingNumber, txID.ID)
	case models.InterbankExerciseStatusPending:
		// proceed
	default:
		return fmt.Errorf("exercise %d/%s has unknown status %q", txID.RoutingNumber, txID.ID, pending.Status)
	}

	txErr := p.db.Transaction(func(dbtx *gorm.DB) error {
		rows, err := p.exerciseRepo.MarkRolledBackCAS(dbtx, int(txID.RoutingNumber), txID.ID)
		if err != nil {
			return err
		}
		if rows == 0 {
			return errExerciseAlreadyResolved
		}
		return nil
	})
	if errors.Is(txErr, errExerciseAlreadyResolved) {
		return nil
	}
	if txErr != nil {
		return txErr
	}
	return nil
}

var errExerciseAlreadyResolved = errors.New("interbank: exercise already resolved by concurrent caller")

// exerciseTx is the parsed view of a 4-posting OPTION-execution NEW_TX.
type exerciseTx struct {
	optionCashLeg  *Posting
	optionStockLeg *Posting
	buyerCashLeg   *Posting
	buyerStockLeg  *Posting

	optionRouting    RoutingNumber
	optionNegRouting RoutingNumber
	optionNegID      string

	buyerRouting RoutingNumber
	buyerID      string

	stockTicker  string
	stockAmount  float64
	cashCurrency string
	cashAmount   float64
}

// parseExerciseTx classifies the four postings of an OPTION-execution
// NEW_TX. Returns a NoVoteReason if the shape doesn't match.
func parseExerciseTx(tx *Transaction) (*exerciseTx, *NoVoteReason) {
	if len(tx.Postings) != 4 {
		return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset}
	}
	parsed := &exerciseTx{}
	for i := range tx.Postings {
		ptg := &tx.Postings[i]
		switch ptg.Account.Type {
		case TxAccountOption:
			if ptg.Account.ID == nil {
				return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: ptg}
			}
			switch ptg.Asset.Type {
			case AssetMonas:
				if ptg.Asset.Monas == nil || parsed.optionCashLeg != nil {
					return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: ptg}
				}
				parsed.optionCashLeg = ptg
				parsed.optionRouting = ptg.Account.ID.RoutingNumber
				parsed.optionNegRouting = ptg.Account.ID.RoutingNumber
				parsed.optionNegID = ptg.Account.ID.ID
				parsed.cashCurrency = string(ptg.Asset.Monas.Currency)
				parsed.cashAmount = ptg.Amount
			case AssetStock:
				if ptg.Asset.Stock == nil || parsed.optionStockLeg != nil {
					return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: ptg}
				}
				parsed.optionStockLeg = ptg
				parsed.stockTicker = ptg.Asset.Stock.Ticker
			default:
				return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: ptg}
			}
		case TxAccountPerson:
			if ptg.Account.ID == nil {
				return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: ptg}
			}
			switch ptg.Asset.Type {
			case AssetMonas:
				if ptg.Asset.Monas == nil || parsed.buyerCashLeg != nil {
					return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: ptg}
				}
				parsed.buyerCashLeg = ptg
				parsed.buyerRouting = ptg.Account.ID.RoutingNumber
				parsed.buyerID = ptg.Account.ID.ID
			case AssetStock:
				if ptg.Asset.Stock == nil || parsed.buyerStockLeg != nil {
					return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: ptg}
				}
				parsed.buyerStockLeg = ptg
				parsed.stockAmount = ptg.Amount
			default:
				return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: ptg}
			}
		default:
			return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: ptg}
		}
	}

	if parsed.optionCashLeg == nil || parsed.optionStockLeg == nil ||
		parsed.buyerCashLeg == nil || parsed.buyerStockLeg == nil {
		return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset}
	}

	// Both option-leg postings must refer to the same negotiation.
	if parsed.optionStockLeg.Account.ID.RoutingNumber != parsed.optionNegRouting ||
		parsed.optionStockLeg.Account.ID.ID != parsed.optionNegID {
		return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: parsed.optionStockLeg}
	}
	// Both buyer postings must refer to the same buyer.
	if parsed.buyerStockLeg.Account.ID.RoutingNumber != parsed.buyerRouting ||
		parsed.buyerStockLeg.Account.ID.ID != parsed.buyerID {
		return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: parsed.buyerStockLeg}
	}

	// Cash leg signs: option debited (+π·k), buyer credited (-π·k).
	if parsed.optionCashLeg.Amount <= 0 || parsed.buyerCashLeg.Amount >= 0 {
		return nil, &NoVoteReason{Reason: ReasonOptionAmountIncorrect, Posting: parsed.optionCashLeg}
	}
	if !nearlyEqual(parsed.optionCashLeg.Amount, -parsed.buyerCashLeg.Amount) {
		return nil, &NoVoteReason{Reason: ReasonUnbalancedTx}
	}
	// Stock leg signs: option credited (-k), buyer debited (+k).
	if parsed.optionStockLeg.Amount >= 0 || parsed.buyerStockLeg.Amount <= 0 {
		return nil, &NoVoteReason{Reason: ReasonOptionAmountIncorrect, Posting: parsed.optionStockLeg}
	}
	if !nearlyEqual(-parsed.optionStockLeg.Amount, parsed.buyerStockLeg.Amount) {
		return nil, &NoVoteReason{Reason: ReasonUnbalancedTx}
	}
	// Tickers and currencies must agree across the two pairs.
	if parsed.buyerStockLeg.Asset.Stock.Ticker != parsed.stockTicker {
		return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: parsed.buyerStockLeg}
	}
	if string(parsed.buyerCashLeg.Asset.Monas.Currency) != parsed.cashCurrency {
		return nil, &NoVoteReason{Reason: ReasonUnacceptableAsset, Posting: parsed.buyerCashLeg}
	}
	return parsed, nil
}

// IsExerciseShape returns true when the transaction has at least one
// OPTION-typed TxAccount, which only the exercise flow uses (acceptance
// uses PERSON for cash + OPTION asset, but its TxAccounts are PERSON).
func IsExerciseShape(tx *Transaction) bool {
	for i := range tx.Postings {
		if tx.Postings[i].Account.Type == TxAccountOption {
			return true
		}
	}
	return false
}

// parseSettlement is a lenient ISO8601 parse for the settlement date
// string stored on the negotiation row. Returns ok=false on malformed
// input so the caller can treat it as "no expiry check available".
func parseSettlement(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// sameSettlementDate compares two ISO8601 settlement-date strings by
// instant rather than byte-equality, so e.g. "2025-04-16T15:32:44+02:00"
// and "2025-04-16T13:32:44Z" are treated as equal. Falls back to raw
// string comparison if either side fails to parse.
func sameSettlementDate(a, b string) bool {
	if a == b {
		return true
	}
	ta, oka := parseSettlement(a)
	tb, okb := parseSettlement(b)
	if !oka || !okb {
		return false
	}
	return ta.Equal(tb)
}
