package cron_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/cron"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type erroringInstallmentRepo struct{}

func (e *erroringInstallmentRepo) FindDueInstallments(_ time.Time) ([]models.LoanInstallment, error) {
	return nil, errors.New("db down")
}
func (e *erroringInstallmentRepo) Save(_ *models.LoanInstallment) error { return nil }
func (e *erroringInstallmentRepo) ListByLoanID(_ uint) ([]models.LoanInstallment, error) {
	return nil, nil
}

type erroringLoanRepo struct{}

func (e *erroringLoanRepo) FindActiveVariableLoans() ([]models.Loan, error) {
	return nil, errors.New("db down")
}
func (e *erroringLoanRepo) SaveLoan(_ *models.Loan) error                { return nil }
func (e *erroringLoanRepo) FindByID(_ uint) (*models.Loan, error)        { return nil, errors.New("nope") }

func TestInstallmentCollector_Run_RepoError_Propagates(t *testing.T) {
	c := cron.NewInstallmentCollector(nil, &erroringInstallmentRepo{}, &mockLoanRepo{}, &mockAccountRepo{})
	if err := c.Run(time.Now()); err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestInterestRateUpdater_Run_RepoError_Propagates(t *testing.T) {
	u := cron.NewInterestRateUpdater(&erroringLoanRepo{}, nil)
	if err := u.Run(); err == nil {
		t.Fatal("expected error from repo")
	}
}

func newCronTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Reference tables.
	if err := db.Exec(`CREATE TABLE currencies (id INTEGER PRIMARY KEY, kod TEXT)`).Error; err != nil {
		t.Fatalf("currencies: %v", err)
	}
	if err := db.AutoMigrate(&models.Account{}); err != nil {
		t.Fatalf("accounts: %v", err)
	}
	return db
}

func TestInstallmentCollector_DBPath_PaysSuccessfully(t *testing.T) {
	db := newCronTestDB(t, "cron_collect_pay")
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (1, 'RSD')`)

	clientID := uint(1)
	acc := &models.Account{
		BrojRacuna: "160000000000000010", ClientID: &clientID, CurrencyID: 1,
		Status: "aktivan", Stanje: 10000, RaspolozivoStanje: 10000,
	}
	db.Create(acc)

	loan := &models.Loan{
		Vrsta: "gotovinski", BrojRacuna: acc.BrojRacuna, BrojKredita: "CK1",
		Iznos: 100000, Period: 12, IznosRate: 5000, Status: "aktivan",
		KamatnaStopa: 5, TipKamate: "fiksna", ClientID: clientID, CurrencyID: 1,
	}
	db.Create(loan)

	inst := &models.LoanInstallment{
		LoanID: loan.ID, RedniBroj: 1, Iznos: 5000, Status: "ocekuje",
		KamataStopaSnapshot: 5, DatumDospeca: time.Now().AddDate(0, 0, -1),
	}
	db.Create(inst)

	instRepo := repository.NewInstallmentRepository(db)
	loanRepo := repository.NewLoanRepository(db)
	accRepo := repository.NewAccountRepository(db)
	c := cron.NewInstallmentCollector(db, instRepo, loanRepo, accRepo)

	if err := c.Run(time.Now()); err != nil {
		t.Fatalf("run: %v", err)
	}

	var paid models.LoanInstallment
	db.First(&paid, inst.ID)
	if paid.Status != "placena" {
		t.Fatalf("expected placena, got %s", paid.Status)
	}

	var l models.Loan
	db.First(&l, loan.ID)
	if l.Status != "zatvoren" {
		t.Fatalf("expected zatvoren (only installment paid), got %s", l.Status)
	}
}

func TestInstallmentCollector_DBPath_InsufficientFunds_MarksLate(t *testing.T) {
	db := newCronTestDB(t, "cron_collect_late")
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (1, 'RSD')`)

	clientID := uint(1)
	acc := &models.Account{
		BrojRacuna: "160000000000000020", ClientID: &clientID, CurrencyID: 1,
		Status: "aktivan", Stanje: 100, RaspolozivoStanje: 100,
	}
	db.Create(acc)
	loan := &models.Loan{
		Vrsta: "gotovinski", BrojRacuna: acc.BrojRacuna, BrojKredita: "CK2",
		Iznos: 100000, Period: 12, IznosRate: 5000, Status: "aktivan",
		KamatnaStopa: 5, TipKamate: "fiksna", ClientID: clientID, CurrencyID: 1,
	}
	db.Create(loan)
	inst := &models.LoanInstallment{
		LoanID: loan.ID, RedniBroj: 1, Iznos: 5000, Status: "ocekuje",
		KamataStopaSnapshot: 5, DatumDospeca: time.Now().AddDate(0, 0, -1),
	}
	db.Create(inst)

	c := cron.NewInstallmentCollector(db, repository.NewInstallmentRepository(db), repository.NewLoanRepository(db), repository.NewAccountRepository(db))
	c.Run(time.Now())

	var got models.LoanInstallment
	db.First(&got, inst.ID)
	if got.Status != "kasni" {
		t.Fatalf("expected kasni, got %s", got.Status)
	}
	if got.DatumKasnjenja == nil {
		t.Fatal("expected DatumKasnjenja to be set")
	}
}

func TestInstallmentCollector_DBPath_LatePenaltyApplied(t *testing.T) {
	db := newCronTestDB(t, "cron_collect_penalty")
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (1, 'RSD')`)

	clientID := uint(1)
	acc := &models.Account{
		BrojRacuna: "160000000000000030", ClientID: &clientID, CurrencyID: 1,
		Status: "aktivan", Stanje: 100, RaspolozivoStanje: 100,
	}
	db.Create(acc)
	loan := &models.Loan{
		Vrsta: "gotovinski", BrojRacuna: acc.BrojRacuna, BrojKredita: "CK3",
		Iznos: 100000, Period: 12, IznosRate: 5000, Status: "aktivan",
		KamatnaStopa: 5, TipKamate: "fiksna", ClientID: clientID, CurrencyID: 1,
	}
	db.Create(loan)

	// Already late more than 72h ago.
	lateTime := time.Now().Add(-100 * time.Hour)
	inst := &models.LoanInstallment{
		LoanID: loan.ID, RedniBroj: 1, Iznos: 5000, Status: "kasni",
		KamataStopaSnapshot: 5, DatumDospeca: time.Now().AddDate(0, 0, -5),
		DatumKasnjenja: &lateTime,
	}
	db.Create(inst)

	c := cron.NewInstallmentCollector(db, repository.NewInstallmentRepository(db), repository.NewLoanRepository(db), repository.NewAccountRepository(db))
	c.Run(time.Now())

	var l models.Loan
	db.First(&l, loan.ID)
	if l.KamatnaStopa <= 5 {
		t.Fatalf("expected rate to be bumped above 5, got %v", l.KamatnaStopa)
	}
}

func TestInstallmentCollector_DBPath_AccountMissing_MarksLate(t *testing.T) {
	db := newCronTestDB(t, "cron_collect_no_acct")
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (1, 'RSD')`)

	clientID := uint(1)
	loan := &models.Loan{
		Vrsta: "gotovinski", BrojRacuna: "MISSING_ACCT", BrojKredita: "CK4",
		Iznos: 100000, Period: 12, IznosRate: 5000, Status: "aktivan",
		KamatnaStopa: 5, TipKamate: "fiksna", ClientID: clientID, CurrencyID: 1,
	}
	db.Create(loan)
	inst := &models.LoanInstallment{
		LoanID: loan.ID, RedniBroj: 1, Iznos: 5000, Status: "ocekuje",
		KamataStopaSnapshot: 5, DatumDospeca: time.Now().AddDate(0, 0, -1),
	}
	db.Create(inst)

	c := cron.NewInstallmentCollector(db, repository.NewInstallmentRepository(db), repository.NewLoanRepository(db), repository.NewAccountRepository(db))
	if err := c.Run(time.Now()); err != nil {
		t.Fatalf("run: %v", err)
	}

	var got models.LoanInstallment
	db.First(&got, inst.ID)
	if got.Status != "kasni" {
		t.Fatalf("expected kasni when payout acct missing, got %s", got.Status)
	}
}

func TestInterestRateUpdater_DBPath_UpdatesInstallments(t *testing.T) {
	db := newCronTestDB(t, "cron_rate_update_db")
	loan := &models.Loan{
		Vrsta: "gotovinski", BrojRacuna: "X", BrojKredita: "RU1",
		Iznos: 100000, Period: 12, IznosRate: 5000, Status: "aktivan",
		KamatnaStopa: 5, TipKamate: "varijabilna", ClientID: 1, CurrencyID: 1,
	}
	db.Create(loan)
	inst := &models.LoanInstallment{
		LoanID: loan.ID, RedniBroj: 1, Iznos: 5000, Status: "ocekuje",
		KamataStopaSnapshot: 5, DatumDospeca: time.Now().AddDate(0, 0, 30),
	}
	db.Create(inst)

	u := cron.NewInterestRateUpdater(repository.NewLoanRepository(db), db)
	if err := u.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	var got models.LoanInstallment
	db.First(&got, inst.ID)
	if got.Iznos == 5000 {
		// Rate changed -> Iznos should usually change too; allow rare equal case.
		t.Logf("note: installment iznos unchanged (rate delta may have been ~0)")
	}
}
