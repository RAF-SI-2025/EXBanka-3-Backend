package service

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
	"gorm.io/gorm"
)

// ErrInvalidInput is returned by service methods when the caller provides invalid data.
var ErrInvalidInput = errors.New("invalid input")

// LoanFilter holds optional filter criteria for loan queries.
type LoanFilter struct {
	Vrsta      string
	BrojRacuna string
	Status     string
}

// LoanRepositoryInterface allows mocking in tests.
type LoanRepositoryInterface interface {
	Create(loan *models.Loan) error
	FindByID(id uint) (*models.Loan, error)
	Save(loan *models.Loan) error
	ListByClientID(clientID uint) ([]models.Loan, error)
	ListByStatus(status string) ([]models.Loan, error)
	ListFiltered(filter LoanFilter) ([]models.Loan, error)
}

// InstallmentRepositoryInterface allows mocking in tests.
type InstallmentRepositoryInterface interface {
	CreateBatch(items []models.LoanInstallment) error
	ListByLoanID(loanID uint) ([]models.LoanInstallment, error)
}

type AccountRepositoryInterface interface {
	FindByBrojRacuna(brojRacuna string) (*models.Account, error)
	FindByID(id uint) (*models.Account, error)
	UpdateFields(id uint, fields map[string]interface{}) error
}

// LoanService handles loan request, approval, and rejection logic.
type LoanService struct {
	db              *gorm.DB
	loanRepo        LoanRepositoryInterface
	installmentRepo InstallmentRepositoryInterface
	accountRepo     AccountRepositoryInterface
	notifier        *NotificationService
}

func NewLoanService(db *gorm.DB, loanRepo LoanRepositoryInterface, installmentRepo InstallmentRepositoryInterface, accountRepo AccountRepositoryInterface) *LoanService {
	return &LoanService{
		db:              db,
		loanRepo:        loanRepo,
		installmentRepo: installmentRepo,
		accountRepo:     accountRepo,
	}
}

func NewLoanServiceWithNotifier(db *gorm.DB, loanRepo LoanRepositoryInterface, installmentRepo InstallmentRepositoryInterface, accountRepo AccountRepositoryInterface, notifier *NotificationService) *LoanService {
	return &LoanService{
		db:              db,
		loanRepo:        loanRepo,
		installmentRepo: installmentRepo,
		accountRepo:     accountRepo,
		notifier:        notifier,
	}
}

// CalculateInstallment computes the monthly annuity installment.
//
//	A = P * r * (1+r)^n / ((1+r)^n - 1)
//	where r = annualRate / 12 / 100
//
// Exported so tests and handlers can verify the formula directly.
func CalculateInstallment(amount, annualRate float64, months int) float64 {
	if annualRate == 0 {
		return math.Round(amount/float64(months)*100) / 100
	}
	r := annualRate / 12.0 / 100.0
	n := float64(months)
	factor := math.Pow(1+r, n)
	a := amount * r * factor / (factor - 1)
	return math.Round(a*100) / 100
}

// BaseInterestRate returns the base annual interest rate (%) for the given
// loan amount in RSD and interest type (fiksna / varijabilna).
// Exported for use in tests and handler previews.
func BaseInterestRate(amountRSD float64, tipKamate string) float64 {
	type band struct {
		limit       float64
		fiksna      float64
		varijabilna float64
	}
	bands := []band{
		{100_000, 6.5, 4.5},
		{500_000, 5.8, 3.8},
		{1_000_000, 5.2, 3.2},
		{5_000_000, 4.5, 2.5},
		{math.MaxFloat64, 4.0, 2.0},
	}
	for _, b := range bands {
		if amountRSD <= b.limit {
			if tipKamate == "varijabilna" {
				return b.varijabilna
			}
			return b.fiksna
		}
	}
	if tipKamate == "varijabilna" {
		return 2.0
	}
	return 4.0
}

// MarginForVrsta returns the type-specific margin (%) to add to the base rate.
// Exported for use in tests and handler previews.
func MarginForVrsta(vrsta string) float64 {
	margins := map[string]float64{
		"gotovinski":      1.5,
		"stambeni":        0.0,
		"auto":            0.5,
		"refinansirajuci": 0.0,
		"studentski":      -0.5,
	}
	return margins[vrsta]
}

// CreateLoanInput carries the data for a new loan request.
type CreateLoanInput struct {
	Vrsta      string
	BrojRacuna string
	Iznos      float64
	Period     int
	TipKamate  string
	ClientID   uint
	CurrencyID uint
	// EURIBORRate is used only for varijabilna loans; defaults to 0.
	EURIBORRate float64

	// Additional fields from specification
	SvrhaKredita      string
	IznosMesecnePlate float64
	StatusZaposlenja  string
	PeriodZaposlenja  string
	KontaktTelefon    string
}

// RequestLoan creates a new loan request (status = "zahtev").
func (s *LoanService) RequestLoan(input CreateLoanInput) (*models.Loan, error) {
	if !contains(models.ValidLoanTypes(), input.Vrsta) {
		return nil, fmt.Errorf("%w: invalid vrsta: %s", ErrInvalidInput, input.Vrsta)
	}
	if !contains(models.ValidInterestTypes(), input.TipKamate) {
		return nil, fmt.Errorf("%w: invalid tip kamate: %s", ErrInvalidInput, input.TipKamate)
	}
	if input.Iznos <= 0 {
		return nil, fmt.Errorf("%w: iznos must be positive", ErrInvalidInput)
	}
	if input.Period < 1 {
		return nil, fmt.Errorf("%w: period must be at least 1 month", ErrInvalidInput)
	}
	// Validate period against allowed values for loan type
	validPeriods := models.ValidPeriods()
	if allowed, ok := validPeriods[input.Vrsta]; ok {
		periodValid := false
		for _, p := range allowed {
			if input.Period == p {
				periodValid = true
				break
			}
		}
		if !periodValid {
			return nil, fmt.Errorf("%w: period %d is not allowed for loan type %s", ErrInvalidInput, input.Period, input.Vrsta)
		}
	}
	// Validate required additional fields
	if strings.TrimSpace(input.SvrhaKredita) == "" {
		return nil, fmt.Errorf("%w: svrha_kredita is required", ErrInvalidInput)
	}
	if input.IznosMesecnePlate <= 0 {
		return nil, fmt.Errorf("%w: iznos_mesecne_plate must be positive", ErrInvalidInput)
	}
	if !contains(models.ValidEmploymentStatuses(), input.StatusZaposlenja) {
		return nil, fmt.Errorf("%w: invalid status_zaposlenja: %s", ErrInvalidInput, input.StatusZaposlenja)
	}
	if strings.TrimSpace(input.PeriodZaposlenja) == "" {
		return nil, fmt.Errorf("%w: period_zaposlenja is required", ErrInvalidInput)
	}
	if strings.TrimSpace(input.KontaktTelefon) == "" {
		return nil, fmt.Errorf("%w: kontakt_telefon is required", ErrInvalidInput)
	}
	if strings.TrimSpace(input.BrojRacuna) == "" {
		return nil, fmt.Errorf("%w: broj_racuna is required", ErrInvalidInput)
	}
	if s.accountRepo != nil {
		account, err := s.accountRepo.FindByBrojRacuna(strings.TrimSpace(input.BrojRacuna))
		if err != nil {
			return nil, fmt.Errorf("%w: payout account not found", ErrInvalidInput)
		}
		if account.Status != "" && account.Status != "aktivan" {
			return nil, fmt.Errorf("%w: payout account is not active", ErrInvalidInput)
		}
		if account.ClientID == nil || *account.ClientID != input.ClientID {
			return nil, fmt.Errorf("%w: payout account does not belong to the client", ErrInvalidInput)
		}
		if input.CurrencyID != 0 && account.CurrencyID != input.CurrencyID {
			return nil, fmt.Errorf("%w: account currency does not match requested currency", ErrInvalidInput)
		}
		if account.CurrencyKod != "" && account.CurrencyKod != "RSD" {
			return nil, fmt.Errorf("%w: loans can be requested only against RSD payout accounts", ErrInvalidInput)
		}
	}

	base := BaseInterestRate(input.Iznos, input.TipKamate)
	margin := MarginForVrsta(input.Vrsta)
	kamatnaStopa := base + margin
	if input.TipKamate == "varijabilna" {
		kamatnaStopa += input.EURIBORRate
	}

	iznosRate := CalculateInstallment(input.Iznos, kamatnaStopa, input.Period)

	loan := &models.Loan{
		Vrsta:             input.Vrsta,
		BrojRacuna:        input.BrojRacuna,
		BrojKredita:       generateLoanNumber(),
		Iznos:             input.Iznos,
		Period:            input.Period,
		KamatnaStopa:      kamatnaStopa,
		TipKamate:         input.TipKamate,
		DatumKreiranja:    time.Now(),
		DatumDospeca:      time.Now().AddDate(0, input.Period, 0),
		IznosRate:         iznosRate,
		Status:            "zahtev",
		ClientID:          input.ClientID,
		CurrencyID:        input.CurrencyID,
		SvrhaKredita:      input.SvrhaKredita,
		IznosMesecnePlate: input.IznosMesecnePlate,
		StatusZaposlenja:  input.StatusZaposlenja,
		PeriodZaposlenja:  input.PeriodZaposlenja,
		KontaktTelefon:    input.KontaktTelefon,
	}

	if err := s.loanRepo.Create(loan); err != nil {
		return nil, fmt.Errorf("failed to save loan: %w", err)
	}
	return loan, nil
}

// ApproveLoan approves a loan request, sets it to "aktivan", and generates installments.
func (s *LoanService) ApproveLoan(loanID, zaposleniID uint) (*models.Loan, error) {
	if s.db != nil {
		var approved *models.Loan
		err := s.db.Transaction(func(tx *gorm.DB) error {
			var loan models.Loan
			if err := tx.First(&loan, loanID).Error; err != nil {
				return fmt.Errorf("loan not found: %w", err)
			}
			if loan.Status != "zahtev" {
				return fmt.Errorf("loan must be in status 'zahtev' to approve, got '%s'", loan.Status)
			}

			var account models.Account
			if err := tx.Table("accounts").
				Select("accounts.*, currencies.kod as currency_kod").
				Joins("LEFT JOIN currencies ON currencies.id = accounts.currency_id").
				Where("accounts.broj_racuna = ?", loan.BrojRacuna).
				First(&account).Error; err != nil {
				return fmt.Errorf("payout account not found: %w", err)
			}
			if account.Status != "" && account.Status != "aktivan" {
				return fmt.Errorf("payout account is not active")
			}
			if account.ClientID == nil || *account.ClientID != loan.ClientID {
				return fmt.Errorf("payout account does not belong to the client")
			}

			if err := tx.Table("accounts").Where("id = ?", account.ID).Updates(map[string]interface{}{
				"stanje":             account.Stanje + loan.Iznos,
				"raspolozivo_stanje": account.RaspolozivoStanje + loan.Iznos,
			}).Error; err != nil {
				return fmt.Errorf("failed to disburse loan funds: %w", err)
			}

			loan.Status = "aktivan"
			loan.ZaposleniID = &zaposleniID
			if err := tx.Save(&loan).Error; err != nil {
				return fmt.Errorf("failed to save loan: %w", err)
			}

			installments := generateInstallments(&loan)
			if err := tx.Create(&installments).Error; err != nil {
				return fmt.Errorf("failed to create installments: %w", err)
			}
			approved = &loan
			return nil
		})
		if err != nil {
			return nil, err
		}
		s.sendLoanApprovedEmail(approved)
		return approved, nil
	}

	loan, err := s.loanRepo.FindByID(loanID)
	if err != nil {
		return nil, fmt.Errorf("loan not found: %w", err)
	}
	if loan.Status != "zahtev" {
		return nil, fmt.Errorf("loan must be in status 'zahtev' to approve, got '%s'", loan.Status)
	}

	loan.Status = "aktivan"
	loan.ZaposleniID = &zaposleniID

	if err := s.loanRepo.Save(loan); err != nil {
		return nil, fmt.Errorf("failed to save loan: %w", err)
	}

	installments := generateInstallments(loan)
	if err := s.installmentRepo.CreateBatch(installments); err != nil {
		return nil, fmt.Errorf("failed to create installments: %w", err)
	}

	s.sendLoanApprovedEmail(loan)
	return loan, nil
}

func (s *LoanService) sendLoanApprovedEmail(loan *models.Loan) {
	if s.notifier == nil || s.db == nil {
		return
	}
	var client struct {
		Email   string
		Ime     string
		Prezime string
	}
	if err := s.db.Table("clients").Select("email, ime, prezime").Where("id = ?", loan.ClientID).First(&client).Error; err != nil {
		return
	}
	fullName := strings.TrimSpace(client.Ime + " " + client.Prezime)
	if fullName == "" {
		fullName = client.Email
	}
	_ = s.notifier.SendLoanApprovedEmail(client.Email, fullName, loan.Iznos, loan.Vrsta, loan.Period, loan.IznosRate, loan.KamatnaStopa, loan.BrojKredita)
}

// RejectLoan rejects a loan request and sets it to "odbijen".
func (s *LoanService) RejectLoan(loanID, zaposleniID uint) (*models.Loan, error) {
	loan, err := s.loanRepo.FindByID(loanID)
	if err != nil {
		return nil, fmt.Errorf("loan not found: %w", err)
	}
	if loan.Status != "zahtev" {
		return nil, fmt.Errorf("loan must be in status 'zahtev' to reject, got '%s'", loan.Status)
	}

	loan.Status = "odbijen"
	loan.ZaposleniID = &zaposleniID

	if err := s.loanRepo.Save(loan); err != nil {
		return nil, fmt.Errorf("failed to save loan: %w", err)
	}
	return loan, nil
}

// ListRequests returns all loans with status "zahtev" (for employee review).
func (s *LoanService) ListRequests() ([]models.Loan, error) {
	return s.loanRepo.ListByStatus("zahtev")
}

// ListByClient returns all loans for a client, sorted descending by amount.
func (s *LoanService) ListByClient(clientID uint) ([]models.Loan, error) {
	loans, err := s.loanRepo.ListByClientID(clientID)
	if err != nil {
		return nil, err
	}
	if loans == nil {
		return []models.Loan{}, nil
	}
	// sort descending by Iznos
	for i := 0; i < len(loans)-1; i++ {
		for j := i + 1; j < len(loans); j++ {
			if loans[j].Iznos > loans[i].Iznos {
				loans[i], loans[j] = loans[j], loans[i]
			}
		}
	}
	return loans, nil
}

// GetByID returns a loan by ID with its installments preloaded.
func (s *LoanService) GetByID(loanID uint) (*models.Loan, error) {
	loan, err := s.loanRepo.FindByID(loanID)
	if err != nil {
		return nil, fmt.Errorf("loan not found: %w", err)
	}
	installments, err := s.installmentRepo.ListByLoanID(loanID)
	if err != nil {
		return nil, fmt.Errorf("failed to load installments: %w", err)
	}
	loan.Installments = installments
	return loan, nil
}

// ListInstallments returns all installments for a loan.
func (s *LoanService) ListInstallments(loanID uint) ([]models.LoanInstallment, error) {
	return s.installmentRepo.ListByLoanID(loanID)
}

// ListRequestsFiltered returns pending loan requests (status="zahtev") with optional filters.
func (s *LoanService) ListRequestsFiltered(vrsta, brojRacuna string) ([]models.Loan, error) {
	return s.loanRepo.ListFiltered(LoanFilter{
		Status:     "zahtev",
		Vrsta:      vrsta,
		BrojRacuna: brojRacuna,
	})
}

// ListAllFiltered returns all loans matching the given filter.
func (s *LoanService) ListAllFiltered(filter LoanFilter) ([]models.Loan, error) {
	return s.loanRepo.ListFiltered(filter)
}

// RemainingDebt sums the amounts of all unpaid (ocekuje / kasni) installments.
// Exported for use in tests and handlers.
func RemainingDebt(installments []models.LoanInstallment) float64 {
	var total float64
	for _, inst := range installments {
		if inst.Status == "ocekuje" || inst.Status == "kasni" {
			total += inst.Iznos
		}
	}
	return total
}

// NextInstallment returns the earliest upcoming installment with status "ocekuje",
// or nil if all installments are paid.
// Exported for use in tests and handlers.
func NextInstallment(installments []models.LoanInstallment) *models.LoanInstallment {
	var next *models.LoanInstallment
	for i := range installments {
		inst := &installments[i]
		if inst.Status != "ocekuje" {
			continue
		}
		if next == nil || inst.DatumDospeca.Before(next.DatumDospeca) {
			next = inst
		}
	}
	return next
}

// generateInstallments builds the full installment schedule for a loan.
func generateInstallments(loan *models.Loan) []models.LoanInstallment {
	installments := make([]models.LoanInstallment, loan.Period)
	now := time.Now()
	for i := range installments {
		installments[i] = models.LoanInstallment{
			LoanID:              loan.ID,
			RedniBroj:           i + 1,
			Iznos:               loan.IznosRate,
			KamataStopaSnapshot: loan.KamatnaStopa,
			DatumDospeca:        now.AddDate(0, i+1, 0),
			Status:              "ocekuje",
		}
	}
	return installments
}

// generateLoanNumber produces a unique loan number string.
func generateLoanNumber() string {
	return fmt.Sprintf("KRED-%d-%06d", time.Now().UnixMilli(), rand.Intn(1_000_000))
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
