package service_test

import (
	"errors"
	"testing"
	"time"

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
	created               *models.Transfer
	listByAccountResult   []models.Transfer
	listByAccountTotal    int64
	listByClientResult    []models.Transfer
	listByClientTotal     int64
	capturedAccountFilter models.TransferFilter
	capturedClientFilter  models.TransferFilter
}

func (m *mockTransferRepo) Create(t *models.Transfer) error {
	m.created = t
	return nil
}
func (m *mockTransferRepo) FindByID(id uint) (*models.Transfer, error) {
	if m.created != nil && m.created.ID == id {
		return m.created, nil
	}
	return nil, errors.New("not found")
}
func (m *mockTransferRepo) Save(t *models.Transfer) error { m.created = t; return nil }
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

type mockNotificationSender struct {
	called   bool
	toEmail  string
	toName   string
	transfer *models.Transfer
	err      error
}

func (m *mockNotificationSender) SendTransferVerificationCode(toEmail, toName string, transfer *models.Transfer) error {
	m.called = true
	m.toEmail = toEmail
	m.toName = toName
	m.transfer = transfer
	return m.err
}

func rsdAccount(id uint, balance float64) *models.Account {
	clientID := uint(1)
	return &models.Account{
		ID: id, RaspolozivoStanje: balance, Stanje: balance,
		DnevniLimit: 100000, MesecniLimit: 1000000, CurrencyID: 1,
		ClientID: &clientID,
		Currency: models.Currency{ID: 1, Kod: "RSD"},
		Client:   &models.Client{ID: 1, Ime: "Ana", Prezime: "Jovic", Email: "ana@example.com"},
	}
}

// captureAccountRepo records all UpdateFields calls indexed by account ID.
type captureAccountRepo struct {
	accounts map[uint]*models.Account
	updates  map[uint]map[string]interface{}
}

func newCaptureRepo(accounts map[uint]*models.Account) *captureAccountRepo {
	return &captureAccountRepo{accounts: accounts, updates: make(map[uint]map[string]interface{})}
}

func (r *captureAccountRepo) FindByID(id uint) (*models.Account, error) {
	if a, ok := r.accounts[id]; ok {
		return a, nil
	}
	return nil, errors.New("account not found")
}

func (r *captureAccountRepo) UpdateFields(id uint, fields map[string]interface{}) error {
	r.updates[id] = fields
	return nil
}

func eurAccount(id uint, balance float64) *models.Account {
	clientID := uint(1)
	return &models.Account{
		ID: id, RaspolozivoStanje: balance, Stanje: balance,
		DnevniLimit: 10000, MesecniLimit: 100000, CurrencyID: 2,
		ClientID: &clientID,
		Currency: models.Currency{ID: 2, Kod: "EUR"},
		Client:   &models.Client{ID: 1, Ime: "Ana", Prezime: "Jovic", Email: "ana@example.com"},
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
	if tr.Status != "u_obradi" {
		t.Errorf("expected Status=u_obradi (pending verification), got %s", tr.Status)
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
	clientID := uint(1)
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: {ID: 1, ClientID: &clientID, RaspolozivoStanje: 200000, Stanje: 200000, DnevniLimit: 100000, CurrencyID: 1, Currency: models.Currency{Kod: "RSD"}},
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

// --- Verify flow tests ---

func TestCreateTransfer_SetsStatusUObradi(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 1000),
	}}
	transferRepo := &mockTransferRepo{}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, &mockExchangeRateService{})

	tr, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 1000, Svrha: "Test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != "u_obradi" {
		t.Errorf("expected Status=u_obradi, got %s", tr.Status)
	}
}

func TestCreateTransfer_GeneratesVerifikacioniKod(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 1000),
	}}
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{})

	tr, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tr.VerifikacioniKod) != 6 {
		t.Errorf("expected 6-digit code, got %q", tr.VerifikacioniKod)
	}
}

func TestCreateTransfer_SetsVerificationExpiry(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 1000),
	}}
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{})

	tr, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.VerificationExpiresAt == nil {
		t.Fatal("expected verification expiry to be set")
	}
	if delta := tr.VerificationExpiresAt.Sub(tr.VremeTransakcije); delta < 4*time.Minute || delta > 6*time.Minute {
		t.Errorf("expected verification expiry about 5 minutes after creation, got %v", delta)
	}
}

func TestCreateTransfer_SendsVerificationEmail(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 1000),
	}}
	notifier := &mockNotificationSender{}
	svc := service.NewTransferServiceWithReposAndNotifier(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{}, notifier)

	tr, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 100, Svrha: "Test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !notifier.called {
		t.Fatal("expected notification sender to be called")
	}
	if notifier.toEmail != "ana@example.com" {
		t.Errorf("expected email ana@example.com, got %q", notifier.toEmail)
	}
	if notifier.transfer == nil || notifier.transfer.VerifikacioniKod != tr.VerifikacioniKod {
		t.Fatal("expected notification sender to receive the created transfer")
	}
}

func TestCreateTransfer_DoesNotUpdateSenderBalance(t *testing.T) {
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 0),
	})
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{})

	_, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, updated := accountRepo.updates[1]; updated {
		t.Error("expected sender balance NOT to be updated in CreateTransfer (deferred to VerifyTransfer)")
	}
}

func TestVerifyTransfer_ValidCode_SetsStatusUspesno(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, Status: "u_obradi", VerifikacioniKod: "111111",
		CreatedAt: time.Now(),
	}
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 0),
	})
	transferRepo := &mockTransferRepo{created: tr}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, &mockExchangeRateService{})

	result, err := svc.VerifyTransfer(1, "111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "uspesno" {
		t.Errorf("expected Status=uspesno, got %s", result.Status)
	}
}

func TestVerifyTransfer_ValidCode_UpdatesSenderBalance(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, Status: "u_obradi", VerifikacioniKod: "222222",
		ValutaIznosa: "RSD", KonvertovaniIznos: 500,
		CreatedAt: time.Now(),
	}
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 0),
	})
	transferRepo := &mockTransferRepo{created: tr}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, &mockExchangeRateService{})

	_, err := svc.VerifyTransfer(1, "222222")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	update, ok := accountRepo.updates[1]
	if !ok {
		t.Fatal("expected sender account to be updated in VerifyTransfer")
	}
	newBalance, _ := update["stanje"].(float64)
	if newBalance != 4500 {
		t.Errorf("expected sender stanje=4500, got %f", newBalance)
	}
}

func TestVerifyTransfer_InvalidCode_ReturnsError(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, Status: "u_obradi", VerifikacioniKod: "333333",
		CreatedAt: time.Now(),
	}
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{1: rsdAccount(1, 1000)}),
		&mockTransferRepo{created: tr},
		&mockExchangeRateService{},
	)

	_, err := svc.VerifyTransfer(1, "000000")
	if err == nil {
		t.Fatal("expected error for invalid code, got nil")
	}
}

func TestVerifyTransfer_NonPendingTransfer_ReturnsError(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, Status: "uspesno", VerifikacioniKod: "444444",
	}
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{}),
		&mockTransferRepo{created: tr},
		&mockExchangeRateService{},
	)

	_, err := svc.VerifyTransfer(1, "444444")
	if err == nil {
		t.Fatal("expected error for non-pending transfer, got nil")
	}
}

// --- TTL tests ---

func TestVerifyTransfer_ExpiredCode_ReturnsError(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, Status: "u_obradi", VerifikacioniKod: "777777",
		CreatedAt: time.Now().Add(-6 * time.Minute), // 6 min ago → past TTL
	}
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{1: rsdAccount(1, 1000), 2: rsdAccount(2, 0)}),
		&mockTransferRepo{created: tr},
		&mockExchangeRateService{},
	)

	_, err := svc.VerifyTransfer(1, "777777")
	if err == nil {
		t.Fatal("expected expired code error, got nil")
	}
}

func TestVerifyTransfer_ExpiredCode_SetsStatusStornirano(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, Status: "u_obradi", VerifikacioniKod: "888888",
		CreatedAt: time.Now().Add(-6 * time.Minute),
	}
	transferRepo := &mockTransferRepo{created: tr}
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{1: rsdAccount(1, 1000), 2: rsdAccount(2, 0)}),
		transferRepo,
		&mockExchangeRateService{},
	)

	svc.VerifyTransfer(1, "888888")

	if transferRepo.created == nil || transferRepo.created.Status != "stornirano" {
		t.Errorf("expected transfer status=stornirano after expiry, got %v", transferRepo.created)
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

// --- DnevnaPotrosnja / MesecnaPotrosnja tests ---

func TestCreateTransfer_DailySpendingExceedsLimit_ReturnsError(t *testing.T) {
	clientID := uint(1)
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: {
			ID: 1, RaspolozivoStanje: 50000, Stanje: 50000,
			DnevniLimit: 100000, MesecniLimit: 1000000,
			DnevnaPotrosnja: 90000, // already spent 90k today
			ClientID:        &clientID,
			CurrencyID:      1, Currency: models.Currency{Kod: "RSD"},
		},
		2: rsdAccount(2, 0),
	})
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{})

	_, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 20000, // 90000+20000=110000 > 100000
	})
	if err == nil {
		t.Fatal("expected daily spending limit error, got nil")
	}
}

func TestCreateTransfer_MonthlySpendingExceedsLimit_ReturnsError(t *testing.T) {
	clientID := uint(1)
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: {
			ID: 1, RaspolozivoStanje: 100000, Stanje: 100000,
			DnevniLimit: 500000, MesecniLimit: 1000000,
			MesecnaPotrosnja: 970000, // already spent 970k this month
			ClientID:         &clientID,
			CurrencyID:       1, Currency: models.Currency{Kod: "RSD"},
		},
		2: rsdAccount(2, 0),
	})
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{}, &mockExchangeRateService{})

	_, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1, RacunPrimaocaID: 2, Iznos: 50000, // 970000+50000=1020000 > 1000000
	})
	if err == nil {
		t.Fatal("expected monthly spending limit error, got nil")
	}
}

func TestVerifyTransfer_UpdatesDnevnaPotrosnja(t *testing.T) {
	clientID := uint(1)
	sender := &models.Account{
		ID: 1, RaspolozivoStanje: 10000, Stanje: 10000,
		DnevniLimit: 100000, MesecniLimit: 1000000,
		DnevnaPotrosnja: 1000, MesecnaPotrosnja: 5000,
		ClientID: &clientID,
		CurrencyID: 1, Currency: models.Currency{Kod: "RSD"},
	}
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, KonvertovaniIznos: 500, ValutaIznosa: "RSD",
		Status: "u_obradi", VerifikacioniKod: "555555",
		CreatedAt: time.Now(),
	}
	accountRepo := newCaptureRepo(map[uint]*models.Account{1: sender, 2: rsdAccount(2, 0)})
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{created: tr}, &mockExchangeRateService{})

	_, err := svc.VerifyTransfer(1, "555555")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	senderUpdate := accountRepo.updates[1]
	newDnevna, _ := senderUpdate["dnevna_potrosnja"].(float64)
	if newDnevna != 1500 {
		t.Errorf("expected dnevna_potrosnja=1500 after verify, got %f", newDnevna)
	}
}

func TestVerifyTransfer_UpdatesMesecnaPotrosnja(t *testing.T) {
	clientID := uint(1)
	sender := &models.Account{
		ID: 1, RaspolozivoStanje: 10000, Stanje: 10000,
		DnevniLimit: 100000, MesecniLimit: 1000000,
		DnevnaPotrosnja: 1000, MesecnaPotrosnja: 5000,
		ClientID: &clientID,
		CurrencyID: 1, Currency: models.Currency{Kod: "RSD"},
	}
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, KonvertovaniIznos: 500, ValutaIznosa: "RSD",
		Status: "u_obradi", VerifikacioniKod: "666666",
		CreatedAt: time.Now(),
	}
	accountRepo := newCaptureRepo(map[uint]*models.Account{1: sender, 2: rsdAccount(2, 0)})
	svc := service.NewTransferServiceWithRepos(accountRepo, &mockTransferRepo{created: tr}, &mockExchangeRateService{})

	_, err := svc.VerifyTransfer(1, "666666")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	senderUpdate := accountRepo.updates[1]
	newMesecna, _ := senderUpdate["mesecna_potrosnja"].(float64)
	if newMesecna != 5500 {
		t.Errorf("expected mesecna_potrosnja=5500 after verify, got %f", newMesecna)
	}
}

// --- BrojPokusaja / max-attempts tests ---

func TestVerifyTransfer_WrongCode_IncrementsBrojPokusaja(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, Status: "u_obradi", VerifikacioniKod: "111111",
		CreatedAt: time.Now(),
	}
	transferRepo := &mockTransferRepo{created: tr}
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{1: rsdAccount(1, 5000), 2: rsdAccount(2, 0)}),
		transferRepo, &mockExchangeRateService{},
	)

	svc.VerifyTransfer(1, "000000") // wrong code

	if transferRepo.created == nil {
		t.Fatal("expected transfer to be saved after wrong code")
	}
	if transferRepo.created.BrojPokusaja != 1 {
		t.Errorf("expected BrojPokusaja=1 after one wrong attempt, got %d", transferRepo.created.BrojPokusaja)
	}
}

func TestVerifyTransfer_TwoWrongCodes_StillPending(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, Status: "u_obradi", VerifikacioniKod: "111111",
		CreatedAt: time.Now(),
	}
	transferRepo := &mockTransferRepo{created: tr}
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{1: rsdAccount(1, 5000), 2: rsdAccount(2, 0)}),
		transferRepo, &mockExchangeRateService{},
	)

	svc.VerifyTransfer(1, "000000")
	svc.VerifyTransfer(1, "000000")

	if transferRepo.created.Status != "u_obradi" {
		t.Errorf("expected status=u_obradi after 2 wrong attempts, got %s", transferRepo.created.Status)
	}
}

func TestVerifyTransfer_ThreeWrongCodes_SetsStatusStornirano(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, Status: "u_obradi", VerifikacioniKod: "111111",
		CreatedAt: time.Now(),
	}
	transferRepo := &mockTransferRepo{created: tr}
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{1: rsdAccount(1, 5000), 2: rsdAccount(2, 0)}),
		transferRepo, &mockExchangeRateService{},
	)

	svc.VerifyTransfer(1, "000000")
	svc.VerifyTransfer(1, "000000")
	svc.VerifyTransfer(1, "000000")

	if transferRepo.created == nil || transferRepo.created.Status != "stornirano" {
		t.Errorf("expected status=stornirano after 3 wrong attempts, got %v", transferRepo.created)
	}
}

func TestVerifyTransfer_ThreeWrongCodes_ReturnsError(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, Status: "u_obradi", VerifikacioniKod: "111111",
		CreatedAt: time.Now(),
	}
	transferRepo := &mockTransferRepo{created: tr}
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{1: rsdAccount(1, 5000), 2: rsdAccount(2, 0)}),
		transferRepo, &mockExchangeRateService{},
	)

	svc.VerifyTransfer(1, "000000")
	svc.VerifyTransfer(1, "000000")
	_, err := svc.VerifyTransfer(1, "000000")

	if err == nil {
		t.Fatal("expected error after 3 wrong attempts, got nil")
	}
}

func TestVerifyTransfer_ClearsVerificationCodeAfterSuccess(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, KonvertovaniIznos: 500, ValutaIznosa: "RSD",
		Status: "u_obradi", VerifikacioniKod: "123456",
		CreatedAt: time.Now(), VerificationExpiresAt: ptrTime(time.Now().Add(5 * time.Minute)),
	}
	transferRepo := &mockTransferRepo{created: tr}
	svc := service.NewTransferServiceWithRepos(
		newCaptureRepo(map[uint]*models.Account{1: rsdAccount(1, 5000), 2: rsdAccount(2, 0)}),
		transferRepo,
		&mockExchangeRateService{},
	)

	result, err := svc.VerifyTransfer(1, "123456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.VerifikacioniKod != "" {
		t.Errorf("expected verification code to be cleared after success, got %q", result.VerifikacioniKod)
	}
	if result.VerificationExpiresAt != nil {
		t.Error("expected verification expiry to be cleared after success")
	}
}

func TestVerifyTransfer_InsufficientBalanceAtVerify_CancelsTransfer(t *testing.T) {
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 500, KonvertovaniIznos: 500, ValutaIznosa: "RSD",
		Status: "u_obradi", VerifikacioniKod: "123456",
		CreatedAt: time.Now(), VerificationExpiresAt: ptrTime(time.Now().Add(5 * time.Minute)),
	}
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: rsdAccount(1, 300),
		2: rsdAccount(2, 0),
	})
	transferRepo := &mockTransferRepo{created: tr}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, &mockExchangeRateService{})

	_, err := svc.VerifyTransfer(1, "123456")
	if err == nil {
		t.Fatal("expected insufficient balance error, got nil")
	}
	if transferRepo.created.Status != "stornirano" {
		t.Errorf("expected status=stornirano, got %s", transferRepo.created.Status)
	}
	if len(accountRepo.updates) != 0 {
		t.Fatal("expected no account updates when verification fails")
	}
}

func TestApproveTransferMobile_CodeReturnsExistingCodeWithoutSettlement(t *testing.T) {
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 0),
	})
	transferRepo := &mockTransferRepo{}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, &mockExchangeRateService{})

	created, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaID:   2,
		Iznos:             100,
		Svrha:             "Mobile code",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	transfer, code, expiresAt, err := svc.ApproveTransferMobile(created.ID, "code")
	if err != nil {
		t.Fatalf("approve mobile failed: %v", err)
	}
	if transfer.Status != "u_obradi" {
		t.Fatalf("expected transfer to remain pending, got %s", transfer.Status)
	}
	if code == "" {
		t.Fatal("expected verification code")
	}
	if expiresAt == nil {
		t.Fatal("expected verification expiry")
	}
	if len(accountRepo.updates) != 0 {
		t.Fatal("expected no balance updates in code mode")
	}
}

func TestApproveTransferMobile_ConfirmExecutesSettlement(t *testing.T) {
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 0),
	})
	transferRepo := &mockTransferRepo{}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, &mockExchangeRateService{})

	created, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaID:   2,
		Iznos:             120,
		Svrha:             "Mobile confirm",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	transfer, code, expiresAt, err := svc.ApproveTransferMobile(created.ID, "confirm")
	if err != nil {
		t.Fatalf("confirm mobile failed: %v", err)
	}
	if transfer.Status != "uspesno" {
		t.Fatalf("expected completed transfer, got %s", transfer.Status)
	}
	if code != "" || expiresAt != nil {
		t.Fatal("expected confirm mode not to return verification code or expiry")
	}
	if accountRepo.updates[1]["raspolozivo_stanje"].(float64) != 4880 {
		t.Fatalf("expected sender balance 4880, got %v", accountRepo.updates[1]["raspolozivo_stanje"])
	}
	if accountRepo.updates[2]["raspolozivo_stanje"].(float64) != 120 {
		t.Fatalf("expected receiver balance 120, got %v", accountRepo.updates[2]["raspolozivo_stanje"])
	}
}

func TestRejectTransfer_CancelsPendingTransfer(t *testing.T) {
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: rsdAccount(1, 5000),
		2: rsdAccount(2, 0),
	})
	transferRepo := &mockTransferRepo{}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, &mockExchangeRateService{})

	created, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaID:   2,
		Iznos:             80,
		Svrha:             "Mobile reject",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	rejected, err := svc.RejectTransfer(created.ID)
	if err != nil {
		t.Fatalf("reject mobile failed: %v", err)
	}
	if rejected.Status != "stornirano" {
		t.Fatalf("expected cancelled transfer, got %s", rejected.Status)
	}
	if len(accountRepo.updates) != 0 {
		t.Fatal("expected reject not to touch balances")
	}
}

func TestVerifyTransfer_DailyLimitExceededAtVerify_CancelsTransfer(t *testing.T) {
	sender := rsdAccount(1, 5000)
	sender.DnevnaPotrosnja = 990
	sender.DnevniLimit = 1000
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 20, KonvertovaniIznos: 20, ValutaIznosa: "RSD",
		Status: "u_obradi", VerifikacioniKod: "123456",
		CreatedAt: time.Now(), VerificationExpiresAt: ptrTime(time.Now().Add(5 * time.Minute)),
	}
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: sender,
		2: rsdAccount(2, 0),
	})
	transferRepo := &mockTransferRepo{created: tr}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, &mockExchangeRateService{})

	_, err := svc.VerifyTransfer(1, "123456")
	if err == nil {
		t.Fatal("expected daily limit error, got nil")
	}
	if transferRepo.created.Status != "stornirano" {
		t.Errorf("expected status=stornirano, got %s", transferRepo.created.Status)
	}
}

func TestVerifyTransfer_MonthlyLimitExceededAtVerify_CancelsTransfer(t *testing.T) {
	sender := rsdAccount(1, 5000)
	sender.MesecnaPotrosnja = 995
	sender.MesecniLimit = 1000
	tr := &models.Transfer{
		ID: 1, RacunPosiljaocaID: 1, RacunPrimaocaID: 2,
		Iznos: 20, KonvertovaniIznos: 20, ValutaIznosa: "RSD",
		Status: "u_obradi", VerifikacioniKod: "123456",
		CreatedAt: time.Now(), VerificationExpiresAt: ptrTime(time.Now().Add(5 * time.Minute)),
	}
	accountRepo := newCaptureRepo(map[uint]*models.Account{
		1: sender,
		2: rsdAccount(2, 0),
	})
	transferRepo := &mockTransferRepo{created: tr}
	svc := service.NewTransferServiceWithRepos(accountRepo, transferRepo, &mockExchangeRateService{})

	_, err := svc.VerifyTransfer(1, "123456")
	if err == nil {
		t.Fatal("expected monthly limit error, got nil")
	}
	if transferRepo.created.Status != "stornirano" {
		t.Errorf("expected status=stornirano, got %s", transferRepo.created.Status)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
