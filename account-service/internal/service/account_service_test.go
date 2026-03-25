package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
)

// --- mocks ---

type mockAccountRepo struct {
	created            *models.Account
	err                error
	findByIDResult     *models.Account
	findByIDErr        error
	listByClientResult []models.Account
	listAllResult      []models.Account
	listAllTotal       int64
	capturedFilter     models.AccountFilter
	updatedID          uint
	updatedFields      map[string]interface{}
}

func (m *mockAccountRepo) Create(a *models.Account) error {
	if m.err != nil {
		return m.err
	}
	m.created = a
	return nil
}
func (m *mockAccountRepo) FindByID(_ uint) (*models.Account, error) {
	return m.findByIDResult, m.findByIDErr
}
func (m *mockAccountRepo) FindByBrojRacuna(_ string) (*models.Account, error) {
	return nil, nil
}
func (m *mockAccountRepo) ListByClientID(_ uint) ([]models.Account, error) {
	return m.listByClientResult, nil
}
func (m *mockAccountRepo) ListAll(filter models.AccountFilter) ([]models.Account, int64, error) {
	m.capturedFilter = filter
	return m.listAllResult, m.listAllTotal, nil
}
func (m *mockAccountRepo) UpdateFields(id uint, fields map[string]interface{}) error {
	if m.err != nil {
		return m.err
	}
	m.updatedID = id
	m.updatedFields = fields
	return nil
}
func (m *mockAccountRepo) ExistsByNameForClient(_ uint, _ string, _ uint) (bool, error) {
	return false, nil
}

type mockCurrencyRepo struct {
	currency *models.Currency
	err      error
}

func (m *mockCurrencyRepo) FindByID(_ uint) (*models.Currency, error) {
	if m.currency == nil && m.err == nil {
		return &models.Currency{ID: 1, Kod: "RSD"}, nil
	}
	return m.currency, m.err
}
func (m *mockCurrencyRepo) FindByKod(_ string) (*models.Currency, error) { return nil, nil }
func (m *mockCurrencyRepo) FindAll() ([]models.Currency, error)          { return nil, nil }

func ptr(u uint) *uint { return &u }

// --- CreateAccount tests ---

func TestCreateAccount_TekuciLicni_Success(t *testing.T) {
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{}, nil)

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
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, currencyRepo, nil)

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
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, currencyRepo, nil)

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

func TestCreateAccount_TekuciWithNonRSD_ReturnsError(t *testing.T) {
	currencyRepo := &mockCurrencyRepo{currency: &models.Currency{ID: 2, Kod: "EUR"}}
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, currencyRepo, nil)

	_, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 2,
		Tip:        "tekuci",
		Vrsta:      "licni",
		Podvrsta:   "standardni",
	})
	if err == nil {
		t.Fatal("expected error for tekuci+EUR, got nil")
	}
}

func TestCreateAccount_DevizniWithPodvrsta_ReturnsError(t *testing.T) {
	currencyRepo := &mockCurrencyRepo{currency: &models.Currency{ID: 2, Kod: "EUR"}}
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, currencyRepo, nil)

	_, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 2,
		Tip:        "devizni",
		Vrsta:      "licni",
		Podvrsta:   "standardni",
	})
	if err == nil {
		t.Fatal("expected error for devizni with podvrsta, got nil")
	}
}

func TestCreateAccount_LicniWithFirma_ReturnsError(t *testing.T) {
	firmaID := uint(2)
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{}, nil)

	_, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		FirmaID:    &firmaID,
		CurrencyID: 1,
		Tip:        "tekuci",
		Vrsta:      "licni",
	})
	if err == nil {
		t.Fatal("expected error for licni account with firma, got nil")
	}
}

func TestCreateAccount_InvalidPodvrstaForTekuciLicni_ReturnsError(t *testing.T) {
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{}, nil)

	_, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 1,
		Tip:        "tekuci",
		Vrsta:      "licni",
		Podvrsta:   "doo",
	})
	if err == nil {
		t.Fatal("expected error for invalid licni podvrsta, got nil")
	}
}

func TestCreateAccount_PersistsPodvrsta(t *testing.T) {
	repo := &mockAccountRepo{}
	firmaID := uint(3)
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	acc, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		FirmaID:    &firmaID,
		CurrencyID: 1,
		Tip:        "tekuci",
		Vrsta:      "poslovni",
		Podvrsta:   "ad",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.Podvrsta != "ad" {
		t.Fatalf("expected created account podvrsta to be persisted, got %q", acc.Podvrsta)
	}
	if repo.created == nil || repo.created.Podvrsta != "ad" {
		t.Fatalf("expected repository create to receive podvrsta 'ad', got %+v", repo.created)
	}
}

func TestCreateAccount_PoslovniWithoutFirma_ReturnsError(t *testing.T) {
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{}, nil)

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
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{}, nil)

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
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{}, nil)

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

func TestCreateAccount_TekuciSetsOdrzavanjeRacuna255(t *testing.T) {
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{}, nil)

	acc, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 1,
		Tip:        "tekuci",
		Vrsta:      "licni",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.OdrzavanjeRacuna != 255.00 {
		t.Errorf("expected OdrzavanjeRacuna=255.00 for tekuci account, got %v", acc.OdrzavanjeRacuna)
	}
}

func TestCreateAccount_DevizniSetsOdrzavanjeRacunaZero(t *testing.T) {
	eurCurrency := &models.Currency{ID: 2, Kod: "EUR"}
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{currency: eurCurrency}, nil)

	acc, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 2,
		Tip:        "devizni",
		Vrsta:      "licni",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.OdrzavanjeRacuna != 0 {
		t.Errorf("expected OdrzavanjeRacuna=0 for devizni account, got %v", acc.OdrzavanjeRacuna)
	}
}

func TestCreateAccount_SetsDatumIsteka5YearsFromNow(t *testing.T) {
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{}, nil)

	acc, err := svc.CreateAccount(service.CreateAccountInput{
		ClientID:   ptr(1),
		CurrencyID: 1,
		Tip:        "tekuci",
		Vrsta:      "licni",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.DatumIsteka == nil {
		t.Fatal("expected DatumIsteka to be set, got nil")
	}
	yearsFromNow := acc.DatumIsteka.Year() - time.Now().Year()
	if yearsFromNow < 4 || yearsFromNow > 6 {
		t.Errorf("expected DatumIsteka ~5 years from now, got %v (years diff=%d)", acc.DatumIsteka, yearsFromNow)
	}
}

// --- GetAccount tests ---

func TestGetAccount_ReturnsAccount(t *testing.T) {
	expected := &models.Account{ID: 7, BrojRacuna: "000123456789012345", Naziv: "Moj račun"}
	repo := &mockAccountRepo{findByIDResult: expected}
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	acc, err := svc.GetAccount(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc == nil || acc.ID != 7 {
		t.Errorf("expected account with ID=7, got %v", acc)
	}
}

func TestGetAccount_InvalidID_ReturnsError(t *testing.T) {
	repo := &mockAccountRepo{findByIDErr: errors.New("record not found")}
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	_, err := svc.GetAccount(999)
	if err == nil {
		t.Fatal("expected error for invalid ID, got nil")
	}
}

// --- ListAccountsByClient tests ---

func TestListAccountsByClient_ReturnsClientAccounts(t *testing.T) {
	accounts := []models.Account{
		{ID: 1, ClientID: ptr(5)},
		{ID: 2, ClientID: ptr(5)},
	}
	repo := &mockAccountRepo{listByClientResult: accounts}
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	result, err := svc.ListAccountsByClient(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(result))
	}
}

// --- UpdateAccountName tests ---

func TestUpdateAccountName_ChangesName(t *testing.T) {
	repo := &mockAccountRepo{findByIDResult: &models.Account{ID: 3}}
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	err := svc.UpdateAccountName(3, "Novi naziv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedID != 3 {
		t.Errorf("expected UpdateFields called with id=3, got %d", repo.updatedID)
	}
	if repo.updatedFields["naziv"] != "Novi naziv" {
		t.Errorf("expected naziv='Novi naziv', got %v", repo.updatedFields["naziv"])
	}
}

// --- UpdateAccountLimits tests ---

func TestUpdateAccountLimits_ValidPositiveAmounts(t *testing.T) {
	ownerID := uint(7)
	repo := &mockAccountRepo{findByIDResult: &models.Account{ID: 4, ClientID: &ownerID}}
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	err := svc.UpdateAccountLimits(4, ownerID, 50000, 500000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedID != 4 {
		t.Errorf("expected UpdateFields called with id=4, got %d", repo.updatedID)
	}
	if repo.updatedFields["dnevni_limit"] != float64(50000) {
		t.Errorf("expected dnevni_limit=50000, got %v", repo.updatedFields["dnevni_limit"])
	}
	if repo.updatedFields["mesecni_limit"] != float64(500000) {
		t.Errorf("expected mesecni_limit=500000, got %v", repo.updatedFields["mesecni_limit"])
	}
}

func TestUpdateAccountLimits_RejectsNegativeDnevniLimit(t *testing.T) {
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{}, nil)

	err := svc.UpdateAccountLimits(4, 0, -1, 500000)
	if err == nil {
		t.Fatal("expected error for negative dnevni limit, got nil")
	}
}

func TestUpdateAccountLimits_RejectsNegativeMesecniLimit(t *testing.T) {
	svc := service.NewAccountServiceWithRepos(&mockAccountRepo{}, &mockCurrencyRepo{}, nil)

	err := svc.UpdateAccountLimits(4, 0, 50000, -1)
	if err == nil {
		t.Fatal("expected error for negative mesecni limit, got nil")
	}
}

// --- ListAllAccounts tests ---

func TestListAllAccounts_ReturnsPaginatedResults(t *testing.T) {
	accounts := []models.Account{{ID: 1}, {ID: 2}, {ID: 3}}
	repo := &mockAccountRepo{listAllResult: accounts, listAllTotal: 3}
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	result, total, err := svc.ListAllAccounts(models.AccountFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 accounts, got %d", len(result))
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
}

func TestListAllAccounts_FilterByTip(t *testing.T) {
	repo := &mockAccountRepo{listAllResult: []models.Account{{ID: 1, Tip: "tekuci"}}, listAllTotal: 1}
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	_, _, err := svc.ListAllAccounts(models.AccountFilter{Tip: "tekuci"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.capturedFilter.Tip != "tekuci" {
		t.Errorf("expected filter Tip=tekuci, got %q", repo.capturedFilter.Tip)
	}
}

func TestListAllAccounts_FilterByStatus(t *testing.T) {
	repo := &mockAccountRepo{listAllResult: []models.Account{{ID: 1, Status: "aktivan"}}, listAllTotal: 1}
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	_, _, err := svc.ListAllAccounts(models.AccountFilter{Status: "aktivan"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.capturedFilter.Status != "aktivan" {
		t.Errorf("expected filter Status=aktivan, got %q", repo.capturedFilter.Status)
	}
}

func TestListAllAccounts_FilterByAccountNumber(t *testing.T) {
	repo := &mockAccountRepo{listAllResult: []models.Account{{ID: 1}}, listAllTotal: 1}
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	_, _, err := svc.ListAllAccounts(models.AccountFilter{AccountNumber: "265-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.capturedFilter.AccountNumber != "265-123" {
		t.Errorf("expected filter AccountNumber=265-123, got %q", repo.capturedFilter.AccountNumber)
	}
}

func TestListAllAccounts_FilterByCurrency(t *testing.T) {
	currID := uint(2)
	repo := &mockAccountRepo{listAllResult: []models.Account{{ID: 1, CurrencyID: 2}}, listAllTotal: 1}
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	_, _, err := svc.ListAllAccounts(models.AccountFilter{CurrencyID: &currID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.capturedFilter.CurrencyID == nil || *repo.capturedFilter.CurrencyID != 2 {
		t.Errorf("expected filter CurrencyID=2, got %v", repo.capturedFilter.CurrencyID)
	}
}

func TestListAllAccounts_PaginationPassedThrough(t *testing.T) {
	repo := &mockAccountRepo{listAllResult: []models.Account{}, listAllTotal: 0}
	svc := service.NewAccountServiceWithRepos(repo, &mockCurrencyRepo{}, nil)

	_, _, err := svc.ListAllAccounts(models.AccountFilter{Page: 3, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.capturedFilter.Page != 3 {
		t.Errorf("expected Page=3, got %d", repo.capturedFilter.Page)
	}
	if repo.capturedFilter.PageSize != 20 {
		t.Errorf("expected PageSize=20, got %d", repo.capturedFilter.PageSize)
	}
}
