package repository

import (
	"errors"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

// InterbankOtcRepository persists our local copy of every cross-bank
// OTC negotiation. Each row is keyed by (NegotiationRoutingNumber,
// NegotiationID) — the seller bank's coordinates.
type InterbankOtcRepository struct {
	db *gorm.DB
}

func NewInterbankOtcRepository(db *gorm.DB) *InterbankOtcRepository {
	return &InterbankOtcRepository{db: db}
}

// Create persists a new negotiation. Timestamps are set here so
// callers don't have to remember.
func (r *InterbankOtcRepository) Create(neg *models.InterbankOtcNegotiation) error {
	now := time.Now().UTC()
	neg.CreatedAt = now
	neg.UpdatedAt = now
	if !neg.IsOngoing {
		neg.IsOngoing = true
	}
	return r.db.Create(neg).Error
}

// Get fetches a negotiation by its global key. Returns (nil, nil)
// when no row exists — callers branch on that for "negotiation does
// not exist on our side yet" cases (which happen on the first
// inbound POST /negotiations).
func (r *InterbankOtcRepository) Get(negotiationRoutingNumber int, negotiationID string) (*models.InterbankOtcNegotiation, error) {
	var neg models.InterbankOtcNegotiation
	err := r.db.
		Where("negotiation_routing_number = ? AND negotiation_id = ?", negotiationRoutingNumber, negotiationID).
		First(&neg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &neg, nil
}

// UpdateTerms applies a counter-offer's term changes. Caller supplies
// the new amount, price, premium, settlement date, and the identity
// of whoever made the change. updated_at is set automatically.
func (r *InterbankOtcRepository) UpdateTerms(
	negotiationRoutingNumber int,
	negotiationID string,
	amount float64,
	pricePerUnitCurrency string, pricePerUnitAmount float64,
	premiumCurrency string, premiumAmount float64,
	settlementDate string,
	modifiedByRoutingNumber int, modifiedByID string,
) error {
	return r.db.Model(&models.InterbankOtcNegotiation{}).
		Where("negotiation_routing_number = ? AND negotiation_id = ?", negotiationRoutingNumber, negotiationID).
		Updates(map[string]interface{}{
			"amount":                          amount,
			"price_per_unit_currency":         pricePerUnitCurrency,
			"price_per_unit_amount":           pricePerUnitAmount,
			"premium_currency":                premiumCurrency,
			"premium_amount":                  premiumAmount,
			"settlement_date":                 settlementDate,
			"last_modified_by_routing_number": modifiedByRoutingNumber,
			"last_modified_by_id":             modifiedByID,
			"updated_at":                      time.Now().UTC(),
		}).Error
}

// MarkClosed flips IsOngoing=false. Used when either side accepts or
// declines the negotiation. Acceptance is not modelled separately
// because the protocol expresses it as a side-effect of GET
// /negotiations/{...}/accept (which triggers NEW_TX); the row's only
// state change is "ongoing → closed".
func (r *InterbankOtcRepository) MarkClosed(negotiationRoutingNumber int, negotiationID string) error {
	return r.db.Model(&models.InterbankOtcNegotiation{}).
		Where("negotiation_routing_number = ? AND negotiation_id = ?", negotiationRoutingNumber, negotiationID).
		Updates(map[string]interface{}{
			"is_ongoing": false,
			"updated_at": time.Now().UTC(),
		}).Error
}

// MarkOngoing flips IsOngoing back to true. The accept handler calls
// this when NEW_TX dispatch fails or the buyer's bank votes NO — the
// negotiation reopens so the participants can keep haggling. The
// LastModifiedBy fields are reset to whoever last touched the row so
// the wire copy stays consistent with what the partner already saw.
func (r *InterbankOtcRepository) MarkOngoing(negotiationRoutingNumber int, negotiationID string, lastModRouting int, lastModID string) error {
	return r.db.Model(&models.InterbankOtcNegotiation{}).
		Where("negotiation_routing_number = ? AND negotiation_id = ?", negotiationRoutingNumber, negotiationID).
		Updates(map[string]interface{}{
			"is_ongoing":                      true,
			"last_modified_by_routing_number": lastModRouting,
			"last_modified_by_id":             lastModID,
			"updated_at":                      time.Now().UTC(),
		}).Error
}

// SetSellerReservedHoldingTx records (or clears, when holdingID is nil)
// the local portfolio holding whose shares were reserved to back this
// negotiation's option contract. Transaction-composable so the update
// lands atomically with the reserve/release it accompanies.
func (r *InterbankOtcRepository) SetSellerReservedHoldingTx(tx *gorm.DB, negotiationRoutingNumber int, negotiationID string, holdingID *uint) error {
	return tx.Model(&models.InterbankOtcNegotiation{}).
		Where("negotiation_routing_number = ? AND negotiation_id = ?", negotiationRoutingNumber, negotiationID).
		Updates(map[string]interface{}{
			"seller_reserved_holding_id": holdingID,
			"updated_at":                 time.Now().UTC(),
		}).Error
}

// ListSellerReservations returns every seller-role negotiation that still
// holds a live stock reservation (seller_reserved_holding_id IS NOT NULL).
// The settlement-expiry sweep filters these in Go, since settlement_date
// is stored as an opaque ISO8601 string that can't be range-compared in
// SQL across timezones.
func (r *InterbankOtcRepository) ListSellerReservations() ([]models.InterbankOtcNegotiation, error) {
	var negs []models.InterbankOtcNegotiation
	if err := r.db.
		Where("seller_reserved_holding_id IS NOT NULL").
		Find(&negs).Error; err != nil {
		return nil, err
	}
	return negs, nil
}

// ReleaseSellerReservationTx releases up to `quantity` reserved shares on
// the given holding and clears the negotiation's reservation marker, both
// inside the caller's transaction. The release is clamped to whatever is
// actually reserved so a double-release (e.g. a retried sweep) can't drive
// reserved_quantity negative.
func (r *InterbankOtcRepository) ReleaseSellerReservationTx(
	tx *gorm.DB,
	negotiationRoutingNumber int, negotiationID string,
	holdingID uint, quantity float64,
) error {
	var h models.PortfolioHoldingRecord
	if err := tx.First(&h, holdingID).Error; err != nil {
		return err
	}
	release := quantity
	if release > h.ReservedQuantity {
		release = h.ReservedQuantity
	}
	now := time.Now().UTC()
	if release > 0 {
		if err := tx.Model(&h).Updates(map[string]interface{}{
			"reserved_quantity": h.ReservedQuantity - release,
			"updated_at":        now,
		}).Error; err != nil {
			return err
		}
	}
	return tx.Model(&models.InterbankOtcNegotiation{}).
		Where("negotiation_routing_number = ? AND negotiation_id = ?", negotiationRoutingNumber, negotiationID).
		Updates(map[string]interface{}{
			"seller_reserved_holding_id": nil,
			"updated_at":                 now,
		}).Error
}

// ExpireDueSellerReservations releases stock reservations for seller-role
// negotiations whose option was never exercised and whose settlement date
// has passed (honouring the same 24h grace the exercise path allows, so
// cron, exercise window, and reservation lifetime stay in agreement per
// spec §2.7.2). Returns the number of reservations released. Settlement
// dates are opaque ISO8601 strings, so candidates are filtered in Go.
func (r *InterbankOtcRepository) ExpireDueSellerReservations(now time.Time) (int, error) {
	negs, err := r.ListSellerReservations()
	if err != nil {
		return 0, err
	}
	released := 0
	for i := range negs {
		neg := &negs[i]
		if neg.SellerReservedHoldingID == nil {
			continue
		}
		exp, ok := parseSettlementDate(neg.SettlementDate)
		if !ok {
			// Unparseable date — leave the reservation rather than
			// release it on a bad guess; an operator can intervene.
			continue
		}
		if !now.After(exp.Add(24 * time.Hour)) {
			continue
		}
		holdingID := *neg.SellerReservedHoldingID
		amount := neg.Amount
		routing := neg.NegotiationRoutingNumber
		id := neg.NegotiationID
		if err := r.db.Transaction(func(tx *gorm.DB) error {
			return r.ReleaseSellerReservationTx(tx, routing, id, holdingID, amount)
		}); err != nil {
			return released, err
		}
		released++
	}
	return released, nil
}

// parseSettlementDate is a lenient ISO8601 parse mirroring the
// interbank package's settlement parser. Returns ok=false on malformed
// input so callers can skip rather than act on a bad date.
func parseSettlementDate(s string) (time.Time, bool) {
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

// SetAcceptDispatched records the option-acceptance NEW_TX's transactionId
// on the negotiation, marking that the buyer voted YES and a COMMIT_TX +
// seller credit are now owed. Called by the accept path right after the
// YES vote, before COMMIT_TX dispatch, so a crash in between leaves a row
// the reconcile cron can drive to completion.
func (r *InterbankOtcRepository) SetAcceptDispatched(negotiationRoutingNumber int, negotiationID string, txRoutingNumber int, txID string) error {
	return r.db.Model(&models.InterbankOtcNegotiation{}).
		Where("negotiation_routing_number = ? AND negotiation_id = ?", negotiationRoutingNumber, negotiationID).
		Updates(map[string]interface{}{
			"accept_tx_routing_number": txRoutingNumber,
			"accept_tx_id":             txID,
			"updated_at":               time.Now().UTC(),
		}).Error
}

// MarkAcceptCommitFinalisedCASTx stamps accept_commit_finalised_at under
// the caller's transaction and returns the number of rows affected (1 if
// this caller won the CAS, 0 if it was already finalised). The seller
// credit is gated on a 1 result so it happens exactly once even if two
// retries race. The row-level lock taken by the UPDATE…WHERE serialises
// concurrent callers.
func (r *InterbankOtcRepository) MarkAcceptCommitFinalisedCASTx(tx *gorm.DB, negotiationRoutingNumber int, negotiationID string) (int64, error) {
	now := time.Now().UTC()
	res := tx.Model(&models.InterbankOtcNegotiation{}).
		Where("negotiation_routing_number = ? AND negotiation_id = ? AND accept_commit_finalised_at IS NULL",
			negotiationRoutingNumber, negotiationID).
		Updates(map[string]interface{}{
			"accept_commit_finalised_at": now,
			"updated_at":                 now,
		})
	return res.RowsAffected, res.Error
}

// ListUndispatchedAcceptCommits returns seller-role negotiations that got a
// YES vote (accept_tx_id set) but whose COMMIT_TX + seller credit haven't
// been confirmed (accept_commit_finalised_at IS NULL) past the staleness
// threshold. The reconcile cron resends COMMIT_TX and finalises these.
func (r *InterbankOtcRepository) ListUndispatchedAcceptCommits(threshold time.Time, limit int) ([]models.InterbankOtcNegotiation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var negs []models.InterbankOtcNegotiation
	err := r.db.
		Where("accept_tx_id <> '' AND accept_commit_finalised_at IS NULL AND updated_at < ?", threshold).
		Order("updated_at ASC").
		Limit(limit).
		Find(&negs).Error
	if err != nil {
		return nil, err
	}
	return negs, nil
}

// ListByLocalParticipant returns the open negotiations where a given
// local user is one of the two sides. role filters to "buyer" or
// "seller"; pass "" for both.
func (r *InterbankOtcRepository) ListByLocalParticipant(localID string, role string, includeClosed bool) ([]models.InterbankOtcNegotiation, error) {
	query := r.db.Model(&models.InterbankOtcNegotiation{})
	switch role {
	case models.InterbankNegotiationRoleBuyer:
		query = query.Where("buyer_id = ? AND local_role = ?", localID, models.InterbankNegotiationRoleBuyer)
	case models.InterbankNegotiationRoleSeller:
		query = query.Where("seller_id = ? AND local_role = ?", localID, models.InterbankNegotiationRoleSeller)
	case "":
		query = query.Where(
			"(local_role = ? AND buyer_id = ?) OR (local_role = ? AND seller_id = ?)",
			models.InterbankNegotiationRoleBuyer, localID,
			models.InterbankNegotiationRoleSeller, localID,
		)
	default:
		return nil, errors.New("unknown role filter")
	}
	if !includeClosed {
		query = query.Where("is_ongoing = ?", true)
	}
	var negs []models.InterbankOtcNegotiation
	if err := query.Order("updated_at DESC, id DESC").Find(&negs).Error; err != nil {
		return nil, err
	}
	return negs, nil
}
