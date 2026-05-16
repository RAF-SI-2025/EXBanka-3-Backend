package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// listOptionContracts returns the caller's cross-bank option contracts.
// We only persist InterbankOptionContract rows on the BUYER side (the
// seller has the underlying portfolio holding instead), so this list
// is implicitly "options I bought from a partner bank".
func (h *InterbankOtcHTTPHandler) listOptionContracts(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}
	localID, ok := localParticipantIDFromClaims(claims)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "only client tokens can list interbank option contracts"})
		return
	}
	rows, err := h.contractRepo.ListByBuyerLocalID(localID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": fmt.Sprintf("listing option contracts: %v", err)})
		return
	}
	items := make([]map[string]interface{}, 0, len(rows))
	for i := range rows {
		items = append(items, optionContractRowToResponse(&rows[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contracts": items,
		"count":     len(items),
	})
}

func (h *InterbankOtcHTTPHandler) getOptionContract(w http.ResponseWriter, r *http.Request, id uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}
	localID, ok := localParticipantIDFromClaims(claims)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "only client tokens can read interbank option contracts"})
		return
	}
	row, err := h.contractRepo.GetByID(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": fmt.Sprintf("loading contract: %v", err)})
		return
	}
	if row == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "no such contract"})
		return
	}
	if row.BuyerLocalID != localID {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "you do not own that contract"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contract": optionContractRowToResponse(row),
	})
}

// exerciseOptionContract is the buyer-side initiator for cross-bank
// option exercise (protocol §2.7.2). The flow:
//
//  1. Load the local InterbankOptionContract, verify ownership, status,
//     and that the settlement window hasn't passed (mirror of the
//     buyer-bank validation that the partner will also enforce).
//  2. Build the 4-posting NEW_TX (OPTION pseudo + PERSON buyer; MONAS +
//     STOCK assets). The OPTION pseudo lives at the seller's bank.
//  3. In a single DB tx: reserve the buyer's strike-price cash and
//     persist InterbankPendingExercise (direction=outbound).
//  4. Send NEW_TX to the seller's bank. Apply the response:
//     YES → debit cash, add stock holding, mark contract exercised,
//     mark pending row committed; then send COMMIT_TX.
//     NO  → release cash, mark pending rejected.
//     err → release cash, mark pending failed; best-effort ROLLBACK_TX.
func (h *InterbankOtcHTTPHandler) exerciseOptionContract(w http.ResponseWriter, r *http.Request, contractID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}
	localID, ok := localParticipantIDFromClaims(claims)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "only client tokens can exercise interbank option contracts"})
		return
	}

	contract, err := h.contractRepo.GetByID(contractID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": fmt.Sprintf("loading contract: %v", err)})
		return
	}
	if contract == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "no such contract"})
		return
	}
	if contract.BuyerLocalID != localID {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "you do not own that contract"})
		return
	}
	if contract.Status != models.InterbankOptionContractStatusValid {
		writeJSON(w, http.StatusConflict, map[string]string{"message": fmt.Sprintf("contract status %q, cannot exercise", contract.Status)})
		return
	}
	if exp, parsed := parseContractSettlement(contract.SettlementDate); parsed {
		if time.Now().UTC().After(exp.Add(24 * time.Hour)) {
			writeJSON(w, http.StatusConflict, map[string]string{"message": "contract settlement window has passed"})
			return
		}
	}

	// Look up the asset by ticker. Required for the buyer-side
	// portfolio holding upsert on COMMIT.
	listing, err := h.marketRepo.GetListingRecordByTicker(contract.StockTicker)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": fmt.Sprintf("unknown asset ticker %q: %v", contract.StockTicker, err)})
		return
	}

	cashAmount := contract.PricePerUnitAmount * contract.Amount

	ownRouting := h.registry.OwnRoutingNumber()
	txID := interbank.ForeignBankId{RoutingNumber: ownRouting, ID: uuid.NewString()}
	sellerRouting := interbank.RoutingNumber(contract.SellerRoutingNumber)

	// Phase 1: reserve cash locally + persist outbound pending row.
	var (
		pendingRow     *models.InterbankPendingExercise
		buyerAccountID uint
	)
	prepareErr := h.db.Transaction(func(dbtx *gorm.DB) error {
		accID, lookupErr := h.walletRepo.LookupClientAccountID(dbtx, localID, contract.PricePerUnitCurrency)
		if errors.Is(lookupErr, repository.ErrInterbankWalletInsufficient) {
			return preparePaymentErr{
				status: http.StatusUnprocessableEntity,
				msg:    fmt.Sprintf("no active %s account on your profile to debit the strike price from", contract.PricePerUnitCurrency),
			}
		}
		if lookupErr != nil {
			return lookupErr
		}
		if err := h.walletRepo.Reserve(dbtx, localID, contract.PricePerUnitCurrency, cashAmount); err != nil {
			if errors.Is(err, repository.ErrInterbankWalletInsufficient) {
				return preparePaymentErr{
					status: http.StatusUnprocessableEntity,
					msg:    fmt.Sprintf("insufficient available balance to cover strike price %.2f %s", cashAmount, contract.PricePerUnitCurrency),
				}
			}
			return err
		}
		buyerAccountID = accID
		clientID := claims.ClientID
		ctrID := contract.ID
		row := &models.InterbankPendingExercise{
			TxRoutingNumber:          int(txID.RoutingNumber),
			TxID:                     txID.ID,
			Direction:                models.InterbankExerciseDirectionOutbound,
			PartnerRoutingNumber:     int(sellerRouting),
			NegotiationRoutingNumber: contract.NegotiationRoutingNumber,
			NegotiationID:            contract.NegotiationID,
			StockTicker:              contract.StockTicker,
			StockAmount:              contract.Amount,
			PricePerUnitCurrency:     contract.PricePerUnitCurrency,
			PricePerUnitAmount:       contract.PricePerUnitAmount,
			CashAmount:               cashAmount,
			BuyerRoutingNumber:       int(ownRouting),
			BuyerID:                  localID,
			SellerRoutingNumber:      contract.SellerRoutingNumber,
			SellerID:                 contract.SellerID,
			BuyerLocalAccountID:      &buyerAccountID,
			BuyerLocalClientID:       &clientID,
			OptionContractID:         &ctrID,
			Status:                   models.InterbankExerciseStatusPending,
			CreatedAt:                time.Now().UTC(),
		}
		if err := h.exerciseRepo.CreateTx(dbtx, row); err != nil {
			return err
		}
		pendingRow = row
		return nil
	})
	if prepareErr != nil {
		var pe preparePaymentErr
		if errors.As(prepareErr, &pe) {
			writeJSON(w, pe.status, map[string]string{"message": pe.msg})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": fmt.Sprintf("preparing exercise: %v", prepareErr)})
		return
	}

	// Phase 2 — NEW_TX outbound.
	tx := buildExerciseTx(txID, contract, ownRouting, cashAmount)
	idemKey := h.client.NewIdempotenceKey()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	vote, dispatchErr := h.client.SendNewTx(ctx, sellerRouting, idemKey, &tx)
	if dispatchErr != nil {
		h.finaliseExerciseTransportFailure(pendingRow, contract, sellerRouting, dispatchErr)
		writeJSON(w, http.StatusBadGateway, map[string]interface{}{
			"message":    fmt.Sprintf("dispatching NEW_TX to seller's bank failed: %v", dispatchErr),
			"exerciseId": pendingRow.ID,
			"status":     models.InterbankExerciseStatusFailed,
		})
		return
	}
	if vote.Vote != interbank.VoteYes {
		reason := summariseNoVote(vote)
		if err := h.finaliseExerciseRejected(pendingRow, contract, reason); err != nil {
			slog.Error("interbank: exercise rejection finalise failed",
				"err", err, "exercise_id", pendingRow.ID)
		}
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"message":    "seller's bank refused the exercise",
			"exerciseId": pendingRow.ID,
			"status":     models.InterbankExerciseStatusRejected,
			"vote":       vote,
		})
		return
	}

	// YES — apply local commit: debit cash, add stock holding, mark
	// contract exercised, CAS pending row committed. Then send
	// COMMIT_TX. A local commit failure leaves the row in pending
	// for the reconciliation cron to retry (future work).
	if err := h.finaliseExerciseCommit(pendingRow, contract, listing.ID, buyerAccountID, claims.ClientID); err != nil {
		slog.Error("interbank: exercise local commit failed after YES vote",
			"err", err, "exercise_id", pendingRow.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"message":    fmt.Sprintf("seller's bank voted YES but local commit failed: %v", err),
			"exerciseId": pendingRow.ID,
		})
		return
	}

	commitKey := h.client.NewIdempotenceKey()
	if err := h.client.SendCommitTx(ctx, sellerRouting, commitKey, txID); err != nil {
		slog.Warn("interbank: exercise COMMIT_TX dispatch failed after local commit",
			"err", err, "exercise_id", pendingRow.ID, "tx_id", txID.ID)
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"message":    fmt.Sprintf("local commit succeeded but COMMIT_TX dispatch failed; partner will reconcile: %v", err),
			"exerciseId": pendingRow.ID,
			"status":     models.InterbankExerciseStatusCommitted,
		})
		return
	}
	h.markExerciseCommitDispatched(pendingRow)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"exerciseId": pendingRow.ID,
		"status":     models.InterbankExerciseStatusCommitted,
		"contract":   optionContractRowToResponse(contract),
	})
}

// finaliseExerciseCommit applies the buyer-side local effects after a
// YES vote: cash debit, portfolio holding upsert, contract status flip,
// pending row CAS. All four land in one DB transaction.
func (h *InterbankOtcHTTPHandler) finaliseExerciseCommit(
	row *models.InterbankPendingExercise,
	contract *models.InterbankOptionContract,
	assetID, buyerAccountID, clientID uint,
) error {
	err := h.db.Transaction(func(dbtx *gorm.DB) error {
		rows, err := h.exerciseRepo.MarkCommittedCAS(dbtx, row.TxRoutingNumber, row.TxID)
		if err != nil {
			return err
		}
		if rows == 0 {
			return nil
		}

		// Debit the strike-price cash.
		if err := h.walletRepo.Debit(dbtx, row.BuyerID, row.PricePerUnitCurrency, row.CashAmount); err != nil {
			return fmt.Errorf("debiting buyer cash: %w", err)
		}

		// Add the stocks to the buyer's portfolio. RecordBuyFill
		// handles weighted-average update if a holding already
		// exists for this asset.
		if err := upsertBuyerStockHolding(dbtx, clientID, assetID, buyerAccountID, row.StockAmount, row.PricePerUnitAmount); err != nil {
			return fmt.Errorf("recording stock fill: %w", err)
		}

		// Flip the contract to exercised.
		if _, err := h.contractRepo.MarkExercisedCAS(dbtx, contract.ID); err != nil {
			return fmt.Errorf("marking contract exercised: %w", err)
		}
		row.Status = models.InterbankExerciseStatusCommitted
		contract.Status = models.InterbankOptionContractStatusExercised
		return nil
	})
	return err
}

// finaliseExerciseRejected releases the cash reservation, marks the
// row rejected, and stamps partner_finalised_at (no terminal partner
// message owed — partner voted NO and holds no resources).
func (h *InterbankOtcHTTPHandler) finaliseExerciseRejected(row *models.InterbankPendingExercise, _ *models.InterbankOptionContract, reason string) error {
	return h.db.Transaction(func(dbtx *gorm.DB) error {
		rows, err := h.exerciseRepo.MarkRejectedCAS(dbtx, row.TxRoutingNumber, row.TxID, reason)
		if err != nil {
			return err
		}
		if rows == 0 {
			return nil
		}
		if _, err := h.exerciseRepo.MarkPartnerFinalised(dbtx, row.TxRoutingNumber, row.TxID); err != nil {
			return err
		}
		row.Status = models.InterbankExerciseStatusRejected
		row.LastError = reason
		return h.walletRepo.Release(dbtx, row.BuyerID, row.PricePerUnitCurrency, row.CashAmount)
	})
}

// finaliseExerciseTransportFailure releases the cash reservation and
// marks failed, then best-effort sends ROLLBACK_TX so the partner can
// drop any partial state.
func (h *InterbankOtcHTTPHandler) finaliseExerciseTransportFailure(row *models.InterbankPendingExercise, _ *models.InterbankOptionContract, sellerRouting interbank.RoutingNumber, dispatchErr error) {
	txErr := h.db.Transaction(func(dbtx *gorm.DB) error {
		rows, err := h.exerciseRepo.MarkFailedCAS(dbtx, row.TxRoutingNumber, row.TxID, dispatchErr.Error())
		if err != nil {
			return err
		}
		if rows == 0 {
			return nil
		}
		row.Status = models.InterbankExerciseStatusFailed
		row.LastError = dispatchErr.Error()
		return h.walletRepo.Release(dbtx, row.BuyerID, row.PricePerUnitCurrency, row.CashAmount)
	})
	if txErr != nil {
		slog.Error("interbank: exercise transport-failure finalise failed",
			"err", txErr, "exercise_id", row.ID)
		return
	}

	rollbackCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rbKey := h.client.NewIdempotenceKey()
	txID := interbank.ForeignBankId{
		RoutingNumber: interbank.RoutingNumber(row.TxRoutingNumber),
		ID:            row.TxID,
	}
	if err := h.client.SendRollbackTx(rollbackCtx, sellerRouting, rbKey, txID); err != nil {
		slog.Warn("interbank: best-effort ROLLBACK_TX failed after exercise transport failure",
			"err", err, "exercise_id", row.ID)
		return
	}
	if err := h.db.Transaction(func(dbtx *gorm.DB) error {
		_, mErr := h.exerciseRepo.MarkPartnerFinalised(dbtx, row.TxRoutingNumber, row.TxID)
		return mErr
	}); err != nil {
		slog.Warn("interbank: failed to stamp exercise partner_finalised_at after ROLLBACK_TX",
			"err", err, "exercise_id", row.ID)
	}
}

func (h *InterbankOtcHTTPHandler) markExerciseCommitDispatched(row *models.InterbankPendingExercise) {
	if err := h.db.Transaction(func(dbtx *gorm.DB) error {
		_, mErr := h.exerciseRepo.MarkPartnerFinalised(dbtx, row.TxRoutingNumber, row.TxID)
		return mErr
	}); err != nil {
		slog.Warn("interbank: failed to stamp exercise partner_finalised_at after COMMIT_TX",
			"err", err, "exercise_id", row.ID)
	}
}

// upsertBuyerStockHolding adds the buyer's newly-acquired stocks to
// their local portfolio at the strike price. Delegates to the existing
// portfolio repo's tx-aware helper so the buy-fill update lands inside
// the caller's transaction.
func upsertBuyerStockHolding(tx *gorm.DB, clientID, assetID, accountID uint, qty, price float64) error {
	now := time.Now().UTC()
	var h models.PortfolioHoldingRecord
	err := tx.Where("user_id = ? AND user_type = ? AND asset_id = ?", clientID, "client", assetID).
		First(&h).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h = models.PortfolioHoldingRecord{
			UserID:      clientID,
			UserType:    "client",
			AssetID:     assetID,
			AccountID:   accountID,
			Quantity:    qty,
			AvgBuyPrice: price,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		return tx.Create(&h).Error
	}
	if err != nil {
		return err
	}
	newQty := h.Quantity + qty
	newAvg := (h.Quantity*h.AvgBuyPrice + qty*price) / newQty
	return tx.Model(&h).Updates(map[string]interface{}{
		"quantity":      newQty,
		"avg_buy_price": newAvg,
		"account_id":    accountID,
		"updated_at":    now,
	}).Error
}

// buildExerciseTx assembles the 4-posting OPTION-execution NEW_TX per
// protocol §2.7.2. The OPTION pseudo-account is at the seller's bank;
// the PERSON account is the buyer in our bank.
func buildExerciseTx(txID interbank.ForeignBankId, contract *models.InterbankOptionContract, ownRouting interbank.RoutingNumber, cashAmount float64) interbank.Transaction {
	monas := interbank.Asset{
		Type:  interbank.AssetMonas,
		Monas: &interbank.MonetaryAsset{Currency: interbank.CurrencyCode(contract.PricePerUnitCurrency)},
	}
	stock := interbank.Asset{
		Type:  interbank.AssetStock,
		Stock: &interbank.StockDescription{Ticker: contract.StockTicker},
	}
	optionAcc := interbank.TxAccount{
		Type: interbank.TxAccountOption,
		ID: &interbank.ForeignBankId{
			RoutingNumber: interbank.RoutingNumber(contract.NegotiationRoutingNumber),
			ID:            contract.NegotiationID,
		},
	}
	buyerAcc := interbank.TxAccount{
		Type: interbank.TxAccountPerson,
		ID: &interbank.ForeignBankId{
			RoutingNumber: ownRouting,
			ID:            contract.BuyerLocalID,
		},
	}
	return interbank.Transaction{
		TransactionID:  txID,
		Message:        fmt.Sprintf("OTC option exercise for negotiation %s", contract.NegotiationID),
		PaymentCode:    "OTC",
		PaymentPurpose: "OTC option exercise — cash for stocks",
		Postings: []interbank.Posting{
			{Account: optionAcc, Amount: cashAmount, Asset: monas},   // option debited cash (+π·k)
			{Account: buyerAcc, Amount: -cashAmount, Asset: monas},   // buyer credited cash (-π·k)
			{Account: optionAcc, Amount: -contract.Amount, Asset: stock}, // option credited stocks (-k)
			{Account: buyerAcc, Amount: contract.Amount, Asset: stock},   // buyer debited stocks (+k)
		},
	}
}

// parseContractSettlement is a lenient ISO8601 parse for the
// contract.SettlementDate string. Mirrors the parser in the seller-side
// exercise processor so both ends apply the same expiry rule.
func parseContractSettlement(s string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func optionContractRowToResponse(c *models.InterbankOptionContract) map[string]interface{} {
	return map[string]interface{}{
		"id":                       c.ID,
		"negotiationRoutingNumber": c.NegotiationRoutingNumber,
		"negotiationId":            c.NegotiationID,
		"buyerLocalId":             c.BuyerLocalID,
		"sellerRoutingNumber":      c.SellerRoutingNumber,
		"sellerId":                 c.SellerID,
		"stockTicker":              c.StockTicker,
		"amount":                   c.Amount,
		"pricePerUnit":             map[string]interface{}{"currency": c.PricePerUnitCurrency, "amount": c.PricePerUnitAmount},
		"premium":                  map[string]interface{}{"currency": c.PremiumCurrency, "amount": c.PremiumAmount},
		"settlementDate":           c.SettlementDate,
		"status":                   c.Status,
		"createdAt":                c.CreatedAt,
		"updatedAt":                c.UpdatedAt,
	}
}
