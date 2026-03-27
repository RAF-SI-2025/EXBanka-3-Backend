package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"gorm.io/gorm"
)

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) FindByID(id uint) (*models.Account, error) {
	var account models.Account
	if err := r.db.
		Preload("Currency").
		Preload("Client").
		First(&account, id).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepository) UpdateFields(id uint, fields map[string]interface{}) error {
	return r.db.Model(&models.Account{}).Where("id = ?", id).Updates(fields).Error
}

// FindBankAccountByCurrency returns the bank's own account for the given currency code.
// Bank accounts are identified by having firma_id set and client_id NULL.
func (r *AccountRepository) FindBankAccountByCurrency(currencyKod string) (*models.Account, error) {
	var account models.Account
	err := r.db.
		Joins("JOIN currencies ON currencies.id = accounts.currency_id").
		Where("currencies.kod = ? AND accounts.firma_id IS NOT NULL AND accounts.client_id IS NULL", currencyKod).
		Preload("Currency").
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}
