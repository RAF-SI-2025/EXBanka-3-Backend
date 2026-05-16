package repository

import (
	"errors"
	"fmt"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OtcRepository struct {
	db *gorm.DB
}

func NewOtcRepository(db *gorm.DB) *OtcRepository {
	return &OtcRepository{db: db}
}

func (r *OtcRepository) CreateOffer(offer *models.OtcOfferRecord) error {
	now := time.Now().UTC()
	if offer.Status == "" {
		offer.Status = models.OtcOfferStatusPending
	}
	if offer.LastModified.IsZero() {
		offer.LastModified = now
	}
	return r.db.Create(offer).Error
}

type OtcAccountReference struct {
	ID                uint
	ClientID          *uint   `gorm:"column:client_id"`
	FirmaID           *uint   `gorm:"column:firma_id"`
	ZaposleniID       *uint   `gorm:"column:zaposleni_id"`
	CurrencyCode      string  `gorm:"column:currency_kod"`
	Stanje            float64 `gorm:"column:stanje"`
	RaspolozivoStanje float64 `gorm:"column:raspolozivo_stanje"`
	DnevnaPotrosnja   float64 `gorm:"column:dnevna_potrosnja"`
	MesecnaPotrosnja  float64 `gorm:"column:mesecna_potrosnja"`
	Status            string  `gorm:"column:status"`
}

func (a OtcAccountReference) IsOwnedBy(userID uint, userType string) bool {
	switch userType {
	case "client":
		return a.ClientID != nil && *a.ClientID == userID
	case "bank":
		return a.ClientID == nil
	default:
		return false
	}
}

func (r *OtcRepository) GetAccountReference(accountID uint) (*OtcAccountReference, error) {
	return r.getAccountReference(r.db, accountID, false)
}

func (r *OtcRepository) GetOfferByID(id uint) (*models.OtcOfferRecord, error) {
	var offer models.OtcOfferRecord
	if err := r.offerPreloads(r.db).First(&offer, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &offer, nil
}

func (r *OtcRepository) ListOffersForParticipant(userID uint, userType, status string) ([]models.OtcOfferRecord, error) {
	q := r.offerPreloads(r.db).
		Where("(buyer_id = ? AND buyer_type = ?) OR (seller_id = ? AND seller_type = ?)",
			userID, userType, userID, userType)
	if status != "" {
		q = q.Where("status = ?", status)
	}

	var offers []models.OtcOfferRecord
	if err := q.Order("last_modified DESC, id DESC").Find(&offers).Error; err != nil {
		return nil, err
	}
	return offers, nil
}

func (r *OtcRepository) UpdateOfferTerms(id uint, amount, pricePerStock float64, settlementDate time.Time, premium float64, modifiedByID uint, modifiedByType string) error {
	return r.db.Model(&models.OtcOfferRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"amount":           amount,
		"price_per_stock":  pricePerStock,
		"settlement_date":  settlementDate,
		"premium":          premium,
		"last_modified":    time.Now().UTC(),
		"modified_by_id":   modifiedByID,
		"modified_by_type": modifiedByType,
	}).Error
}

func (r *OtcRepository) UpdateOfferStatus(id uint, status string, modifiedByID uint, modifiedByType string) error {
	return r.db.Model(&models.OtcOfferRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":           status,
		"last_modified":    time.Now().UTC(),
		"modified_by_id":   modifiedByID,
		"modified_by_type": modifiedByType,
	}).Error
}

func (r *OtcRepository) AcceptOfferAndCreateContract(offerID uint, acceptorID uint, acceptorType string) (*models.OtcContractRecord, error) {
	var contractID uint
	if err := r.db.Transaction(func(tx *gorm.DB) error {
		var offer models.OtcOfferRecord
		if err := r.offerPreloads(tx).First(&offer, offerID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("offer not found")
			}
			return err
		}
		if offer.Status != models.OtcOfferStatusPending {
			return fmt.Errorf("only pending offers can be accepted")
		}
		isBuyer := offer.BuyerID == acceptorID && offer.BuyerType == acceptorType
		isSeller := offer.SellerID == acceptorID && offer.SellerType == acceptorType
		if !isBuyer && !isSeller {
			return fmt.Errorf("only a negotiation participant can accept an offer")
		}

		var holding models.PortfolioHoldingRecord
		// OTC-4: row-lock the holding so concurrent acceptances of different
		// pending offers on the same holding cannot over-reserve
		// reserved_quantity (which is read-modify-write).
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Asset").First(&holding, offer.SellerHoldingID).Error; err != nil {
			return err
		}
		if holding.Asset.Type != string(models.ListingTypeStock) {
			return fmt.Errorf("only stock holdings can be reserved for OTC")
		}
		if holding.AvailableForOTC() < offer.Amount {
			return fmt.Errorf("insufficient public OTC quantity")
		}
		buyerAccount, err := r.getAccountReference(tx, offer.BuyerAccountID, true)
		if err != nil {
			return err
		}
		sellerAccount, err := r.getAccountReference(tx, offer.SellerAccountID, true)
		if err != nil {
			return err
		}
		if err := validatePremiumAccounts(offer, buyerAccount, sellerAccount); err != nil {
			return err
		}
		if offer.Premium > 0 {
			if buyerAccount.RaspolozivoStanje < offer.Premium {
				return fmt.Errorf("insufficient funds for OTC premium")
			}
			if err := debitOtcPremium(tx, offer.BuyerAccountID, offer.Premium); err != nil {
				return err
			}
			if err := creditOtcPremium(tx, offer.SellerAccountID, offer.Premium); err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		if err := tx.Model(&holding).Updates(map[string]interface{}{
			"reserved_quantity": holding.ReservedQuantity + offer.Amount,
			"updated_at":        now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&offer).Updates(map[string]interface{}{
			"status":           models.OtcOfferStatusAccepted,
			"last_modified":    now,
			"modified_by_id":   acceptorID,
			"modified_by_type": acceptorType,
			"updated_at":       now,
		}).Error; err != nil {
			return err
		}

		sourceOfferID := offer.ID
		contract := models.OtcContractRecord{
			OfferID:         &sourceOfferID,
			StockListingID:  offer.StockListingID,
			SellerHoldingID: offer.SellerHoldingID,
			Amount:          offer.Amount,
			StrikePrice:     offer.PricePerStock,
			Premium:         offer.Premium,
			SettlementDate:  offer.SettlementDate,
			BuyerID:         offer.BuyerID,
			BuyerType:       offer.BuyerType,
			BuyerAccountID:  offer.BuyerAccountID,
			SellerID:        offer.SellerID,
			SellerType:      offer.SellerType,
			SellerAccountID: offer.SellerAccountID,
			Status:          models.OtcContractStatusValid,
			BankID:          offer.BankID,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(&contract).Error; err != nil {
			return err
		}
		contractID = contract.ID
		return nil
	}); err != nil {
		return nil, err
	}

	return r.GetContractByID(contractID)
}

func (r *OtcRepository) CreateContract(contract *models.OtcContractRecord) error {
	if contract.Status == "" {
		contract.Status = models.OtcContractStatusValid
	}
	return r.db.Create(contract).Error
}

func (r *OtcRepository) GetContractByID(id uint) (*models.OtcContractRecord, error) {
	var contract models.OtcContractRecord
	if err := r.contractPreloads(r.db).First(&contract, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &contract, nil
}

func (r *OtcRepository) ListContractsForParticipant(userID uint, userType, status string) ([]models.OtcContractRecord, error) {
	q := r.contractPreloads(r.db).
		Where("(buyer_id = ? AND buyer_type = ?) OR (seller_id = ? AND seller_type = ?)",
			userID, userType, userID, userType)
	if status != "" {
		q = q.Where("status = ?", status)
	}

	var contracts []models.OtcContractRecord
	if err := q.Order("created_at DESC, id DESC").Find(&contracts).Error; err != nil {
		return nil, err
	}
	return contracts, nil
}

// OTC-1: contracts remain valid through the entire calendar day of
// settlement_date. They expire only once 24h have passed since settlement_date
// (i.e. at 00:00 UTC of the day AFTER settlement_date). The same cutoff is
// applied by OtcService.ExerciseContract — keeping cron, BE exercise window,
// and FE Iskoristi-button in agreement.
func otcExpiryCutoff(referenceTime time.Time) time.Time {
	return referenceTime.Add(-24 * time.Hour)
}

func (r *OtcRepository) ListExpiredValidContracts(referenceTime time.Time) ([]models.OtcContractRecord, error) {
	var contracts []models.OtcContractRecord
	if err := r.contractPreloads(r.db).
		Where("status = ? AND settlement_date <= ?", models.OtcContractStatusValid, otcExpiryCutoff(referenceTime)).
		Order("settlement_date ASC, id ASC").
		Find(&contracts).Error; err != nil {
		return nil, err
	}
	return contracts, nil
}

func (r *OtcRepository) ExpireValidContracts(referenceTime time.Time) (int, error) {
	expiredCount := 0
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var contracts []models.OtcContractRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ? AND settlement_date <= ?", models.OtcContractStatusValid, otcExpiryCutoff(referenceTime)).
			Order("settlement_date ASC, id ASC").
			Find(&contracts).Error; err != nil {
			return err
		}

		now := time.Now().UTC()
		for _, contract := range contracts {
			var holding models.PortfolioHoldingRecord
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&holding, contract.SellerHoldingID).Error; err != nil {
				return err
			}

			newReservedQuantity := holding.ReservedQuantity - contract.Amount
			if newReservedQuantity < 0 {
				newReservedQuantity = 0
			}
			if err := tx.Model(&holding).Updates(map[string]interface{}{
				"reserved_quantity": newReservedQuantity,
				"updated_at":        now,
			}).Error; err != nil {
				return err
			}

			result := tx.Model(&models.OtcContractRecord{}).
				Where("id = ? AND status = ?", contract.ID, models.OtcContractStatusValid).
				Updates(map[string]interface{}{
					"status":     models.OtcContractStatusExpired,
					"updated_at": now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				expiredCount++
			}
		}
		return nil
	})
	return expiredCount, err
}

func (r *OtcRepository) UpdateContractStatus(id uint, status string) error {
	return r.db.Model(&models.OtcContractRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":     status,
		"updated_at": time.Now().UTC(),
	}).Error
}

func (r *OtcRepository) offerPreloads(db *gorm.DB) *gorm.DB {
	return db.
		Preload("StockListing").
		Preload("StockListing.Exchange").
		Preload("SellerHolding").
		Preload("SellerHolding.Asset").
		Preload("SellerHolding.Asset.Exchange")
}

func (r *OtcRepository) contractPreloads(db *gorm.DB) *gorm.DB {
	return db.
		Preload("StockListing").
		Preload("StockListing.Exchange").
		Preload("SellerHolding").
		Preload("SellerHolding.Asset").
		Preload("SellerHolding.Asset.Exchange")
}

func (r *OtcRepository) getAccountReference(db *gorm.DB, accountID uint, lock bool) (*OtcAccountReference, error) {
	if accountID == 0 {
		return nil, nil
	}
	q := db.Table("accounts").
		Select("accounts.id, accounts.client_id, accounts.firma_id, accounts.zaposleni_id, currencies.kod AS currency_kod, accounts.stanje, accounts.raspolozivo_stanje, accounts.dnevna_potrosnja, accounts.mesecna_potrosnja, accounts.status").
		Joins("LEFT JOIN currencies ON currencies.id = accounts.currency_id").
		Where("accounts.id = ?", accountID)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "accounts"}})
	}

	var account OtcAccountReference
	if err := q.First(&account).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &account, nil
}

func validatePremiumAccounts(offer models.OtcOfferRecord, buyerAccount, sellerAccount *OtcAccountReference) error {
	if buyerAccount == nil {
		return fmt.Errorf("buyer account not found")
	}
	if sellerAccount == nil {
		return fmt.Errorf("seller account not found")
	}
	if buyerAccount.Status != "aktivan" {
		return fmt.Errorf("buyer account is not active")
	}
	if sellerAccount.Status != "aktivan" {
		return fmt.Errorf("seller account is not active")
	}
	expectedCurrency := offer.StockListing.Exchange.Currency
	if buyerAccount.CurrencyCode != expectedCurrency || sellerAccount.CurrencyCode != expectedCurrency {
		return fmt.Errorf("premium account currency must match stock currency")
	}
	if !buyerAccount.IsOwnedBy(offer.BuyerID, offer.BuyerType) {
		return fmt.Errorf("buyer account does not belong to buyer")
	}
	if !sellerAccount.IsOwnedBy(offer.SellerID, offer.SellerType) {
		return fmt.Errorf("seller account does not belong to seller")
	}
	return nil
}

func debitOtcPremium(tx *gorm.DB, accountID uint, amount float64) error {
	result := tx.Table("accounts").
		Where("id = ? AND raspolozivo_stanje >= ?", accountID, amount).
		Updates(map[string]interface{}{
			"stanje":             gorm.Expr("stanje - ?", amount),
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje - ?", amount),
			"dnevna_potrosnja":   gorm.Expr("dnevna_potrosnja + ?", amount),
			"mesecna_potrosnja":  gorm.Expr("mesecna_potrosnja + ?", amount),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("insufficient funds for OTC premium")
	}
	return nil
}

func creditOtcPremium(tx *gorm.DB, accountID uint, amount float64) error {
	return tx.Table("accounts").
		Where("id = ?", accountID).
		Updates(map[string]interface{}{
			"stanje":             gorm.Expr("stanje + ?", amount),
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje + ?", amount),
		}).Error
}
