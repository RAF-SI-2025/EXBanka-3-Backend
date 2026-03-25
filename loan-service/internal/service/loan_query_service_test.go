package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/service"
)

// Extended mocks that also implement the query methods.

type queryLoanRepo struct {
	loans []models.Loan
	saved *models.Loan
}

func (r *queryLoanRepo) Create(l *models.Loan) error { r.saved = l; l.ID = 1; return nil }
func (r *queryLoanRepo) FindByID(_ uint) (*models.Loan, error) {
	if len(r.loans) > 0 {
		return &r.loans[0], nil
	}
	return nil, nil
}
func (r *queryLoanRepo) Save(l *models.Loan) error                    { r.saved = l; return nil }
func (r *queryLoanRepo) ListByClientID(_ uint) ([]models.Loan, error) { return r.loans, nil }
func (r *queryLoanRepo) ListByStatus(_ string) ([]models.Loan, error) { return r.loans, nil }
func (r *queryLoanRepo) ListFiltered(_ service.LoanFilter) ([]models.Loan, error) {
	return r.loans, nil
}

type queryInstallmentRepo struct {
	items []models.LoanInstallment
}

func (r *queryInstallmentRepo) CreateBatch(items []models.LoanInstallment) error {
	r.items = items
	return nil
}
func (r *queryInstallmentRepo) ListByLoanID(_ uint) ([]models.LoanInstallment, error) {
	return r.items, nil
}

func newQuerySvc(loans []models.Loan, installments []models.LoanInstallment) *service.LoanService {
	return service.NewLoanService(nil, &queryLoanRepo{loans: loans}, &queryInstallmentRepo{items: installments}, nil)
}

// --- ListByClient tests ---

func TestListByClient_ReturnsAllLoans(t *testing.T) {
	loans := []models.Loan{
		{ID: 1, ClientID: 5, Iznos: 100000, Status: "aktivan"},
		{ID: 2, ClientID: 5, Iznos: 50000, Status: "zatvoren"},
	}
	svc := newQuerySvc(loans, nil)
	result, err := svc.ListByClient(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 loans, got %d", len(result))
	}
}

func TestListByClient_SortedDescByIznos(t *testing.T) {
	loans := []models.Loan{
		{ID: 1, Iznos: 50000},
		{ID: 2, Iznos: 200000},
		{ID: 3, Iznos: 100000},
	}
	svc := newQuerySvc(loans, nil)
	result, err := svc.ListByClient(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Iznos < result[1].Iznos || result[1].Iznos < result[2].Iznos {
		t.Errorf("expected descending order by Iznos, got %v %v %v",
			result[0].Iznos, result[1].Iznos, result[2].Iznos)
	}
}

func TestListByClient_Empty_ReturnsEmptySlice(t *testing.T) {
	svc := newQuerySvc(nil, nil)
	result, err := svc.ListByClient(99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil slice for empty result")
	}
}

// --- GetByID tests ---

func TestGetByID_ReturnsLoan(t *testing.T) {
	loans := []models.Loan{{ID: 7, ClientID: 1, Iznos: 300000, Status: "aktivan"}}
	svc := newQuerySvc(loans, nil)
	loan, err := svc.GetByID(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loan.ID != 7 {
		t.Errorf("expected loan ID=7, got %d", loan.ID)
	}
}

func TestGetByID_PreloadsInstallments(t *testing.T) {
	loans := []models.Loan{{ID: 7, ClientID: 1, Iznos: 100000, Status: "aktivan", Period: 3}}
	installments := []models.LoanInstallment{
		{ID: 1, LoanID: 7, RedniBroj: 1, Status: "placena"},
		{ID: 2, LoanID: 7, RedniBroj: 2, Status: "ocekuje"},
		{ID: 3, LoanID: 7, RedniBroj: 3, Status: "ocekuje"},
	}
	svc := newQuerySvc(loans, installments)
	loan, err := svc.GetByID(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(loan.Installments) != 3 {
		t.Errorf("expected 3 installments preloaded, got %d", len(loan.Installments))
	}
}

// --- ListInstallments tests ---

func TestListInstallments_ReturnsAll(t *testing.T) {
	installments := []models.LoanInstallment{
		{ID: 1, LoanID: 5, RedniBroj: 1, Status: "placena"},
		{ID: 2, LoanID: 5, RedniBroj: 2, Status: "ocekuje"},
	}
	svc := newQuerySvc(nil, installments)
	result, err := svc.ListInstallments(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 installments, got %d", len(result))
	}
}

// --- RemainingDebt tests ---

func TestRemainingDebt_SumsUnpaidInstallments(t *testing.T) {
	installments := []models.LoanInstallment{
		{Status: "placena", Iznos: 1000},
		{Status: "ocekuje", Iznos: 1000},
		{Status: "kasni", Iznos: 1000},
		{Status: "ocekuje", Iznos: 1000},
	}
	debt := service.RemainingDebt(installments)
	if debt != 3000 {
		t.Errorf("expected remaining debt=3000, got %f", debt)
	}
}

func TestRemainingDebt_AllPaid_ReturnsZero(t *testing.T) {
	installments := []models.LoanInstallment{
		{Status: "placena", Iznos: 500},
		{Status: "placena", Iznos: 500},
	}
	debt := service.RemainingDebt(installments)
	if debt != 0 {
		t.Errorf("expected 0 remaining debt, got %f", debt)
	}
}

// --- NextInstallment tests ---

func TestNextInstallment_ReturnsEarliestOcekuje(t *testing.T) {
	now := time.Now()
	installments := []models.LoanInstallment{
		{RedniBroj: 2, Status: "ocekuje", DatumDospeca: now.AddDate(0, 2, 0), Iznos: 1000},
		{RedniBroj: 1, Status: "ocekuje", DatumDospeca: now.AddDate(0, 1, 0), Iznos: 1000},
		{RedniBroj: 3, Status: "ocekuje", DatumDospeca: now.AddDate(0, 3, 0), Iznos: 1000},
	}
	next := service.NextInstallment(installments)
	if next == nil {
		t.Fatal("expected non-nil next installment")
	}
	if next.RedniBroj != 1 {
		t.Errorf("expected RedniBroj=1 (earliest), got %d", next.RedniBroj)
	}
}

func TestNextInstallment_SkipsPaidInstallments(t *testing.T) {
	now := time.Now()
	installments := []models.LoanInstallment{
		{RedniBroj: 1, Status: "placena", DatumDospeca: now.AddDate(0, 1, 0), Iznos: 1000},
		{RedniBroj: 2, Status: "ocekuje", DatumDospeca: now.AddDate(0, 2, 0), Iznos: 1000},
	}
	next := service.NextInstallment(installments)
	if next == nil {
		t.Fatal("expected non-nil next installment")
	}
	if next.RedniBroj != 2 {
		t.Errorf("expected RedniBroj=2 (skipping paid), got %d", next.RedniBroj)
	}
}

func TestNextInstallment_AllPaid_ReturnsNil(t *testing.T) {
	installments := []models.LoanInstallment{
		{Status: "placena", Iznos: 1000},
	}
	next := service.NextInstallment(installments)
	if next != nil {
		t.Errorf("expected nil next installment when all paid, got %+v", next)
	}
}
