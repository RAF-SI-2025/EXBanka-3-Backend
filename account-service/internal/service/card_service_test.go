package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
)

// --- mock card repository ---

type mockCardRepo struct {
	saved             *models.Card
	countByAccount    int64
	countByClientAcct int64
}

func (m *mockCardRepo) Create(c *models.Card) error           { c.ID = 1; m.saved = c; return nil }
func (m *mockCardRepo) FindByID(_ uint) (*models.Card, error) { return m.saved, nil }
func (m *mockCardRepo) CountByAccountID(_ uint) (int64, error) {
	return m.countByAccount, nil
}
func (m *mockCardRepo) CountByClientAndAccount(_, _ uint) (int64, error) {
	return m.countByClientAcct, nil
}
func (m *mockCardRepo) ListByAccountID(_ uint) ([]models.Card, error) { return nil, nil }
func (m *mockCardRepo) ListByClientID(_ uint) ([]models.Card, error)  { return nil, nil }
func (m *mockCardRepo) Save(c *models.Card) error                     { m.saved = c; return nil }

// --- mock account repository (reuse AccountRepositoryInterface from service package) ---

type mockAcctRepoForCard struct {
	account *models.Account
}

func (m *mockAcctRepoForCard) Create(a *models.Account) error           { return nil }
func (m *mockAcctRepoForCard) FindByID(_ uint) (*models.Account, error) { return m.account, nil }
func (m *mockAcctRepoForCard) FindByBrojRacuna(_ string) (*models.Account, error) {
	return nil, nil
}
func (m *mockAcctRepoForCard) ListByClientID(_ uint) ([]models.Account, error) { return nil, nil }
func (m *mockAcctRepoForCard) ListAll(_ models.AccountFilter) ([]models.Account, int64, error) {
	return nil, 0, nil
}
func (m *mockAcctRepoForCard) UpdateFields(_ uint, _ map[string]interface{}) error { return nil }
func (m *mockAcctRepoForCard) ExistsByNameForClient(_ uint, _ string, _ uint) (bool, error) {
	return false, nil
}

func licniAccount() *models.Account {
	return &models.Account{ID: 1, Vrsta: "licni", ClientID: uintPtr(5)}
}

func poslovniAccount() *models.Account {
	return &models.Account{ID: 2, Vrsta: "poslovni", ClientID: uintPtr(5)}
}

func uintPtr(v uint) *uint { return &v }

func newCardSvc(card *mockCardRepo, acct *mockAcctRepoForCard) *service.CardService {
	return service.NewCardService(card, acct, nil)
}

// --- tests ---

func TestCreateCard_SetsStatusAktivna(t *testing.T) {
	cr := &mockCardRepo{}
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: licniAccount()})
	card, err := svc.CreateCard(service.CreateCardInput{
		AccountID: 1, ClientID: 5, VrstaKartice: "visa", NazivKartice: "Moja visa",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if card.Status != "aktivna" {
		t.Errorf("expected status=aktivna, got %s", card.Status)
	}
}

func TestCreateCard_GeneratesValidCardNumber(t *testing.T) {
	cr := &mockCardRepo{}
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: licniAccount()})
	card, err := svc.CreateCard(service.CreateCardInput{
		AccountID: 1, ClientID: 5, VrstaKartice: "visa",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(card.BrojKartice) != 16 {
		t.Errorf("expected 16-digit card number, got %d: %s", len(card.BrojKartice), card.BrojKartice)
	}
	if !util.ValidateLuhn(card.BrojKartice) {
		t.Errorf("generated card number %s fails Luhn check", card.BrojKartice)
	}
}

func TestCreateCard_GeneratesCVV(t *testing.T) {
	cr := &mockCardRepo{}
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: licniAccount()})
	card, err := svc.CreateCard(service.CreateCardInput{
		AccountID: 1, ClientID: 5, VrstaKartice: "visa",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(card.CVV) != 3 {
		t.Errorf("expected 3-digit CVV, got %q", card.CVV)
	}
}

func TestCreateCard_DatumIsteka_Is5YearsLater(t *testing.T) {
	cr := &mockCardRepo{}
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: licniAccount()})
	before := time.Now()
	card, err := svc.CreateCard(service.CreateCardInput{
		AccountID: 1, ClientID: 5, VrstaKartice: "visa",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := before.AddDate(5, 0, 0)
	diff := card.DatumIsteka.Sub(expected)
	if diff < -2*time.Second || diff > 2*time.Second {
		t.Errorf("expected DatumIsteka ~+5 years, got %v", card.DatumIsteka)
	}
}

func TestCreateCard_InvalidVrstaKartice_ReturnsError(t *testing.T) {
	cr := &mockCardRepo{}
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: licniAccount()})
	_, err := svc.CreateCard(service.CreateCardInput{
		AccountID: 1, ClientID: 5, VrstaKartice: "discover",
	})
	if err == nil {
		t.Error("expected error for invalid vrsta kartice")
	}
}

func TestCreateCard_LicniAccount_SecondCardAllowed(t *testing.T) {
	cr := &mockCardRepo{countByAccount: 1} // already has 1, max is 2
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: licniAccount()})
	_, err := svc.CreateCard(service.CreateCardInput{
		AccountID: 1, ClientID: 5, VrstaKartice: "mastercard",
	})
	if err != nil {
		t.Errorf("expected second card allowed on licni account, got: %v", err)
	}
}

func TestCreateCard_LicniAccount_ThirdCardRejected(t *testing.T) {
	cr := &mockCardRepo{countByAccount: 2} // already has 2, max is 2
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: licniAccount()})
	_, err := svc.CreateCard(service.CreateCardInput{
		AccountID: 1, ClientID: 5, VrstaKartice: "visa",
	})
	if err == nil {
		t.Error("expected error: licni account already has max 2 cards")
	}
}

func TestCreateCard_PoslovniAccount_FirstCardAllowed(t *testing.T) {
	cr := &mockCardRepo{countByClientAcct: 0}
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: poslovniAccount()})
	_, err := svc.CreateCard(service.CreateCardInput{
		AccountID: 2, ClientID: 5, VrstaKartice: "visa",
	})
	if err != nil {
		t.Errorf("expected first card allowed on poslovni account, got: %v", err)
	}
}

func TestCreateCard_LicniAccount_RejectsForeignClient(t *testing.T) {
	cr := &mockCardRepo{}
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: licniAccount()})
	_, err := svc.CreateCard(service.CreateCardInput{
		AccountID: 1, ClientID: 99, VrstaKartice: "visa",
	})
	if err == nil {
		t.Error("expected foreign client to be rejected on licni account")
	}
}

func TestCreateCard_PoslovniAccount_SecondCardForSameClientRejected(t *testing.T) {
	cr := &mockCardRepo{countByClientAcct: 1} // same client already has 1 card on this poslovni account
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: poslovniAccount()})
	_, err := svc.CreateCard(service.CreateCardInput{
		AccountID: 2, ClientID: 5, VrstaKartice: "mastercard",
	})
	if err == nil {
		t.Error("expected error: client already has a card on this poslovni account")
	}
}

func TestCreateCard_PoslovniAccount_SetsAuthorizedPersonWhenDifferentFromOwner(t *testing.T) {
	cr := &mockCardRepo{}
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: poslovniAccount()})
	card, err := svc.CreateCard(service.CreateCardInput{
		AccountID: 2, ClientID: 77, VrstaKartice: "visa",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if card.OvlascenoLiceID == nil || *card.OvlascenoLiceID != 77 {
		t.Errorf("expected ovlasceno lice to be stored, got %v", card.OvlascenoLiceID)
	}
}

func TestCreateCard_SavedToRepo(t *testing.T) {
	cr := &mockCardRepo{}
	svc := newCardSvc(cr, &mockAcctRepoForCard{account: licniAccount()})
	svc.CreateCard(service.CreateCardInput{
		AccountID: 1, ClientID: 5, VrstaKartice: "dinacard", NazivKartice: "Dina",
	})
	if cr.saved == nil {
		t.Error("expected card to be saved to repository")
	}
	if cr.saved.AccountID != 1 {
		t.Errorf("expected AccountID=1, got %d", cr.saved.AccountID)
	}
	if cr.saved.ClientID != 5 {
		t.Errorf("expected ClientID=5, got %d", cr.saved.ClientID)
	}
}
