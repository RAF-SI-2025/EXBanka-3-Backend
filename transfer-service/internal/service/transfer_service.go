package service

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
)

const (
	transferStatusPending   = "u_obradi"
	transferStatusCompleted = "uspesno"
	transferStatusCancelled = "stornirano"

	verificationCodeTTL            = 5 * time.Minute
	maxVerificationAttempts        = 3
	crossCurrencyCommissionPercent = 0.5
)

type TransferVerificationError struct {
	Code              string
	Message           string
	Status            string
	AttemptsRemaining int
}

func (e *TransferVerificationError) Error() string {
	return e.Message
}

// AccountRepositoryInterface defined here to avoid circular imports.
type AccountRepositoryInterface interface {
	FindByID(id uint) (*models.Account, error)
	UpdateFields(id uint, fields map[string]interface{}) error
}

// TransferRepositoryInterface defined here to avoid circular imports.
type TransferRepositoryInterface interface {
	Create(transfer *models.Transfer) error
	FindByID(id uint) (*models.Transfer, error)
	Save(transfer *models.Transfer) error
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

type TransferPreview struct {
	RacunPosiljaocaID uint
	RacunPrimaocaID   uint
	Iznos             float64
	ValutaIznosa      string
	KonvertovaniIznos float64
	Kurs              float64
	Provizija         float64
	ProvizijaProcent  float64
	Svrha             string
}

type TransferService struct {
	accountRepo  AccountRepositoryInterface
	transferRepo TransferRepositoryInterface
	exchangeSvc  ExchangeRateServiceInterface
	notifier     TransferNotificationSender
}

func NewTransferServiceWithRepos(
	accountRepo AccountRepositoryInterface,
	transferRepo TransferRepositoryInterface,
	exchangeSvc ExchangeRateServiceInterface,
) *TransferService {
	return NewTransferServiceWithReposAndNotifier(accountRepo, transferRepo, exchangeSvc, nil)
}

func NewTransferServiceWithReposAndNotifier(
	accountRepo AccountRepositoryInterface,
	transferRepo TransferRepositoryInterface,
	exchangeSvc ExchangeRateServiceInterface,
	notifier TransferNotificationSender,
) *TransferService {
	return &TransferService{
		accountRepo:  accountRepo,
		transferRepo: transferRepo,
		exchangeSvc:  exchangeSvc,
		notifier:     notifier,
	}
}

func (s *TransferService) ListTransfersByAccount(accountID uint, filter models.TransferFilter) ([]models.Transfer, int64, error) {
	return s.transferRepo.ListByAccountID(accountID, filter)
}

func (s *TransferService) ListTransfersByClient(clientID uint, filter models.TransferFilter) ([]models.Transfer, int64, error) {
	return s.transferRepo.ListByClientID(clientID, filter)
}

func (s *TransferService) PreviewTransfer(input CreateTransferInput) (*TransferPreview, error) {
	preview, _, _, err := s.prepareTransfer(input)
	if err != nil {
		return nil, err
	}
	return preview, nil
}

func (s *TransferService) CreateTransfer(input CreateTransferInput) (*models.Transfer, error) {
	preview, sender, _, err := s.prepareTransfer(input)
	if err != nil {
		return nil, err
	}

	code := fmt.Sprintf("%06d", rand.Intn(1_000_000))
	expiresAt := time.Now().UTC().Add(verificationCodeTTL)

	transfer := &models.Transfer{
		RacunPosiljaocaID:     input.RacunPosiljaocaID,
		RacunPrimaocaID:       input.RacunPrimaocaID,
		Iznos:                 input.Iznos,
		ValutaIznosa:          preview.ValutaIznosa,
		KonvertovaniIznos:     preview.KonvertovaniIznos,
		Kurs:                  preview.Kurs,
		Provizija:             preview.Provizija,
		ProvizijaProcent:      preview.ProvizijaProcent,
		Svrha:                 input.Svrha,
		Status:                transferStatusPending,
		VerifikacioniKod:      code,
		VerificationExpiresAt: &expiresAt,
		VremeTransakcije:      time.Now().UTC(),
	}

	if err := s.transferRepo.Create(transfer); err != nil {
		return nil, fmt.Errorf("failed to save transfer: %w", err)
	}

	if err := s.sendVerificationCode(sender, transfer); err != nil {
		s.cancelTransfer(transfer)
		return nil, err
	}

	return transfer, nil
}

func (s *TransferService) VerifyTransfer(transferID uint, verificationCode string) (*models.Transfer, error) {
	transfer, err := s.transferRepo.FindByID(transferID)
	if err != nil {
		return nil, fmt.Errorf("transfer not found: %w", err)
	}

	if transfer.Status != transferStatusPending {
		return nil, &TransferVerificationError{
			Code:    "transfer_not_pending",
			Message: fmt.Sprintf("transfer is not pending: status=%s", transfer.Status),
			Status:  transfer.Status,
		}
	}

	expiresAt := transfer.CreatedAt.Add(verificationCodeTTL)
	if transfer.VerificationExpiresAt != nil {
		expiresAt = transfer.VerificationExpiresAt.UTC()
	}
	if time.Now().UTC().After(expiresAt) {
		s.cancelTransfer(transfer)
		return nil, &TransferVerificationError{
			Code:    "verification_code_expired",
			Message: "verification code expired",
			Status:  transferStatusCancelled,
		}
	}

	if transfer.VerifikacioniKod != strings.TrimSpace(verificationCode) {
		transfer.BrojPokusaja++
		attemptsRemaining := maxVerificationAttempts - transfer.BrojPokusaja
		if transfer.BrojPokusaja >= maxVerificationAttempts {
			s.cancelTransfer(transfer)
			return nil, &TransferVerificationError{
				Code:              "verification_attempts_exceeded",
				Message:           "maximum verification attempts exceeded, transfer cancelled",
				Status:            transferStatusCancelled,
				AttemptsRemaining: 0,
			}
		}
		_ = s.transferRepo.Save(transfer)
		return nil, &TransferVerificationError{
			Code:              "invalid_verification_code",
			Message:           "invalid verification code",
			Status:            transferStatusPending,
			AttemptsRemaining: attemptsRemaining,
		}
	}

	sender, err := s.accountRepo.FindByID(transfer.RacunPosiljaocaID)
	if err != nil {
		return nil, fmt.Errorf("sender account not found: %w", err)
	}
	receiver, err := s.accountRepo.FindByID(transfer.RacunPrimaocaID)
	if err != nil {
		return nil, fmt.Errorf("receiver account not found: %w", err)
	}
	if sender.ClientID == nil || receiver.ClientID == nil || *sender.ClientID != *receiver.ClientID {
		s.cancelTransfer(transfer)
		return nil, &TransferVerificationError{
			Code:    "transfer_ownership_mismatch",
			Message: "transfer accounts must belong to the same client",
			Status:  transferStatusCancelled,
		}
	}

	ukupnoZaSkidanje := transfer.Iznos + transfer.Provizija
	if sender.RaspolozivoStanje < ukupnoZaSkidanje {
		s.cancelTransfer(transfer)
		return nil, &TransferVerificationError{
			Code:    "insufficient_balance",
			Message: fmt.Sprintf("insufficient balance: available %.2f, required %.2f", sender.RaspolozivoStanje, ukupnoZaSkidanje),
			Status:  transferStatusCancelled,
		}
	}
	if transfer.Iznos > sender.DnevniLimit || sender.DnevnaPotrosnja+transfer.Iznos > sender.DnevniLimit {
		s.cancelTransfer(transfer)
		return nil, &TransferVerificationError{
			Code:    "daily_limit_exceeded",
			Message: "daily spending limit exceeded",
			Status:  transferStatusCancelled,
		}
	}
	if sender.MesecnaPotrosnja+transfer.Iznos > sender.MesecniLimit {
		s.cancelTransfer(transfer)
		return nil, &TransferVerificationError{
			Code:    "monthly_limit_exceeded",
			Message: "monthly spending limit exceeded",
			Status:  transferStatusCancelled,
		}
	}

	if err := s.accountRepo.UpdateFields(sender.ID, map[string]interface{}{
		"stanje":             sender.Stanje - ukupnoZaSkidanje,
		"raspolozivo_stanje": sender.RaspolozivoStanje - ukupnoZaSkidanje,
		"dnevna_potrosnja":   sender.DnevnaPotrosnja + transfer.Iznos,
		"mesecna_potrosnja":  sender.MesecnaPotrosnja + transfer.Iznos,
	}); err != nil {
		return nil, fmt.Errorf("failed to update sender balance: %w", err)
	}

	if err := s.accountRepo.UpdateFields(receiver.ID, map[string]interface{}{
		"stanje":             receiver.Stanje + transfer.KonvertovaniIznos,
		"raspolozivo_stanje": receiver.RaspolozivoStanje + transfer.KonvertovaniIznos,
	}); err != nil {
		return nil, fmt.Errorf("failed to update receiver balance: %w", err)
	}

	transfer.Status = transferStatusCompleted
	transfer.VerifikacioniKod = ""
	transfer.VerificationExpiresAt = nil
	transfer.VremeTransakcije = time.Now().UTC()
	if err := s.transferRepo.Save(transfer); err != nil {
		return nil, fmt.Errorf("failed to save transfer: %w", err)
	}

	return transfer, nil
}

func (s *TransferService) ApproveTransferMobile(transferID uint, mode string) (*models.Transfer, string, *time.Time, error) {
	transfer, expiresAt, err := s.pendingTransferForMobile(transferID)
	if err != nil {
		return nil, "", nil, err
	}

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "code":
		return transfer, transfer.VerifikacioniKod, &expiresAt, nil
	case "confirm":
		approved, err := s.VerifyTransfer(transferID, transfer.VerifikacioniKod)
		if err != nil {
			return nil, "", nil, err
		}
		return approved, "", nil, nil
	default:
		return nil, "", nil, fmt.Errorf("unsupported approval mode")
	}
}

func (s *TransferService) RejectTransfer(transferID uint) (*models.Transfer, error) {
	transfer, _, err := s.pendingTransferForMobile(transferID)
	if err != nil {
		return nil, err
	}

	s.cancelTransfer(transfer)
	return transfer, nil
}

func (s *TransferService) prepareTransfer(input CreateTransferInput) (*TransferPreview, *models.Account, *models.Account, error) {
	if input.Iznos <= 0 {
		return nil, nil, nil, fmt.Errorf("transfer amount must be positive")
	}
	if input.RacunPosiljaocaID == input.RacunPrimaocaID {
		return nil, nil, nil, fmt.Errorf("sender and receiver accounts must be different")
	}

	sender, err := s.accountRepo.FindByID(input.RacunPosiljaocaID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("sender account not found: %w", err)
	}
	receiver, err := s.accountRepo.FindByID(input.RacunPrimaocaID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("receiver account not found: %w", err)
	}
	if sender.ClientID == nil || receiver.ClientID == nil || *sender.ClientID != *receiver.ClientID {
		return nil, nil, nil, fmt.Errorf("transfer accounts must belong to the same client")
	}

	if sender.RaspolozivoStanje < input.Iznos {
		return nil, nil, nil, fmt.Errorf("insufficient balance: available %.2f, requested %.2f",
			sender.RaspolozivoStanje, input.Iznos)
	}
	if input.Iznos > sender.DnevniLimit {
		return nil, nil, nil, fmt.Errorf("amount %.2f exceeds daily limit %.2f",
			input.Iznos, sender.DnevniLimit)
	}
	if sender.DnevnaPotrosnja+input.Iznos > sender.DnevniLimit {
		return nil, nil, nil, fmt.Errorf("daily spending limit exceeded: spent %.2f, limit %.2f, requested %.2f",
			sender.DnevnaPotrosnja, sender.DnevniLimit, input.Iznos)
	}
	if sender.MesecnaPotrosnja+input.Iznos > sender.MesecniLimit {
		return nil, nil, nil, fmt.Errorf("monthly spending limit exceeded: spent %.2f, limit %.2f, requested %.2f",
			sender.MesecnaPotrosnja, sender.MesecniLimit, input.Iznos)
	}

	kurs := 1.0
	konvertovaniIznos := input.Iznos
	valutaIznosa := sender.Currency.Kod
	provizijaProcent := 0.0
	provizija := 0.0

	if sender.CurrencyID != receiver.CurrencyID {
		var rsdAmount float64
		if sender.Currency.Kod == "RSD" {
			rsdAmount = input.Iznos
		} else {
			kursToRSD, err2 := s.exchangeSvc.GetRate(sender.Currency.Kod, "RSD")
			if err2 != nil {
				return nil, nil, nil, fmt.Errorf("failed to get exchange rate %s→RSD: %w", sender.Currency.Kod, err2)
			}
			rsdAmount = input.Iznos * kursToRSD
			kurs = kursToRSD
		}
		if receiver.Currency.Kod == "RSD" {
			konvertovaniIznos = math.Round(rsdAmount*100) / 100
		} else {
			kursFromRSD, err2 := s.exchangeSvc.GetRate("RSD", receiver.Currency.Kod)
			if err2 != nil {
				return nil, nil, nil, fmt.Errorf("failed to get exchange rate RSD→%s: %w", receiver.Currency.Kod, err2)
			}
			konvertovaniIznos = math.Round(rsdAmount*kursFromRSD*100) / 100
			if input.Iznos > 0 {
				kurs = konvertovaniIznos / input.Iznos
			}
		}
		provizijaProcent = crossCurrencyCommissionPercent
		provizija = math.Round(input.Iznos*provizijaProcent) / 100

		ukupnoZaSkidanje := input.Iznos + provizija
		if sender.RaspolozivoStanje < ukupnoZaSkidanje {
			return nil, nil, nil, fmt.Errorf("insufficient balance: available %.2f, required %.2f (amount %.2f + commission %.2f)",
				sender.RaspolozivoStanje, ukupnoZaSkidanje, input.Iznos, provizija)
		}
	}

	return &TransferPreview{
		RacunPosiljaocaID: input.RacunPosiljaocaID,
		RacunPrimaocaID:   input.RacunPrimaocaID,
		Iznos:             input.Iznos,
		ValutaIznosa:      valutaIznosa,
		KonvertovaniIznos: konvertovaniIznos,
		Kurs:              kurs,
		Provizija:         provizija,
		ProvizijaProcent:  provizijaProcent,
		Svrha:             input.Svrha,
	}, sender, receiver, nil
}

func (s *TransferService) sendVerificationCode(sender *models.Account, transfer *models.Transfer) error {
	if s.notifier == nil {
		return nil
	}
	if sender.Client == nil || strings.TrimSpace(sender.Client.Email) == "" {
		return fmt.Errorf("failed to deliver verification code: sender email not found")
	}

	fullName := strings.TrimSpace(strings.TrimSpace(sender.Client.Ime) + " " + strings.TrimSpace(sender.Client.Prezime))
	if fullName == "" {
		fullName = sender.Client.Email
	}

	if err := s.notifier.SendTransferVerificationCode(sender.Client.Email, fullName, transfer); err != nil {
		return fmt.Errorf("failed to deliver verification code: %w", err)
	}

	return nil
}

func (s *TransferService) pendingTransferForMobile(transferID uint) (*models.Transfer, time.Time, error) {
	transfer, err := s.transferRepo.FindByID(transferID)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("transfer not found: %w", err)
	}

	if transfer.Status != transferStatusPending {
		return nil, time.Time{}, &TransferVerificationError{
			Code:    "transfer_not_pending",
			Message: fmt.Sprintf("transfer is not pending: status=%s", transfer.Status),
			Status:  transfer.Status,
		}
	}

	expiresAt := transfer.CreatedAt.Add(verificationCodeTTL)
	if transfer.VerificationExpiresAt != nil {
		expiresAt = transfer.VerificationExpiresAt.UTC()
	}
	if time.Now().UTC().After(expiresAt) {
		s.cancelTransfer(transfer)
		return nil, expiresAt, &TransferVerificationError{
			Code:    "verification_code_expired",
			Message: "verification code expired",
			Status:  transferStatusCancelled,
		}
	}

	if strings.TrimSpace(transfer.VerifikacioniKod) == "" {
		return nil, expiresAt, &TransferVerificationError{
			Code:    "verification_code_unavailable",
			Message: "verification code is unavailable",
			Status:  transfer.Status,
		}
	}

	return transfer, expiresAt, nil
}

func (s *TransferService) cancelTransfer(transfer *models.Transfer) {
	transfer.Status = transferStatusCancelled
	transfer.VerifikacioniKod = ""
	transfer.VerificationExpiresAt = nil
	_ = s.transferRepo.Save(transfer)
}
