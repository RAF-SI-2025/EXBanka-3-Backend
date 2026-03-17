package repository

import "github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"

type AccountRepositoryInterface interface {
	Create(account *models.Account) error
	FindByID(id uint) (*models.Account, error)
	FindByBrojRacuna(broj string) (*models.Account, error)
	ListByClientID(clientID uint) ([]models.Account, error)
	ListAll(filter models.AccountFilter) ([]models.Account, int64, error)
	UpdateFields(id uint, fields map[string]interface{}) error
}

type CurrencyRepositoryInterface interface {
	FindByID(id uint) (*models.Currency, error)
	FindByKod(kod string) (*models.Currency, error)
	FindAll() ([]models.Currency, error)
}

type FirmaRepositoryInterface interface {
	Create(firma *models.Firma) error
	FindByID(id uint) (*models.Firma, error)
	FindByMaticniBroj(maticniBroj string) (*models.Firma, error)
	FindAll() ([]models.Firma, error)
}

type SifraDelatnostiRepositoryInterface interface {
	FindAll() ([]models.SifraDelatnosti, error)
	FindByID(id uint) (*models.SifraDelatnosti, error)
}

// Compile-time interface compliance checks.
var _ FirmaRepositoryInterface = (*FirmaRepository)(nil)
var _ SifraDelatnostiRepositoryInterface = (*SifraDelatnostiRepository)(nil)
var _ AccountRepositoryInterface = (*AccountRepository)(nil)
var _ CurrencyRepositoryInterface = (*CurrencyRepository)(nil)
