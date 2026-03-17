package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"gorm.io/gorm"
)

type CurrencyRepository struct {
	db *gorm.DB
}

func NewCurrencyRepository(db *gorm.DB) *CurrencyRepository {
	return &CurrencyRepository{db: db}
}

func (r *CurrencyRepository) FindByID(id uint) (*models.Currency, error) {
	var currency models.Currency
	if err := r.db.First(&currency, id).Error; err != nil {
		return nil, err
	}
	return &currency, nil
}

func (r *CurrencyRepository) FindByKod(kod string) (*models.Currency, error) {
	var currency models.Currency
	if err := r.db.Where("kod = ?", kod).First(&currency).Error; err != nil {
		return nil, err
	}
	return &currency, nil
}

func (r *CurrencyRepository) FindAll() ([]models.Currency, error) {
	var currencies []models.Currency
	if err := r.db.Find(&currencies).Error; err != nil {
		return nil, err
	}
	return currencies, nil
}
