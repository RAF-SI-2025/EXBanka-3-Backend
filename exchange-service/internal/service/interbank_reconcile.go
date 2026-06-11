package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// InterbankReconcileRunner drives the BANKA-BE-8 reconciliation cron
// for cross-bank payments. Two responsibilities per tick:
//
//  1. Re-dispatch NEW_TX for outbound payments stuck in `pending` past
//     a staleness threshold. This covers crashes between Reserve and
//     SendNewTx, partner timeouts, and 202-poll budget exhaustion.
//     The partner's business-level idempotency (keyed by transactionId)
//     replays the cached vote without re-applying effects, so a retry
//     with a fresh idempotence key is safe.
//
//  2. Re-dispatch terminal messages (COMMIT_TX for committed,
//     ROLLBACK_TX for failed) where partner_finalised_at is still NULL.
//     Receiver-side idempotency makes resends a no-op once the partner
//     has applied the effect.
//
// `rejected` rows are never re-dispatched — the partner voted NO and
// holds no resources, so no terminal message is owed; finaliseRejected
// stamps partner_finalised_at at the same time as the status flip.
//
// The runner is intentionally pull-driven (cron scans rows on each
// tick) instead of push-driven (queued from the HTTP path) so a crash
// of the inline path can't lose work — the row in the DB is the
// authoritative source of "this needs reconciling".
type InterbankReconcileRunner struct {
	db          *gorm.DB
	registry    *interbank.Registry
	client      *interbank.Client
	paymentRepo *repository.InterbankPaymentRepository
	walletRepo  *repository.InterbankPaymentWalletRepository

	// Exercise reconciliation deps. exerciseRepo + otcWalletRepo drive
	// the cross-bank OTC option-exercise recovery (terminal resend +
	// stuck-pending rollback). Both may be nil — when so, the exercise
	// half of Run() is skipped, so existing payment-only wiring keeps
	// working.
	exerciseRepo  *repository.InterbankExerciseRepository
	otcWalletRepo *repository.InterbankWalletRepository

	// negRepo drives OTC option-acceptance COMMIT_TX recovery (resend
	// COMMIT_TX + seller credit for accepts that got a YES vote but never
	// confirmed). Optional; nil skips that pass.
	negRepo *repository.InterbankOtcRepository

	// staleness is how old updated_at must be before a row is eligible.
	// Defaults to 5 minutes; tests can override to exercise the cron
	// without waiting wall-clock time.
	staleness time.Duration

	// batchSize caps rows examined per tick to keep one slow partner
	// from starving the scheduler.
	batchSize int
}

// NewInterbankReconcileRunner wires the cron runner. exerciseRepo and
// otcWalletRepo are optional; pass them to enable cross-bank OTC exercise
// recovery alongside payment reconciliation.
func NewInterbankReconcileRunner(
	db *gorm.DB,
	registry *interbank.Registry,
	client *interbank.Client,
	paymentRepo *repository.InterbankPaymentRepository,
	walletRepo *repository.InterbankPaymentWalletRepository,
	exerciseRepo *repository.InterbankExerciseRepository,
	otcWalletRepo *repository.InterbankWalletRepository,
	negRepo *repository.InterbankOtcRepository,
) *InterbankReconcileRunner {
	return &InterbankReconcileRunner{
		db:            db,
		registry:      registry,
		client:        client,
		paymentRepo:   paymentRepo,
		walletRepo:    walletRepo,
		exerciseRepo:  exerciseRepo,
		otcWalletRepo: otcWalletRepo,
		negRepo:       negRepo,
		staleness:     5 * time.Minute,
		batchSize:     25,
	}
}

// WithStaleness overrides the default staleness window. Primarily for tests.
func (r *InterbankReconcileRunner) WithStaleness(d time.Duration) *InterbankReconcileRunner {
	r.staleness = d
	return r
}

// Run executes one reconciliation tick. Errors per-row are logged and
// the run continues; the cron caller doesn't need a result.
func (r *InterbankReconcileRunner) Run() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	threshold := time.Now().UTC().Add(-r.staleness)

	pending, err := r.paymentRepo.ListStuckPending(threshold, r.batchSize)
	if err != nil {
		slog.Error("interbank reconcile: listing stuck pending payments", "err", err)
	} else {
		for i := range pending {
			r.reconcilePending(ctx, &pending[i])
		}
	}

	terminal, err := r.paymentRepo.ListUndispatchedTerminal(threshold, r.batchSize)
	if err != nil {
		slog.Error("interbank reconcile: listing undispatched terminal payments", "err", err)
	} else {
		for i := range terminal {
			r.reconcileTerminal(ctx, &terminal[i])
		}
	}

	r.runExercise(ctx, threshold)
	r.runAcceptCommits(ctx, threshold)
}

// runAcceptCommits resends COMMIT_TX (and applies the seller credit) for
// OTC option acceptances that got a YES vote but whose commit never
// confirmed — closing the window where a crash after the vote would
// strand the buyer's reserved premium. Idempotent: COMMIT_TX replays as a
// no-op at the buyer, and FinaliseAcceptCommit CAS-guards the credit.
func (r *InterbankReconcileRunner) runAcceptCommits(ctx context.Context, threshold time.Time) {
	if r.negRepo == nil {
		return
	}
	negs, err := r.negRepo.ListUndispatchedAcceptCommits(threshold, r.batchSize)
	if err != nil {
		slog.Error("interbank reconcile: listing undispatched accept commits", "err", err)
		return
	}
	for i := range negs {
		r.reconcileAcceptCommit(ctx, &negs[i])
	}
}

func (r *InterbankReconcileRunner) reconcileAcceptCommit(ctx context.Context, neg *models.InterbankOtcNegotiation) {
	// COMMIT_TX goes to the OTHER bank. CounterpartyRoutingNumber is "the
	// other bank" for both seller- and buyer-coordinated accepts (and
	// equals BuyerRoutingNumber on seller-role rows, so this is
	// behaviour-preserving for the pre-existing seller path).
	counterpartyCode := interbank.RoutingNumber(neg.CounterpartyRoutingNumber)
	txID := interbank.ForeignBankId{
		RoutingNumber: interbank.RoutingNumber(neg.AcceptTxRoutingNumber),
		ID:            neg.AcceptTxID,
	}
	key := r.client.NewIdempotenceKey()
	if err := r.client.SendCommitTx(ctx, counterpartyCode, key, txID); err != nil {
		slog.Info("interbank reconcile: accept COMMIT_TX resend transient failure",
			"err", err, "negotiation", neg.NegotiationID, "tx_id", neg.AcceptTxID)
		r.bumpNegotiationUpdatedAt(neg)
		return
	}
	// Apply OUR side's local effect, branching on the role we coordinated
	// as: seller credit, or buyer debit + option-contract creation. Both
	// are CAS-guarded so this is safe alongside the inline accept path.
	var finErr error
	if neg.LocalRole == models.InterbankNegotiationRoleSeller {
		finErr = interbank.FinaliseAcceptCommit(r.db, r.otcWalletRepo, r.negRepo, neg)
	} else {
		finErr = interbank.FinaliseBuyerAcceptCommit(r.db, r.otcWalletRepo, r.negRepo, neg)
	}
	if finErr != nil {
		slog.Error("interbank reconcile: local credit / finalise failed after accept COMMIT_TX resend",
			"err", finErr, "role", neg.LocalRole, "negotiation", neg.NegotiationID, "tx_id", neg.AcceptTxID)
		return
	}
	slog.Info("interbank reconcile: accept COMMIT_TX resent and finalised",
		"negotiation", neg.NegotiationID, "tx_id", neg.AcceptTxID)
}

func (r *InterbankReconcileRunner) bumpNegotiationUpdatedAt(neg *models.InterbankOtcNegotiation) {
	now := time.Now().UTC()
	if err := r.db.Model(&models.InterbankOtcNegotiation{}).
		Where("id = ?", neg.ID).
		Update("updated_at", now).Error; err != nil {
		slog.Warn("interbank reconcile: failed to bump negotiation updated_at",
			"err", err, "negotiation", neg.NegotiationID)
		return
	}
	neg.UpdatedAt = now
}

// runExercise reconciles cross-bank OTC option exercises (the buyer-bank
// outbound side). Two passes mirror the payment flow:
//
//  1. Stuck `pending` rows — the local commit never ran, so we release the
//     buyer's strike-cash reservation and mark the row failed. The terminal
//     pass then sends ROLLBACK_TX. The option contract is untouched, so the
//     buyer can re-exercise.
//  2. Resolved-but-undispatched rows — replay COMMIT_TX (committed) or
//     ROLLBACK_TX (failed) until the seller's bank acknowledges.
func (r *InterbankReconcileRunner) runExercise(ctx context.Context, threshold time.Time) {
	if r.exerciseRepo == nil || r.otcWalletRepo == nil {
		return
	}

	pending, err := r.exerciseRepo.ListStuckPendingOutbound(threshold, r.batchSize)
	if err != nil {
		slog.Error("interbank reconcile: listing stuck pending exercises", "err", err)
	} else {
		for i := range pending {
			r.reconcileExercisePending(&pending[i])
		}
	}

	terminal, err := r.exerciseRepo.ListUndispatchedTerminalOutbound(threshold, r.batchSize)
	if err != nil {
		slog.Error("interbank reconcile: listing undispatched terminal exercises", "err", err)
	} else {
		for i := range terminal {
			r.reconcileExerciseTerminal(ctx, &terminal[i])
		}
	}
}

// reconcileExercisePending rolls back an exercise stuck in `pending`:
// releases the reserved strike cash and marks the row failed. The next
// terminal pass sends ROLLBACK_TX to the seller's bank. Safe because a
// `pending` row means the local commit (cash debit + stock credit +
// contract flip) never ran — only the reservation is outstanding.
func (r *InterbankReconcileRunner) reconcileExercisePending(row *models.InterbankPendingExercise) {
	err := r.db.Transaction(func(dbtx *gorm.DB) error {
		rows, err := r.exerciseRepo.MarkFailedCAS(dbtx, row.TxRoutingNumber, row.TxID,
			"reconcile: rolled back stuck pending exercise")
		if err != nil {
			return err
		}
		if rows == 0 {
			return nil // already resolved by a concurrent path
		}
		row.Status = models.InterbankExerciseStatusFailed
		return r.otcWalletRepo.Release(dbtx, row.BuyerID, row.PricePerUnitCurrency, row.CashAmount)
	})
	if err != nil {
		slog.Error("interbank reconcile: rolling back stuck pending exercise",
			"err", err, "exercise_id", row.ID, "tx_id", row.TxID)
		return
	}
	slog.Info("interbank reconcile: released stuck pending exercise; ROLLBACK_TX will follow",
		"exercise_id", row.ID, "tx_id", row.TxID)
}

// reconcileExerciseTerminal replays the terminal message for a resolved
// exercise until the seller's bank acknowledges. Idempotent on the
// receiver side.
func (r *InterbankReconcileRunner) reconcileExerciseTerminal(ctx context.Context, row *models.InterbankPendingExercise) {
	partnerCode := interbank.RoutingNumber(row.PartnerRoutingNumber)
	key := r.client.NewIdempotenceKey()
	txID := interbank.ForeignBankId{
		RoutingNumber: interbank.RoutingNumber(row.TxRoutingNumber),
		ID:            row.TxID,
	}

	var sendErr error
	switch row.Status {
	case models.InterbankExerciseStatusCommitted:
		sendErr = r.client.SendCommitTx(ctx, partnerCode, key, txID)
	case models.InterbankExerciseStatusFailed:
		sendErr = r.client.SendRollbackTx(ctx, partnerCode, key, txID)
	default:
		slog.Warn("interbank reconcile: unexpected terminal exercise status",
			"exercise_id", row.ID, "status", row.Status)
		return
	}

	if sendErr != nil {
		slog.Info("interbank reconcile: terminal exercise retry transient failure",
			"err", sendErr, "exercise_id", row.ID, "status", row.Status)
		r.bumpExerciseUpdatedAt(row)
		return
	}
	if err := r.db.Transaction(func(dbtx *gorm.DB) error {
		_, mErr := r.exerciseRepo.MarkPartnerFinalised(dbtx, row.TxRoutingNumber, row.TxID)
		return mErr
	}); err != nil {
		slog.Warn("interbank reconcile: failed to stamp exercise partner_finalised_at",
			"err", err, "exercise_id", row.ID)
	}
}

// bumpExerciseUpdatedAt nudges updated_at forward so a transiently-failing
// row doesn't get re-picked on the very next tick (coarse back-off).
func (r *InterbankReconcileRunner) bumpExerciseUpdatedAt(row *models.InterbankPendingExercise) {
	now := time.Now().UTC()
	if err := r.db.Model(&models.InterbankPendingExercise{}).
		Where("id = ?", row.ID).
		Update("updated_at", now).Error; err != nil {
		slog.Warn("interbank reconcile: failed to bump exercise updated_at",
			"err", err, "exercise_id", row.ID)
		return
	}
	row.UpdatedAt = now
}

// reconcilePending retries NEW_TX for a single stuck row. The partner's
// business-level idempotency means a retry collapses with any previous
// successful delivery — we get back the same vote either way.
func (r *InterbankReconcileRunner) reconcilePending(ctx context.Context, row *models.InterbankPayment) {
	tx := interbank.BuildPaymentTx(
		interbank.ForeignBankId{
			RoutingNumber: interbank.RoutingNumber(row.TxRoutingNumber),
			ID:            row.TxID,
		},
		row.SenderAccountNumber, row.RecipientAccountNumber,
		row.Currency, row.Amount,
		row.Message, row.PaymentCode, row.PaymentPurpose,
	)
	idemKey := r.client.NewIdempotenceKey()
	partnerCode := interbank.RoutingNumber(row.PartnerRoutingNumber)

	vote, err := r.client.SendNewTx(ctx, partnerCode, idemKey, &tx)
	if err != nil {
		// Permanent 4xx → mark failed + release reservation; the next
		// tick will pick up the failed row and send ROLLBACK_TX.
		if isPermanentRemoteError(err) {
			if err := r.applyFailed(row, err.Error()); err != nil {
				slog.Error("interbank reconcile: marking pending payment failed",
					"err", err, "payment_id", row.ID)
			}
			return
		}
		// Transient — leave for next tick. Bump updated_at so the
		// scan ordering keeps stale rows ahead of recently-retried
		// ones (a simple back-off).
		r.bumpUpdatedAt(row)
		slog.Info("interbank reconcile: NEW_TX retry transient failure, leaving for next tick",
			"err", err, "payment_id", row.ID, "tx_id", row.TxID)
		return
	}

	if vote.Vote != interbank.VoteYes {
		reason := summarisePartnerVote(vote)
		if err := r.applyRejected(row, reason); err != nil {
			slog.Error("interbank reconcile: marking pending payment rejected",
				"err", err, "payment_id", row.ID)
		}
		return
	}

	// YES vote — apply local commit, then send COMMIT_TX inline. The
	// resend loop will retry COMMIT_TX on the next tick if dispatch
	// itself fails.
	if err := r.applyCommit(row); err != nil {
		slog.Error("interbank reconcile: applying local commit after retry YES vote",
			"err", err, "payment_id", row.ID)
		return
	}
	commitKey := r.client.NewIdempotenceKey()
	txID := interbank.ForeignBankId{
		RoutingNumber: interbank.RoutingNumber(row.TxRoutingNumber),
		ID:            row.TxID,
	}
	if err := r.client.SendCommitTx(ctx, partnerCode, commitKey, txID); err != nil {
		slog.Warn("interbank reconcile: COMMIT_TX dispatch failed after retry YES vote, will retry next tick",
			"err", err, "payment_id", row.ID)
		return
	}
	r.markPartnerFinalised(row)
}

// reconcileTerminal re-sends the appropriate terminal message
// (COMMIT_TX for committed, ROLLBACK_TX for failed) until the partner
// acknowledges. Idempotent on the receiver side.
func (r *InterbankReconcileRunner) reconcileTerminal(ctx context.Context, row *models.InterbankPayment) {
	partnerCode := interbank.RoutingNumber(row.PartnerRoutingNumber)
	key := r.client.NewIdempotenceKey()
	txID := interbank.ForeignBankId{
		RoutingNumber: interbank.RoutingNumber(row.TxRoutingNumber),
		ID:            row.TxID,
	}

	var sendErr error
	switch row.Status {
	case models.InterbankPaymentStatusCommitted:
		sendErr = r.client.SendCommitTx(ctx, partnerCode, key, txID)
	case models.InterbankPaymentStatusFailed:
		sendErr = r.client.SendRollbackTx(ctx, partnerCode, key, txID)
	default:
		// Belt-and-braces — the scan filter shouldn't return other
		// statuses, but log and skip if it ever does.
		slog.Warn("interbank reconcile: unexpected terminal-row status",
			"payment_id", row.ID, "status", row.Status)
		return
	}

	if sendErr != nil {
		slog.Info("interbank reconcile: terminal retry transient failure",
			"err", sendErr, "payment_id", row.ID, "status", row.Status)
		r.bumpUpdatedAt(row)
		return
	}
	r.markPartnerFinalised(row)
}

// applyCommit applies the local debit + status flip after a retry YES
// vote. Mirrors the inline path in interbank_payment_http_handler.go.
func (r *InterbankReconcileRunner) applyCommit(row *models.InterbankPayment) error {
	if row.LocalAccountID == nil {
		return fmt.Errorf("payment row missing local_account_id")
	}
	return r.db.Transaction(func(dbtx *gorm.DB) error {
		rows, err := r.paymentRepo.MarkCommittedCAS(dbtx, row.TxRoutingNumber, row.TxID)
		if err != nil {
			return err
		}
		if rows == 0 {
			return nil
		}
		row.Status = models.InterbankPaymentStatusCommitted
		return r.walletRepo.Debit(dbtx, *row.LocalAccountID, row.Amount)
	})
}

// applyRejected releases the reservation and CAS-marks the row
// rejected, mirroring the inline rejection path. Stamps
// partner_finalised_at since no terminal message is owed for a NO vote.
func (r *InterbankReconcileRunner) applyRejected(row *models.InterbankPayment, reason string) error {
	if row.LocalAccountID == nil {
		return fmt.Errorf("payment row missing local_account_id")
	}
	return r.db.Transaction(func(dbtx *gorm.DB) error {
		rows, err := r.paymentRepo.MarkRejectedCAS(dbtx, row.TxRoutingNumber, row.TxID, reason)
		if err != nil {
			return err
		}
		if rows == 0 {
			return nil
		}
		row.Status = models.InterbankPaymentStatusRejected
		row.LastError = reason
		if _, err := r.paymentRepo.MarkPartnerFinalised(dbtx, row.TxRoutingNumber, row.TxID); err != nil {
			return err
		}
		return r.walletRepo.Release(dbtx, *row.LocalAccountID, row.Amount)
	})
}

// applyFailed releases the reservation and CAS-marks the row failed.
// Does NOT stamp partner_finalised_at — the next tick will pick the
// row up and send ROLLBACK_TX (the partner may have a stranded pending
// row).
func (r *InterbankReconcileRunner) applyFailed(row *models.InterbankPayment, errMsg string) error {
	if row.LocalAccountID == nil {
		return fmt.Errorf("payment row missing local_account_id")
	}
	return r.db.Transaction(func(dbtx *gorm.DB) error {
		rows, err := r.paymentRepo.MarkFailedCAS(dbtx, row.TxRoutingNumber, row.TxID, errMsg)
		if err != nil {
			return err
		}
		if rows == 0 {
			return nil
		}
		row.Status = models.InterbankPaymentStatusFailed
		row.LastError = errMsg
		return r.walletRepo.Release(dbtx, *row.LocalAccountID, row.Amount)
	})
}

func (r *InterbankReconcileRunner) markPartnerFinalised(row *models.InterbankPayment) {
	err := r.db.Transaction(func(dbtx *gorm.DB) error {
		_, mErr := r.paymentRepo.MarkPartnerFinalised(dbtx, row.TxRoutingNumber, row.TxID)
		return mErr
	})
	if err != nil {
		slog.Warn("interbank reconcile: failed to stamp partner_finalised_at",
			"err", err, "payment_id", row.ID)
		return
	}
	now := time.Now().UTC()
	row.PartnerFinalisedAt = &now
}

// bumpUpdatedAt nudges the row's updated_at forward so it doesn't get
// picked up again on the very next tick. Acts as a coarse back-off
// without needing a separate "next_retry_at" column.
func (r *InterbankReconcileRunner) bumpUpdatedAt(row *models.InterbankPayment) {
	now := time.Now().UTC()
	err := r.db.Model(&models.InterbankPayment{}).
		Where("id = ?", row.ID).
		Update("updated_at", now).Error
	if err != nil {
		slog.Warn("interbank reconcile: failed to bump updated_at",
			"err", err, "payment_id", row.ID)
		return
	}
	row.UpdatedAt = now
}

// isPermanentRemoteError returns true when the partner's HTTP response
// indicates a permanent (not retry-able) failure. 4xx other than 408
// (request timeout) and 429 (rate limit) are permanent.
func isPermanentRemoteError(err error) bool {
	var rerr *interbank.RemoteError
	if !errors.As(err, &rerr) {
		return false
	}
	if rerr.StatusCode < 400 || rerr.StatusCode >= 500 {
		return false
	}
	switch rerr.StatusCode {
	case 408, 429:
		return false
	}
	return true
}

// summarisePartnerVote turns a NO TransactionVote into a short string
// for last_error. Mirrors the handler-side helper so cron-reconciled
// rejections look the same in the FE as inline ones.
func summarisePartnerVote(vote *interbank.TransactionVote) string {
	if vote == nil || len(vote.Reasons) == 0 {
		return "partner voted NO without reason"
	}
	parts := make([]string, 0, len(vote.Reasons))
	for _, r := range vote.Reasons {
		parts = append(parts, string(r.Reason))
	}
	out := "partner voted NO: "
	for i, s := range parts {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
