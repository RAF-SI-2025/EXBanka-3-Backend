package service

import (
	"fmt"
	"time"

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
	ExistsByNameForClient(clientID uint, naziv string, excludeID uint) (bool, error)
}

// CurrencyRepositoryInterface is defined here to avoid circular imports.
type CurrencyRepositoryInterface interface {
	FindByID(id uint) (*models.Currency, error)
	FindByKod(kod string) (*models.Currency, error)
	FindAll() ([]models.Currency, error)
}

type CreateAccountInput struct {
	ClientID       *uint
	FirmaID        *uint
	ZaposleniID    *uint
	CurrencyID     uint
	Tip            string
	Vrsta          string
	Naziv          string
	PocetnoStanje  float64
	ClientEmail    string
	ClientName     string
}

type AccountService struct {
	accountRepo  AccountRepositoryInterface
	currencyRepo CurrencyRepositoryInterface
	notifSvc     *NotificationService
}

func NewAccountServiceWithRepos(accountRepo AccountRepositoryInterface, currencyRepo CurrencyRepositoryInterface, notifSvc *NotificationService) *AccountService {
	return &AccountService{
		accountRepo:  accountRepo,
		currencyRepo: currencyRepo,
		notifSvc:     notifSvc,
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

	expires := time.Now().AddDate(5, 0, 0)
	odrzavanje := 0.0
	if input.Tip == "tekuci" {
		odrzavanje = 255.00
	}
	account := &models.Account{
		BrojRacuna:        util.GenerateAccountNumber(input.Tip, input.Vrsta),
		ClientID:          input.ClientID,
		FirmaID:           input.FirmaID,
		ZaposleniID:       input.ZaposleniID,
		CurrencyID:        input.CurrencyID,
		Tip:               input.Tip,
		Vrsta:             input.Vrsta,
		Naziv:             input.Naziv,
		Stanje:            input.PocetnoStanje,
		RaspolozivoStanje: input.PocetnoStanje,
		DnevniLimit:       100000,
		MesecniLimit:      1000000,
		DatumIsteka:       &expires,
		OdrzavanjeRacuna:  odrzavanje,
		Status:            "aktivan",
	}

	if err := s.accountRepo.Create(account); err != nil {
		return nil, err
	}

	// Send email notification to the account owner
	if s.notifSvc != nil && input.ClientEmail != "" {
		currency, _ := s.currencyRepo.FindByID(input.CurrencyID)
		valuta := "RSD"
		if currency != nil {
			valuta = currency.Kod
		}
		_ = s.notifSvc.SendAccountCreatedEmail(input.ClientEmail, input.ClientName, account.BrojRacuna, input.Tip, valuta)
	}

	return account, nil
}

func (s *AccountService) ListAllAccounts(filter models.AccountFilter) ([]models.Account, int64, error) {
	return s.accountRepo.ListAll(filter)
}

func (s *AccountService) GetAccount(id uint) (*models.Account, error) {
	return s.accountRepo.FindByID(id)
}

func (s *AccountService) ListAccountsByClient(clientID uint) ([]models.Account, error) {
	return s.accountRepo.ListByClientID(clientID)
}

func (s *AccountService) UpdateAccountName(id uint, naziv string) error {
	if naziv == "" {
		return fmt.Errorf("naziv ne može biti prazan")
	}
	// Check uniqueness of name within client's accounts
	account, err := s.accountRepo.FindByID(id)
	if err != nil {
		return fmt.Errorf("račun nije pronađen: %w", err)
	}
	if account.ClientID != nil {
		exists, err := s.accountRepo.ExistsByNameForClient(*account.ClientID, naziv, id)
		if err != nil {
			return fmt.Errorf("greška pri proveri naziva: %w", err)
		}
		if exists {
			return fmt.Errorf("naziv '%s' već postoji za ovog klijenta", naziv)
		}
	}
	return s.accountRepo.UpdateFields(id, map[string]interface{}{"naziv": naziv})
}

func (s *AccountService) UpdateAccountLimits(id uint, clientID uint, dnevniLimit, mesecniLimit float64) error {
	if dnevniLimit < 0 {
		return fmt.Errorf("dnevni limit ne može biti negativan")
	}
	if mesecniLimit < 0 {
		return fmt.Errorf("mesečni limit ne može biti negativan")
	}
	// Only account owner can change limits
	account, err := s.accountRepo.FindByID(id)
	if err != nil {
		return fmt.Errorf("račun nije pronađen: %w", err)
	}
	if account.ClientID == nil || *account.ClientID != clientID {
		return fmt.Errorf("samo vlasnik računa može menjati limite")
	}
	return s.accountRepo.UpdateFields(id, map[string]interface{}{
		"dnevni_limit":  dnevniLimit,
		"mesecni_limit": mesecniLimit,
	})
}
