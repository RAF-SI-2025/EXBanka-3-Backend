package repository

import "github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"

type AccountRepositoryInterface interface {
	FindByID(id uint) (*models.Account, error)
	UpdateFields(id uint, fields map[string]interface{}) error
}

type TransferRepositoryInterface interface {
	Create(transfer *models.Transfer) error
	FindByID(id uint) (*models.Transfer, error)
	ListByAccountID(accountID uint, filter models.TransferFilter) ([]models.Transfer, int64, error)
	ListByClientID(clientID uint, filter models.TransferFilter) ([]models.Transfer, int64, error)
}

// Compile-time interface compliance checks.
var _ AccountRepositoryInterface = (*AccountRepository)(nil)
var _ TransferRepositoryInterface = (*TransferRepository)(nil)
