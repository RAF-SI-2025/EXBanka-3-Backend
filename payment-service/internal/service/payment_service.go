package service

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
)

// Repository interfaces defined here to avoid circular imports.

type PaymentAccountRepositoryInterface interface {
	FindByID(id uint) (*models.Account, error)
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

type CreatePaymentInput struct {
	RacunPosiljaocaID uint
	RacunPrimaocaBroj string
	Iznos             float64
	SifraPlacanja     string
	PozivNaBroj       string
	Svrha             string
	RecipientID       *uint
	// If set, creates a new saved recipient during payment
	AddRecipient      bool
	RecipientNaziv    string
}

type PaymentService struct {
	accountRepo   PaymentAccountRepositoryInterface
	paymentRepo   PaymentRepositoryInterface
	recipientRepo RecipientRepositoryInterface
}

func NewPaymentServiceWithRepos(
	accountRepo PaymentAccountRepositoryInterface,
	paymentRepo PaymentRepositoryInterface,
	recipientRepo RecipientRepositoryInterface,
) *PaymentService {
	return &PaymentService{
		accountRepo:   accountRepo,
		paymentRepo:   paymentRepo,
		recipientRepo: recipientRepo,
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

	if sender.RaspolozivoStanje < input.Iznos {
		return nil, fmt.Errorf("insufficient balance: available %.2f, requested %.2f",
			sender.RaspolozivoStanje, input.Iznos)
	}

	code := fmt.Sprintf("%06d", rand.Intn(1_000_000))

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
		RacunPosiljaocaID: input.RacunPosiljaocaID,
		RacunPrimaocaBroj: input.RacunPrimaocaBroj,
		Iznos:             input.Iznos,
		SifraPlacanja:     input.SifraPlacanja,
		PozivNaBroj:       input.PozivNaBroj,
		Svrha:             input.Svrha,
		Status:            "u_obradi",
		VerifikacioniKod:  code,
		RecipientID:       input.RecipientID,
		VremeTransakcije:  time.Now(),
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

	if payment.Status != "u_obradi" {
		return nil, fmt.Errorf("payment is not pending: status=%s", payment.Status)
	}

	if payment.VerifikacioniKod != verificationCode {
		return nil, fmt.Errorf("invalid verification code")
	}

	// Deduct amount from sender
	sender, err := s.accountRepo.FindByID(payment.RacunPosiljaocaID)
	if err != nil {
		return nil, fmt.Errorf("sender account not found: %w", err)
	}

	if err := s.accountRepo.UpdateFields(sender.ID, map[string]interface{}{
		"stanje":             sender.Stanje - payment.Iznos,
		"raspolozivo_stanje": sender.RaspolozivoStanje - payment.Iznos,
	}); err != nil {
		return nil, fmt.Errorf("failed to deduct balance: %w", err)
	}

	payment.Status = "uspesno"
	if err := s.paymentRepo.Save(payment); err != nil {
		return nil, fmt.Errorf("failed to update payment status: %w", err)
	}

	return payment, nil
}
