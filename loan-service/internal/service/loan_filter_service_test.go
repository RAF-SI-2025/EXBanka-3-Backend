package service_test

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/service"
)

// captureFilterLoanRepo records what filter was passed to ListFiltered.
type captureFilterLoanRepo struct {
	loans          []models.Loan
	capturedFilter service.LoanFilter
}

func (r *captureFilterLoanRepo) Create(l *models.Loan) error                  { l.ID = 1; return nil }
func (r *captureFilterLoanRepo) FindByID(_ uint) (*models.Loan, error)        { return nil, nil }
func (r *captureFilterLoanRepo) Save(l *models.Loan) error                    { return nil }
func (r *captureFilterLoanRepo) ListByClientID(_ uint) ([]models.Loan, error) { return r.loans, nil }
func (r *captureFilterLoanRepo) ListByStatus(_ string) ([]models.Loan, error) { return r.loans, nil }
func (r *captureFilterLoanRepo) ListFiltered(f service.LoanFilter) ([]models.Loan, error) {
	r.capturedFilter = f
	// apply simple in-memory filter so tests can verify results
	var result []models.Loan
	for _, l := range r.loans {
		if f.Vrsta != "" && l.Vrsta != f.Vrsta {
			continue
		}
		if f.BrojRacuna != "" && l.BrojRacuna != f.BrojRacuna {
			continue
		}
		if f.Status != "" && l.Status != f.Status {
			continue
		}
		result = append(result, l)
	}
	return result, nil
}

type captureFilterInstallmentRepo struct{}

func (r *captureFilterInstallmentRepo) CreateBatch(_ []models.LoanInstallment) error { return nil }
func (r *captureFilterInstallmentRepo) ListByLoanID(_ uint) ([]models.LoanInstallment, error) {
	return nil, nil
}

func newFilterSvc(loans []models.Loan) (*service.LoanService, *captureFilterLoanRepo) {
	lr := &captureFilterLoanRepo{loans: loans}
	return service.NewLoanService(nil, lr, &captureFilterInstallmentRepo{}, nil), lr
}

// --- ListRequestsFiltered tests ---

func TestListRequestsFiltered_NoFilter_ReturnsAll(t *testing.T) {
	loans := []models.Loan{
		{ID: 1, Status: "zahtev", Vrsta: "gotovinski"},
		{ID: 2, Status: "zahtev", Vrsta: "stambeni"},
	}
	svc, _ := newFilterSvc(loans)
	result, err := svc.ListRequestsFiltered("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
}

func TestListRequestsFiltered_ByVrsta_FiltersResults(t *testing.T) {
	loans := []models.Loan{
		{ID: 1, Status: "zahtev", Vrsta: "gotovinski"},
		{ID: 2, Status: "zahtev", Vrsta: "stambeni"},
	}
	svc, _ := newFilterSvc(loans)
	result, err := svc.ListRequestsFiltered("gotovinski", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
}

func TestListRequestsFiltered_AlwaysFiltersStatusZahtev(t *testing.T) {
	loans := []models.Loan{} // repo mock always enforces status filter via LoanFilter.Status
	svc, lr := newFilterSvc(loans)
	svc.ListRequestsFiltered("", "")
	if lr.capturedFilter.Status != "zahtev" {
		t.Errorf("expected Status filter='zahtev', got %q", lr.capturedFilter.Status)
	}
}

func TestListRequestsFiltered_ByBrojRacuna_PassedToRepo(t *testing.T) {
	svc, lr := newFilterSvc(nil)
	svc.ListRequestsFiltered("", "160000000000000002")
	if lr.capturedFilter.BrojRacuna != "160000000000000002" {
		t.Errorf("expected BrojRacuna filter passed, got %q", lr.capturedFilter.BrojRacuna)
	}
}

// --- ListAllFiltered tests ---

func TestListAllFiltered_NoFilter_ReturnsAll(t *testing.T) {
	loans := []models.Loan{
		{ID: 1, Status: "aktivan", Vrsta: "auto"},
		{ID: 2, Status: "zatvoren", Vrsta: "stambeni"},
		{ID: 3, Status: "zahtev", Vrsta: "gotovinski"},
	}
	svc, _ := newFilterSvc(loans)
	result, err := svc.ListAllFiltered(service.LoanFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 results, got %d", len(result))
	}
}

func TestListAllFiltered_ByStatus_FiltersResults(t *testing.T) {
	loans := []models.Loan{
		{ID: 1, Status: "aktivan"},
		{ID: 2, Status: "zatvoren"},
		{ID: 3, Status: "aktivan"},
	}
	svc, _ := newFilterSvc(loans)
	result, err := svc.ListAllFiltered(service.LoanFilter{Status: "aktivan"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 active loans, got %d", len(result))
	}
}

func TestListAllFiltered_ByVrstaAndStatus_CombinesFilters(t *testing.T) {
	loans := []models.Loan{
		{ID: 1, Status: "aktivan", Vrsta: "gotovinski"},
		{ID: 2, Status: "aktivan", Vrsta: "stambeni"},
		{ID: 3, Status: "zatvoren", Vrsta: "gotovinski"},
	}
	svc, _ := newFilterSvc(loans)
	result, err := svc.ListAllFiltered(service.LoanFilter{Status: "aktivan", Vrsta: "gotovinski"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result for aktivan+gotovinski, got %d", len(result))
	}
}

func TestListAllFiltered_FilterPassedToRepo(t *testing.T) {
	svc, lr := newFilterSvc(nil)
	svc.ListAllFiltered(service.LoanFilter{Status: "aktivan", Vrsta: "auto", BrojRacuna: "123"})
	if lr.capturedFilter.Status != "aktivan" {
		t.Errorf("expected Status=aktivan, got %q", lr.capturedFilter.Status)
	}
	if lr.capturedFilter.Vrsta != "auto" {
		t.Errorf("expected Vrsta=auto, got %q", lr.capturedFilter.Vrsta)
	}
	if lr.capturedFilter.BrojRacuna != "123" {
		t.Errorf("expected BrojRacuna=123, got %q", lr.capturedFilter.BrojRacuna)
	}
}
