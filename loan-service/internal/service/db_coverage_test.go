package service_test

import (
	"fmt"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/service"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newLoanTestDB sets up a sqlite DB with the loan-service-managed tables plus the
// reference tables (currencies, firmas, clients, accounts) needed for ApproveLoan's
// DB transaction path.
func newLoanTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Create the reference tables loan-service reads.
	stmts := []string{
		`CREATE TABLE currencies (id INTEGER PRIMARY KEY, kod TEXT)`,
		`CREATE TABLE firmas (id INTEGER PRIMARY KEY, is_state BOOLEAN DEFAULT FALSE)`,
		`CREATE TABLE clients (id INTEGER PRIMARY KEY, email TEXT, ime TEXT, prezime TEXT)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("create ref tbl: %v (%s)", err, s)
		}
	}
	if err := db.AutoMigrate(&models.Account{}); err != nil {
		t.Fatalf("migrate accounts: %v", err)
	}
	// loan-service's reference Account model omits firma_id, but the ApproveLoan
	// transaction filters on it; add the column explicitly for these tests.
	if err := db.Exec(`ALTER TABLE accounts ADD COLUMN firma_id INTEGER`).Error; err != nil {
		t.Fatalf("alter accounts: %v", err)
	}
	return db
}

func TestApproveLoan_DBPath_Success(t *testing.T) {
	db := newLoanTestDB(t, "loan_approve_db_ok")
	clientID := uint(1)

	// seed currency, client, firma (bank's own), client account, bank account.
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (1, 'RSD')`)
	db.Exec(`INSERT INTO clients (id, email, ime, prezime) VALUES (?, ?, ?, ?)`, clientID, "c@b.com", "I", "P")
	db.Exec(`INSERT INTO firmas (id, is_state) VALUES (?, ?)`, 7, false)

	clientAcc := &models.Account{
		BrojRacuna: "160000000000000002", ClientID: &clientID, CurrencyID: 1,
		Status: "aktivan", Stanje: 0, RaspolozivoStanje: 0,
	}
	if err := db.Create(clientAcc).Error; err != nil {
		t.Fatalf("seed client acct: %v", err)
	}
	// Bank's own account: firma_id set, client_id nil, currency_id=1 (RSD), linked to non-state firma.
	if err := db.Exec(
		`INSERT INTO accounts (broj_racuna, currency_id, stanje, raspolozivo_stanje, status, firma_id) VALUES (?, ?, ?, ?, ?, ?)`,
		"BANK_RSD", 1, 1_000_000.0, 1_000_000.0, "aktivan", 7,
	).Error; err != nil {
		t.Fatalf("seed bank acct: %v", err)
	}

	loanRepo := repository.NewLoanRepository(db)
	instRepo := repository.NewInstallmentRepository(db)
	accRepo := repository.NewAccountRepository(db)

	svc := service.NewLoanServiceWithNotifier(db, loanRepo, instRepo, accRepo, nil)

	loan, err := svc.RequestLoan(service.CreateLoanInput{
		Vrsta: "gotovinski", BrojRacuna: clientAcc.BrojRacuna,
		Iznos: 100000, Period: 12, TipKamate: "fiksna",
		ClientID: clientID, CurrencyID: 1,
		SvrhaKredita: "x", IznosMesecnePlate: 50000, StatusZaposlenja: "stalno", PeriodZaposlenja: "5", KontaktTelefon: "0611234567",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	approved, err := svc.ApproveLoan(loan.ID, 42)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != "aktivan" {
		t.Fatalf("expected aktivan, got %s", approved.Status)
	}

	var inst []models.LoanInstallment
	db.Where("loan_id = ?", approved.ID).Find(&inst)
	if len(inst) == 0 {
		t.Fatal("expected installments to be created")
	}
}

func TestApproveLoan_DBPath_LoanNotFound(t *testing.T) {
	db := newLoanTestDB(t, "loan_approve_db_notfound")
	loanRepo := repository.NewLoanRepository(db)
	instRepo := repository.NewInstallmentRepository(db)
	accRepo := repository.NewAccountRepository(db)
	svc := service.NewLoanService(db, loanRepo, instRepo, accRepo)
	if _, err := svc.ApproveLoan(9999, 1); err == nil {
		t.Fatal("expected loan-not-found error")
	}
}

func TestApproveLoan_DBPath_LoanNotPending(t *testing.T) {
	db := newLoanTestDB(t, "loan_approve_db_notpending")
	db.Create(&models.Loan{Status: "odbijen", Vrsta: "gotovinski", BrojRacuna: "X", Iznos: 1000, Period: 12, IznosRate: 100, BrojKredita: "K1", KamatnaStopa: 5, TipKamate: "fiksna", ClientID: 1, CurrencyID: 1})

	loanRepo := repository.NewLoanRepository(db)
	instRepo := repository.NewInstallmentRepository(db)
	accRepo := repository.NewAccountRepository(db)
	svc := service.NewLoanService(db, loanRepo, instRepo, accRepo)

	var loan models.Loan
	db.First(&loan)
	if _, err := svc.ApproveLoan(loan.ID, 1); err == nil {
		t.Fatal("expected status error")
	}
}

func TestApproveLoan_DBPath_PayoutAccountMissing(t *testing.T) {
	db := newLoanTestDB(t, "loan_approve_db_no_payout")
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (1, 'RSD')`)
	db.Create(&models.Loan{Status: "zahtev", Vrsta: "gotovinski", BrojRacuna: "MISSING", Iznos: 1000, Period: 12, IznosRate: 100, BrojKredita: "K2", KamatnaStopa: 5, TipKamate: "fiksna", ClientID: 1, CurrencyID: 1})

	loanRepo := repository.NewLoanRepository(db)
	instRepo := repository.NewInstallmentRepository(db)
	accRepo := repository.NewAccountRepository(db)
	svc := service.NewLoanService(db, loanRepo, instRepo, accRepo)

	var loan models.Loan
	db.First(&loan)
	if _, err := svc.ApproveLoan(loan.ID, 1); err == nil {
		t.Fatal("expected payout-not-found error")
	}
}

func TestNewNotificationService_Constructs(t *testing.T) {
	if n := service.NewNotificationService(&config.Config{}); n == nil {
		t.Fatal("expected non-nil notifier")
	}
}

func TestSendLoanApprovedEmail_BadSMTP_ReturnsError(t *testing.T) {
	n := service.NewNotificationService(&config.Config{
		SMTPHost: "localhost", SMTPPort: 1, SMTPFrom: "noreply@bank.com",
	})
	err := n.SendLoanApprovedEmail("x@y.com", "Test", 1000, "gotovinski", 12, 100, 5, "K1")
	if err == nil {
		t.Fatal("expected SMTP dial error")
	}
}

func TestListRequests_DelegatesToRepo(t *testing.T) {
	db := newLoanTestDB(t, "loan_list_requests")
	db.Create(&models.Loan{Status: "zahtev", Vrsta: "gotovinski", BrojRacuna: "X", Iznos: 100, Period: 12, IznosRate: 10, BrojKredita: "LR1", KamatnaStopa: 5, TipKamate: "fiksna", ClientID: 1, CurrencyID: 1})
	db.Create(&models.Loan{Status: "aktivan", Vrsta: "gotovinski", BrojRacuna: "Y", Iznos: 100, Period: 12, IznosRate: 10, BrojKredita: "LR2", KamatnaStopa: 5, TipKamate: "fiksna", ClientID: 1, CurrencyID: 1})

	loanRepo := repository.NewLoanRepository(db)
	instRepo := repository.NewInstallmentRepository(db)
	accRepo := repository.NewAccountRepository(db)
	svc := service.NewLoanService(db, loanRepo, instRepo, accRepo)

	loans, err := svc.ListRequests()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(loans) != 1 {
		t.Fatalf("expected 1 pending loan, got %d", len(loans))
	}
}
