package service_test

import (
	"errors"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/service"
)

// --- mocks ---

type mockAccountRepo struct {
	accounts      map[uint]*models.Account
	updatedID     uint
	updatedFields map[string]interface{}
	findErr       error
}

func (m *mockAccountRepo) FindByID(id uint) (*models.Account, error) {
	if m.findErr != nil {
		return nil, m.findErr
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

type mockPaymentRepo struct {
	created   *models.Payment
	saved     *models.Payment
	findByID  map[uint]*models.Payment
	nextID    uint
	createErr error
	saveErr   error
	findErr   error
}

func newMockPaymentRepo() *mockPaymentRepo {
	return &mockPaymentRepo{findByID: make(map[uint]*models.Payment), nextID: 1}
}

func (m *mockPaymentRepo) Create(p *models.Payment) error {
	if m.createErr != nil {
		return m.createErr
	}
	p.ID = m.nextID
	m.nextID++
	m.created = p
	m.findByID[p.ID] = p
	return nil
}

func (m *mockPaymentRepo) FindByID(id uint) (*models.Payment, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	if p, ok := m.findByID[id]; ok {
		return p, nil
	}
	return nil, errors.New("payment not found")
}

func (m *mockPaymentRepo) Save(p *models.Payment) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = p
	m.findByID[p.ID] = p
	return nil
}

func (m *mockPaymentRepo) ListByAccountID(accountID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	return nil, 0, nil
}

func (m *mockPaymentRepo) ListByClientID(clientID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	return nil, 0, nil
}

type mockNewRecipientRepo struct {
	created *models.PaymentRecipient
	nextID  uint
}

func (m *mockNewRecipientRepo) Create(r *models.PaymentRecipient) error {
	m.nextID++
	r.ID = m.nextID
	m.created = r
	return nil
}

func rsdAccount(id uint, balance float64, clientID *uint) *models.Account {
	return &models.Account{
		ID: id, RaspolozivoStanje: balance, Stanje: balance,
		DnevniLimit: 100000, ClientID: clientID,
	}
}

// --- tests ---

func TestCreatePayment_Success_StatusUObradi(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000, nil),
	}}
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil)

	p, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             500,
		Svrha:             "Test",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Status != "u_obradi" {
		t.Errorf("expected status=u_obradi, got %s", p.Status)
	}
}

func TestCreatePayment_GeneratesSixDigitCode(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000, nil),
	}}
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil)

	p, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.VerifikacioniKod) != 6 {
		t.Errorf("expected 6-digit code, got %q (len=%d)", p.VerifikacioniKod, len(p.VerifikacioniKod))
	}
	for _, c := range p.VerifikacioniKod {
		if c < '0' || c > '9' {
			t.Errorf("code contains non-digit character: %q", p.VerifikacioniKod)
			break
		}
	}
}

func TestCreatePayment_InsufficientBalance_ReturnsError(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 100, nil),
	}}
	svc := service.NewPaymentServiceWithRepos(accountRepo, newMockPaymentRepo(), nil)

	_, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             500,
	})

	if err == nil {
		t.Fatal("expected insufficient balance error, got nil")
	}
}

func TestVerifyPayment_CorrectCode_SetsStatusUspesno(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000, nil),
	}}
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil)

	created, _ := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             200,
	})

	verified, err := svc.VerifyPayment(created.ID, created.VerifikacioniKod)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verified.Status != "uspesno" {
		t.Errorf("expected status=uspesno, got %s", verified.Status)
	}
}

func TestVerifyPayment_WrongCode_ReturnsError(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000, nil),
	}}
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil)

	created, _ := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             200,
	})

	_, err := svc.VerifyPayment(created.ID, "000000")

	if err == nil {
		t.Fatal("expected wrong code error, got nil")
	}
}

func TestVerifyPayment_DeductsSenderBalance(t *testing.T) {
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000, nil),
	}}
	paymentRepo := newMockPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, nil)

	created, _ := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             300,
	})
	svc.VerifyPayment(created.ID, created.VerifikacioniKod)

	if accountRepo.updatedID != 1 {
		t.Errorf("expected account 1 to be updated, got %d", accountRepo.updatedID)
	}
	newBalance, ok := accountRepo.updatedFields["raspolozivo_stanje"].(float64)
	if !ok {
		t.Fatal("raspolozivo_stanje not updated")
	}
	if newBalance != 4700 {
		t.Errorf("expected new balance=4700, got %f", newBalance)
	}
}

func TestCreatePayment_WithAddRecipient_CreatesRecipient(t *testing.T) {
	clientID := uint(7)
	accountRepo := &mockAccountRepo{accounts: map[uint]*models.Account{
		1: rsdAccount(1, 5000, &clientID),
	}}
	paymentRepo := newMockPaymentRepo()
	recipientRepo := &mockNewRecipientRepo{}
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, recipientRepo)

	_, err := svc.CreatePayment(service.CreatePaymentInput{
		RacunPosiljaocaID: 1,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             100,
		AddRecipient:      true,
		RecipientNaziv:    "Novi Primalac",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recipientRepo.created == nil {
		t.Fatal("expected recipient to be created, got nil")
	}
	if recipientRepo.created.ClientID != clientID {
		t.Errorf("expected recipient ClientID=%d, got %d", clientID, recipientRepo.created.ClientID)
	}
	if recipientRepo.created.Naziv != "Novi Primalac" {
		t.Errorf("expected recipient Naziv=Novi Primalac, got %s", recipientRepo.created.Naziv)
	}
}
