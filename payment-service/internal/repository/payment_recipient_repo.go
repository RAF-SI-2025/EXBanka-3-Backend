package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"gorm.io/gorm"
)

type PaymentRecipientRepository struct {
	db *gorm.DB
}

func NewPaymentRecipientRepository(db *gorm.DB) *PaymentRecipientRepository {
	return &PaymentRecipientRepository{db: db}
}

func (r *PaymentRecipientRepository) Create(recipient *models.PaymentRecipient) error {
	return r.db.Create(recipient).Error
}

func (r *PaymentRecipientRepository) FindByID(id uint) (*models.PaymentRecipient, error) {
	var recipient models.PaymentRecipient
	if err := r.db.First(&recipient, id).Error; err != nil {
		return nil, err
	}
	return &recipient, nil
}

func (r *PaymentRecipientRepository) ListByClientID(clientID uint) ([]models.PaymentRecipient, error) {
	var recipients []models.PaymentRecipient
	if err := r.db.Where("client_id = ?", clientID).Find(&recipients).Error; err != nil {
		return nil, err
	}
	return recipients, nil
}

func (r *PaymentRecipientRepository) Update(recipient *models.PaymentRecipient) error {
	return r.db.Save(recipient).Error
}

func (r *PaymentRecipientRepository) Delete(id uint) error {
	return r.db.Delete(&models.PaymentRecipient{}, id).Error
}
