package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

type PriceAlertRepository struct {
	db *gorm.DB
}

func NewPriceAlertRepository(db *gorm.DB) *PriceAlertRepository {
	return &PriceAlertRepository{db: db}
}

func (r *PriceAlertRepository) Create(alert *models.PriceAlert) error {
	return r.db.Create(alert).Error
}

// ListByUser returns all active alerts for the given (userID, userType) pair,
// most recent first.
func (r *PriceAlertRepository) ListByUser(userID uint, userType string) ([]models.PriceAlert, error) {
	var alerts []models.PriceAlert
	err := r.db.
		Where("user_id = ? AND user_type = ? AND is_active = true", userID, userType).
		Order("created_at DESC").
		Find(&alerts).Error
	return alerts, err
}

// GetByID fetches a single alert by primary key; returns (nil, nil) if not found.
func (r *PriceAlertRepository) GetByID(id uint) (*models.PriceAlert, error) {
	var alert models.PriceAlert
	if err := r.db.First(&alert, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &alert, nil
}

// Deactivate sets is_active = false for the given alert ID.
func (r *PriceAlertRepository) Deactivate(id uint) error {
	return r.db.Model(&models.PriceAlert{}).
		Where("id = ?", id).
		Update("is_active", false).Error
}

// GetActiveByTicker returns all active alerts watching the given ticker.
// Called by the cron price-refresh loop after each price update.
func (r *PriceAlertRepository) GetActiveByTicker(ticker string) ([]models.PriceAlert, error) {
	var alerts []models.PriceAlert
	err := r.db.Where("ticker = ? AND is_active = true", ticker).Find(&alerts).Error
	return alerts, err
}
