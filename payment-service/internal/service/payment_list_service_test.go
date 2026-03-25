package service_test

import (
	"errors"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/service"
)

// mockListPaymentRepo supports list/get operations with controllable results.
type mockListPaymentRepo struct {
	payments          map[uint]*models.Payment
	byAccountResult   []models.Payment
	byAccountTotal    int64
	byClientResult    []models.Payment
	byClientTotal     int64
	capturedAccFilter models.PaymentFilter
	capturedCliFilter models.PaymentFilter
	listErr           error
	nextID            uint
}

func newMockListPaymentRepo() *mockListPaymentRepo {
	return &mockListPaymentRepo{payments: make(map[uint]*models.Payment), nextID: 1}
}

func (m *mockListPaymentRepo) Create(p *models.Payment) error {
	p.ID = m.nextID
	m.nextID++
	m.payments[p.ID] = p
	return nil
}

func (m *mockListPaymentRepo) FindByID(id uint) (*models.Payment, error) {
	if p, ok := m.payments[id]; ok {
		return p, nil
	}
	return nil, errors.New("payment not found")
}

func (m *mockListPaymentRepo) Save(p *models.Payment) error {
	m.payments[p.ID] = p
	return nil
}

func (m *mockListPaymentRepo) ListByAccountID(accountID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	m.capturedAccFilter = filter
	return m.byAccountResult, m.byAccountTotal, m.listErr
}

func (m *mockListPaymentRepo) ListByClientID(clientID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	m.capturedCliFilter = filter
	return m.byClientResult, m.byClientTotal, m.listErr
}

// stubAccountRepo satisfies PaymentAccountRepositoryInterface for list tests.
type stubAccountRepo struct{}

func (s *stubAccountRepo) FindByID(id uint) (*models.Account, error) {
	return &models.Account{ID: id, RaspolozivoStanje: 10000, Stanje: 10000}, nil
}
func (s *stubAccountRepo) FindByBrojRacuna(brojRacuna string) (*models.Account, error) {
	return &models.Account{ID: 1, BrojRacuna: brojRacuna, RaspolozivoStanje: 10000, Stanje: 10000}, nil
}
func (s *stubAccountRepo) UpdateFields(id uint, fields map[string]interface{}) error { return nil }

// --- tests ---

func TestGetPayment_ReturnsPayment(t *testing.T) {
	repo := newMockListPaymentRepo()
	repo.payments[5] = &models.Payment{ID: 5, Iznos: 250, Status: "uspesno"}
	svc := service.NewPaymentServiceWithRepos(&stubAccountRepo{}, repo, nil, nil)

	p, err := svc.GetPayment(5)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID != 5 {
		t.Errorf("expected ID=5, got %d", p.ID)
	}
	if p.Iznos != 250 {
		t.Errorf("expected Iznos=250, got %f", p.Iznos)
	}
}

func TestGetPayment_NotFound_ReturnsError(t *testing.T) {
	repo := newMockListPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(&stubAccountRepo{}, repo, nil, nil)

	_, err := svc.GetPayment(99)

	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}

func TestListPaymentsByAccount_ReturnsPayments(t *testing.T) {
	payments := []models.Payment{{ID: 1, Status: "uspesno"}, {ID: 2, Status: "u_obradi"}}
	repo := newMockListPaymentRepo()
	repo.byAccountResult = payments
	repo.byAccountTotal = 2
	svc := service.NewPaymentServiceWithRepos(&stubAccountRepo{}, repo, nil, nil)

	result, total, err := svc.ListPaymentsByAccount(10, models.PaymentFilter{Page: 1, PageSize: 20})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 payments, got %d", len(result))
	}
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
}

func TestListPaymentsByAccount_FilterByStatus(t *testing.T) {
	repo := newMockListPaymentRepo()
	repo.byAccountResult = []models.Payment{{ID: 1, Status: "uspesno"}}
	repo.byAccountTotal = 1
	svc := service.NewPaymentServiceWithRepos(&stubAccountRepo{}, repo, nil, nil)

	svc.ListPaymentsByAccount(10, models.PaymentFilter{Status: "uspesno", Page: 1, PageSize: 10})

	if repo.capturedAccFilter.Status != "uspesno" {
		t.Errorf("expected Status filter=uspesno, got %q", repo.capturedAccFilter.Status)
	}
}

func TestListPaymentsByClient_ReturnsPayments(t *testing.T) {
	payments := []models.Payment{{ID: 3}, {ID: 4}, {ID: 5}}
	repo := newMockListPaymentRepo()
	repo.byClientResult = payments
	repo.byClientTotal = 3
	svc := service.NewPaymentServiceWithRepos(&stubAccountRepo{}, repo, nil, nil)

	result, total, err := svc.ListPaymentsByClient(7, models.PaymentFilter{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 payments, got %d", len(result))
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
}

func TestListPaymentsByClient_PaginationPassedThrough(t *testing.T) {
	repo := newMockListPaymentRepo()
	svc := service.NewPaymentServiceWithRepos(&stubAccountRepo{}, repo, nil, nil)

	svc.ListPaymentsByClient(7, models.PaymentFilter{Page: 2, PageSize: 5})

	if repo.capturedCliFilter.Page != 2 {
		t.Errorf("expected Page=2, got %d", repo.capturedCliFilter.Page)
	}
	if repo.capturedCliFilter.PageSize != 5 {
		t.Errorf("expected PageSize=5, got %d", repo.capturedCliFilter.PageSize)
	}
}
