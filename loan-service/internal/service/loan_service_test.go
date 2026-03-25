package service_test

import (
	"errors"
	"math"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/service"
)

// --- pure-function tests (no mocks needed) ---

func TestCalculateInstallment_BasicFormula(t *testing.T) {
	// P=12000, annual=12%, n=12 → known annuity ≈ 1066.19
	result := service.CalculateInstallment(12000, 12.0, 12)
	expected := 1066.19
	if math.Abs(result-expected) > 0.05 {
		t.Errorf("CalculateInstallment(12000, 12%%, 12) = %.4f, want ≈ %.2f", result, expected)
	}
}

func TestCalculateInstallment_ZeroRate_DividesEvenly(t *testing.T) {
	// 0% interest → installment = P/n
	result := service.CalculateInstallment(12000, 0, 12)
	expected := 1000.0
	if math.Abs(result-expected) > 0.01 {
		t.Errorf("CalculateInstallment(12000, 0%%, 12) = %.4f, want 1000.00", result)
	}
}

func TestCalculateInstallment_OneMonth(t *testing.T) {
	// 1 month loan: installment = P + one month's interest
	// P=10000, annual=12% → monthly=1% → A = 10000*1.01 = 10100
	result := service.CalculateInstallment(10000, 12.0, 1)
	expected := 10100.0
	if math.Abs(result-expected) > 0.02 {
		t.Errorf("CalculateInstallment(10000, 12%%, 1) = %.4f, want ≈ 10100.00", result)
	}
}

func TestCalculateInstallment_IsPositive(t *testing.T) {
	result := service.CalculateInstallment(500000, 5.5, 60)
	if result <= 0 {
		t.Errorf("expected positive installment, got %f", result)
	}
}

// --- interest rate table tests ---

func TestBaseInterestRate_Under100k_Fiksna(t *testing.T) {
	rate := service.BaseInterestRate(50000, "fiksna")
	if rate != 6.5 {
		t.Errorf("expected 6.5%% for <100k fiksna, got %f", rate)
	}
}

func TestBaseInterestRate_100kTo500k_Fiksna(t *testing.T) {
	rate := service.BaseInterestRate(250000, "fiksna")
	if rate != 5.8 {
		t.Errorf("expected 5.8%% for 100k-500k fiksna, got %f", rate)
	}
}

func TestBaseInterestRate_500kTo1M_Fiksna(t *testing.T) {
	rate := service.BaseInterestRate(750000, "fiksna")
	if rate != 5.2 {
		t.Errorf("expected 5.2%% for 500k-1M fiksna, got %f", rate)
	}
}

func TestBaseInterestRate_1MTo5M_Fiksna(t *testing.T) {
	rate := service.BaseInterestRate(3000000, "fiksna")
	if rate != 4.5 {
		t.Errorf("expected 4.5%% for 1M-5M fiksna, got %f", rate)
	}
}

func TestBaseInterestRate_Over5M_Fiksna(t *testing.T) {
	rate := service.BaseInterestRate(6000000, "fiksna")
	if rate != 4.0 {
		t.Errorf("expected 4.0%% for >5M fiksna, got %f", rate)
	}
}

func TestBaseInterestRate_Under100k_Varijabilna(t *testing.T) {
	rate := service.BaseInterestRate(50000, "varijabilna")
	if rate != 4.5 {
		t.Errorf("expected 4.5%% for <100k varijabilna, got %f", rate)
	}
}

func TestBaseInterestRate_Over5M_Varijabilna(t *testing.T) {
	rate := service.BaseInterestRate(6000000, "varijabilna")
	if rate != 2.0 {
		t.Errorf("expected 2.0%% for >5M varijabilna, got %f", rate)
	}
}

// --- margin by vrsta tests ---

func TestMarginForVrsta_Gotovinski(t *testing.T) {
	if service.MarginForVrsta("gotovinski") != 1.5 {
		t.Errorf("expected margin=1.5 for gotovinski")
	}
}

func TestMarginForVrsta_Stambeni(t *testing.T) {
	if service.MarginForVrsta("stambeni") != 0.0 {
		t.Errorf("expected margin=0.0 for stambeni")
	}
}

func TestMarginForVrsta_Auto(t *testing.T) {
	if service.MarginForVrsta("auto") != 0.5 {
		t.Errorf("expected margin=0.5 for auto")
	}
}

func TestMarginForVrsta_Refinansirajuci(t *testing.T) {
	if service.MarginForVrsta("refinansirajuci") != 0.0 {
		t.Errorf("expected margin=0.0 for refinansirajuci")
	}
}

func TestMarginForVrsta_Studentski(t *testing.T) {
	if service.MarginForVrsta("studentski") != -0.5 {
		t.Errorf("expected margin=-0.5 for studentski")
	}
}

// --- mock repos ---

type mockLoanRepo struct {
	saved   *models.Loan
	findErr error
}

func (m *mockLoanRepo) Create(l *models.Loan) error { m.saved = l; l.ID = 1; return nil }
func (m *mockLoanRepo) FindByID(id uint) (*models.Loan, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	if m.saved != nil {
		return m.saved, nil
	}
	return nil, errors.New("not found")
}
func (m *mockLoanRepo) Save(l *models.Loan) error                                { m.saved = l; return nil }
func (m *mockLoanRepo) ListByClientID(_ uint) ([]models.Loan, error)             { return nil, nil }
func (m *mockLoanRepo) ListByStatus(_ string) ([]models.Loan, error)             { return nil, nil }
func (m *mockLoanRepo) ListFiltered(_ service.LoanFilter) ([]models.Loan, error) { return nil, nil }

type mockInstallmentRepo struct {
	batch []models.LoanInstallment
}

func (m *mockInstallmentRepo) CreateBatch(items []models.LoanInstallment) error {
	m.batch = items
	return nil
}
func (m *mockInstallmentRepo) ListByLoanID(_ uint) ([]models.LoanInstallment, error) {
	return m.batch, nil
}

type mockAccountRepo struct {
	account *models.Account
	err     error
}

func uintPtr(v uint) *uint { return &v }

func (m *mockAccountRepo) FindByBrojRacuna(_ string) (*models.Account, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.account, nil
}

func (m *mockAccountRepo) FindByID(_ uint) (*models.Account, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.account, nil
}

func (m *mockAccountRepo) UpdateFields(_ uint, _ map[string]interface{}) error { return nil }

func newSvc() (*service.LoanService, *mockLoanRepo, *mockInstallmentRepo, *mockAccountRepo) {
	lr := &mockLoanRepo{}
	ir := &mockInstallmentRepo{}
	ar := &mockAccountRepo{
		account: &models.Account{
			ID:                1,
			BrojRacuna:        "160000000000000002",
			ClientID:          uintPtr(1),
			CurrencyID:        1,
			CurrencyKod:       "RSD",
			Status:            "aktivan",
			Stanje:            100000,
			RaspolozivoStanje: 100000,
		},
	}
	return service.NewLoanService(nil, lr, ir, ar), lr, ir, ar
}

// --- RequestLoan tests ---

func TestRequestLoan_CreatesWithStatusZahtev(t *testing.T) {
	svc, lr, _, _ := newSvc()
	loan, err := svc.RequestLoan(service.CreateLoanInput{
		Vrsta: "gotovinski", BrojRacuna: "160000000000000002",
		Iznos: 100000, Period: 12, TipKamate: "fiksna",
		ClientID: 1, CurrencyID: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loan.Status != "zahtev" {
		t.Errorf("expected Status=zahtev, got %s", loan.Status)
	}
	if lr.saved == nil {
		t.Error("expected loan to be saved to repo")
	}
}

func TestRequestLoan_CalculatesIznosRate(t *testing.T) {
	svc, lr, _, _ := newSvc()
	_, err := svc.RequestLoan(service.CreateLoanInput{
		Vrsta: "gotovinski", BrojRacuna: "160000000000000002",
		Iznos: 100000, Period: 12, TipKamate: "fiksna",
		ClientID: 1, CurrencyID: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lr.saved.IznosRate <= 0 {
		t.Errorf("expected positive IznosRate, got %f", lr.saved.IznosRate)
	}
}

func TestRequestLoan_GeneratesBrojKredita(t *testing.T) {
	svc, lr, _, _ := newSvc()
	_, err := svc.RequestLoan(service.CreateLoanInput{
		Vrsta: "stambeni", BrojRacuna: "160000000000000002",
		Iznos: 500000, Period: 120, TipKamate: "varijabilna",
		ClientID: 1, CurrencyID: 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lr.saved.BrojKredita == "" {
		t.Error("expected non-empty BrojKredita")
	}
}

func TestRequestLoan_InvalidVrsta_ReturnsError(t *testing.T) {
	svc, _, _, _ := newSvc()
	_, err := svc.RequestLoan(service.CreateLoanInput{
		Vrsta: "invalid", BrojRacuna: "160000000000000002",
		Iznos: 100000, Period: 12, TipKamate: "fiksna",
		ClientID: 1, CurrencyID: 1,
	})
	if err == nil {
		t.Fatal("expected error for invalid vrsta, got nil")
	}
}

func TestRequestLoan_NegativeAmount_ReturnsError(t *testing.T) {
	svc, _, _, _ := newSvc()
	_, err := svc.RequestLoan(service.CreateLoanInput{
		Vrsta: "gotovinski", BrojRacuna: "160000000000000002",
		Iznos: -1000, Period: 12, TipKamate: "fiksna",
		ClientID: 1, CurrencyID: 1,
	})
	if err == nil {
		t.Fatal("expected error for negative amount, got nil")
	}
}

func TestRequestLoan_ZeroPeriod_ReturnsError(t *testing.T) {
	svc, _, _, _ := newSvc()
	_, err := svc.RequestLoan(service.CreateLoanInput{
		Vrsta: "gotovinski", BrojRacuna: "160000000000000002",
		Iznos: 100000, Period: 0, TipKamate: "fiksna",
		ClientID: 1, CurrencyID: 1,
	})
	if err == nil {
		t.Fatal("expected error for period=0, got nil")
	}
}

func TestRequestLoan_InvalidTipKamate_ReturnsError(t *testing.T) {
	svc, _, _, _ := newSvc()
	_, err := svc.RequestLoan(service.CreateLoanInput{
		Vrsta: "gotovinski", BrojRacuna: "160000000000000002",
		Iznos: 100000, Period: 12, TipKamate: "nepoznata",
		ClientID: 1, CurrencyID: 1,
	})
	if err == nil {
		t.Fatal("expected error for invalid tip kamate, got nil")
	}
}

func TestRequestLoan_RejectsAccountOwnedByAnotherClient(t *testing.T) {
	svc, _, _, ar := newSvc()
	ar.account.ClientID = uintPtr(2)

	_, err := svc.RequestLoan(service.CreateLoanInput{
		Vrsta: "gotovinski", BrojRacuna: "160000000000000002",
		Iznos: 100000, Period: 12, TipKamate: "fiksna",
		ClientID: 1, CurrencyID: 1,
	})
	if err == nil {
		t.Fatal("expected error for foreign payout account")
	}
}

func TestRequestLoan_RejectsNonRSDAccount(t *testing.T) {
	svc, _, _, ar := newSvc()
	ar.account.CurrencyKod = "EUR"
	ar.account.CurrencyID = 2

	_, err := svc.RequestLoan(service.CreateLoanInput{
		Vrsta: "gotovinski", BrojRacuna: "160000000000000002",
		Iznos: 100000, Period: 12, TipKamate: "fiksna",
		ClientID: 1, CurrencyID: 2,
	})
	if err == nil {
		t.Fatal("expected error for non-RSD payout account")
	}
}

// --- ApproveLoan tests ---

func TestApproveLoan_SetsStatusAktivan(t *testing.T) {
	svc, lr, _, _ := newSvc()
	lr.saved = &models.Loan{
		ID: 1, Status: "zahtev", Iznos: 100000, Period: 12,
		KamatnaStopa: 8.0, TipKamate: "fiksna", IznosRate: 8698,
	}
	loan, err := svc.ApproveLoan(1, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loan.Status != "aktivan" {
		t.Errorf("expected Status=aktivan, got %s", loan.Status)
	}
}

func TestApproveLoan_SetsZaposleniID(t *testing.T) {
	svc, lr, _, _ := newSvc()
	lr.saved = &models.Loan{
		ID: 1, Status: "zahtev", Iznos: 100000, Period: 12,
		KamatnaStopa: 8.0, TipKamate: "fiksna", IznosRate: 8698,
	}
	loan, err := svc.ApproveLoan(1, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loan.ZaposleniID == nil || *loan.ZaposleniID != 42 {
		t.Errorf("expected ZaposleniID=42, got %v", loan.ZaposleniID)
	}
}

func TestApproveLoan_GeneratesInstallments(t *testing.T) {
	svc, lr, ir, _ := newSvc()
	lr.saved = &models.Loan{
		ID: 1, Status: "zahtev", Iznos: 100000, Period: 12,
		KamatnaStopa: 8.0, TipKamate: "fiksna", IznosRate: 8698,
	}
	_, err := svc.ApproveLoan(1, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ir.batch) != 12 {
		t.Errorf("expected 12 installments, got %d", len(ir.batch))
	}
}

func TestApproveLoan_InstallmentsHaveCorrectRedniBroj(t *testing.T) {
	svc, lr, ir, _ := newSvc()
	lr.saved = &models.Loan{
		ID: 1, Status: "zahtev", Iznos: 60000, Period: 6,
		KamatnaStopa: 6.0, TipKamate: "fiksna", IznosRate: 10200,
	}
	_, err := svc.ApproveLoan(1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, inst := range ir.batch {
		if inst.RedniBroj != i+1 {
			t.Errorf("installment[%d] expected RedniBroj=%d, got %d", i, i+1, inst.RedniBroj)
		}
	}
}

func TestApproveLoan_InstallmentsStatusIsOcekuje(t *testing.T) {
	svc, lr, ir, _ := newSvc()
	lr.saved = &models.Loan{
		ID: 1, Status: "zahtev", Iznos: 60000, Period: 3,
		KamatnaStopa: 6.0, TipKamate: "fiksna", IznosRate: 20300,
	}
	_, err := svc.ApproveLoan(1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, inst := range ir.batch {
		if inst.Status != "ocekuje" {
			t.Errorf("expected installment Status=ocekuje, got %s", inst.Status)
		}
	}
}

func TestApproveLoan_NotZahtev_ReturnsError(t *testing.T) {
	svc, lr, _, _ := newSvc()
	lr.saved = &models.Loan{ID: 1, Status: "aktivan"}
	_, err := svc.ApproveLoan(1, 42)
	if err == nil {
		t.Fatal("expected error when approving non-zahtev loan")
	}
}

// --- RejectLoan tests ---

func TestRejectLoan_SetsStatusOdbijen(t *testing.T) {
	svc, lr, _, _ := newSvc()
	lr.saved = &models.Loan{ID: 1, Status: "zahtev"}
	loan, err := svc.RejectLoan(1, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loan.Status != "odbijen" {
		t.Errorf("expected Status=odbijen, got %s", loan.Status)
	}
}

func TestRejectLoan_SetsZaposleniID(t *testing.T) {
	svc, lr, _, _ := newSvc()
	lr.saved = &models.Loan{ID: 1, Status: "zahtev"}
	loan, err := svc.RejectLoan(1, 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loan.ZaposleniID == nil || *loan.ZaposleniID != 99 {
		t.Errorf("expected ZaposleniID=99, got %v", loan.ZaposleniID)
	}
}

func TestRejectLoan_NotZahtev_ReturnsError(t *testing.T) {
	svc, lr, _, _ := newSvc()
	lr.saved = &models.Loan{ID: 1, Status: "odbijen"}
	_, err := svc.RejectLoan(1, 42)
	if err == nil {
		t.Fatal("expected error when rejecting non-zahtev loan")
	}
}
