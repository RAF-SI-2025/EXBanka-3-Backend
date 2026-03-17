package service

import (
	"fmt"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
)

// AccountRepositoryInterface defined here to avoid circular imports.
type AccountRepositoryInterface interface {
	FindByID(id uint) (*models.Account, error)
	UpdateFields(id uint, fields map[string]interface{}) error
}

// TransferRepositoryInterface defined here to avoid circular imports.
type TransferRepositoryInterface interface {
	Create(transfer *models.Transfer) error
	FindByID(id uint) (*models.Transfer, error)
	ListByAccountID(accountID uint, filter models.TransferFilter) ([]models.Transfer, int64, error)
	ListByClientID(clientID uint, filter models.TransferFilter) ([]models.Transfer, int64, error)
}

// ExchangeRateServiceInterface allows mocking in tests.
type ExchangeRateServiceInterface interface {
	GetRate(fromCurrencyKod, toCurrencyKod string) (float64, error)
}

type CreateTransferInput struct {
	RacunPosiljaocaID uint
	RacunPrimaocaID   uint
	Iznos             float64
	Svrha             string
}

type TransferService struct {
	accountRepo  AccountRepositoryInterface
	transferRepo TransferRepositoryInterface
	exchangeSvc  ExchangeRateServiceInterface
}

func NewTransferServiceWithRepos(
	accountRepo AccountRepositoryInterface,
	transferRepo TransferRepositoryInterface,
	exchangeSvc ExchangeRateServiceInterface,
) *TransferService {
	return &TransferService{
		accountRepo:  accountRepo,
		transferRepo: transferRepo,
		exchangeSvc:  exchangeSvc,
	}
}

func (s *TransferService) ListTransfersByAccount(accountID uint, filter models.TransferFilter) ([]models.Transfer, int64, error) {
	return s.transferRepo.ListByAccountID(accountID, filter)
}

func (s *TransferService) ListTransfersByClient(clientID uint, filter models.TransferFilter) ([]models.Transfer, int64, error) {
	return s.transferRepo.ListByClientID(clientID, filter)
}

func (s *TransferService) CreateTransfer(input CreateTransferInput) (*models.Transfer, error) {
	if input.Iznos <= 0 {
		return nil, fmt.Errorf("transfer amount must be positive")
	}
	if input.RacunPosiljaocaID == input.RacunPrimaocaID {
		return nil, fmt.Errorf("sender and receiver accounts must be different")
	}

	sender, err := s.accountRepo.FindByID(input.RacunPosiljaocaID)
	if err != nil {
		return nil, fmt.Errorf("sender account not found: %w", err)
	}
	receiver, err := s.accountRepo.FindByID(input.RacunPrimaocaID)
	if err != nil {
		return nil, fmt.Errorf("receiver account not found: %w", err)
	}

	if sender.RaspolozivoStanje < input.Iznos {
		return nil, fmt.Errorf("insufficient balance: available %.2f, requested %.2f",
			sender.RaspolozivoStanje, input.Iznos)
	}
	if input.Iznos > sender.DnevniLimit {
		return nil, fmt.Errorf("amount %.2f exceeds daily limit %.2f",
			input.Iznos, sender.DnevniLimit)
	}

	// Determine converted amount and exchange rate.
	kurs := 1.0
	konvertovaniIznos := input.Iznos
	valutaIznosa := sender.Currency.Kod

	if sender.CurrencyID != receiver.CurrencyID {
		kurs, err = s.exchangeSvc.GetRate(sender.Currency.Kod, receiver.Currency.Kod)
		if err != nil {
			return nil, fmt.Errorf("failed to get exchange rate: %w", err)
		}
		konvertovaniIznos = input.Iznos * kurs
	}

	// Update sender balance.
	if err := s.accountRepo.UpdateFields(sender.ID, map[string]interface{}{
		"stanje":             sender.Stanje - input.Iznos,
		"raspolozivo_stanje": sender.RaspolozivoStanje - input.Iznos,
	}); err != nil {
		return nil, fmt.Errorf("failed to update sender balance: %w", err)
	}

	// Update receiver balance.
	if err := s.accountRepo.UpdateFields(receiver.ID, map[string]interface{}{
		"stanje":             receiver.Stanje + konvertovaniIznos,
		"raspolozivo_stanje": receiver.RaspolozivoStanje + konvertovaniIznos,
	}); err != nil {
		return nil, fmt.Errorf("failed to update receiver balance: %w", err)
	}

	transfer := &models.Transfer{
		RacunPosiljaocaID: input.RacunPosiljaocaID,
		RacunPrimaocaID:   input.RacunPrimaocaID,
		Iznos:             input.Iznos,
		ValutaIznosa:      valutaIznosa,
		KonvertovaniIznos: konvertovaniIznos,
		Kurs:              kurs,
		Svrha:             input.Svrha,
		Status:            "uspesno",
		VremeTransakcije:  time.Now(),
	}

	if err := s.transferRepo.Create(transfer); err != nil {
		return nil, fmt.Errorf("failed to save transfer: %w", err)
	}

	return transfer, nil
}
