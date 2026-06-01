package interbank

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// AcceptOutcome is the structured result of a runAcceptDispatch call.
// It lets the two HTTP entry points (partner-triggered /accept and the
// local-frontend POST /api/v1/interbank-otc/.../accept) translate the
// same dispatch outcome into their own response conventions.
type AcceptOutcome struct {
	// Vote is the buyer-bank's response to NEW_TX, or nil if NEW_TX
	// never got a reply (transport failure). On Vote=NO and on
	// transport failure the negotiation has been reopened.
	Vote *TransactionVote

	// DispatchErr is non-nil when NEW_TX itself failed (network, 5xx,
	// 202-poll timeout). The negotiation has been reopened.
	DispatchErr error

	// CommitErr is non-nil when NEW_TX returned YES but the follow-up
	// COMMIT_TX failed. The negotiation stays closed — operator
	// action is required (the buyer's bank has already promised to
	// hold the resources for our YES vote).
	CommitErr error
}

// accept handles GET /negotiations/{routingNumber}/{id}/accept (the
// partner-triggered entry point). Per spec §3.6 this is a GET that
// mutates state — accepting an open negotiation closes it and triggers
// an outbound NEW_TX from the seller's bank to the buyer's bank
// carrying the four postings that move the premium and create the
// option contract.
//
// Only the seller's bank can accept (mirrors the local OTC-5 rule).
// Authz here checks the calling partner; the actual dispatch logic
// is shared with AcceptForLocalSeller via runAcceptDispatch.
func (h *NegotiationsHandler) accept(w http.ResponseWriter, r *http.Request, routing RoutingNumber, id string) {
	partner := PartnerFromContext(r.Context())
	if partner == nil {
		writeProblemJSON(w, http.StatusUnauthorized, "no partner in context")
		return
	}

	neg, err := h.repo.Get(int(routing), id)
	if err != nil {
		writeProblemJSON(w, http.StatusInternalServerError, fmt.Sprintf("loading negotiation: %v", err))
		return
	}
	if neg == nil {
		writeProblemJSON(w, http.StatusNotFound, "no such negotiation")
		return
	}
	if !partnerMayAccess(neg, partner) {
		writeProblemJSON(w, http.StatusForbidden, "this X-Api-Key is not a party to that negotiation")
		return
	}
	if !neg.IsOngoing {
		writeProblemJSON(w, http.StatusConflict, "negotiation is no longer ongoing")
		return
	}
	if neg.LocalRole != models.InterbankNegotiationRoleSeller {
		writeProblemJSON(w, http.StatusForbidden,
			"only the seller's bank may accept — the buyer's bank cannot self-accept")
		return
	}

	outcome := h.runAcceptDispatch(r.Context(), neg)
	switch {
	case outcome.DispatchErr != nil:
		writeProblemJSON(w, http.StatusBadGateway, fmt.Sprintf("dispatching NEW_TX: %v", outcome.DispatchErr))
	case outcome.Vote != nil && outcome.Vote.Vote != VoteYes:
		writeJSON(w, http.StatusConflict, outcome.Vote)
	case outcome.CommitErr != nil:
		writeProblemJSON(w, http.StatusBadGateway,
			fmt.Sprintf("buyer voted YES but COMMIT_TX failed; operator action required: %v", outcome.CommitErr))
	default:
		writeJSON(w, http.StatusOK, outcome.Vote)
	}
}

// AcceptForLocalSeller is the local-frontend analogue of accept(). It
// validates that the caller's local seller id matches the negotiation
// and that the negotiation is in a state to be accepted, then runs the
// same dispatch as the partner-triggered path.
//
// statusCode > 0 means a precondition failed and the dispatch was not
// run; the caller should return that status with errMsg as the body.
// statusCode == 0 means dispatch ran and outcome carries the result.
func (h *NegotiationsHandler) AcceptForLocalSeller(
	ctx context.Context,
	routing RoutingNumber,
	id string,
	localSellerID string,
) (outcome AcceptOutcome, statusCode int, errMsg string) {
	neg, err := h.repo.Get(int(routing), id)
	if err != nil {
		return AcceptOutcome{}, http.StatusInternalServerError, fmt.Sprintf("loading negotiation: %v", err)
	}
	if neg == nil {
		return AcceptOutcome{}, http.StatusNotFound, "no such negotiation"
	}
	if neg.LocalRole != models.InterbankNegotiationRoleSeller {
		return AcceptOutcome{}, http.StatusForbidden,
			"only the local seller may accept — for buyer-side acceptance, the seller's bank must trigger the accept"
	}
	if neg.SellerID != localSellerID {
		return AcceptOutcome{}, http.StatusForbidden, "you are not the seller on that negotiation"
	}
	if !neg.IsOngoing {
		return AcceptOutcome{}, http.StatusConflict, "negotiation is no longer ongoing"
	}

	return h.runAcceptDispatch(ctx, neg), 0, ""
}

// runAcceptDispatch performs the close-and-dispatch sequence shared by
// the two accept entry points. Preconditions checked by the caller:
//   - neg loaded and IsOngoing == true
//   - neg.LocalRole == seller
//   - operator (partner or local user) is authorised to accept
//
// On NEW_TX transport failure or a NO vote, the negotiation is
// reopened so participants can resume haggling. On a YES vote followed
// by a COMMIT_TX failure the negotiation stays closed — the buyer's
// bank has already promised to hold the resources for our YES vote,
// so reopening would risk double-spend.
func (h *NegotiationsHandler) runAcceptDispatch(ctx context.Context, neg *models.InterbankOtcNegotiation) AcceptOutcome {
	// Close the negotiation AND reserve the seller's stock in one local
	// transaction (§2.7.2): the option contract we're about to form must
	// be backed by reserved shares, and closing first stops a concurrent
	// second accept from double-dispatching. If the seller can't back the
	// option, we abort before dispatch — the negotiation stays open so the
	// participants can renegotiate.
	reservedHoldingID, err := h.closeAndReserveSeller(neg)
	if err != nil {
		slog.Warn("interbank: accept aborted — could not reserve seller stock",
			"err", err, "negotiation", neg.NegotiationID)
		return AcceptOutcome{DispatchErr: fmt.Errorf("reserving seller stock: %w", err)}
	}

	tx := buildOptionAcceptanceTx(h.registry.OwnRoutingNumber(), neg)
	txKey := h.client.NewIdempotenceKey()
	buyerCode := RoutingNumber(neg.BuyerRoutingNumber)

	vote, err := h.client.SendNewTx(ctx, buyerCode, txKey, &tx)
	if err != nil {
		slog.Error("interbank: NEW_TX dispatch failed during accept",
			"err", err, "negotiation", neg.NegotiationID, "buyer", buyerCode)
		// We don't know whether the buyer processed the NEW_TX and voted
		// YES (reserving the premium) before the response was lost. Send a
		// best-effort ROLLBACK_TX so any premium the buyer reserved is
		// released — it's a no-op at the buyer if they never saw the
		// NEW_TX. Then release our own reservation and reopen.
		h.bestEffortRollback(buyerCode, tx.TransactionID, neg.NegotiationID)
		h.releaseSellerReservation(neg, reservedHoldingID)
		h.reopenAfterDispatchFailure(neg.NegotiationRoutingNumber, neg.NegotiationID,
			neg.LastModifiedByRoutingNumber, neg.LastModifiedByID)
		return AcceptOutcome{DispatchErr: err}
	}

	if vote.Vote != VoteYes {
		// Buyer's bank refused — we don't send ROLLBACK_TX because
		// NEW_TX with vote=NO is itself the rollback; the buyer's
		// bank holds no resources after a NO. Release our reservation
		// and reopen so participants can keep going.
		slog.Info("interbank: NEW_TX received NO vote during accept",
			"negotiation", neg.NegotiationID, "buyer", buyerCode, "reasons", vote.Reasons)
		h.releaseSellerReservation(neg, reservedHoldingID)
		h.reopenAfterDispatchFailure(neg.NegotiationRoutingNumber, neg.NegotiationID,
			neg.LastModifiedByRoutingNumber, neg.LastModifiedByID)
		return AcceptOutcome{Vote: vote}
	}

	// YES vote — record the dispatched transactionId BEFORE committing, so
	// a crash between here and a confirmed COMMIT_TX leaves a row the
	// reconcile cron can finish (otherwise the buyer's reserved premium is
	// stranded). The negotiation stays closed; the buyer's bank has voted
	// to hold the resources, so resolution is by replay, not reopening.
	if err := h.repo.SetAcceptDispatched(neg.NegotiationRoutingNumber, neg.NegotiationID,
		int(tx.TransactionID.RoutingNumber), tx.TransactionID.ID); err != nil {
		slog.Error("interbank: persisting accept dispatch state failed",
			"err", err, "negotiation", neg.NegotiationID, "transaction", tx.TransactionID.ID)
		return AcceptOutcome{Vote: vote, CommitErr: err}
	}

	commitKey := h.client.NewIdempotenceKey()
	if err := h.client.SendCommitTx(ctx, buyerCode, commitKey, tx.TransactionID); err != nil {
		slog.Error("interbank: COMMIT_TX dispatch failed after YES vote; reconcile cron will resend",
			"err", err, "negotiation", neg.NegotiationID, "transaction", tx.TransactionID.ID, "buyer", buyerCode)
		return AcceptOutcome{Vote: vote, CommitErr: err}
	}

	// COMMIT_TX succeeded — credit our seller and stamp the accept as
	// finalised in one transaction so the credit lands exactly once even
	// if the cron also runs. A failure here is reported via CommitErr; the
	// cron retries (idempotent COMMIT_TX + CAS-guarded credit).
	if err := FinaliseAcceptCommit(h.db, h.walletRepo, h.repo, neg); err != nil {
		slog.Error("interbank: seller credit / finalise failed after COMMIT_TX; cron will retry",
			"err", err, "negotiation", neg.NegotiationID, "transaction", tx.TransactionID.ID,
			"seller", neg.SellerID, "currency", neg.PremiumCurrency, "amount", neg.PremiumAmount)
		return AcceptOutcome{Vote: vote, CommitErr: fmt.Errorf("local seller credit failed after commit: %w", err)}
	}

	return AcceptOutcome{Vote: vote}
}

// FinaliseAcceptCommit credits the local seller's premium and stamps the
// negotiation's accept-commit as finalised, atomically, so the credit
// happens exactly once across retries. Shared by the inline accept path
// and the reconcile cron. The stamp is a CAS (WHERE finalised IS NULL):
// the credit only runs when this caller wins the CAS, so a concurrent
// retry can't double-credit. For bank-side sellers (no client wallet) the
// credit is skipped but the stamp still lands. A nil db/negRepo (older
// test wiring) is a silent no-op.
func FinaliseAcceptCommit(
	db *gorm.DB,
	walletRepo *repository.InterbankWalletRepository,
	negRepo *repository.InterbankOtcRepository,
	neg *models.InterbankOtcNegotiation,
) error {
	if db == nil || negRepo == nil {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		won, err := negRepo.MarkAcceptCommitFinalisedCASTx(tx, neg.NegotiationRoutingNumber, neg.NegotiationID)
		if err != nil {
			return err
		}
		if won == 0 {
			// Already finalised by a concurrent caller — don't credit again.
			return nil
		}
		if walletRepo == nil {
			return nil
		}
		// Only client sellers hold a local wallet to credit.
		if sellerType, _, derr := DecodeLocalParticipantID(neg.SellerID); derr != nil || sellerType != LocalParticipantClient {
			return nil
		}
		return walletRepo.Credit(tx, neg.SellerID, neg.PremiumCurrency, neg.PremiumAmount)
	})
}

// bestEffortRollback sends a ROLLBACK_TX to the buyer's bank on its own
// short-lived context, used when a NEW_TX dispatch failed and we can't
// tell whether the buyer reserved the premium. A failure here is logged,
// not surfaced — if the buyer did reserve and this also fails, their own
// reconcile / operator path is the backstop.
func (h *NegotiationsHandler) bestEffortRollback(buyerCode RoutingNumber, txID ForeignBankId, negID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key := h.client.NewIdempotenceKey()
	if err := h.client.SendRollbackTx(ctx, buyerCode, key, txID); err != nil {
		slog.Warn("interbank: best-effort ROLLBACK_TX after accept NEW_TX failure did not land",
			"err", err, "negotiation", negID, "transaction", txID.ID, "buyer", buyerCode)
	}
}

// closeAndReserveSeller closes the negotiation and reserves the seller's
// stock to back the option contract (§2.7.2), atomically. It returns the
// id of the holding reserved against, or nil when no reservation was
// taken (bank-side seller, or test wiring without portfolio/market
// repos). A non-nil error means the seller cannot back the option and the
// negotiation was left open — the caller must NOT dispatch.
func (h *NegotiationsHandler) closeAndReserveSeller(neg *models.InterbankOtcNegotiation) (*uint, error) {
	// Without the ledger repos (older/test wiring) we can't reserve — just
	// close. The wire state stays correct; only the local-books
	// reservation is skipped.
	if h.db == nil || h.portfolioRepo == nil || h.marketRepo == nil {
		if err := h.repo.MarkClosed(neg.NegotiationRoutingNumber, neg.NegotiationID); err != nil {
			return nil, err
		}
		return nil, nil
	}

	// Bank-side sellers hold no portfolio reservation — exercise likewise
	// only supports client sellers. Close without reserving.
	sellerType, sellerUID, err := DecodeLocalParticipantID(neg.SellerID)
	if err != nil || sellerType != LocalParticipantClient {
		if cErr := h.repo.MarkClosed(neg.NegotiationRoutingNumber, neg.NegotiationID); cErr != nil {
			return nil, cErr
		}
		return nil, nil
	}

	listing, err := h.marketRepo.GetListingRecordByTicker(neg.StockTicker)
	if err != nil {
		return nil, fmt.Errorf("looking up listing for %q: %w", neg.StockTicker, err)
	}
	if listing == nil {
		return nil, fmt.Errorf("no listing for ticker %q", neg.StockTicker)
	}
	holding, err := h.portfolioRepo.GetHoldingByUserAndAsset(sellerUID, "client", listing.ID)
	if err != nil {
		return nil, fmt.Errorf("looking up seller holding: %w", err)
	}
	if holding == nil {
		return nil, fmt.Errorf("seller holds no %s to back the option", neg.StockTicker)
	}

	holdingID := holding.ID
	if err := h.db.Transaction(func(dbtx *gorm.DB) error {
		if err := h.portfolioRepo.ReserveHoldingQuantityTx(dbtx, holdingID, neg.Amount); err != nil {
			return err
		}
		return dbtx.Model(&models.InterbankOtcNegotiation{}).
			Where("negotiation_routing_number = ? AND negotiation_id = ?",
				neg.NegotiationRoutingNumber, neg.NegotiationID).
			Updates(map[string]interface{}{
				"is_ongoing":                 false,
				"seller_reserved_holding_id": holdingID,
				"updated_at":                 time.Now().UTC(),
			}).Error
	}); err != nil {
		return nil, err
	}
	return &holdingID, nil
}

// releaseSellerReservation undoes the reservation taken by
// closeAndReserveSeller. Called when a dispatch fails or the buyer's bank
// votes NO, so the seller's shares aren't stranded. Best-effort: a
// failure here is logged, not surfaced — the settlement-expiry sweep is
// the backstop that reclaims any reservation left behind.
func (h *NegotiationsHandler) releaseSellerReservation(neg *models.InterbankOtcNegotiation, holdingID *uint) {
	if holdingID == nil || h.db == nil || h.repo == nil {
		return
	}
	if err := h.db.Transaction(func(dbtx *gorm.DB) error {
		return h.repo.ReleaseSellerReservationTx(dbtx,
			neg.NegotiationRoutingNumber, neg.NegotiationID, *holdingID, neg.Amount)
	}); err != nil {
		slog.Error("interbank: releasing seller reservation after failed accept",
			"err", err, "negotiation", neg.NegotiationID, "holding", *holdingID)
	}
}

// buildOptionAcceptanceTx builds the protocol Transaction that
// /accept dispatches: four postings expressing the premium transfer
// (cash leg) and option-contract creation (option leg).
//
//	cash leg:    buyer -P   premium currency      → seller +P
//	option leg:  seller -1  OPTION{neg, stock, …} → buyer  +1
//
// The TransactionID is owned by the seller's bank (= us) since
// we initiated the NEW_TX. The buyer's bank stores it on receipt.
func buildOptionAcceptanceTx(ownRouting RoutingNumber, neg *models.InterbankOtcNegotiation) Transaction {
	buyer := TxAccount{
		Type: TxAccountPerson,
		ID: &ForeignBankId{
			RoutingNumber: RoutingNumber(neg.BuyerRoutingNumber),
			ID:            neg.BuyerID,
		},
	}
	seller := TxAccount{
		Type: TxAccountPerson,
		ID: &ForeignBankId{
			RoutingNumber: RoutingNumber(neg.SellerRoutingNumber),
			ID:            neg.SellerID,
		},
	}

	premiumAsset := Asset{
		Type:  AssetMonas,
		Monas: &MonetaryAsset{Currency: CurrencyCode(neg.PremiumCurrency)},
	}

	optionAsset := Asset{
		Type: AssetOption,
		Option: &OptionDescription{
			NegotiationID: ForeignBankId{
				RoutingNumber: RoutingNumber(neg.NegotiationRoutingNumber),
				ID:            neg.NegotiationID,
			},
			Stock: StockDescription{Ticker: neg.StockTicker},
			PricePerUnit: MonetaryValue{
				Currency: CurrencyCode(neg.PricePerUnitCurrency),
				Amount:   neg.PricePerUnitAmount,
			},
			SettlementDate: neg.SettlementDate,
			Amount:         neg.Amount,
		},
	}

	return Transaction{
		Postings: []Posting{
			{Account: buyer, Amount: -neg.PremiumAmount, Asset: premiumAsset},
			{Account: seller, Amount: neg.PremiumAmount, Asset: premiumAsset},
			{Account: seller, Amount: -1, Asset: optionAsset},
			{Account: buyer, Amount: 1, Asset: optionAsset},
		},
		TransactionID: ForeignBankId{
			RoutingNumber: ownRouting,
			ID:            uuid.NewString(),
		},
		Message:        fmt.Sprintf("OTC option acceptance for negotiation %s", neg.NegotiationID),
		PaymentCode:    "OTC",
		PaymentPurpose: "OTC option contract premium + creation",
	}
}

// reopenAfterDispatchFailure flips IsOngoing back to true so the
// participants can resume the negotiation. We don't roll back to a
// prior offer state — the most recent terms stay on record, just
// re-marked open.
func (h *NegotiationsHandler) reopenAfterDispatchFailure(routing int, id string, lastModRouting int, lastModID string) {
	if err := h.repo.MarkOngoing(routing, id, lastModRouting, lastModID); err != nil {
		slog.Error("interbank: reopening negotiation after dispatch failure",
			"err", err, "negotiation", id)
	}
}
