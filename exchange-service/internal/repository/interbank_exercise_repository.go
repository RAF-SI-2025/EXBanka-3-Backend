package repository

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

// InterbankExerciseRepository persists InterbankPendingExercise rows
// and the CAS status flips that the exercise TxProcessor + initiator
// drive. Both sides (outbound buyer-bank and inbound seller-bank) use
// the same table; rows are distinguished by Direction.
type InterbankExerciseRepository struct {
	db *gorm.DB
}

func NewInterbankExerciseRepository(db *gorm.DB) *InterbankExerciseRepository {
	return &InterbankExerciseRepository{db: db}
}

func (r *InterbankExerciseRepository) DB() *gorm.DB { return r.db }

// GetByTxID returns the exercise row keyed by the protocol transactionId.
func (r *InterbankExerciseRepository) GetByTxID(routing int, id string) (*models.InterbankPendingExercise, error) {
	var row models.InterbankPendingExercise
	err := r.db.Where("tx_routing_number = ? AND tx_id = ?", routing, id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("loading pending exercise %d/%s: %w", routing, id, err)
	}
	return &row, nil
}

// HasCommittedForNegotiation returns true when any exercise for the
// given negotiation has already committed. Used by the inbound
// processor to vote NO with OPTION_USED_OR_EXPIRED on a double-exercise
// attempt.
func (r *InterbankExerciseRepository) HasCommittedForNegotiation(routing int, id string) (bool, error) {
	var count int64
	err := r.db.Model(&models.InterbankPendingExercise{}).
		Where("negotiation_routing_number = ? AND negotiation_id = ? AND status = ?",
			routing, id, models.InterbankExerciseStatusCommitted).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("checking exercise commit history for %d/%s: %w", routing, id, err)
	}
	return count > 0, nil
}

// CreateTx inserts a new exercise row under the caller's DB tx.
func (r *InterbankExerciseRepository) CreateTx(tx *gorm.DB, row *models.InterbankPendingExercise) error {
	now := time.Now().UTC()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	if row.Status == "" {
		row.Status = models.InterbankExerciseStatusPending
	}
	return tx.Create(row).Error
}

// MarkCommittedCAS flips pending → committed atomically.
func (r *InterbankExerciseRepository) MarkCommittedCAS(tx *gorm.DB, routing int, id string) (int64, error) {
	now := time.Now().UTC()
	res := tx.Model(&models.InterbankPendingExercise{}).
		Where("tx_routing_number = ? AND tx_id = ? AND status = ?",
			routing, id, models.InterbankExerciseStatusPending).
		Updates(map[string]interface{}{
			"status":      models.InterbankExerciseStatusCommitted,
			"resolved_at": now,
			"updated_at":  now,
		})
	if res.Error != nil {
		return 0, fmt.Errorf("marking exercise committed: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// MarkRolledBackCAS flips pending → rolled_back atomically.
func (r *InterbankExerciseRepository) MarkRolledBackCAS(tx *gorm.DB, routing int, id string) (int64, error) {
	now := time.Now().UTC()
	res := tx.Model(&models.InterbankPendingExercise{}).
		Where("tx_routing_number = ? AND tx_id = ? AND status = ?",
			routing, id, models.InterbankExerciseStatusPending).
		Updates(map[string]interface{}{
			"status":      models.InterbankExerciseStatusRolledBack,
			"resolved_at": now,
			"updated_at":  now,
		})
	if res.Error != nil {
		return 0, fmt.Errorf("marking exercise rolled back: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// MarkRejectedCAS flips pending → rejected (sender side, partner NO).
func (r *InterbankExerciseRepository) MarkRejectedCAS(tx *gorm.DB, routing int, id string, reason string) (int64, error) {
	now := time.Now().UTC()
	res := tx.Model(&models.InterbankPendingExercise{}).
		Where("tx_routing_number = ? AND tx_id = ? AND status = ?",
			routing, id, models.InterbankExerciseStatusPending).
		Updates(map[string]interface{}{
			"status":      models.InterbankExerciseStatusRejected,
			"last_error":  reason,
			"resolved_at": now,
			"updated_at":  now,
		})
	if res.Error != nil {
		return 0, fmt.Errorf("marking exercise rejected: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// MarkFailedCAS flips pending → failed (sender side, transport error).
func (r *InterbankExerciseRepository) MarkFailedCAS(tx *gorm.DB, routing int, id string, errMsg string) (int64, error) {
	now := time.Now().UTC()
	res := tx.Model(&models.InterbankPendingExercise{}).
		Where("tx_routing_number = ? AND tx_id = ? AND status = ?",
			routing, id, models.InterbankExerciseStatusPending).
		Updates(map[string]interface{}{
			"status":      models.InterbankExerciseStatusFailed,
			"last_error":  errMsg,
			"resolved_at": now,
			"updated_at":  now,
		})
	if res.Error != nil {
		return 0, fmt.Errorf("marking exercise failed: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// ListStuckPendingOutbound returns outbound exercises stuck in `pending`
// past the staleness threshold — the buyer-bank reserved the strike cash
// and persisted the row but never recorded a vote (crash/timeout between
// reserve and the NEW_TX response). The reconcile cron rolls these back:
// the local commit never ran, so releasing the reservation is the safe
// recovery and the buyer can re-exercise (the option is not consumed).
func (r *InterbankExerciseRepository) ListStuckPendingOutbound(threshold time.Time, limit int) ([]models.InterbankPendingExercise, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []models.InterbankPendingExercise
	err := r.db.
		Where("direction = ? AND status = ? AND updated_at < ?",
			models.InterbankExerciseDirectionOutbound,
			models.InterbankExerciseStatusPending,
			threshold,
		).
		Order("updated_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing stuck pending exercises: %w", err)
	}
	return rows, nil
}

// ListUndispatchedTerminalOutbound returns outbound exercises whose status
// is resolved (committed / failed) but whose terminal partner message has
// not been acknowledged (partner_finalised_at IS NULL). The cron replays
// COMMIT_TX (committed) or ROLLBACK_TX (failed) for these. `rejected` is
// excluded — the partner voted NO and holds no resources.
func (r *InterbankExerciseRepository) ListUndispatchedTerminalOutbound(threshold time.Time, limit int) ([]models.InterbankPendingExercise, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []models.InterbankPendingExercise
	err := r.db.
		Where("direction = ? AND status IN ? AND partner_finalised_at IS NULL AND updated_at < ?",
			models.InterbankExerciseDirectionOutbound,
			[]string{models.InterbankExerciseStatusCommitted, models.InterbankExerciseStatusFailed},
			threshold,
		).
		Order("updated_at ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing undispatched terminal exercises: %w", err)
	}
	return rows, nil
}

// MarkPartnerFinalised stamps PartnerFinalisedAt — used by the
// reconciliation cron to stop replaying the terminal message once the
// partner has ACKed it.
func (r *InterbankExerciseRepository) MarkPartnerFinalised(tx *gorm.DB, routing int, id string) (int64, error) {
	now := time.Now().UTC()
	res := tx.Model(&models.InterbankPendingExercise{}).
		Where("tx_routing_number = ? AND tx_id = ? AND partner_finalised_at IS NULL", routing, id).
		Updates(map[string]interface{}{
			"partner_finalised_at": now,
			"updated_at":           now,
		})
	if res.Error != nil {
		return 0, fmt.Errorf("marking exercise partner-finalised: %w", res.Error)
	}
	return res.RowsAffected, nil
}
