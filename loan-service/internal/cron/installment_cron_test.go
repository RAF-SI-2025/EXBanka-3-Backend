package cron_test

import (
	"errors"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/cron"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
)

// --- mock installment repo ---

type mockInstallmentRepo struct {
	due     []models.LoanInstallment
	saved   []*models.LoanInstallment
	saveErr error
	byLoan  map[uint][]models.LoanInstallment
}

func (m *mockInstallmentRepo) FindDueInstallments(asOf time.Time) ([]models.LoanInstallment, error) {
	return m.due, nil
}

func (m *mockInstallmentRepo) Save(inst *models.LoanInstallment) error {
	m.saved = append(m.saved, inst)
	return m.saveErr
}

func (m *mockInstallmentRepo) ListByLoanID(loanID uint) ([]models.LoanInstallment, error) {
	if m.byLoan == nil {
		return nil, nil
	}
	return m.byLoan[loanID], nil
}

// --- mock loan repo (for interest rate cron) ---

type mockLoanRepo struct {
	loans   []models.Loan
	saved   []*models.Loan
	saveErr error
}

func (m *mockLoanRepo) FindActiveVariableLoans() ([]models.Loan, error) {
	return m.loans, nil
}

func (m *mockLoanRepo) SaveLoan(loan *models.Loan) error {
	m.saved = append(m.saved, loan)
	return m.saveErr
}

func (m *mockLoanRepo) FindByID(id uint) (*models.Loan, error) {
	for i := range m.loans {
		if m.loans[i].ID == id {
			return &m.loans[i], nil
		}
	}
	return nil, errors.New("not found")
}

type mockAccountRepo struct {
	account *models.Account
	updates []map[string]interface{}
}

func (m *mockAccountRepo) FindByBrojRacuna(_ string) (*models.Account, error) {
	if m.account == nil {
		return nil, errors.New("not found")
	}
	return m.account, nil
}

func (m *mockAccountRepo) UpdateFields(_ uint, fields map[string]interface{}) error {
	m.updates = append(m.updates, fields)
	return nil
}

// --- InstallmentCollector tests ---

func TestInstallmentCollector_NoDueInstallments_DoesNothing(t *testing.T) {
	repo := &mockInstallmentRepo{due: nil}
	c := cron.NewInstallmentCollector(nil, repo, &mockLoanRepo{}, &mockAccountRepo{})
	if err := c.Run(time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.saved) != 0 {
		t.Errorf("expected no saves, got %d", len(repo.saved))
	}
}

func TestInstallmentCollector_DueInstallment_MarkedPlacena(t *testing.T) {
	inst := models.LoanInstallment{
		ID:           1,
		LoanID:       10,
		Status:       "ocekuje",
		DatumDospeca: time.Now().AddDate(0, 0, -1),
		Iznos:        5000,
	}
	repo := &mockInstallmentRepo{
		due:    []models.LoanInstallment{inst},
		byLoan: map[uint][]models.LoanInstallment{10: {inst}},
	}
	accountRepo := &mockAccountRepo{account: &models.Account{
		ID:                1,
		BrojRacuna:        "160000000000000002",
		Status:            "aktivan",
		Stanje:            10000,
		RaspolozivoStanje: 10000,
		DnevnaPotrosnja:   0,
		MesecnaPotrosnja:  0,
	}}
	loanRepo := &mockLoanRepo{loans: []models.Loan{{ID: 10, BrojRacuna: "160000000000000002", Status: "aktivan"}}}
	c := cron.NewInstallmentCollector(nil, repo, loanRepo, accountRepo)
	if err := c.Run(time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 save, got %d", len(repo.saved))
	}
	if repo.saved[0].Status != "placena" {
		t.Errorf("expected status=placena, got %s", repo.saved[0].Status)
	}
	if repo.saved[0].DatumPlacanja == nil {
		t.Error("expected DatumPlacanja to be set")
	}
	if len(accountRepo.updates) != 1 {
		t.Fatalf("expected one account update, got %d", len(accountRepo.updates))
	}
	if got := accountRepo.updates[0]["raspolozivo_stanje"]; got != 5000.0 {
		t.Errorf("expected updated available balance 5000, got %v", got)
	}
	if len(loanRepo.saved) != 1 || loanRepo.saved[0].Status != "zatvoren" {
		t.Errorf("expected loan to close after last installment, got %+v", loanRepo.saved)
	}
}

func TestInstallmentCollector_DueInstallment_DatumPlacanjaSetToToday(t *testing.T) {
	before := time.Now()
	inst := models.LoanInstallment{
		ID: 2, LoanID: 10, Status: "ocekuje",
		DatumDospeca: before.AddDate(0, 0, -3),
	}
	repo := &mockInstallmentRepo{due: []models.LoanInstallment{inst}}
	c := cron.NewInstallmentCollector(nil, repo, &mockLoanRepo{loans: []models.Loan{{ID: 10, BrojRacuna: "160000000000000002"}}}, &mockAccountRepo{account: &models.Account{
		ID:                1,
		BrojRacuna:        "160000000000000002",
		Status:            "aktivan",
		Stanje:            10000,
		RaspolozivoStanje: 10000,
	}})
	c.Run(before)
	if repo.saved[0].DatumPlacanja == nil {
		t.Fatal("DatumPlacanja is nil")
	}
	diff := repo.saved[0].DatumPlacanja.Sub(before)
	if diff < 0 || diff > 5*time.Second {
		t.Errorf("DatumPlacanja not close to now: %v", *repo.saved[0].DatumPlacanja)
	}
}

func TestInstallmentCollector_MultipleInstallments_AllProcessed(t *testing.T) {
	due := []models.LoanInstallment{
		{ID: 1, LoanID: 10, Status: "ocekuje", DatumDospeca: time.Now().AddDate(0, 0, -1)},
		{ID: 2, LoanID: 11, Status: "ocekuje", DatumDospeca: time.Now().AddDate(0, 0, -2)},
		{ID: 3, LoanID: 12, Status: "ocekuje", DatumDospeca: time.Now().AddDate(0, 0, -3)},
	}
	repo := &mockInstallmentRepo{due: due}
	c := cron.NewInstallmentCollector(nil, repo, &mockLoanRepo{loans: []models.Loan{
		{ID: 10, BrojRacuna: "160000000000000010"},
		{ID: 11, BrojRacuna: "160000000000000011"},
		{ID: 12, BrojRacuna: "160000000000000012"},
	}}, &mockAccountRepo{account: &models.Account{
		ID:                1,
		BrojRacuna:        "160000000000000010",
		Status:            "aktivan",
		Stanje:            50000,
		RaspolozivoStanje: 50000,
	}})
	c.Run(time.Now())
	if len(repo.saved) != 3 {
		t.Errorf("expected 3 saves, got %d", len(repo.saved))
	}
}

func TestInstallmentCollector_InsufficientFunds_MarkedKasni(t *testing.T) {
	inst := models.LoanInstallment{
		ID:           3,
		LoanID:       10,
		Status:       "ocekuje",
		DatumDospeca: time.Now().AddDate(0, 0, -1),
		Iznos:        5000,
	}
	repo := &mockInstallmentRepo{due: []models.LoanInstallment{inst}}
	accountRepo := &mockAccountRepo{account: &models.Account{
		ID:                1,
		BrojRacuna:        "160000000000000002",
		Status:            "aktivan",
		Stanje:            1000,
		RaspolozivoStanje: 1000,
	}}
	loanRepo := &mockLoanRepo{loans: []models.Loan{{ID: 10, BrojRacuna: "160000000000000002", Status: "aktivan"}}}
	c := cron.NewInstallmentCollector(nil, repo, loanRepo, accountRepo)
	if err := c.Run(time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 save, got %d", len(repo.saved))
	}
	if repo.saved[0].Status != "kasni" {
		t.Errorf("expected status=kasni, got %s", repo.saved[0].Status)
	}
	if len(accountRepo.updates) != 0 {
		t.Errorf("expected no account updates, got %d", len(accountRepo.updates))
	}
}

// --- InterestRateUpdater tests ---

func TestInterestRateUpdater_NoLoans_DoesNothing(t *testing.T) {
	lrepo := &mockLoanRepo{loans: nil}
	u := cron.NewInterestRateUpdater(lrepo)
	if err := u.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lrepo.saved) != 0 {
		t.Errorf("expected no saves, got %d", len(lrepo.saved))
	}
}

func TestInterestRateUpdater_VariableLoan_RateUpdated(t *testing.T) {
	loan := models.Loan{
		ID: 1, TipKamate: "varijabilna", KamatnaStopa: 5.0,
		Iznos: 120000, Period: 12, IznosRate: 10250,
	}
	lrepo := &mockLoanRepo{loans: []models.Loan{loan}}
	u := cron.NewInterestRateUpdater(lrepo)
	u.Run()
	if len(lrepo.saved) != 1 {
		t.Fatalf("expected 1 save, got %d", len(lrepo.saved))
	}
	updated := lrepo.saved[0]
	// Rate must be within [-1.5, +1.5] of original
	delta := updated.KamatnaStopa - 5.0
	if delta < -1.5 || delta > 1.5 {
		t.Errorf("rate delta %f out of [-1.5, +1.5] range", delta)
	}
}

func TestInterestRateUpdater_VariableLoan_IznosRateRecalculated(t *testing.T) {
	loan := models.Loan{
		ID: 1, TipKamate: "varijabilna", KamatnaStopa: 6.0,
		Iznos: 120000, Period: 24, IznosRate: 5320,
	}
	lrepo := &mockLoanRepo{loans: []models.Loan{loan}}
	u := cron.NewInterestRateUpdater(lrepo)
	u.Run()
	// IznosRate must be recalculated (should differ from original if rate changed)
	// At minimum, it must be positive
	if lrepo.saved[0].IznosRate <= 0 {
		t.Errorf("expected positive IznosRate, got %f", lrepo.saved[0].IznosRate)
	}
}

func TestInterestRateUpdater_RateCannotGoBelowZero(t *testing.T) {
	loan := models.Loan{
		ID: 1, TipKamate: "varijabilna", KamatnaStopa: 0.5,
		Iznos: 10000, Period: 6, IznosRate: 1700,
	}
	lrepo := &mockLoanRepo{loans: []models.Loan{loan}}
	u := cron.NewInterestRateUpdater(lrepo)
	for range 20 { // run many times to test floor
		lrepo.saved = nil
		lrepo.loans[0].KamatnaStopa = loan.KamatnaStopa
		u.Run()
		if lrepo.saved[0].KamatnaStopa < 0 {
			t.Errorf("rate went below 0: %f", lrepo.saved[0].KamatnaStopa)
		}
	}
}
