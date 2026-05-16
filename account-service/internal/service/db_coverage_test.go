package service_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/service"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newAcctSvcTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestNewCardServiceWithDB_Constructs(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_card_svc_ctor")
	if s := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db); s == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestNewNotificationService_Constructs(t *testing.T) {
	if n := service.NewNotificationService(&config.Config{}); n == nil {
		t.Fatal("expected non-nil notif")
	}
}

func TestNotificationService_SendEmails_BadSMTP_ReturnsError(t *testing.T) {
	n := service.NewNotificationService(&config.Config{SMTPHost: "localhost", SMTPPort: 1, SMTPFrom: "n@b.com"})
	if err := n.SendAccountCreatedEmail("x@y.com", "X", "ACC", "tekuci", "RSD"); err == nil {
		t.Fatal("expected SMTP error")
	}
	if err := n.SendCardCreatedEmail("x@y.com", "X", "1234", "visa"); err == nil {
		t.Fatal("expected SMTP error")
	}
	if err := n.SendCardStatusEmail("x@y.com", "X", "1234", "visa", "blokirana"); err == nil {
		t.Fatal("expected SMTP error")
	}
	if err := n.SendCardVerificationEmail("x@y.com", "X", "123456"); err == nil {
		t.Fatal("expected SMTP error")
	}
	if err := n.SendCardRequestResultEmail("x@y.com", "X", true, ""); err == nil {
		t.Fatal("expected SMTP error")
	}
}

func seedAcctSvcAccount(t *testing.T, db *gorm.DB, clientID, currencyID uint, vrsta string) *models.Account {
	t.Helper()
	cid := clientID
	acc := &models.Account{
		BrojRacuna: fmt.Sprintf("AC-%d-%d-%d", clientID, currencyID, time.Now().UnixNano()),
		ClientID:   &cid, CurrencyID: currencyID, Vrsta: vrsta, Tip: "tekuci",
		Status: "aktivan",
	}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("seed acct: %v", err)
	}
	return acc
}

func TestGetCard_Found(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_card_get")
	clientID := uint(1)
	acc := seedAcctSvcAccount(t, db, clientID, 1, "licni")
	card := &models.Card{
		BrojKartice: "4111111111111111", VrstaKartice: "visa", AccountID: acc.ID, ClientID: clientID,
		Status: "aktivna", DatumKreiranja: time.Now(),
	}
	if err := db.Create(card).Error; err != nil {
		t.Fatalf("seed card: %v", err)
	}
	svc := service.NewCardService(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil)
	got, err := svc.GetCard(card.ID)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got == nil || got.ID != card.ID {
		t.Fatalf("expected card, got %+v", got)
	}
}

func TestRequestCardClient_NoDB_ReturnsError(t *testing.T) {
	svc := service.NewCardService(&mockCardRepo{}, &mockAcctRepoForCard{}, nil)
	_, err := svc.RequestCardClient(service.ClientCardRequestInput{VrstaKartice: "visa"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRequestCardClient_InvalidVrsta(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_req_invalid_vrsta")
	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db)
	_, err := svc.RequestCardClient(service.ClientCardRequestInput{VrstaKartice: "bogus", ClientID: 1, AccountID: 1})
	if err == nil {
		t.Fatal("expected error for invalid vrsta")
	}
}

func TestRequestCardClient_LicniSuccess(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_req_licni_ok")
	clientID := uint(1)
	acc := seedAcctSvcAccount(t, db, clientID, 1, "licni")

	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db)
	req, err := svc.RequestCardClient(service.ClientCardRequestInput{
		AccountID: acc.ID, ClientID: clientID,
		VrstaKartice: "visa", ClientEmail: "c@b.com", ClientName: "C",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if req == nil || req.VerifikacioniKod == "" {
		t.Fatal("expected request with verification code")
	}
}

func TestRequestCardClient_NotOwned(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_req_notowned")
	owner := uint(1)
	other := uint(99)
	acc := seedAcctSvcAccount(t, db, owner, 1, "licni")
	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db)
	_, err := svc.RequestCardClient(service.ClientCardRequestInput{
		AccountID: acc.ID, ClientID: other, VrstaKartice: "visa",
	})
	if err == nil {
		t.Fatal("expected error for non-owner")
	}
}

func TestRequestCardClient_LicniOverLimit(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_req_over_limit")
	clientID := uint(1)
	acc := seedAcctSvcAccount(t, db, clientID, 1, "licni")
	for i := 0; i < 2; i++ {
		db.Create(&models.Card{
			BrojKartice: fmt.Sprintf("411111111111000%d", i), VrstaKartice: "visa",
			AccountID: acc.ID, ClientID: clientID, Status: "aktivna",
		})
	}
	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db)
	_, err := svc.RequestCardClient(service.ClientCardRequestInput{
		AccountID: acc.ID, ClientID: clientID, VrstaKartice: "visa",
	})
	if !errors.Is(err, service.ErrCardLimitExceeded) {
		t.Fatalf("expected ErrCardLimitExceeded, got %v", err)
	}
}

func TestVerifyCardRequest_Success(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_verify_ok")
	clientID := uint(1)
	acc := seedAcctSvcAccount(t, db, clientID, 1, "licni")

	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db)
	req, err := svc.RequestCardClient(service.ClientCardRequestInput{
		AccountID: acc.ID, ClientID: clientID, VrstaKartice: "visa", ClientEmail: "c@b.com", ClientName: "C",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	card, err := svc.VerifyCardRequest(req.ID, req.VerifikacioniKod)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if card == nil {
		t.Fatal("expected card")
	}
}

func TestVerifyCardRequest_NotFound(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_verify_notfound")
	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db)
	if _, err := svc.VerifyCardRequest(99999, "x"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestVerifyCardRequest_WrongCode(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_verify_wrong")
	clientID := uint(1)
	acc := seedAcctSvcAccount(t, db, clientID, 1, "licni")
	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db)
	req, _ := svc.RequestCardClient(service.ClientCardRequestInput{
		AccountID: acc.ID, ClientID: clientID, VrstaKartice: "visa", ClientEmail: "c@b.com", ClientName: "C",
	})
	if _, err := svc.VerifyCardRequest(req.ID, "wrong"); err == nil {
		t.Fatal("expected wrong-code error")
	}
}

func TestVerifyCardRequest_Expired(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_verify_expired")
	clientID := uint(1)
	seedAcctSvcAccount(t, db, clientID, 1, "licni")
	// Create an expired pending request directly.
	req := &models.CardRequest{
		AccountID: 1, ClientID: clientID, VrstaKartice: "visa",
		VerifikacioniKod: "111111", Status: "pending",
		ExpiresAt: time.Now().Add(-time.Minute), ClientEmail: "c@b.com",
	}
	if err := db.Create(req).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db)
	if _, err := svc.VerifyCardRequest(req.ID, "111111"); err == nil {
		t.Fatal("expected expired error")
	}
}

func TestUpdateAccountName_Empty_ReturnsError(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_name_empty")
	svc := service.NewAccountServiceWithRepos(repository.NewAccountRepository(db), repository.NewCurrencyRepository(db), nil)
	if err := svc.UpdateAccountName(1, ""); err == nil {
		t.Fatal("expected empty-name error")
	}
}

func TestUpdateAccountName_NotFound(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_name_notfound")
	svc := service.NewAccountServiceWithRepos(repository.NewAccountRepository(db), repository.NewCurrencyRepository(db), nil)
	if err := svc.UpdateAccountName(99999, "X"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestUpdateAccountName_Duplicate(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_name_dup")
	clientID := uint(1)
	a1 := seedAcctSvcAccount(t, db, clientID, 1, "licni")
	a2 := seedAcctSvcAccount(t, db, clientID, 1, "licni")
	accRepo := repository.NewAccountRepository(db)
	currRepo := repository.NewCurrencyRepository(db)
	svc := service.NewAccountServiceWithRepos(accRepo, currRepo, nil)
	if err := svc.UpdateAccountName(a1.ID, "Same"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := svc.UpdateAccountName(a2.ID, "Same"); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestUpdateAccountLimits_NegativeDnevni(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_lim_negd")
	svc := service.NewAccountServiceWithRepos(repository.NewAccountRepository(db), repository.NewCurrencyRepository(db), nil)
	if err := svc.UpdateAccountLimits(1, 1, -1, 100); err == nil {
		t.Fatal("expected negative-limit error")
	}
}

func TestUpdateAccountLimits_NotFound(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_lim_notfound")
	svc := service.NewAccountServiceWithRepos(repository.NewAccountRepository(db), repository.NewCurrencyRepository(db), nil)
	if err := svc.UpdateAccountLimits(99999, 1, 100, 100); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestUpdateAccountLimits_NotOwner(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_lim_notowner")
	clientID := uint(1)
	acc := seedAcctSvcAccount(t, db, clientID, 1, "licni")
	svc := service.NewAccountServiceWithRepos(repository.NewAccountRepository(db), repository.NewCurrencyRepository(db), nil)
	if err := svc.UpdateAccountLimits(acc.ID, 999, 100, 100); err == nil {
		t.Fatal("expected not-owner error")
	}
}

func TestCreateAccount_Devizni_RejectsRSD(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_create_devizni_rsd")
	rsdCur := &models.Currency{Kod: "RSD"}
	db.Create(rsdCur)
	svc := service.NewAccountServiceWithRepos(repository.NewAccountRepository(db), repository.NewCurrencyRepository(db), nil)
	_, err := svc.CreateAccount(service.CreateAccountInput{
		Tip: "devizni", Vrsta: "licni", CurrencyID: rsdCur.ID, ClientID: uintPtr(1),
	})
	if err == nil {
		t.Fatal("expected RSD-rejection error")
	}
}

func TestCreateAccount_PoslovniRequiresFirma(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_create_poslovni_no_firma")
	rsdCur := &models.Currency{Kod: "RSD"}
	db.Create(rsdCur)
	svc := service.NewAccountServiceWithRepos(repository.NewAccountRepository(db), repository.NewCurrencyRepository(db), nil)
	_, err := svc.CreateAccount(service.CreateAccountInput{
		Tip: "tekuci", Vrsta: "poslovni", CurrencyID: rsdCur.ID,
	})
	if err == nil {
		t.Fatal("expected error for poslovni without firma")
	}
}

func TestCreateAccount_BadCurrency(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_create_bad_curr")
	svc := service.NewAccountServiceWithRepos(repository.NewAccountRepository(db), repository.NewCurrencyRepository(db), nil)
	_, err := svc.CreateAccount(service.CreateAccountInput{
		Tip: "tekuci", Vrsta: "licni", CurrencyID: 99999,
	})
	if err == nil {
		t.Fatal("expected currency-not-found error")
	}
}

func TestUpdateAccountName_Likely(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_update_name")
	clientID := uint(1)
	acc := seedAcctSvcAccount(t, db, clientID, 1, "licni")
	accRepo := repository.NewAccountRepository(db)
	currRepo := repository.NewCurrencyRepository(db)
	svc := service.NewAccountServiceWithRepos(accRepo, currRepo, nil)

	if err := svc.UpdateAccountName(acc.ID, "New Name"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestSendCardStatusEmail_AllActions(t *testing.T) {
	n := service.NewNotificationService(&config.Config{SMTPHost: "localhost", SMTPPort: 1, SMTPFrom: "n@b.com"})
	for _, a := range []string{"blokirana", "aktivna", "deaktivirana", "other"} {
		_ = n.SendCardStatusEmail("x@y.com", "X", "4111111111111111", "visa", a)
	}
}

func TestSendCardRequestResultEmail_FailureBranch(t *testing.T) {
	n := service.NewNotificationService(&config.Config{SMTPHost: "localhost", SMTPPort: 1, SMTPFrom: "n@b.com"})
	if err := n.SendCardRequestResultEmail("x@y.com", "X", false, "limit exceeded"); err == nil {
		t.Fatal("expected SMTP error")
	}
}

func TestVerifyCardRequest_NotPending(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_verify_notpending")
	req := &models.CardRequest{
		AccountID: 1, ClientID: 1, VrstaKartice: "visa",
		VerifikacioniKod: "1", Status: "verified", ExpiresAt: time.Now().Add(time.Hour),
	}
	db.Create(req)
	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db)
	if _, err := svc.VerifyCardRequest(req.ID, "1"); err == nil {
		t.Fatal("expected not-pending error")
	}
}

func TestVerifyCardRequest_TooManyAttempts(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_verify_toomany")
	req := &models.CardRequest{
		AccountID: 1, ClientID: 1, VrstaKartice: "visa",
		VerifikacioniKod: "1", Status: "pending", BrojPokusaja: 3,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	db.Create(req)
	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db)
	if _, err := svc.VerifyCardRequest(req.ID, "anything"); err == nil {
		t.Fatal("expected too-many-attempts error")
	}
}

func TestVerifyCardRequest_WrongCodeExhaustsAttempts(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_verify_exhaust")
	req := &models.CardRequest{
		AccountID: 1, ClientID: 1, VrstaKartice: "visa",
		VerifikacioniKod: "111111", Status: "pending", BrojPokusaja: 2, // one wrong attempt away
		ExpiresAt: time.Now().Add(time.Hour),
	}
	db.Create(req)
	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), nil, db)
	if _, err := svc.VerifyCardRequest(req.ID, "wrong!"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLookupNotifyFromDB_WithClientAndOvlasceno(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_lookup_client_ol")
	clientID := uint(42)
	if err := db.Create(&models.Client{ID: clientID, Email: "c@b.com", Ime: "I", Prezime: "P"}).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	acc := seedAcctSvcAccount(t, db, clientID, 1, "poslovni")
	ol := models.OvlascenoLice{Ime: "O", Prezime: "L", Email: "ol@b.com", BrojTelefona: "1", AccountID: acc.ID}
	if err := db.Create(&ol).Error; err != nil {
		t.Fatalf("seed ol: %v", err)
	}
	card := &models.Card{
		BrojKartice: "4111111111119999", VrstaKartice: "visa", AccountID: acc.ID, ClientID: clientID,
		OvlascenoLiceID: &ol.ID, Status: "aktivna", DatumKreiranja: time.Now(),
	}
	db.Create(card)

	cfg := &config.Config{SMTPHost: "localhost", SMTPPort: 1, SMTPFrom: "n@b.com"}
	notif := service.NewNotificationService(cfg)
	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), notif, db)
	_, _ = svc.DeactivateCardWithNotify(card.ID, nil)
}

func TestLookupNotifyFromDB_NoClient_ReturnsNil(t *testing.T) {
	db := newAcctSvcTestDB(t, "acct_lookup_noclient")
	clientID := uint(99999) // not seeded
	card := &models.Card{
		BrojKartice: "4111111111110000", VrstaKartice: "visa", AccountID: 1, ClientID: clientID,
		Status: "blokirana", DatumKreiranja: time.Now(),
	}
	db.Create(card)
	cfg := &config.Config{SMTPHost: "localhost", SMTPPort: 1, SMTPFrom: "n@b.com"}
	notif := service.NewNotificationService(cfg)
	svc := service.NewCardServiceWithDB(repository.NewCardRepository(db), repository.NewAccountRepository(db), notif, db)
	// Trigger sendCardStatusNotification via Block→error or via DeactivateCardWithNotify with nil notify.
	// Trigger directly: nil notify → tries to lookup, fails because no client row → returns nil → no email sent.
	// We call DeactivateCardWithNotify which routes through sendCardStatusNotification.
	_, _ = svc.DeactivateCardWithNotify(card.ID, nil)
	// No assertion: just exercise the lookupNotifyFromDB code path.
}
