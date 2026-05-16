package repository

import (
	"errors"
	"fmt"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

type PortfolioRepository struct {
	db *gorm.DB
}

func NewPortfolioRepository(db *gorm.DB) *PortfolioRepository {
	return &PortfolioRepository{db: db}
}

// GetHoldingByUserAndAsset returns the holding for a user+asset pair, or nil if not found.
func (r *PortfolioRepository) GetHoldingByUserAndAsset(userID uint, userType string, assetID uint) (*models.PortfolioHoldingRecord, error) {
	var h models.PortfolioHoldingRecord
	err := r.db.Where("user_id = ? AND user_type = ? AND asset_id = ?", userID, userType, assetID).
		First(&h).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &h, nil
}

// GetHoldingByID returns a holding by primary key with Asset preloaded.
func (r *PortfolioRepository) GetHoldingByID(id uint) (*models.PortfolioHoldingRecord, error) {
	var h models.PortfolioHoldingRecord
	if err := r.db.Preload("Asset").Preload("Asset.Exchange").First(&h, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &h, nil
}

// ListHoldingsForUser returns all holdings for a user with Asset preloaded.
func (r *PortfolioRepository) ListHoldingsForUser(userID uint, userType string) ([]models.PortfolioHoldingRecord, error) {
	var records []models.PortfolioHoldingRecord
	if err := r.db.Preload("Asset").Preload("Asset.Exchange").
		Where("user_id = ? AND user_type = ? AND quantity > 0", userID, userType).
		Order("created_at ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// ListPublicOTCHoldings returns stock holdings with available public OTC quantity,
// excluding the requester so buyers do not see their own advertised shares.
func (r *PortfolioRepository) ListPublicOTCHoldings(excludeUserID uint, excludeUserType string) ([]models.PortfolioHoldingRecord, error) {
	var records []models.PortfolioHoldingRecord
	query := r.db.Preload("Asset").Preload("Asset.Exchange").
		Joins("JOIN market_listings ON market_listings.id = portfolio_holdings.asset_id").
		Where("portfolio_holdings.quantity > 0").
		Where("market_listings.type = ?", string(models.ListingTypeStock)).
		Where(`(
			portfolio_holdings.public_quantity > portfolio_holdings.reserved_quantity
			OR (
				portfolio_holdings.is_public = ?
				AND portfolio_holdings.public_quantity = 0
				AND portfolio_holdings.quantity > portfolio_holdings.reserved_quantity
			)
		)`, true)

	if excludeUserType != "" {
		query = query.Where("NOT (portfolio_holdings.user_id = ? AND portfolio_holdings.user_type = ?)", excludeUserID, excludeUserType)
	}

	if err := query.Order("market_listings.ticker ASC, portfolio_holdings.id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// SetHoldingAccountID backfills the account_id column on a holding. Used to
// recover legacy rows that were created before the buy-account was tracked.
func (r *PortfolioRepository) SetHoldingAccountID(id uint, accountID uint) error {
	return r.db.Model(&models.PortfolioHoldingRecord{}).
		Where("id = ? AND account_id = 0", id).
		Update("account_id", accountID).Error
}

// RecordBuyFill atomically upserts the portfolio holding for a buy fill.
// Creates a new holding if none exists; otherwise increases quantity and
// recalculates the weighted average buy price.
func (r *PortfolioRepository) RecordBuyFill(userID uint, userType string, assetID, accountID uint, filledQty float64, fillPrice float64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var h models.PortfolioHoldingRecord
		err := tx.Where("user_id = ? AND user_type = ? AND asset_id = ?", userID, userType, assetID).
			First(&h).Error

		now := time.Now().UTC()

		if errors.Is(err, gorm.ErrRecordNotFound) {
			// New holding.
			h = models.PortfolioHoldingRecord{
				UserID:      userID,
				UserType:    userType,
				AssetID:     assetID,
				AccountID:   accountID,
				Quantity:    filledQty,
				AvgBuyPrice: fillPrice,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			return tx.Create(&h).Error
		}
		if err != nil {
			return err
		}

		// Weighted average: (old_qty * old_avg + new_qty * new_price) / total_qty
		newQty := h.Quantity + filledQty
		newAvg := (h.Quantity*h.AvgBuyPrice + filledQty*fillPrice) / newQty

		// Track the account used for the latest buy so the sell flow can lock
		// the proceeds back to the same account the client purchased with.
		return tx.Model(&h).Updates(map[string]interface{}{
			"quantity":      newQty,
			"avg_buy_price": newAvg,
			"account_id":    accountID,
			"updated_at":    now,
		}).Error
	})
}

// RecordSellFill atomically decreases the holding quantity and accumulates
// realized profit. Returns the realized profit for this fill.
func (r *PortfolioRepository) RecordSellFill(userID uint, userType string, assetID uint, filledQty float64, sellPrice float64) (realizedProfit float64, err error) {
	err = r.db.Transaction(func(tx *gorm.DB) error {
		profit, txErr := RecordSellFillTx(tx, userID, userType, assetID, filledQty, sellPrice)
		realizedProfit = profit
		return txErr
	})
	return
}

// RecordSellFillTx is the transaction-aware variant of RecordSellFill. Used by
// fund-service so liquidation can run inside the withdrawal transaction (FUND-3).
func RecordSellFillTx(tx *gorm.DB, userID uint, userType string, assetID uint, filledQty float64, sellPrice float64) (float64, error) {
	var h models.PortfolioHoldingRecord
	if err := tx.Where("user_id = ? AND user_type = ? AND asset_id = ?", userID, userType, assetID).
		First(&h).Error; err != nil {
		return 0, err
	}

	realizedProfit := (sellPrice - h.AvgBuyPrice) * filledQty
	newQty := h.Quantity - filledQty
	if newQty < 0 {
		newQty = 0
	}
	if newQty < h.ReservedQuantity {
		return 0, fmt.Errorf("cannot sell reserved OTC quantity")
	}
	publicQuantity := h.PublicQuantity
	isPublic := h.IsPublic
	if publicQuantity > newQty {
		publicQuantity = newQty
	}
	if newQty == 0 {
		publicQuantity = 0
		isPublic = false
	}
	if publicQuantity > 0 {
		isPublic = true
	}

	if err := tx.Model(&h).Updates(map[string]interface{}{
		"quantity":        newQty,
		"realized_profit": h.RealizedProfit + realizedProfit,
		"is_public":       isPublic,
		"public_quantity": publicQuantity,
		"updated_at":      time.Now().UTC(),
	}).Error; err != nil {
		return 0, err
	}
	return realizedProfit, nil
}

// ExerciseOptionHolding zeroes out an option holding's quantity and adds the
// exercise profit to its cumulative realized_profit.
func (r *PortfolioRepository) ExerciseOptionHolding(id uint, exerciseProfit float64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var h models.PortfolioHoldingRecord
		if err := tx.First(&h, id).Error; err != nil {
			return err
		}
		return tx.Model(&h).Updates(map[string]interface{}{
			"quantity":        0,
			"realized_profit": h.RealizedProfit + exerciseProfit,
			"updated_at":      time.Now().UTC(),
		}).Error
	})
}

// SetHoldingPublic toggles the legacy is_public flag on a holding (used for OTC shares).
func (r *PortfolioRepository) SetHoldingPublic(id uint, isPublic bool) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var h models.PortfolioHoldingRecord
		if err := tx.Preload("Asset").First(&h, id).Error; err != nil {
			return err
		}

		publicQuantity := 0.0
		if isPublic {
			publicQuantity = h.Quantity
		}

		return setHoldingPublicQuantity(tx, &h, publicQuantity)
	})
}

func (r *PortfolioRepository) SetHoldingPublicQuantity(id uint, publicQuantity float64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var h models.PortfolioHoldingRecord
		if err := tx.Preload("Asset").First(&h, id).Error; err != nil {
			return err
		}
		return setHoldingPublicQuantity(tx, &h, publicQuantity)
	})
}

func (r *PortfolioRepository) ReserveHoldingQuantity(id uint, quantity float64) error {
	if quantity <= 0 {
		return fmt.Errorf("reserved quantity must be positive")
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var h models.PortfolioHoldingRecord
		if err := tx.Preload("Asset").First(&h, id).Error; err != nil {
			return err
		}
		if h.Asset.Type != string(models.ListingTypeStock) {
			return fmt.Errorf("only stock holdings can be reserved for OTC")
		}
		if h.AvailableForOTC() < quantity {
			return fmt.Errorf("insufficient public OTC quantity")
		}

		return tx.Model(&h).Updates(map[string]interface{}{
			"reserved_quantity": h.ReservedQuantity + quantity,
			"updated_at":        time.Now().UTC(),
		}).Error
	})
}

func (r *PortfolioRepository) ReleaseHoldingReservedQuantity(id uint, quantity float64) error {
	if quantity <= 0 {
		return fmt.Errorf("released quantity must be positive")
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var h models.PortfolioHoldingRecord
		if err := tx.First(&h, id).Error; err != nil {
			return err
		}
		if quantity > h.ReservedQuantity {
			return fmt.Errorf("released quantity exceeds reserved OTC quantity")
		}

		return tx.Model(&h).Updates(map[string]interface{}{
			"reserved_quantity": h.ReservedQuantity - quantity,
			"updated_at":        time.Now().UTC(),
		}).Error
	})
}

func setHoldingPublicQuantity(tx *gorm.DB, h *models.PortfolioHoldingRecord, publicQuantity float64) error {
	if publicQuantity < 0 {
		return fmt.Errorf("public quantity cannot be negative")
	}
	if h.Asset.Type != string(models.ListingTypeStock) && publicQuantity > 0 {
		return fmt.Errorf("only stock holdings can be public for OTC")
	}
	if publicQuantity > h.Quantity {
		return fmt.Errorf("public quantity cannot exceed owned quantity")
	}
	if publicQuantity < h.ReservedQuantity {
		return fmt.Errorf("public quantity cannot be below reserved OTC quantity")
	}

	return tx.Model(h).Updates(map[string]interface{}{
		"is_public":       publicQuantity > 0,
		"public_quantity": publicQuantity,
		"updated_at":      time.Now().UTC(),
	}).Error
}
