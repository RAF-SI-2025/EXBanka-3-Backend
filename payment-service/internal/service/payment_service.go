package service

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
)

const (
	paymentStatusPending   = "u_obradi"
	paymentStatusCompleted = "uspesno"
	paymentStatusCancelled = "stornirano"

	paymentVerificationCodeTTL  = 5 * time.Minute
	paymentMaxVerificationTries = 3
)

type PaymentVerificationError struct {
	Code              string
	Message           string
	Status            string
	AttemptsRemaining int
}

func (e *PaymentVerificationError) Error() string {
	return e.Message
}

// Repository interfaces defined here to avoid circular imports.

type PaymentAccountRepositoryInterface interface {
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

type RecipientRepositoryInterface interface {
	Create(r *models.PaymentRecipient) error
}

type PaymentNotificationSender interface {
	SendVerificationCode(toEmail, clientName, code string, iznos float64, svrha, primaocRacun string) error
}

type CreatePaymentInput struct {
	RacunPosiljaocaID uint
	RacunPrimaocaBroj string
	Iznos             float64
	SifraPlacanja     string
	PozivNaBroj       string
	Svrha             string
	RecipientID       *uint
	AddRecipient      bool
	RecipientNaziv    string
	ClientEmail       string
	ClientName        string
}

type PaymentService struct {
	accountRepo   PaymentAccountRepositoryInterface
	paymentRepo   PaymentRepositoryInterface
	recipientRepo RecipientRepositoryInterface
	notifier      PaymentNotificationSender
}

func NewPaymentServiceWithRepos(
	accountRepo PaymentAccountRepositoryInterface,
	paymentRepo PaymentRepositoryInterface,
	recipientRepo RecipientRepositoryInterface,
	notifier PaymentNotificationSender,
) *PaymentService {
	return &PaymentService{
		accountRepo:   accountRepo,
		paymentRepo:   paymentRepo,
		recipientRepo: recipientRepo,
		notifier:      notifier,
	}
}

func (s *PaymentService) CreatePayment(input CreatePaymentInput) (*models.Payment, error) {
	if input.Iznos <= 0 {
		return nil, fmt.Errorf("payment amount must be positive")
	}

	sender, err := s.accountRepo.FindByID(input.RacunPosiljaocaID)
	if err != nil {
		return nil, fmt.Errorf("sender account not found: %w", err)
	}
	receiver, err := s.accountRepo.FindByBrojRacuna(strings.TrimSpace(input.RacunPrimaocaBroj))
	if err != nil {
		return nil, fmt.Errorf("receiver account not found: %w", err)
	}
	if sender.ID == receiver.ID {
		return nil, fmt.Errorf("sender and receiver accounts must be different")
	}
	if err := validatePaymentAccounts(sender, receiver); err != nil {
		return nil, err
	}

	if sender.RaspolozivoStanje < input.Iznos {
		return nil, fmt.Errorf("insufficient balance: available %.2f, requested %.2f",
			sender.RaspolozivoStanje, input.Iznos)
	}
	if sender.DnevnaPotrosnja+input.Iznos > sender.DnevniLimit {
		return nil, fmt.Errorf("daily spending limit exceeded: spent %.2f, limit %.2f, requested %.2f",
			sender.DnevnaPotrosnja, sender.DnevniLimit, input.Iznos)
	}
	if sender.MesecnaPotrosnja+input.Iznos > sender.MesecniLimit {
		return nil, fmt.Errorf("monthly spending limit exceeded: spent %.2f, limit %.2f, requested %.2f",
			sender.MesecnaPotrosnja, sender.MesecniLimit, input.Iznos)
	}

	code := fmt.Sprintf("%06d", rand.Intn(1_000_000))
	expiresAt := time.Now().UTC().Add(paymentVerificationCodeTTL)

	// Optionally save the receiver as a recipient
	if input.AddRecipient && s.recipientRepo != nil && sender.ClientID != nil {
		recipient := &models.PaymentRecipient{
			ClientID:   *sender.ClientID,
			Naziv:      input.RecipientNaziv,
			BrojRacuna: input.RacunPrimaocaBroj,
		}
		_ = s.recipientRepo.Create(recipient)
		if recipient.ID != 0 {
			input.RecipientID = &recipient.ID
		}
	}

	payment := &models.Payment{
		RacunPosiljaocaID:     input.RacunPosiljaocaID,
		RacunPrimaocaID:       &receiver.ID,
		RacunPrimaocaBroj:     receiver.BrojRacuna,
		Iznos:                 input.Iznos,
		SifraPlacanja:         input.SifraPlacanja,
		PozivNaBroj:           input.PozivNaBroj,
		Svrha:                 input.Svrha,
		Status:                paymentStatusPending,
		VerifikacioniKod:      code,
		VerificationExpiresAt: &expiresAt,
		RecipientID:           input.RecipientID,
		VremeTransakcije:      time.Now().UTC(),
	}

	if err := s.paymentRepo.Create(payment); err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	return payment, nil
}

func (s *PaymentService) VerifyPayment(paymentID uint, verificationCode string) (*models.Payment, error) {
	payment, err := s.paymentRepo.FindByID(paymentID)
	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}

	if payment.Status != paymentStatusPending {
		return nil, &PaymentVerificationError{
			Code:    "payment_not_pending",
			Message: fmt.Sprintf("payment is not pending: status=%s", payment.Status),
			Status:  payment.Status,
		}
	}

	expiresAt := payment.CreatedAt.Add(paymentVerificationCodeTTL)
	if payment.VerificationExpiresAt != nil {
		expiresAt = payment.VerificationExpiresAt.UTC()
	}
	if time.Now().UTC().After(expiresAt) {
		s.cancelPayment(payment)
		return nil, &PaymentVerificationError{
			Code:    "verification_code_expired",
			Message: "verification code expired",
			Status:  paymentStatusCancelled,
		}
	}

	if payment.VerifikacioniKod != strings.TrimSpace(verificationCode) {
		payment.BrojPokusaja++
		attemptsRemaining := paymentMaxVerificationTries - payment.BrojPokusaja
		if payment.BrojPokusaja >= paymentMaxVerificationTries {
			s.cancelPayment(payment)
			return nil, &PaymentVerificationError{
				Code:              "verification_attempts_exceeded",
				Message:           "maximum verification attempts exceeded, payment cancelled",
				Status:            paymentStatusCancelled,
				AttemptsRemaining: 0,
			}
		}
		_ = s.paymentRepo.Save(payment)
		return nil, &PaymentVerificationError{
			Code:              "invalid_verification_code",
			Message:           "invalid verification code",
			Status:            paymentStatusPending,
			AttemptsRemaining: attemptsRemaining,
		}
	}

	sender, err := s.accountRepo.FindByID(payment.RacunPosiljaocaID)
	if err != nil {
		return nil, fmt.Errorf("sender account not found: %w", err)
	}
	receiver, err := s.findReceiverAccount(payment)
	if err != nil {
		s.cancelPayment(payment)
		return nil, fmt.Errorf("receiver account not found: %w", err)
	}
	if err := validatePaymentAccounts(sender, receiver); err != nil {
		s.cancelPayment(payment)
		return nil, &PaymentVerificationError{
			Code:    "unsupported_payment_currency",
			Message: err.Error(),
			Status:  paymentStatusCancelled,
		}
	}

	if sender.RaspolozivoStanje < payment.Iznos {
		s.cancelPayment(payment)
		return nil, &PaymentVerificationError{
			Code:    "insufficient_balance",
			Message: fmt.Sprintf("insufficient balance: available %.2f, required %.2f", sender.RaspolozivoStanje, payment.Iznos),
			Status:  paymentStatusCancelled,
		}
	}
	if sender.DnevnaPotrosnja+payment.Iznos > sender.DnevniLimit {
		s.cancelPayment(payment)
		return nil, &PaymentVerificationError{
			Code:    "daily_limit_exceeded",
			Message: "daily spending limit exceeded",
			Status:  paymentStatusCancelled,
		}
	}
	if sender.MesecnaPotrosnja+payment.Iznos > sender.MesecniLimit {
		s.cancelPayment(payment)
		return nil, &PaymentVerificationError{
			Code:    "monthly_limit_exceeded",
			Message: "monthly spending limit exceeded",
			Status:  paymentStatusCancelled,
		}
	}

	if err := s.accountRepo.UpdateFields(sender.ID, map[string]interface{}{
		"stanje":             sender.Stanje - payment.Iznos,
		"raspolozivo_stanje": sender.RaspolozivoStanje - payment.Iznos,
		"dnevna_potrosnja":   sender.DnevnaPotrosnja + payment.Iznos,
		"mesecna_potrosnja":  sender.MesecnaPotrosnja + payment.Iznos,
	}); err != nil {
		return nil, fmt.Errorf("failed to deduct balance: %w", err)
	}

	if err := s.accountRepo.UpdateFields(receiver.ID, map[string]interface{}{
		"stanje":             receiver.Stanje + payment.Iznos,
		"raspolozivo_stanje": receiver.RaspolozivoStanje + payment.Iznos,
	}); err != nil {
		return nil, fmt.Errorf("failed to credit receiver balance: %w", err)
	}

	payment.Status = paymentStatusCompleted
	payment.VerifikacioniKod = ""
	payment.VerificationExpiresAt = nil
	payment.VremeTransakcije = time.Now().UTC()
	if err := s.paymentRepo.Save(payment); err != nil {
		return nil, fmt.Errorf("failed to update payment status: %w", err)
	}

	return payment, nil
}

func (s *PaymentService) GetPayment(id uint) (*models.Payment, error) {
	payment, err := s.paymentRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}
	return payment, nil
}

func (s *PaymentService) ApprovePaymentMobile(paymentID uint, mode string) (*models.Payment, string, *time.Time, error) {
	payment, expiresAt, err := s.pendingPaymentForMobile(paymentID)
	if err != nil {
		return nil, "", nil, err
	}

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "code":
		return payment, payment.VerifikacioniKod, &expiresAt, nil
	case "confirm":
		approved, err := s.VerifyPayment(paymentID, payment.VerifikacioniKod)
		if err != nil {
			return nil, "", nil, err
		}
		return approved, "", nil, nil
	default:
		return nil, "", nil, fmt.Errorf("unsupported approval mode")
	}
}

func (s *PaymentService) RejectPayment(paymentID uint) (*models.Payment, error) {
	payment, _, err := s.pendingPaymentForMobile(paymentID)
	if err != nil {
		return nil, err
	}

	s.cancelPayment(payment)
	return payment, nil
}

func (s *PaymentService) ListPaymentsByAccount(accountID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	return s.paymentRepo.ListByAccountID(accountID, filter)
}

func (s *PaymentService) ListPaymentsByClient(clientID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	return s.paymentRepo.ListByClientID(clientID, filter)
}

func (s *PaymentService) findReceiverAccount(payment *models.Payment) (*models.Account, error) {
	if payment.RacunPrimaocaID != nil {
		return s.accountRepo.FindByID(*payment.RacunPrimaocaID)
	}
	return s.accountRepo.FindByBrojRacuna(payment.RacunPrimaocaBroj)
}

func (s *PaymentService) pendingPaymentForMobile(paymentID uint) (*models.Payment, time.Time, error) {
	payment, err := s.paymentRepo.FindByID(paymentID)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("payment not found: %w", err)
	}

	if payment.Status != paymentStatusPending {
		return nil, time.Time{}, &PaymentVerificationError{
			Code:    "payment_not_pending",
			Message: fmt.Sprintf("payment is not pending: status=%s", payment.Status),
			Status:  payment.Status,
		}
	}

	expiresAt := payment.CreatedAt.Add(paymentVerificationCodeTTL)
	if payment.VerificationExpiresAt != nil {
		expiresAt = payment.VerificationExpiresAt.UTC()
	}
	if time.Now().UTC().After(expiresAt) {
		s.cancelPayment(payment)
		return nil, expiresAt, &PaymentVerificationError{
			Code:    "verification_code_expired",
			Message: "verification code expired",
			Status:  paymentStatusCancelled,
		}
	}

	if strings.TrimSpace(payment.VerifikacioniKod) == "" {
		return nil, expiresAt, &PaymentVerificationError{
			Code:    "verification_code_unavailable",
			Message: "verification code is unavailable",
			Status:  payment.Status,
		}
	}

	return payment, expiresAt, nil
}

func (s *PaymentService) cancelPayment(payment *models.Payment) {
	payment.Status = paymentStatusCancelled
	payment.VerifikacioniKod = ""
	payment.VerificationExpiresAt = nil
	payment.VremeTransakcije = time.Now().UTC()
	_ = s.paymentRepo.Save(payment)
}

func validatePaymentAccounts(sender, receiver *models.Account) error {
	if sender.Status != "" && sender.Status != "aktivan" {
		return fmt.Errorf("sender account is not active")
	}
	if receiver.Status != "" && receiver.Status != "aktivan" {
		return fmt.Errorf("receiver account is not active")
	}
	if sender.CurrencyID != 0 && receiver.CurrencyID != 0 && sender.CurrencyID != receiver.CurrencyID {
		return fmt.Errorf("cross-currency payments are not supported")
	}
	if sender.CurrencyKod != "" && sender.CurrencyKod != "RSD" {
		return fmt.Errorf("payments are supported only for RSD accounts")
	}
	if receiver.CurrencyKod != "" && receiver.CurrencyKod != "RSD" {
		return fmt.Errorf("payments are supported only for RSD accounts")
	}
	return nil
}
