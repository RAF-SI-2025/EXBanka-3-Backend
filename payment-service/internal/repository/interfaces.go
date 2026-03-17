package repository

import "github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"

type PaymentRecipientRepositoryInterface interface {
	Create(r *models.PaymentRecipient) error
	FindByID(id uint) (*models.PaymentRecipient, error)
	ListByClientID(clientID uint) ([]models.PaymentRecipient, error)
	Update(r *models.PaymentRecipient) error
	Delete(id uint) error
}

// Compile-time interface compliance check.
var _ PaymentRecipientRepositoryInterface = (*PaymentRecipientRepository)(nil)
