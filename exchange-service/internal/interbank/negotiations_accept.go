package interbank

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
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
	// Close locally first so a concurrent second accept can't
	// double-dispatch.
	if err := h.repo.MarkClosed(neg.NegotiationRoutingNumber, neg.NegotiationID); err != nil {
		return AcceptOutcome{DispatchErr: fmt.Errorf("closing negotiation: %w", err)}
	}

	tx := buildOptionAcceptanceTx(h.registry.OwnRoutingNumber(), neg)
	txKey := h.client.NewIdempotenceKey()
	buyerCode := RoutingNumber(neg.BuyerRoutingNumber)

	vote, err := h.client.SendNewTx(ctx, buyerCode, txKey, &tx)
	if err != nil {
		slog.Error("interbank: NEW_TX dispatch failed during accept",
			"err", err, "negotiation", neg.NegotiationID, "buyer", buyerCode)
		h.reopenAfterDispatchFailure(neg.NegotiationRoutingNumber, neg.NegotiationID,
			neg.LastModifiedByRoutingNumber, neg.LastModifiedByID)
		return AcceptOutcome{DispatchErr: err}
	}

	if vote.Vote != VoteYes {
		// Buyer's bank refused — we don't send ROLLBACK_TX because
		// NEW_TX with vote=NO is itself the rollback; the buyer's
		// bank holds no resources after a NO. Reopen so participants
		// can keep going.
		slog.Info("interbank: NEW_TX received NO vote during accept",
			"negotiation", neg.NegotiationID, "buyer", buyerCode, "reasons", vote.Reasons)
		h.reopenAfterDispatchFailure(neg.NegotiationRoutingNumber, neg.NegotiationID,
			neg.LastModifiedByRoutingNumber, neg.LastModifiedByID)
		return AcceptOutcome{Vote: vote}
	}

	// YES vote — commit. If COMMIT_TX itself fails after retries we
	// leave the negotiation closed; the buyer's bank has already
	// voted to hold the resources, so resolution is operator-driven
	// (CHECK_STATUS / replay), not reopening the negotiation.
	commitKey := h.client.NewIdempotenceKey()
	if err := h.client.SendCommitTx(ctx, buyerCode, commitKey, tx.TransactionID); err != nil {
		slog.Error("interbank: COMMIT_TX dispatch failed after YES vote",
			"err", err, "negotiation", neg.NegotiationID, "transaction", tx.TransactionID.ID, "buyer", buyerCode)
		return AcceptOutcome{Vote: vote, CommitErr: err}
	}

	// COMMIT_TX succeeded — the partner has moved the premium out
	// of the buyer's wallet. Mirror it locally by crediting our
	// seller's wallet so the books match the protocol state. A
	// failure here is reported via CommitErr (same operator-action
	// signal as a COMMIT_TX failure) because the negotiation is
	// already closed and the partner has already committed.
	if err := h.creditLocalSeller(neg); err != nil {
		slog.Error("interbank: seller wallet credit failed after COMMIT_TX",
			"err", err, "negotiation", neg.NegotiationID, "transaction", tx.TransactionID.ID,
			"seller", neg.SellerID, "currency", neg.PremiumCurrency, "amount", neg.PremiumAmount)
		return AcceptOutcome{Vote: vote, CommitErr: fmt.Errorf("local seller credit failed after commit: %w", err)}
	}

	return AcceptOutcome{Vote: vote}
}

// creditLocalSeller credits the seller's wallet by the premium
// amount in a single DB transaction. The negotiation row was
// already moved to closed before dispatch, so there's no other
// local state to bundle.
//
// When db or walletRepo are nil (older test wiring), the credit is
// skipped silently — the protocol state is correct on the wire even
// without the local-books update, and the production path always
// supplies both deps via NewNegotiationsHandler in main.go.
func (h *NegotiationsHandler) creditLocalSeller(neg *models.InterbankOtcNegotiation) error {
	if h.db == nil || h.walletRepo == nil {
		return nil
	}
	return h.db.Transaction(func(tx *gorm.DB) error {
		return h.walletRepo.Credit(tx, neg.SellerID, neg.PremiumCurrency, neg.PremiumAmount)
	})
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
