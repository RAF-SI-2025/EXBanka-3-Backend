package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
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
		Table("accounts").
		Select("accounts.*, currencies.kod as currency_kod").
		Joins("LEFT JOIN currencies ON currencies.id = accounts.currency_id").
		Where("accounts.id = ?", id).
		First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepository) FindByBrojRacuna(brojRacuna string) (*models.Account, error) {
	var account models.Account
	if err := r.db.
		Table("accounts").
		Select("accounts.*, currencies.kod as currency_kod").
		Joins("LEFT JOIN currencies ON currencies.id = accounts.currency_id").
		Where("accounts.broj_racuna = ?", brojRacuna).
		First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepository) UpdateFields(id uint, fields map[string]interface{}) error {
	return r.db.Model(&models.Account{}).Where("id = ?", id).Updates(fields).Error
}
