package service_test

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
)

// --- mocks ---

type mockAccountRepo struct {
	created *models.Account
	err     error
}

func (m *mockAccountRepo) Create(a *models.Account) error {
	if m.err != nil {
		return m.err
	}
	m.created = a
	return nil
}
func (m *mockAccountRepo) FindByID(_ uint) (*models.Account, error) { return nil, nil }
func (m *mockAccountRepo) FindByBrojRacuna(_ string) (*models.Account, error) {
	return nil, nil
}
func (m *mockAccountRepo) ListByClientID(_ uint) ([]models.Account, error) { return nil, nil }
func (m *mockAccountRepo) ListAll(_ models.AccountFilter) ([]models.Account, int64, error) {
	return nil, 0, nil
}
func (m *mockAccountRepo) UpdateFields(_ uint, _ map[string]interface{}) error { return nil }

type mockCurrencyRepo struct {
	currency *models.Currency
	err      error
}

func (m *mockCurrencyRepo) FindByID(_ uint) (*models.Currency, error) {
	return m.currency, m.err
}
func (m *mockCurrencyRepo) FindByKod(_ string) (*models.Currency, error) { return nil, nil }
func (m *mockCurrencyRepo) FindAll() ([]models.Currency, error)          { return nil, nil }

func ptr(u uint) *uint { return &u }

// --- tests ---

func TestCreateAccount_TekuciLicni_Success(t *testing.T) {
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{})

	acc, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 1,
		Tip:        "tekuci",
		Vrsta:      "licni",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc == nil {
		t.Fatal("expected non-nil account")
	}
}

func TestCreateAccount_DevizniLicni_NonRSD_Success(t *testing.T) {
	currencyRepo := &mockCurrencyRepo{currency: &models.Currency{ID: 2, Kod: "EUR"}}
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, currencyRepo)

	acc, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 2,
		Tip:        "devizni",
		Vrsta:      "licni",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc == nil {
		t.Fatal("expected non-nil account")
	}
}

func TestCreateAccount_DevizniWithRSD_ReturnsError(t *testing.T) {
	currencyRepo := &mockCurrencyRepo{currency: &models.Currency{ID: 1, Kod: "RSD"}}
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, currencyRepo)

	_, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 1,
		Tip:        "devizni",
		Vrsta:      "licni",
	})
	if err == nil {
		t.Fatal("expected error for devizni+RSD, got nil")
	}
}

func TestCreateAccount_PoslovniWithoutFirma_ReturnsError(t *testing.T) {
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{})

	_, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 1,
		Tip:        "tekuci",
		Vrsta:      "poslovni",
		FirmaID:    nil,
	})
	if err == nil {
		t.Fatal("expected error for poslovni without firmaID, got nil")
	}
}

func TestCreateAccount_GeneratesValid18DigitBrojRacuna(t *testing.T) {
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{})

	acc, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 1,
		Tip:        "tekuci",
		Vrsta:      "licni",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(acc.BrojRacuna) != 18 {
		t.Errorf("expected 18-digit BrojRacuna, got length %d: %s", len(acc.BrojRacuna), acc.BrojRacuna)
	}
	if !util.ValidateAccountNumber(acc.BrojRacuna) {
		t.Errorf("generated BrojRacuna failed validation: %s", acc.BrojRacuna)
	}
}

func TestCreateAccount_SetsDefaultLimits(t *testing.T) {
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{})

	acc, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 1,
		Tip:        "tekuci",
		Vrsta:      "licni",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.DnevniLimit != 100000 {
		t.Errorf("expected DnevniLimit=100000, got %v", acc.DnevniLimit)
	}
	if acc.MesecniLimit != 1000000 {
		t.Errorf("expected MesecniLimit=1000000, got %v", acc.MesecniLimit)
	}
	if acc.Status != "aktivan" {
		t.Errorf("expected Status=aktivan, got %s", acc.Status)
	}
}
