package service

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
)

const prenosPaymentCode = "254"

type CreatePrenosInput struct {
	RacunPosiljaocaID uint
	RacunPrimaocaBroj string
	Iznos             float64
	Svrha             string
	ClientEmail       string
	ClientName        string
}

type PrenosService struct {
	accountRepo PaymentAccountRepositoryInterface
	paymentRepo PaymentRepositoryInterface
	notifier    PaymentNotificationSender
}

func NewPrenosServiceWithRepos(
	accountRepo PaymentAccountRepositoryInterface,
	paymentRepo PaymentRepositoryInterface,
	notifier PaymentNotificationSender,
) *PrenosService {
	return &PrenosService{
		accountRepo: accountRepo,
		paymentRepo: paymentRepo,
		notifier:    notifier,
	}
}

func (s *PrenosService) CreatePrenos(input CreatePrenosInput) (*models.Payment, error) {
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
	if err := validatePrenosAccounts(sender, receiver); err != nil {
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

	payment := &models.Payment{
		RacunPosiljaocaID:     input.RacunPosiljaocaID,
		RacunPrimaocaID:       &receiver.ID,
		RacunPrimaocaBroj:     receiver.BrojRacuna,
		Iznos:                 input.Iznos,
		SifraPlacanja:         prenosPaymentCode,
		PozivNaBroj:           "",
		Svrha:                 input.Svrha,
		Status:                paymentStatusPending,
		VerifikacioniKod:      code,
		VerificationExpiresAt: &expiresAt,
		VremeTransakcije:      time.Now().UTC(),
	}

	if err := s.paymentRepo.Create(payment); err != nil {
		return nil, fmt.Errorf("failed to create prenos: %w", err)
	}

	if s.notifier != nil {
		if strings.TrimSpace(input.ClientEmail) == "" {
			s.cancelPrenos(payment)
			return nil, fmt.Errorf("client email required for prenos verification")
		}
		if err := s.notifier.SendVerificationCode(
			input.ClientEmail,
			input.ClientName,
			code,
			input.Iznos,
			input.Svrha,
			payment.RacunPrimaocaBroj,
		); err != nil {
			s.cancelPrenos(payment)
			return nil, fmt.Errorf("failed to deliver verification code: %w", err)
		}
	}

	return payment, nil
}

func (s *PrenosService) VerifyPrenos(paymentID uint, verificationCode string) (*models.Payment, error) {
	payment, err := s.paymentRepo.FindByID(paymentID)
	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}
	if payment.SifraPlacanja != prenosPaymentCode {
		return nil, fmt.Errorf("prenos not found")
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
		s.cancelPrenos(payment)
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
			s.cancelPrenos(payment)
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
		s.cancelPrenos(payment)
		return nil, fmt.Errorf("receiver account not found: %w", err)
	}
	if err := validatePrenosAccounts(sender, receiver); err != nil {
		s.cancelPrenos(payment)
		return nil, &PaymentVerificationError{
			Code:    "unsupported_prenos_accounts",
			Message: err.Error(),
			Status:  paymentStatusCancelled,
		}
	}

	if sender.RaspolozivoStanje < payment.Iznos {
		s.cancelPrenos(payment)
		return nil, &PaymentVerificationError{
			Code:    "insufficient_balance",
			Message: fmt.Sprintf("insufficient balance: available %.2f, required %.2f", sender.RaspolozivoStanje, payment.Iznos),
			Status:  paymentStatusCancelled,
		}
	}
	if sender.DnevnaPotrosnja+payment.Iznos > sender.DnevniLimit {
		s.cancelPrenos(payment)
		return nil, &PaymentVerificationError{
			Code:    "daily_limit_exceeded",
			Message: "daily spending limit exceeded",
			Status:  paymentStatusCancelled,
		}
	}
	if sender.MesecnaPotrosnja+payment.Iznos > sender.MesecniLimit {
		s.cancelPrenos(payment)
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

func (s *PrenosService) findReceiverAccount(payment *models.Payment) (*models.Account, error) {
	if payment.RacunPrimaocaID != nil {
		return s.accountRepo.FindByID(*payment.RacunPrimaocaID)
	}
	return s.accountRepo.FindByBrojRacuna(payment.RacunPrimaocaBroj)
}

func (s *PrenosService) cancelPrenos(payment *models.Payment) {
	payment.Status = paymentStatusCancelled
	payment.VerifikacioniKod = ""
	payment.VerificationExpiresAt = nil
	payment.VremeTransakcije = time.Now().UTC()
	_ = s.paymentRepo.Save(payment)
}

func validatePrenosAccounts(sender, receiver *models.Account) error {
	if sender.Status != "" && sender.Status != "aktivan" {
		return fmt.Errorf("sender account is not active")
	}
	if receiver.Status != "" && receiver.Status != "aktivan" {
		return fmt.Errorf("receiver account is not active")
	}
	if sender.ClientID == nil || receiver.ClientID == nil {
		return fmt.Errorf("both accounts must belong to clients")
	}
	if *sender.ClientID == *receiver.ClientID {
		return fmt.Errorf("prenos requires accounts owned by different clients")
	}
	if sender.CurrencyID != 0 && receiver.CurrencyID != 0 && sender.CurrencyID != receiver.CurrencyID {
		return fmt.Errorf("prenos is supported only for same-currency accounts")
	}
	if sender.CurrencyKod != "" && receiver.CurrencyKod != "" && sender.CurrencyKod != receiver.CurrencyKod {
		return fmt.Errorf("prenos is supported only for same-currency accounts")
	}
	return nil
}
