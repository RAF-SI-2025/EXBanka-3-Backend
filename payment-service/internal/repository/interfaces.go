package repository

import "github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"

type PaymentRecipientRepositoryInterface interface {
	Create(r *models.PaymentRecipient) error
	FindByID(id uint) (*models.PaymentRecipient, error)
	ListByClientID(clientID uint) ([]models.PaymentRecipient, error)
	Update(r *models.PaymentRecipient) error
	Delete(id uint) error
}

type AccountRepositoryInterface interface {
	FindByID(id uint) (*models.Account, error)
	FindByBrojRacuna(brojRacuna string) (*models.Account, error)
	UpdateFields(id uint, fields map[string]interface{}) error
}

type PaymentRepositoryInterface interface {
	Create(p *models.Payment) error
	FindByID(id uint) (*models.Payment, error)
	Save(p *models.Payment) error
	ListByAccountID(accountID uint, filter models.PaymentFilter) ([]models.Payment, int64, error)
	ListByClientID(clientID uint, filter models.PaymentFilter) ([]models.Payment, int64, error)
}

// Compile-time interface compliance checks.
var _ PaymentRecipientRepositoryInterface = (*PaymentRecipientRepository)(nil)
var _ AccountRepositoryInterface = (*AccountRepository)(nil)
var _ PaymentRepositoryInterface = (*PaymentRepository)(nil)
