package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
)

// ErrCardLimitExceeded is returned when card creation would exceed the allowed limit.
var ErrCardLimitExceeded = errors.New("card limit exceeded")

// CardRepositoryInterface allows mocking in tests.
type CardRepositoryInterface interface {
	Create(card *models.Card) error
	FindByID(id uint) (*models.Card, error)
	CountByAccountID(accountID uint) (int64, error)
	CountByClientAndAccount(clientID, accountID uint) (int64, error)
	ListByAccountID(accountID uint) ([]models.Card, error)
	ListByClientID(clientID uint) ([]models.Card, error)
	Save(card *models.Card) error
}

// CreateCardInput carries the data needed to issue a new card.
type CreateCardInput struct {
	AccountID    uint
	ClientID     uint
	VrstaKartice string // visa, mastercard, dinacard, amex
	NazivKartice string
	ClientEmail  string
	ClientName   string
}

// CardService handles card creation and status management.
type CardService struct {
	cardRepo    CardRepositoryInterface
	accountRepo AccountRepositoryInterface
	notifSvc    *NotificationService
}

func NewCardService(
	cardRepo CardRepositoryInterface,
	accountRepo AccountRepositoryInterface,
	notifSvc *NotificationService,
) *CardService {
	return &CardService{cardRepo: cardRepo, accountRepo: accountRepo, notifSvc: notifSvc}
}

func containsStr(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// CreateCard issues a new card for the given account, enforcing per-account limits.
//
//   - Lični račun:   max 2 cards total on the account
//   - Poslovni račun: max 1 card per client on the account
func (s *CardService) CreateCard(input CreateCardInput) (*models.Card, error) {
	if !containsStr(models.ValidCardTypes(), input.VrstaKartice) {
		return nil, fmt.Errorf("invalid vrsta kartice: %s", input.VrstaKartice)
	}

	// Look up the account to determine its kind (licni/poslovni).
	account, err := s.accountRepo.FindByID(input.AccountID)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}
	if input.ClientID == 0 {
		return nil, fmt.Errorf("client id is required")
	}
	if account.Vrsta != "poslovni" {
		if account.ClientID == nil || *account.ClientID != input.ClientID {
			return nil, fmt.Errorf("licni account cards can only be issued to the account owner")
		}
	}

	// Enforce card limits.
	if account.Vrsta == "poslovni" {
		count, err := s.cardRepo.CountByClientAndAccount(input.ClientID, input.AccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to count cards: %w", err)
		}
		if count >= 1 {
			return nil, fmt.Errorf("%w: poslovni account allows max 1 card per person", ErrCardLimitExceeded)
		}
	} else {
		// licni (and anything else): max 2 per account
		count, err := s.cardRepo.CountByAccountID(input.AccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to count cards: %w", err)
		}
		if count >= 2 {
			return nil, fmt.Errorf("%w: licni account allows max 2 cards", ErrCardLimitExceeded)
		}
	}

	now := time.Now()
	card := &models.Card{
		BrojKartice:    util.GenerateCardNumber(input.VrstaKartice),
		CVV:            util.GenerateCVV(),
		VrstaKartice:   input.VrstaKartice,
		NazivKartice:   input.NazivKartice,
		AccountID:      input.AccountID,
		ClientID:       input.ClientID,
		Status:         "aktivna",
		DatumKreiranja: now,
		DatumIsteka:    now.AddDate(5, 0, 0),
	}
	if account.Vrsta == "poslovni" && account.ClientID != nil && *account.ClientID != input.ClientID {
		card.OvlascenoLiceID = &input.ClientID
	}

	if err := s.cardRepo.Create(card); err != nil {
		return nil, fmt.Errorf("failed to create card: %w", err)
	}

	if s.notifSvc != nil && input.ClientEmail != "" {
		_ = s.notifSvc.SendCardCreatedEmail(input.ClientEmail, input.ClientName, card.BrojKartice, input.VrstaKartice)
	}

	return card, nil
}

// GetCard returns a single card by ID, or nil if not found.
func (s *CardService) GetCard(id uint) (*models.Card, error) {
	return s.cardRepo.FindByID(id)
}

// ListByAccount returns all cards for a given account.
func (s *CardService) ListByAccount(accountID uint) ([]models.Card, error) {
	cards, err := s.cardRepo.ListByAccountID(accountID)
	if err != nil {
		return nil, err
	}
	if cards == nil {
		return []models.Card{}, nil
	}
	return cards, nil
}

// ListByClient returns all cards belonging to a given client.
func (s *CardService) ListByClient(clientID uint) ([]models.Card, error) {
	cards, err := s.cardRepo.ListByClientID(clientID)
	if err != nil {
		return nil, err
	}
	if cards == nil {
		return []models.Card{}, nil
	}
	return cards, nil
}

// BlockCard allows a client to block their own active card.
func (s *CardService) BlockCard(cardID, clientID uint) (*models.Card, error) {
	card, err := s.cardRepo.FindByID(cardID)
	if err != nil {
		return nil, fmt.Errorf("card not found: %w", err)
	}
	if card == nil {
		return nil, errors.New("card not found")
	}
	if card.ClientID != clientID {
		return nil, errors.New("card does not belong to this client")
	}
	if card.Status != "aktivna" {
		return nil, fmt.Errorf("cannot block card with status %s: only aktivna cards can be blocked", card.Status)
	}
	card.Status = "blokirana"
	if err := s.cardRepo.Save(card); err != nil {
		return nil, fmt.Errorf("failed to save card: %w", err)
	}
	return card, nil
}

// UnblockCard allows an employee to unblock a blocked card.
func (s *CardService) UnblockCard(cardID uint) (*models.Card, error) {
	card, err := s.cardRepo.FindByID(cardID)
	if err != nil {
		return nil, fmt.Errorf("card not found: %w", err)
	}
	if card == nil {
		return nil, errors.New("card not found")
	}
	if card.Status != "blokirana" {
		return nil, fmt.Errorf("cannot unblock card with status %s: only blokirana cards can be unblocked", card.Status)
	}
	card.Status = "aktivna"
	if err := s.cardRepo.Save(card); err != nil {
		return nil, fmt.Errorf("failed to save card: %w", err)
	}
	return card, nil
}

// DeactivateCard permanently deactivates a card (employee action, irreversible).
func (s *CardService) DeactivateCard(cardID uint) (*models.Card, error) {
	card, err := s.cardRepo.FindByID(cardID)
	if err != nil {
		return nil, fmt.Errorf("card not found: %w", err)
	}
	if card == nil {
		return nil, errors.New("card not found")
	}
	if card.Status == "deaktivirana" {
		return nil, errors.New("card is already deactivated")
	}
	card.Status = "deaktivirana"
	if err := s.cardRepo.Save(card); err != nil {
		return nil, fmt.Errorf("failed to save card: %w", err)
	}
	return card, nil
}
