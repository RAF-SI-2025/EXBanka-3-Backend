package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	otcExerciseStepReserveBuyerFunds   = "reserve_buyer_funds"
	otcExerciseStepVerifySellerShares  = "verify_seller_shares"
	otcExerciseStepTransferFunds       = "transfer_funds"
	otcExerciseStepTransferOwnership   = "transfer_ownership"
	otcExerciseStepFinalizeReservations = "finalize_and_consistency_check"
)

type OtcExerciseSagaPayload struct {
	ContractID uint    `json:"contractId"`
	BuyerID    uint    `json:"buyerId"`
	BuyerType  string  `json:"buyerType"`
	Amount     float64 `json:"amount"`
	Strike     float64 `json:"strike"`
	Cost       float64 `json:"cost"`
}

// BuildOtcExerciseSteps constructs the 5 SAGA steps that exercise an OTC option
// contract. Currency match is enforced by the offer-acceptance flow (sprint 6),
// so this saga assumes buyer and seller accounts share the same currency.
func BuildOtcExerciseSteps(contract *models.OtcContractRecord) []SagaStep {
	cost := contract.Amount * contract.StrikePrice
	contractID := contract.ID
	buyerAccountID := contract.BuyerAccountID
	sellerAccountID := contract.SellerAccountID
	sellerHoldingID := contract.SellerHoldingID
	assetID := contract.StockListingID
	buyerID := contract.BuyerID
	buyerType := contract.BuyerType
	amount := contract.Amount
	strike := contract.StrikePrice

	return []SagaStep{
		{
			Name: otcExerciseStepReserveBuyerFunds,
			Forward: func(tx *gorm.DB) error {
				return reserveAccountFunds(tx, buyerAccountID, cost)
			},
			Compensate: func(tx *gorm.DB) error {
				return releaseAccountFunds(tx, buyerAccountID, cost)
			},
		},
		{
			Name: otcExerciseStepVerifySellerShares,
			Forward: func(tx *gorm.DB) error {
				return verifySellerReservedShares(tx, sellerHoldingID, amount)
			},
			// Reservation predates the saga (occurred at offer accept), so there is
			// nothing to release here on rollback. We still record a compensation
			// no-op so the orchestrator marks the step compensated cleanly.
			Compensate: func(tx *gorm.DB) error { return nil },
		},
		{
			Name: otcExerciseStepTransferFunds,
			Forward: func(tx *gorm.DB) error {
				return transferStrikeFunds(tx, buyerAccountID, sellerAccountID, cost)
			},
			Compensate: func(tx *gorm.DB) error {
				return reverseStrikeFunds(tx, buyerAccountID, sellerAccountID, cost)
			},
		},
		{
			Name: otcExerciseStepTransferOwnership,
			Forward: func(tx *gorm.DB) error {
				return transferShareOwnership(tx, sellerHoldingID, buyerID, buyerType, buyerAccountID, assetID, amount, strike)
			},
			Compensate: func(tx *gorm.DB) error {
				return reverseShareOwnership(tx, sellerHoldingID, buyerID, buyerType, assetID, amount, strike)
			},
		},
		{
			Name: otcExerciseStepFinalizeReservations,
			Forward: func(tx *gorm.DB) error {
				return finalizeOtcExercise(tx, contractID, buyerAccountID, sellerAccountID, sellerHoldingID, buyerID, buyerType, assetID, amount, cost)
			},
			Compensate: func(tx *gorm.DB) error {
				return revertOtcExerciseFinalization(tx, contractID, buyerAccountID, cost)
			},
		},
	}
}

func MarshalOtcExercisePayload(contract *models.OtcContractRecord) (string, error) {
	payload := OtcExerciseSagaPayload{
		ContractID: contract.ID,
		BuyerID:    contract.BuyerID,
		BuyerType:  contract.BuyerType,
		Amount:     contract.Amount,
		Strike:     contract.StrikePrice,
		Cost:       contract.Amount * contract.StrikePrice,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// reserveAccountFunds row-locks the account, verifies sufficient available
// balance, and decrements raspolozivo_stanje. stanje is unchanged - the funds
// are merely reserved, not yet debited.
func reserveAccountFunds(tx *gorm.DB, accountID uint, amount float64) error {
	var account repository.OtcAccountReference
	if err := lockAccount(tx, accountID, &account); err != nil {
		return err
	}
	if account.RaspolozivoStanje < amount {
		return fmt.Errorf("insufficient available funds for OTC exercise")
	}
	result := tx.Table("accounts").
		Where("id = ? AND raspolozivo_stanje >= ?", accountID, amount).
		Updates(map[string]interface{}{
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje - ?", amount),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("could not reserve buyer funds")
	}
	return nil
}

func releaseAccountFunds(tx *gorm.DB, accountID uint, amount float64) error {
	return tx.Table("accounts").
		Where("id = ?", accountID).
		Updates(map[string]interface{}{
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje + ?", amount),
		}).Error
}

func verifySellerReservedShares(tx *gorm.DB, holdingID uint, amount float64) error {
	var h models.PortfolioHoldingRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&h, holdingID).Error; err != nil {
		return err
	}
	if h.Quantity < amount {
		return fmt.Errorf("seller no longer owns enough shares")
	}
	if h.ReservedQuantity < amount {
		return fmt.Errorf("seller reservation lost")
	}
	return nil
}

// transferStrikeFunds debits the buyer's stanje (the funds were reserved in
// step 1) and credits the seller's stanje + raspolozivo_stanje by the same
// amount. Spending counters are not bumped here because OTC exercise is not a
// daily/monthly outflow in the same sense as a card or transfer payment.
func transferStrikeFunds(tx *gorm.DB, buyerAccountID, sellerAccountID uint, amount float64) error {
	if err := tx.Table("accounts").
		Where("id = ?", buyerAccountID).
		Updates(map[string]interface{}{
			"stanje": gorm.Expr("stanje - ?", amount),
		}).Error; err != nil {
		return err
	}
	if err := tx.Table("accounts").
		Where("id = ?", sellerAccountID).
		Updates(map[string]interface{}{
			"stanje":             gorm.Expr("stanje + ?", amount),
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje + ?", amount),
		}).Error; err != nil {
		return err
	}
	return nil
}

func reverseStrikeFunds(tx *gorm.DB, buyerAccountID, sellerAccountID uint, amount float64) error {
	if err := tx.Table("accounts").
		Where("id = ?", sellerAccountID).
		Updates(map[string]interface{}{
			"stanje":             gorm.Expr("stanje - ?", amount),
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje - ?", amount),
		}).Error; err != nil {
		return err
	}
	if err := tx.Table("accounts").
		Where("id = ?", buyerAccountID).
		Updates(map[string]interface{}{
			"stanje": gorm.Expr("stanje + ?", amount),
		}).Error; err != nil {
		return err
	}
	return nil
}

// transferShareOwnership decrements seller's quantity + reserved + (clamped)
// public_quantity and upserts the buyer's holding with a fresh weighted
// average.
func transferShareOwnership(tx *gorm.DB, sellerHoldingID, buyerID uint, buyerType string, buyerAccountID, assetID uint, amount, strike float64) error {
	now := time.Now().UTC()

	var seller models.PortfolioHoldingRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&seller, sellerHoldingID).Error; err != nil {
		return err
	}
	if seller.Quantity < amount {
		return fmt.Errorf("seller quantity below contract amount")
	}
	if seller.ReservedQuantity < amount {
		return fmt.Errorf("seller reservation below contract amount")
	}

	newQty := seller.Quantity - amount
	newReserved := seller.ReservedQuantity - amount
	newPublic := seller.PublicQuantity - amount
	if newPublic < 0 {
		newPublic = 0
	}
	if newPublic > newQty {
		newPublic = newQty
	}
	isPublic := seller.IsPublic
	if newPublic == 0 {
		isPublic = false
	}

	if err := tx.Model(&seller).Updates(map[string]interface{}{
		"quantity":          newQty,
		"reserved_quantity": newReserved,
		"public_quantity":   newPublic,
		"is_public":         isPublic,
		"updated_at":        now,
	}).Error; err != nil {
		return err
	}

	var buyer models.PortfolioHoldingRecord
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND user_type = ? AND asset_id = ?", buyerID, buyerType, assetID).
		First(&buyer).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err == gorm.ErrRecordNotFound {
		newHolding := models.PortfolioHoldingRecord{
			UserID:      buyerID,
			UserType:    buyerType,
			AssetID:     assetID,
			AccountID:   buyerAccountID,
			Quantity:    amount,
			AvgBuyPrice: strike,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		return tx.Create(&newHolding).Error
	}

	totalQty := buyer.Quantity + amount
	newAvg := (buyer.Quantity*buyer.AvgBuyPrice + amount*strike) / totalQty
	return tx.Model(&buyer).Updates(map[string]interface{}{
		"quantity":      totalQty,
		"avg_buy_price": newAvg,
		"account_id":    buyerAccountID,
		"updated_at":    now,
	}).Error
}

func reverseShareOwnership(tx *gorm.DB, sellerHoldingID, buyerID uint, buyerType string, assetID uint, amount, strike float64) error {
	now := time.Now().UTC()

	var seller models.PortfolioHoldingRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&seller, sellerHoldingID).Error; err != nil {
		return err
	}
	if err := tx.Model(&seller).Updates(map[string]interface{}{
		"quantity":          seller.Quantity + amount,
		"reserved_quantity": seller.ReservedQuantity + amount,
		"updated_at":        now,
	}).Error; err != nil {
		return err
	}

	var buyer models.PortfolioHoldingRecord
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND user_type = ? AND asset_id = ?", buyerID, buyerType, assetID).
		First(&buyer).Error
	if err == gorm.ErrRecordNotFound {
		// Nothing to reverse; the buyer holding never landed.
		return nil
	}
	if err != nil {
		return err
	}
	newQty := buyer.Quantity - amount
	if newQty < 0 {
		newQty = 0
	}
	updates := map[string]interface{}{
		"quantity":   newQty,
		"updated_at": now,
	}
	if newQty == 0 {
		updates["avg_buy_price"] = 0.0
	} else {
		// Reverse the weighted-average update. Numerically safe because
		// totalQty * newAvg = old_qty * old_avg + amount * strike was applied;
		// invert it.
		oldAvg := (buyer.Quantity*buyer.AvgBuyPrice - amount*strike) / newQty
		if oldAvg < 0 {
			oldAvg = 0
		}
		updates["avg_buy_price"] = oldAvg
	}
	return tx.Model(&buyer).Updates(updates).Error
}

// finalizeOtcExercise runs the consistency check that the spec mandates and
// marks the contract as exercised on success. Step 1 reserved (raspolozivo -=
// cost), step 3 debited stanje, step 4 moved shares; here we verify the
// invariants those steps left behind before flipping the contract status.
//
// SAGA-2 fix: previously this was a rubber-stamp status flip. Now we actually
// re-load the buyer/seller accounts and the buyer/seller holdings (under FOR
// UPDATE locks) and assert that no negative balances or quantities exist and
// the buyer is now in possession of at least `amount` shares of the asset.
func finalizeOtcExercise(tx *gorm.DB, contractID uint, buyerAccountID, sellerAccountID, sellerHoldingID, buyerID uint, buyerType string, assetID uint, amount, cost float64) error {
	var buyerAccount repository.OtcAccountReference
	if err := lockAccount(tx, buyerAccountID, &buyerAccount); err != nil {
		return fmt.Errorf("finalize: lock buyer account: %w", err)
	}
	if buyerAccount.Stanje < 0 || buyerAccount.RaspolozivoStanje < 0 {
		return fmt.Errorf("finalize: buyer account has negative balance after exercise")
	}

	var sellerAccount repository.OtcAccountReference
	if err := lockAccount(tx, sellerAccountID, &sellerAccount); err != nil {
		return fmt.Errorf("finalize: lock seller account: %w", err)
	}
	if sellerAccount.Stanje < 0 || sellerAccount.RaspolozivoStanje < 0 {
		return fmt.Errorf("finalize: seller account has negative balance after exercise")
	}

	var sellerHolding models.PortfolioHoldingRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&sellerHolding, sellerHoldingID).Error; err != nil {
		return fmt.Errorf("finalize: load seller holding: %w", err)
	}
	if sellerHolding.Quantity < 0 || sellerHolding.ReservedQuantity < 0 {
		return fmt.Errorf("finalize: seller holding has negative quantity/reservation")
	}
	if sellerHolding.ReservedQuantity > sellerHolding.Quantity {
		return fmt.Errorf("finalize: seller reservation exceeds quantity")
	}

	var buyerHolding models.PortfolioHoldingRecord
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND user_type = ? AND asset_id = ?", buyerID, buyerType, assetID).
		First(&buyerHolding).Error
	if err != nil {
		return fmt.Errorf("finalize: load buyer holding: %w", err)
	}
	if buyerHolding.Quantity < amount {
		return fmt.Errorf("finalize: buyer holding (%.4f) is below contract amount (%.4f)", buyerHolding.Quantity, amount)
	}

	_ = cost // kept in the signature for symmetry with the compensation

	now := time.Now().UTC()
	result := tx.Model(&models.OtcContractRecord{}).
		Where("id = ? AND status = ?", contractID, models.OtcContractStatusValid).
		Updates(map[string]interface{}{
			"status":     models.OtcContractStatusExercised,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("contract not in valid state at finalization")
	}
	return nil
}

func revertOtcExerciseFinalization(tx *gorm.DB, contractID, buyerAccountID uint, cost float64) error {
	_ = buyerAccountID
	_ = cost
	now := time.Now().UTC()
	return tx.Model(&models.OtcContractRecord{}).
		Where("id = ? AND status = ?", contractID, models.OtcContractStatusExercised).
		Updates(map[string]interface{}{
			"status":     models.OtcContractStatusValid,
			"updated_at": now,
		}).Error
}

func lockAccount(tx *gorm.DB, accountID uint, out *repository.OtcAccountReference) error {
	return tx.Table("accounts").
		Select("accounts.id, accounts.client_id, accounts.firma_id, accounts.zaposleni_id, currencies.kod AS currency_kod, accounts.stanje, accounts.raspolozivo_stanje, accounts.dnevna_potrosnja, accounts.mesecna_potrosnja, accounts.status").
		Joins("LEFT JOIN currencies ON currencies.id = accounts.currency_id").
		Where("accounts.id = ?", accountID).
		Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "accounts"}}).
		First(out).Error
}
