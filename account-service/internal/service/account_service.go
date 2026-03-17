package service

import (
	"fmt"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
)

// AccountRepositoryInterface is defined here to avoid circular imports with repository package.
type AccountRepositoryInterface interface {
	Create(account *models.Account) error
	FindByID(id uint) (*models.Account, error)
	FindByBrojRacuna(broj string) (*models.Account, error)
	ListByClientID(clientID uint) ([]models.Account, error)
	ListAll(filter models.AccountFilter) ([]models.Account, int64, error)
	UpdateFields(id uint, fields map[string]interface{}) error
}

// CurrencyRepositoryInterface is defined here to avoid circular imports.
type CurrencyRepositoryInterface interface {
	FindByID(id uint) (*models.Currency, error)
	FindByKod(kod string) (*models.Currency, error)
	FindAll() ([]models.Currency, error)
}

type CreateAccountInput struct {
	ClientID   *uint
	FirmaID    *uint
	CurrencyID uint
	Tip        string
	Vrsta      string
	Naziv      string
}

type AccountService struct {
	accountRepo  AccountRepositoryInterface
	currencyRepo CurrencyRepositoryInterface
}

func NewAccountServiceWithRepos(accountRepo AccountRepositoryInterface, currencyRepo CurrencyRepositoryInterface) *AccountService {
	return &AccountService{
		accountRepo:  accountRepo,
		currencyRepo: currencyRepo,
	}
}

func (s *AccountService) CreateAccount(input CreateAccountInput) (*models.Account, error) {
	if input.Tip != "tekuci" && input.Tip != "devizni" {
		return nil, fmt.Errorf("invalid account type: %s (must be tekuci or devizni)", input.Tip)
	}
	if input.Vrsta != "licni" && input.Vrsta != "poslovni" {
		return nil, fmt.Errorf("invalid account kind: %s (must be licni or poslovni)", input.Vrsta)
	}
	if input.Vrsta == "poslovni" && input.FirmaID == nil {
		return nil, fmt.Errorf("poslovni account requires a firma")
	}
	if input.Tip == "devizni" {
		currency, err := s.currencyRepo.FindByID(input.CurrencyID)
		if err != nil {
			return nil, fmt.Errorf("currency not found: %w", err)
		}
		if currency.Kod == "RSD" {
			return nil, fmt.Errorf("devizni account cannot use RSD currency")
		}
	}

	account := &models.Account{
		BrojRacuna:        util.GenerateAccountNumber(),
		ClientID:          input.ClientID,
		FirmaID:           input.FirmaID,
		CurrencyID:        input.CurrencyID,
		Tip:               input.Tip,
		Vrsta:             input.Vrsta,
		Naziv:             input.Naziv,
		Stanje:            0,
		RaspolozivoStanje: 0,
		DnevniLimit:       100000,
		MesecniLimit:      1000000,
		Status:            "aktivan",
	}

	if err := s.accountRepo.Create(account); err != nil {
		return nil, err
	}
	return account, nil
}
