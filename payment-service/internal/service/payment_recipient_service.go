package service

import (
	"fmt"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/util"
)

// PaymentRecipientRepositoryInterface defined here to avoid circular imports.
type PaymentRecipientRepositoryInterface interface {
	Create(r *models.PaymentRecipient) error
	FindByID(id uint) (*models.PaymentRecipient, error)
	ListByClientID(clientID uint) ([]models.PaymentRecipient, error)
	Update(r *models.PaymentRecipient) error
	Delete(id uint) error
}

type CreateRecipientInput struct {
	ClientID   uint
	Naziv      string
	BrojRacuna string
}

type UpdateRecipientInput struct {
	Naziv      string
	BrojRacuna string
}

type PaymentRecipientService struct {
	repo PaymentRecipientRepositoryInterface
}

func NewPaymentRecipientServiceWithRepo(repo PaymentRecipientRepositoryInterface) *PaymentRecipientService {
	return &PaymentRecipientService{repo: repo}
}

func (s *PaymentRecipientService) CreateRecipient(input CreateRecipientInput) (*models.PaymentRecipient, error) {
	if input.Naziv == "" {
		return nil, fmt.Errorf("recipient name is required")
	}
	if !util.ValidateAccountNumber(input.BrojRacuna) {
		return nil, fmt.Errorf("invalid account number format: %s", input.BrojRacuna)
	}

	recipient := &models.PaymentRecipient{
		ClientID:   input.ClientID,
		Naziv:      input.Naziv,
		BrojRacuna: input.BrojRacuna,
	}

	if err := s.repo.Create(recipient); err != nil {
		return nil, fmt.Errorf("failed to create recipient: %w", err)
	}

	return recipient, nil
}

func (s *PaymentRecipientService) ListRecipientsByClient(clientID uint) ([]models.PaymentRecipient, error) {
	return s.repo.ListByClientID(clientID)
}

func (s *PaymentRecipientService) UpdateRecipient(id, clientID uint, input UpdateRecipientInput) (*models.PaymentRecipient, error) {
	recipient, err := s.repo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("recipient not found: %w", err)
	}

	if recipient.ClientID != clientID {
		return nil, fmt.Errorf("access denied: recipient does not belong to client %d", clientID)
	}

	if input.Naziv != "" {
		recipient.Naziv = input.Naziv
	}
	if input.BrojRacuna != "" {
		if !util.ValidateAccountNumber(input.BrojRacuna) {
			return nil, fmt.Errorf("invalid account number format: %s", input.BrojRacuna)
		}
		recipient.BrojRacuna = input.BrojRacuna
	}

	if err := s.repo.Update(recipient); err != nil {
		return nil, fmt.Errorf("failed to update recipient: %w", err)
	}

	return recipient, nil
}

func (s *PaymentRecipientService) DeleteRecipient(id, clientID uint) error {
	recipient, err := s.repo.FindByID(id)
	if err != nil {
		return fmt.Errorf("recipient not found: %w", err)
	}

	if recipient.ClientID != clientID {
		return fmt.Errorf("access denied: recipient does not belong to client %d", clientID)
	}

	return s.repo.Delete(id)
}
