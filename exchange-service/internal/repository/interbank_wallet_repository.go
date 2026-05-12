package repository

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrInterbankWalletInsufficient is returned when the buyer has no active
// account in the required currency, or that account's available balance
// is below the requested reservation amount.
var ErrInterbankWalletInsufficient = errors.New("interbank wallet: insufficient available funds")

// InterbankWalletRepository owns the buyer-side cash movements that the
// inter-bank OTC TxProcessor performs in response to NEW_TX / COMMIT_TX
// / ROLLBACK_TX. It deliberately accepts the surrounding *gorm.DB so the
// processor can wrap each phase in a single atomic transaction together
// with the pending-row status flip.
//
// Lookup model: the protocol's ForeignBankId.id for our local buyer is
// the opaque string produced by interbank.EncodeLocalParticipantID. For
// inter-bank OTC, that's always "client-{userID}" — the buyer is always
// a client (bank-side accounts are only used for the local OTC flow).
// Anything else returns ErrInterbankWalletInsufficient.
type InterbankWalletRepository struct {
	db *gorm.DB
}

func NewInterbankWalletRepository(db *gorm.DB) *InterbankWalletRepository {
	return &InterbankWalletRepository{db: db}
}

// Reserve decrements raspolozivo_stanje on the buyer's first active
// account matching the requested currency, leaving stanje untouched (so
// the funds stay visible on the books but cannot be spent elsewhere).
// Returns ErrInterbankWalletInsufficient if no eligible account exists
// or the available balance is too low.
func (r *InterbankWalletRepository) Reserve(tx *gorm.DB, localID, currency string, amount float64) error {
	accountID, err := r.lockBuyerAccount(tx, localID, currency)
	if err != nil {
		return err
	}
	res := tx.Table("accounts").
		Where("id = ? AND raspolozivo_stanje >= ?", accountID, amount).
		Updates(map[string]interface{}{
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje - ?", amount),
		})
	if res.Error != nil {
		return fmt.Errorf("reserving buyer wallet: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrInterbankWalletInsufficient
	}
	return nil
}

// Debit completes the reservation by decrementing stanje. Pair with a
// prior successful Reserve — raspolozivo_stanje was already decremented
// there, so we only touch stanje here. The same row-lock is taken so
// the read-modify-write is serialised against any concurrent activity.
func (r *InterbankWalletRepository) Debit(tx *gorm.DB, localID, currency string, amount float64) error {
	accountID, err := r.lockBuyerAccount(tx, localID, currency)
	if err != nil {
		return err
	}
	res := tx.Table("accounts").
		Where("id = ?", accountID).
		Updates(map[string]interface{}{
			"stanje": gorm.Expr("stanje - ?", amount),
		})
	if res.Error != nil {
		return fmt.Errorf("debiting buyer wallet: %w", res.Error)
	}
	return nil
}

// Release refunds a prior Reserve by incrementing raspolozivo_stanje
// back to where it was before the NEW_TX. stanje is unchanged because
// Debit hasn't run yet on a rolled-back transaction.
func (r *InterbankWalletRepository) Release(tx *gorm.DB, localID, currency string, amount float64) error {
	accountID, err := r.lockBuyerAccount(tx, localID, currency)
	if err != nil {
		return err
	}
	res := tx.Table("accounts").
		Where("id = ?", accountID).
		Updates(map[string]interface{}{
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje + ?", amount),
		})
	if res.Error != nil {
		return fmt.Errorf("releasing buyer wallet reservation: %w", res.Error)
	}
	return nil
}

// lockBuyerAccount finds and SELECT-FOR-UPDATE-locks the buyer's first
// active account in the given currency. Returns the account id or
// ErrInterbankWalletInsufficient if there's no match. Deterministic
// ordering by id keeps repeated calls on the same (client, currency)
// converging on the same row.
func (r *InterbankWalletRepository) lockBuyerAccount(tx *gorm.DB, localID, currency string) (uint, error) {
	clientID, err := parseClientLocalID(localID)
	if err != nil {
		return 0, err
	}
	var row struct {
		ID uint `gorm:"column:id"`
	}
	err = tx.Table("accounts").
		Select("accounts.id").
		Joins("JOIN currencies ON currencies.id = accounts.currency_id").
		Where("accounts.client_id = ? AND currencies.kod = ? AND accounts.status = ?",
			clientID, currency, "aktivan").
		Order("accounts.id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Limit(1).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrInterbankWalletInsufficient
		}
		return 0, fmt.Errorf("looking up buyer wallet account: %w", err)
	}
	return row.ID, nil
}

// parseClientLocalID accepts only the "client-{n}" shape minted by
// interbank.EncodeLocalParticipantID for clients. Any other shape
// (including "bank-…") returns ErrInterbankWalletInsufficient — the
// caller treats it as "no eligible account" and votes NO with
// ReasonInsufficientAsset.
func parseClientLocalID(s string) (uint, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 || parts[0] != "client" {
		return 0, ErrInterbankWalletInsufficient
	}
	id, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, ErrInterbankWalletInsufficient
	}
	return uint(id), nil
}
