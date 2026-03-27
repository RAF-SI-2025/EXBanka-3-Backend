package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
	"gorm.io/gorm"
)

// ErrCardLimitExceeded is returned when card creation would exceed the allowed limit.
var ErrCardLimitExceeded = errors.New("card limit exceeded")

// CardRepositoryInterface allows mocking in tests.
type CardRepositoryInterface interface {
	Create(card *models.Card) error
	FindByID(id uint) (*models.Card, error)
	CountByAccountID(accountID uint) (int64, error)
	CountByClientAndAccount(clientID, accountID uint) (int64, error)
	CountByOvlascenoLice(ovlascenoLiceID uint) (int64, error)
	ListByAccountID(accountID uint) ([]models.Card, error)
	ListByClientID(clientID uint) ([]models.Card, error)
	Save(card *models.Card) error
}

// CreateCardInput carries the data needed to issue a new card.
type CreateCardInput struct {
	AccountID       uint
	ClientID        uint
	OvlascenoLiceID *uint // set when card is issued to an authorized person on a poslovni account
	VrstaKartice    string // visa, mastercard, dinacard, amex
	NazivKartice    string
	ClientEmail     string
	ClientName      string
}

// CardService handles card creation and status management.
type CardService struct {
	cardRepo    CardRepositoryInterface
	accountRepo AccountRepositoryInterface
	notifSvc    *NotificationService
	db          *gorm.DB
}

func NewCardService(
	cardRepo CardRepositoryInterface,
	accountRepo AccountRepositoryInterface,
	notifSvc *NotificationService,
) *CardService {
	return &CardService{cardRepo: cardRepo, accountRepo: accountRepo, notifSvc: notifSvc}
}

func NewCardServiceWithDB(
	cardRepo CardRepositoryInterface,
	accountRepo AccountRepositoryInterface,
	notifSvc *NotificationService,
	db *gorm.DB,
) *CardService {
	return &CardService{cardRepo: cardRepo, accountRepo: accountRepo, notifSvc: notifSvc, db: db}
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
		if input.OvlascenoLiceID != nil {
			// Card for authorized person: max 1 card per OvlascenoLice
			count, err := s.cardRepo.CountByOvlascenoLice(*input.OvlascenoLiceID)
			if err != nil {
				return nil, fmt.Errorf("failed to count cards: %w", err)
			}
			if count >= 1 {
				return nil, fmt.Errorf("%w: ovlasceno lice already has a card on this account", ErrCardLimitExceeded)
			}
		} else {
			// Card for account owner: max 1 card per owner on this account
			count, err := s.cardRepo.CountByClientAndAccount(input.ClientID, input.AccountID)
			if err != nil {
				return nil, fmt.Errorf("failed to count cards: %w", err)
			}
			if count >= 1 {
				return nil, fmt.Errorf("%w: poslovni account allows max 1 card per person", ErrCardLimitExceeded)
			}
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
		BrojKartice:     util.GenerateCardNumber(input.VrstaKartice),
		CVV:             util.GenerateCVV(),
		VrstaKartice:    input.VrstaKartice,
		NazivKartice:    input.NazivKartice,
		AccountID:       input.AccountID,
		ClientID:        input.ClientID,
		OvlascenoLiceID: input.OvlascenoLiceID,
		Status:          "aktivna",
		DatumKreiranja:  now,
		DatumIsteka:     now.AddDate(5, 0, 0),
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

// CardStatusNotifyInfo carries optional email info for sending card status notifications.
type CardStatusNotifyInfo struct {
	ClientEmail        string
	ClientName         string
	OvlascenoLiceEmail string // for poslovni accounts
	OvlascenoLiceName  string
}

// BlockCard allows a client to block their own active card.
func (s *CardService) BlockCard(cardID, clientID uint) (*models.Card, error) {
	return s.BlockCardWithNotify(cardID, clientID, nil)
}

// BlockCardWithNotify blocks a card and optionally sends email notifications.
func (s *CardService) BlockCardWithNotify(cardID, clientID uint, notify *CardStatusNotifyInfo) (*models.Card, error) {
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
	s.sendCardStatusNotification(card, "blokirana", notify)
	return card, nil
}

// UnblockCard allows an employee to unblock a blocked card.
func (s *CardService) UnblockCard(cardID uint) (*models.Card, error) {
	return s.UnblockCardWithNotify(cardID, nil)
}

// UnblockCardWithNotify unblocks a card and optionally sends email notifications.
func (s *CardService) UnblockCardWithNotify(cardID uint, notify *CardStatusNotifyInfo) (*models.Card, error) {
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
	s.sendCardStatusNotification(card, "aktivna", notify)
	return card, nil
}

// DeactivateCard permanently deactivates a card (employee action, irreversible).
func (s *CardService) DeactivateCard(cardID uint) (*models.Card, error) {
	return s.DeactivateCardWithNotify(cardID, nil)
}

// DeactivateCardWithNotify deactivates a card and optionally sends email notifications.
func (s *CardService) DeactivateCardWithNotify(cardID uint, notify *CardStatusNotifyInfo) (*models.Card, error) {
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
	s.sendCardStatusNotification(card, "deaktivirana", notify)
	return card, nil
}

// sendCardStatusNotification sends email to the card owner and optionally to OvlascenoLice.
// If notify is nil or has no ClientEmail, it auto-resolves contact info from the DB.
func (s *CardService) sendCardStatusNotification(card *models.Card, newStatus string, notify *CardStatusNotifyInfo) {
	if s.notifSvc == nil {
		return
	}
	if notify == nil || notify.ClientEmail == "" {
		notify = s.lookupNotifyFromDB(card)
	}
	if notify == nil {
		return
	}
	if notify.ClientEmail != "" {
		_ = s.notifSvc.SendCardStatusEmail(notify.ClientEmail, notify.ClientName, card.BrojKartice, card.VrstaKartice, newStatus)
	}
	if notify.OvlascenoLiceEmail != "" {
		_ = s.notifSvc.SendCardStatusEmail(notify.OvlascenoLiceEmail, notify.OvlascenoLiceName, card.BrojKartice, card.VrstaKartice, newStatus)
	}
}

// lookupNotifyFromDB resolves client and ovlasceno lice contact info from the database.
func (s *CardService) lookupNotifyFromDB(card *models.Card) *CardStatusNotifyInfo {
	if s.db == nil {
		return nil
	}
	var client struct {
		Email   string
		Ime     string
		Prezime string
	}
	if err := s.db.Table("clients").Select("email, ime, prezime").Where("id = ?", card.ClientID).First(&client).Error; err != nil {
		return nil
	}
	info := &CardStatusNotifyInfo{
		ClientEmail: client.Email,
		ClientName:  strings.TrimSpace(client.Ime + " " + client.Prezime),
	}
	if card.OvlascenoLiceID != nil {
		var ol models.OvlascenoLice
		if err := s.db.First(&ol, *card.OvlascenoLiceID).Error; err == nil {
			info.OvlascenoLiceEmail = ol.Email
			info.OvlascenoLiceName = strings.TrimSpace(ol.Ime + " " + ol.Prezime)
		}
	}
	return info
}

// ClientCardRequestInput carries the data for a client-initiated card request.
type ClientCardRequestInput struct {
	AccountID    uint
	ClientID     uint
	VrstaKartice string
	NazivKartice string
	ClientEmail  string
	ClientName   string
	// For poslovni: ovlasceno lice info (creates new OvlascenoLice)
	OvlascenoIme          string
	OvlascenoPrezime      string
	OvlascenoEmail        string
	OvlascenoBrojTelefona string
}

// RequestCardClient initiates a client card request: validates limits, generates
// a 6-digit verification code, sends it via email, and stores the pending request.
func (s *CardService) RequestCardClient(input ClientCardRequestInput) (*models.CardRequest, error) {
	if s.db == nil {
		return nil, errors.New("database not available for card requests")
	}
	if !containsStr(models.ValidCardTypes(), input.VrstaKartice) {
		return nil, fmt.Errorf("invalid vrsta kartice: %s", input.VrstaKartice)
	}

	account, err := s.accountRepo.FindByID(input.AccountID)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}
	if account.ClientID == nil || *account.ClientID != input.ClientID {
		return nil, errors.New("account does not belong to this client")
	}

	// Check card limits
	if account.Vrsta == "poslovni" {
		// For poslovni: max 1 card per person — new OvlascenoLice each time
		// Check if owner already has a card (if requesting for self)
		if input.OvlascenoIme == "" {
			count, err := s.cardRepo.CountByClientAndAccount(input.ClientID, input.AccountID)
			if err != nil {
				return nil, fmt.Errorf("failed to count cards: %w", err)
			}
			if count >= 1 {
				return nil, fmt.Errorf("%w: already have a card on this poslovni account", ErrCardLimitExceeded)
			}
		}
	} else {
		count, err := s.cardRepo.CountByAccountID(input.AccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to count cards: %w", err)
		}
		if count >= 2 {
			return nil, fmt.Errorf("%w: max 2 cards per licni account", ErrCardLimitExceeded)
		}
	}

	// Generate 6-digit code
	code := generateVerificationCode()

	req := &models.CardRequest{
		AccountID:             input.AccountID,
		ClientID:              input.ClientID,
		VrstaKartice:          input.VrstaKartice,
		NazivKartice:          input.NazivKartice,
		ClientEmail:           input.ClientEmail,
		ClientName:            input.ClientName,
		VerifikacioniKod:      code,
		ExpiresAt:             time.Now().Add(5 * time.Minute),
		Status:                "pending",
		OvlascenoIme:          input.OvlascenoIme,
		OvlascenoPrezime:      input.OvlascenoPrezime,
		OvlascenoEmail:        input.OvlascenoEmail,
		OvlascenoBrojTelefona: input.OvlascenoBrojTelefona,
	}

	if err := s.db.Create(req).Error; err != nil {
		return nil, fmt.Errorf("failed to save card request: %w", err)
	}

	// Send verification email
	if s.notifSvc != nil {
		_ = s.notifSvc.SendCardVerificationEmail(input.ClientEmail, input.ClientName, code)
	}

	return req, nil
}

// VerifyCardRequest checks the code, creates the card, and sends result notification.
func (s *CardService) VerifyCardRequest(requestID uint, code string) (*models.Card, error) {
	if s.db == nil {
		return nil, errors.New("database not available for card requests")
	}

	var req models.CardRequest
	if err := s.db.First(&req, requestID).Error; err != nil {
		return nil, errors.New("card request not found")
	}
	if req.Status != "pending" {
		return nil, fmt.Errorf("card request already %s", req.Status)
	}
	if time.Now().After(req.ExpiresAt) {
		s.db.Model(&req).Update("status", "expired")
		return nil, errors.New("verification code expired")
	}
	if req.BrojPokusaja >= 3 {
		s.db.Model(&req).Update("status", "failed")
		return nil, errors.New("too many failed attempts")
	}

	if req.VerifikacioniKod != code {
		s.db.Model(&req).Updates(map[string]interface{}{
			"broj_pokusaja": req.BrojPokusaja + 1,
		})
		remaining := 2 - req.BrojPokusaja
		if remaining <= 0 {
			s.db.Model(&req).Update("status", "failed")
			return nil, errors.New("incorrect code, no more attempts remaining")
		}
		return nil, fmt.Errorf("incorrect code, %d attempts remaining", remaining)
	}

	// Code correct — create the card
	createInput := CreateCardInput{
		AccountID:    req.AccountID,
		ClientID:     req.ClientID,
		VrstaKartice: req.VrstaKartice,
		NazivKartice: req.NazivKartice,
		ClientEmail:  req.ClientEmail,
		ClientName:   req.ClientName,
	}

	// For poslovni with OvlascenoLice: create OvlascenoLice first, then pass its ID
	if req.OvlascenoIme != "" {
		account, _ := s.accountRepo.FindByID(req.AccountID)
		if account != nil && account.Vrsta == "poslovni" {
			ol := models.OvlascenoLice{
				Ime:          req.OvlascenoIme,
				Prezime:      req.OvlascenoPrezime,
				Email:        req.OvlascenoEmail,
				BrojTelefona: req.OvlascenoBrojTelefona,
				AccountID:    req.AccountID,
			}
			if account.FirmaID != nil {
				ol.FirmaID = *account.FirmaID
			}
			s.db.Create(&ol)
			if ol.ID != 0 {
				createInput.OvlascenoLiceID = &ol.ID
			}
		}
	}

	card, err := s.CreateCard(createInput)
	if err != nil {
		s.db.Model(&req).Update("status", "failed")
		// Notify failure
		if s.notifSvc != nil {
			_ = s.notifSvc.SendCardRequestResultEmail(req.ClientEmail, req.ClientName, false, err.Error())
		}
		return nil, fmt.Errorf("card creation failed: %w", err)
	}

	s.db.Model(&req).Update("status", "verified")

	// Notify success
	if s.notifSvc != nil {
		_ = s.notifSvc.SendCardRequestResultEmail(req.ClientEmail, req.ClientName, true, "")
	}

	return card, nil
}

func generateVerificationCode() string {
	b := make([]byte, 4)
	rand.Read(b)
	n := (int(b[0])<<16 | int(b[1])<<8 | int(b[2])) % 1000000
	return fmt.Sprintf("%06d", n)
}
