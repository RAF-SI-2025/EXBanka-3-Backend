package service_test

import (
	"errors"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/service"
)

// --- mocks ---

type mockAccountRepo struct {
	accounts      map[uint]*models.Account
	updatedID     uint
	updatedFields map[string]interface{}
	err           error
}

func (m *mockAccountRepo) FindByID(id uint) (*models.Account, error) {
	if m.err != nil {
		return nil, m.err
	}
	if a, ok := m.accounts[id]; ok {
		return a, nil
	}
	return nil, errors.New("account not found")
}

func (m *mockAccountRepo) UpdateFields(id uint, fields map[string]interface{}) error {
	m.updatedID = id
	m.updatedFields = fields
	return nil
}

type mockTransferRepo struct {
	created              *models.Transfer
	listByAccountResult  []models.Transfer
	listByAccountTotal   int64
	listByClientResult   []models.Transfer
	listByClientTotal    int64
	capturedAccountFilter models.TransferFilter
	capturedClientFilter  models.TransferFilter
}

func (m *mockTransferRepo) Create(t *models.Transfer) error {
	m.created = t
	return nil
}
func (m *mockTransferRepo) FindByID(_ uint) (*models.Transfer, error) { return nil, nil }
func (m *mockTransferRepo) ListByAccountID(_ uint, filter models.TransferFilter) ([]models.Transfer, int64, error) {
	m.capturedAccountFilter = filter
	return m.listByAccountResult, m.listByAccountTotal, nil
}
func (m *mockTransferRepo) ListByClientID(_ uint, filter models.TransferFilter) ([]models.Transfer, int64, error) {
	m.capturedClientFilter = filter
	return m.listByClientResult, m.listByClientTotal, nil
}

type mockExchangeRateService struct {
	rate float64
	err  error
}

func (m *mockExchangeRateService) GetRate(from, to string) (float64, error) {
	return m.rate, m.err
}

func rsdAccount(id uint, balance float64) *models.Account {
	return &models.Account{
		ID: id, RaspolozivoStanje: balance, Stanje: balance,
		DnevniLimit: 100000, CurrencyID: 1,
		Currency: models.Currency{ID: 1, Kod: "RSD"},
	}
}

func eurAccount(id uint, balance float64) *models.Account {
	return &models.Account{
		ID: id, RaspolozivoStanje: balance, Stanje: balance,
		DnevniLimit: 10000, CurrencyID: 2,
		Currency: models.Currency{ID: 2, Kod: "EUR"},
	}
}

// --- tests ---

func TestCreateTransfer_SameCurrency_Success(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 1000),
	}}
	transferRepo := &mockTransferRepo{}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, &mockExchangeRateService{})

	tr, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaID:   2,
		Iznos:             1000,
		Svrha:             "Test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil {
		t.Fatal("expected non-nil transfer")
	}
	if tr.Iznos != 1000 {
		t.Errorf("expected Iznos=1000, got %f", tr.Iznos)
	}
	if tr.Status != "uspesno" {
		t.Errorf("expected Status=uspesno, got %s", tr.Status)
	}
	if tr.Kurs != 1.0 {
		t.Errorf("expected Kurs=1.0 for same-currency, got %f", tr.Kurs)
	}
}

func TestCreateTransfer_CrossCurrency_AppliesExchangeRate(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: eurAccount(1, 1000),
		2: rsdAccount(2, 0),
	}}
	transferRepo := &mockTransferRepo{}
	rateSvc := &mockExchangeRateService{rate: 117.0}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, rateSvc)

	tr, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaID:   2,
		Iznos:             100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Kurs != 117.0 {
		t.Errorf("expected Kurs=117.0, got %f", tr.Kurs)
	}
	if tr.KonvertovaniIznos != 11700 {
		t.Errorf("expected KonvertovaniIznos=11700, got %f", tr.KonvertovaniIznos)
	}
	if tr.ValutaIznosa != "EUR" {
		t.Errorf("expected ValutaIznosa=EUR, got %s", tr.ValutaIznosa)
	}
}

func TestCreateTransfer_InsufficientBalance_ReturnsError(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 500),
		2: rsdAccount(2, 0),
	}}
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{})

	_, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 1000,
	})
	if err == nil {
		t.Fatal("expected insufficient balance error, got nil")
	}
}

func TestCreateTransfer_SameAccount_ReturnsError(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000),
	}}
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{})

	_, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 1, Iznos: 100,
	})
	if err == nil {
		t.Fatal("expected same-account error, got nil")
	}
}

func TestCreateTransfer_NegativeAmount_ReturnsError(t *testing.T) {
	svc := service.NewTransferServiceWithRepos(&mockAccountRepo{accounts: map[uint]*models.Account{}}, &mockTransferRepo{}, &mockExchangeRateService{})

	_, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: -50,
	})
	if err == nil {
		t.Fatal("expected negative amount error, got nil")
	}
}

func TestCreateTransfer_DailyLimitExceeded_ReturnsError(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: {ID: 1, RaspolozivoStanje: 200000, Stanje: 200000, DnevniLimit: 100000, CurrencyID: 1, Currency: models.Currency{Kod: "RSD"}},
		2: rsdAccount(2, 0),
	}}
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{})

	_, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 150000,
	})
	if err == nil {
		t.Fatal("expected daily limit error, got nil")
	}
}

// --- ListTransfersByAccount tests ---

func TestListTransfersByAccount_ReturnsTransfers(t *testing.T) {
	transfers := []models.Transfer{{ID: 1}, {ID: 2}}
	transferRepo := &mockTransferRepo{listByAccountResult: transfers, listByAccountTotal: 2}
	svc := service.NewTransferServiceWithRepos(&mockAccountRepo{accounts: map[uint]*models.Account{}}, transferRepo, &mockExchangeRateService{})

	result, total, err := svc.ListTransfersByAccount(5, models.TransferFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 transfers, got %d", len(result))
	}
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
}

func TestListTransfersByAccount_FilterPassedThrough(t *testing.T) {
	transferRepo := &mockTransferRepo{}
	svc := service.NewTransferServiceWithRepos(&mockAccountRepo{accounts: map[uint]*models.Account{}}, transferRepo, &mockExchangeRateService{})

	_, _, err := svc.ListTransfersByAccount(5, models.TransferFilter{Status: "uspesno", Page: 2, PageSize: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transferRepo.capturedAccountFilter.Status != "uspesno" {
		t.Errorf("expected Status filter=uspesno, got %q", transferRepo.capturedAccountFilter.Status)
	}
	if transferRepo.capturedAccountFilter.Page != 2 {
		t.Errorf("expected Page=2, got %d", transferRepo.capturedAccountFilter.Page)
	}
}

// --- ListTransfersByClient tests ---

func TestListTransfersByClient_ReturnsTransfers(t *testing.T) {
	transfers := []models.Transfer{{ID: 10}, {ID: 11}, {ID: 12}}
	transferRepo := &mockTransferRepo{listByClientResult: transfers, listByClientTotal: 3}
	svc := service.NewTransferServiceWithRepos(&mockAccountRepo{accounts: map[uint]*models.Account{}}, transferRepo, &mockExchangeRateService{})

	result, total, err := svc.ListTransfersByClient(7, models.TransferFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 transfers, got %d", len(result))
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
}

func TestListTransfersByClient_PaginationPassedThrough(t *testing.T) {
	transferRepo := &mockTransferRepo{}
	svc := service.NewTransferServiceWithRepos(&mockAccountRepo{accounts: map[uint]*models.Account{}}, transferRepo, &mockExchangeRateService{})

	_, _, err := svc.ListTransfersByClient(7, models.TransferFilter{Page: 3, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transferRepo.capturedClientFilter.Page != 3 {
		t.Errorf("expected Page=3, got %d", transferRepo.capturedClientFilter.Page)
	}
	if transferRepo.capturedClientFilter.PageSize != 20 {
		t.Errorf("expected PageSize=20, got %d", transferRepo.capturedClientFilter.PageSize)
	}
}
