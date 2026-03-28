package service

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	FindByIDForUpdate(tx *gorm.DB, id uint) (*models.Account, error)
	UpdateFields(id uint, fields map[string]interface{}) error
	UpdateFieldsTx(tx *gorm.DB, id uint, fields map[string]interface{}) error
	FindBankAccountByCurrency(currencyKod string) (*models.Account, error)
	FindBankAccountByCurrencyForUpdate(tx *gorm.DB, currencyKod string) (*models.Account, error)
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
	GetSellRate(fromCurrencyKod, toCurrencyKod string) (float64, error)
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
	db           *gorm.DB
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

// WithDB sets the database handle used for transactional balance settlement.
// Call this in production; tests that don't set it fall back to the non-transactional path.
func (s *TransferService) WithDB(db *gorm.DB) *TransferService {
	s.db = db
	return s
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
	preview, sender, receiver, err := s.prepareTransfer(input)
	if err != nil {
		return nil, err
	}

	transfer := &models.Transfer{
		RacunPosiljaocaID: input.RacunPosiljaocaID,
		RacunPrimaocaID:   input.RacunPrimaocaID,
		Iznos:             input.Iznos,
		ValutaIznosa:      preview.ValutaIznosa,
		KonvertovaniIznos: preview.KonvertovaniIznos,
		Kurs:              preview.Kurs,
		Provizija:         preview.Provizija,
		ProvizijaProcent:  preview.ProvizijaProcent,
		Svrha:             input.Svrha,
		Status:            transferStatusPending,
		VremeTransakcije:  time.Now().UTC(),
	}

	if err := s.transferRepo.Create(transfer); err != nil {
		return nil, fmt.Errorf("failed to save transfer: %w", err)
	}

	ukupnoZaSkidanje := transfer.Iznos + transfer.Provizija
	if err := s.accountRepo.UpdateFields(sender.ID, map[string]interface{}{
		"stanje":             sender.Stanje - ukupnoZaSkidanje,
		"raspolozivo_stanje": sender.RaspolozivoStanje - ukupnoZaSkidanje,
		"dnevna_potrosnja":   sender.DnevnaPotrosnja + transfer.Iznos,
		"mesecna_potrosnja":  sender.MesecnaPotrosnja + transfer.Iznos,
	}); err != nil {
		return nil, fmt.Errorf("failed to update sender balance: %w", err)
	}

	if sender.CurrencyID != receiver.CurrencyID {
		bankFrom, err := s.accountRepo.FindBankAccountByCurrency(sender.Currency.Kod)
		if err != nil {
			return nil, fmt.Errorf("bank account for currency %s not found: %w", sender.Currency.Kod, err)
		}
		bankTo, err := s.accountRepo.FindBankAccountByCurrency(receiver.Currency.Kod)
		if err != nil {
			return nil, fmt.Errorf("bank account for currency %s not found: %w", receiver.Currency.Kod, err)
		}
		if err := s.accountRepo.UpdateFields(bankFrom.ID, map[string]interface{}{
			"stanje":             bankFrom.Stanje + ukupnoZaSkidanje,
			"raspolozivo_stanje": bankFrom.RaspolozivoStanje + ukupnoZaSkidanje,
		}); err != nil {
			return nil, fmt.Errorf("failed to update bank from-currency account: %w", err)
		}
		if err := s.accountRepo.UpdateFields(bankTo.ID, map[string]interface{}{
			"stanje":             bankTo.Stanje - transfer.KonvertovaniIznos,
			"raspolozivo_stanje": bankTo.RaspolozivoStanje - transfer.KonvertovaniIznos,
		}); err != nil {
			return nil, fmt.Errorf("failed to update bank to-currency account: %w", err)
		}
	}

	if err := s.accountRepo.UpdateFields(receiver.ID, map[string]interface{}{
		"stanje":             receiver.Stanje + transfer.KonvertovaniIznos,
		"raspolozivo_stanje": receiver.RaspolozivoStanje + transfer.KonvertovaniIznos,
	}); err != nil {
		return nil, fmt.Errorf("failed to update receiver balance: %w", err)
	}

	transfer.Status = transferStatusCompleted
	transfer.VremeTransakcije = time.Now().UTC()
	if err := s.transferRepo.Save(transfer); err != nil {
		return nil, fmt.Errorf("failed to save transfer: %w", err)
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

	return s.settleTransfer(transfer)
}

// settleTransfer executes balance deductions and marks the transfer completed.
// When a db is configured it wraps everything in a SELECT FOR UPDATE transaction
// to prevent concurrent double-spend. Otherwise falls back to the non-transactional path.
func (s *TransferService) settleTransfer(transfer *models.Transfer) (*models.Transfer, error) {
	if s.db == nil {
		return s.settleTransferNonTx(transfer)
	}

	var result *models.Transfer
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Re-fetch and lock the transfer row to prevent double-execution.
		var current models.Transfer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&current, transfer.ID).Error; err != nil {
			return fmt.Errorf("transfer not found: %w", err)
		}
		if current.Status != transferStatusPending {
			return &TransferVerificationError{
				Code:    "transfer_already_processed",
				Message: fmt.Sprintf("transfer already %s", current.Status),
				Status:  current.Status,
			}
		}

		sender, err := s.accountRepo.FindByIDForUpdate(tx, transfer.RacunPosiljaocaID)
		if err != nil {
			return fmt.Errorf("sender account not found: %w", err)
		}
		receiver, err := s.accountRepo.FindByIDForUpdate(tx, transfer.RacunPrimaocaID)
		if err != nil {
			return fmt.Errorf("receiver account not found: %w", err)
		}

		if sender.ClientID == nil || receiver.ClientID == nil || *sender.ClientID != *receiver.ClientID {
			return &TransferVerificationError{
				Code:    "transfer_ownership_mismatch",
				Message: "transfer accounts must belong to the same client",
				Status:  transferStatusCancelled,
			}
		}

		ukupnoZaSkidanje := transfer.Iznos + transfer.Provizija
		if sender.RaspolozivoStanje < ukupnoZaSkidanje {
			return &TransferVerificationError{
				Code:    "insufficient_balance",
				Message: fmt.Sprintf("insufficient balance: available %.2f, required %.2f", sender.RaspolozivoStanje, ukupnoZaSkidanje),
				Status:  transferStatusCancelled,
			}
		}
		if transfer.Iznos > sender.DnevniLimit || sender.DnevnaPotrosnja+transfer.Iznos > sender.DnevniLimit {
			return &TransferVerificationError{
				Code:    "daily_limit_exceeded",
				Message: "daily spending limit exceeded",
				Status:  transferStatusCancelled,
			}
		}
		if sender.MesecnaPotrosnja+transfer.Iznos > sender.MesecniLimit {
			return &TransferVerificationError{
				Code:    "monthly_limit_exceeded",
				Message: "monthly spending limit exceeded",
				Status:  transferStatusCancelled,
			}
		}

		if err := s.accountRepo.UpdateFieldsTx(tx, sender.ID, map[string]interface{}{
			"stanje":             sender.Stanje - ukupnoZaSkidanje,
			"raspolozivo_stanje": sender.RaspolozivoStanje - ukupnoZaSkidanje,
			"dnevna_potrosnja":   sender.DnevnaPotrosnja + transfer.Iznos,
			"mesecna_potrosnja":  sender.MesecnaPotrosnja + transfer.Iznos,
		}); err != nil {
			return fmt.Errorf("failed to update sender balance: %w", err)
		}

		// For cross-currency transfers, route through the bank's own accounts (menjačnica).
		if sender.CurrencyID != receiver.CurrencyID {
			bankFrom, err := s.accountRepo.FindBankAccountByCurrencyForUpdate(tx, sender.Currency.Kod)
			if err != nil {
				return fmt.Errorf("bank account for currency %s not found: %w", sender.Currency.Kod, err)
			}
			bankTo, err := s.accountRepo.FindBankAccountByCurrencyForUpdate(tx, receiver.Currency.Kod)
			if err != nil {
				return fmt.Errorf("bank account for currency %s not found: %w", receiver.Currency.Kod, err)
			}
			if err := s.accountRepo.UpdateFieldsTx(tx, bankFrom.ID, map[string]interface{}{
				"stanje":             bankFrom.Stanje + ukupnoZaSkidanje,
				"raspolozivo_stanje": bankFrom.RaspolozivoStanje + ukupnoZaSkidanje,
			}); err != nil {
				return fmt.Errorf("failed to update bank from-currency account: %w", err)
			}
			if err := s.accountRepo.UpdateFieldsTx(tx, bankTo.ID, map[string]interface{}{
				"stanje":             bankTo.Stanje - transfer.KonvertovaniIznos,
				"raspolozivo_stanje": bankTo.RaspolozivoStanje - transfer.KonvertovaniIznos,
			}); err != nil {
				return fmt.Errorf("failed to update bank to-currency account: %w", err)
			}
		}

		if err := s.accountRepo.UpdateFieldsTx(tx, receiver.ID, map[string]interface{}{
			"stanje":             receiver.Stanje + transfer.KonvertovaniIznos,
			"raspolozivo_stanje": receiver.RaspolozivoStanje + transfer.KonvertovaniIznos,
		}); err != nil {
			return fmt.Errorf("failed to update receiver balance: %w", err)
		}

		if err := tx.Model(&models.Transfer{}).Where("id = ?", transfer.ID).Updates(map[string]interface{}{
			"status":                  transferStatusCompleted,
			"verifikacioni_kod":       "",
			"verification_expires_at": nil,
			"vreme_transakcije":       time.Now().UTC(),
		}).Error; err != nil {
			return fmt.Errorf("failed to save transfer: %w", err)
		}

		transfer.Status = transferStatusCompleted
		transfer.VerifikacioniKod = ""
		transfer.VerificationExpiresAt = nil
		result = transfer
		return nil
	})
	if txErr != nil {
		var verr *TransferVerificationError
		if errors.As(txErr, &verr) && verr.Code != "transfer_already_processed" {
			s.cancelTransfer(transfer)
		}
		return nil, txErr
	}
	return result, nil
}

// settleTransferNonTx is the original non-transactional settlement path used by tests.
func (s *TransferService) settleTransferNonTx(transfer *models.Transfer) (*models.Transfer, error) {
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

	// For cross-currency transfers, route through the bank's own accounts (menjačnica):
	// the full sender deduction (amount + commission) is credited to the bank's account
	// in the sender's currency, and the converted amount is debited from the bank's
	// account in the receiver's currency.
	if sender.CurrencyID != receiver.CurrencyID {
		bankFrom, err := s.accountRepo.FindBankAccountByCurrency(sender.Currency.Kod)
		if err != nil {
			return nil, fmt.Errorf("bank account for currency %s not found: %w", sender.Currency.Kod, err)
		}
		bankTo, err := s.accountRepo.FindBankAccountByCurrency(receiver.Currency.Kod)
		if err != nil {
			return nil, fmt.Errorf("bank account for currency %s not found: %w", receiver.Currency.Kod, err)
		}
		if err := s.accountRepo.UpdateFields(bankFrom.ID, map[string]interface{}{
			"stanje":             bankFrom.Stanje + ukupnoZaSkidanje,
			"raspolozivo_stanje": bankFrom.RaspolozivoStanje + ukupnoZaSkidanje,
		}); err != nil {
			return nil, fmt.Errorf("failed to update bank from-currency account: %w", err)
		}
		if err := s.accountRepo.UpdateFields(bankTo.ID, map[string]interface{}{
			"stanje":             bankTo.Stanje - transfer.KonvertovaniIznos,
			"raspolozivo_stanje": bankTo.RaspolozivoStanje - transfer.KonvertovaniIznos,
		}); err != nil {
			return nil, fmt.Errorf("failed to update bank to-currency account: %w", err)
		}
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
			kursToRSD, err2 := s.exchangeSvc.GetSellRate(sender.Currency.Kod, "RSD")
			if err2 != nil {
				return nil, nil, nil, fmt.Errorf("failed to get exchange rate %s→RSD: %w", sender.Currency.Kod, err2)
			}
			rsdAmount = input.Iznos * kursToRSD
			kurs = kursToRSD
		}
		if receiver.Currency.Kod == "RSD" {
			konvertovaniIznos = math.Round(rsdAmount*100) / 100
		} else {
			kursFromRSD, err2 := s.exchangeSvc.GetSellRate("RSD", receiver.Currency.Kod)
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
