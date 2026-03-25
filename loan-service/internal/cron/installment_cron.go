package cron

import (
	"log/slog"
	"math"
	"math/rand/v2"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
	"gorm.io/gorm"
)

// installmentRepo is the subset of InstallmentRepository used by InstallmentCollector.
type installmentRepo interface {
	FindDueInstallments(asOf time.Time) ([]models.LoanInstallment, error)
	ListByLoanID(loanID uint) ([]models.LoanInstallment, error)
	Save(inst *models.LoanInstallment) error
}

type collectorLoanRepo interface {
	FindByID(id uint) (*models.Loan, error)
	SaveLoan(loan *models.Loan) error
}

type collectorAccountRepo interface {
	FindByBrojRacuna(brojRacuna string) (*models.Account, error)
	UpdateFields(id uint, fields map[string]interface{}) error
}

// InstallmentCollector processes due installments once per run.
type InstallmentCollector struct {
	db          *gorm.DB
	repo        installmentRepo
	loanRepo    collectorLoanRepo
	accountRepo collectorAccountRepo
}

func NewInstallmentCollector(db *gorm.DB, repo installmentRepo, loanRepo collectorLoanRepo, accountRepo collectorAccountRepo) *InstallmentCollector {
	return &InstallmentCollector{
		db:          db,
		repo:        repo,
		loanRepo:    loanRepo,
		accountRepo: accountRepo,
	}
}

func (c *InstallmentCollector) Run(asOf time.Time) error {
	installments, err := c.repo.FindDueInstallments(asOf)
	if err != nil {
		return err
	}

	for i := range installments {
		inst := &installments[i]
		if err := c.collectOne(inst); err != nil {
			slog.Error("Failed to save installment", "id", inst.ID, "error", err)
			continue
		}
		slog.Info("Installment collection attempted", "id", inst.ID, "loan_id", inst.LoanID, "status", inst.Status, "iznos", inst.Iznos)
	}

	slog.Info("Installment collection run complete", "processed", len(installments))
	return nil
}

func (c *InstallmentCollector) collectOne(inst *models.LoanInstallment) error {
	if c.db != nil {
		return c.db.Transaction(func(tx *gorm.DB) error {
			var currentInst models.LoanInstallment
			if err := tx.First(&currentInst, inst.ID).Error; err != nil {
				return err
			}
			var loan models.Loan
			if err := tx.First(&loan, currentInst.LoanID).Error; err != nil {
				return err
			}
			var account models.Account
			if err := tx.Table("accounts").
				Select("accounts.*, currencies.kod as currency_kod").
				Joins("LEFT JOIN currencies ON currencies.id = accounts.currency_id").
				Where("accounts.broj_racuna = ?", loan.BrojRacuna).
				First(&account).Error; err != nil {
				currentInst.Status = "kasni"
				currentInst.DatumPlacanja = nil
				return tx.Save(&currentInst).Error
			}

			if account.Status != "aktivan" || account.RaspolozivoStanje < currentInst.Iznos {
				currentInst.Status = "kasni"
				currentInst.DatumPlacanja = nil
				return tx.Save(&currentInst).Error
			}

			now := time.Now().UTC()
			if err := tx.Table("accounts").Where("id = ?", account.ID).Updates(map[string]interface{}{
				"stanje":             account.Stanje - currentInst.Iznos,
				"raspolozivo_stanje": account.RaspolozivoStanje - currentInst.Iznos,
				"dnevna_potrosnja":   account.DnevnaPotrosnja + currentInst.Iznos,
				"mesecna_potrosnja":  account.MesecnaPotrosnja + currentInst.Iznos,
			}).Error; err != nil {
				return err
			}

			currentInst.Status = "placena"
			currentInst.DatumPlacanja = &now
			if err := tx.Save(&currentInst).Error; err != nil {
				return err
			}

			var remaining int64
			if err := tx.Model(&models.LoanInstallment{}).
				Where("loan_id = ? AND status <> ?", loan.ID, "placena").
				Count(&remaining).Error; err != nil {
				return err
			}
			if remaining == 0 {
				loan.Status = "zatvoren"
				if err := tx.Save(&loan).Error; err != nil {
					return err
				}
			}

			*inst = currentInst
			return nil
		})
	}

	loan, err := c.loanRepo.FindByID(inst.LoanID)
	if err != nil {
		return err
	}
	account, err := c.accountRepo.FindByBrojRacuna(loan.BrojRacuna)
	if err != nil {
		inst.Status = "kasni"
		inst.DatumPlacanja = nil
		return c.repo.Save(inst)
	}

	if account.Status != "aktivan" || account.RaspolozivoStanje < inst.Iznos {
		inst.Status = "kasni"
		inst.DatumPlacanja = nil
		return c.repo.Save(inst)
	}

	if err := c.accountRepo.UpdateFields(account.ID, map[string]interface{}{
		"stanje":             account.Stanje - inst.Iznos,
		"raspolozivo_stanje": account.RaspolozivoStanje - inst.Iznos,
		"dnevna_potrosnja":   account.DnevnaPotrosnja + inst.Iznos,
		"mesecna_potrosnja":  account.MesecnaPotrosnja + inst.Iznos,
	}); err != nil {
		return err
	}

	now := time.Now().UTC()
	inst.Status = "placena"
	inst.DatumPlacanja = &now
	if err := c.repo.Save(inst); err != nil {
		return err
	}

	all, err := c.repo.ListByLoanID(inst.LoanID)
	if err != nil {
		return err
	}
	allPaid := true
	for _, item := range all {
		if item.ID == inst.ID {
			item = *inst
		}
		if item.Status != "placena" {
			allPaid = false
			break
		}
	}
	if allPaid {
		loan.Status = "zatvoren"
		return c.loanRepo.SaveLoan(loan)
	}
	return nil
}

// loanRepo is the subset of LoanRepository used by InterestRateUpdater.
type loanRepo interface {
	FindActiveVariableLoans() ([]models.Loan, error)
	SaveLoan(loan *models.Loan) error
}

// InterestRateUpdater applies a monthly EURIBOR-style random adjustment to variable loans.
type InterestRateUpdater struct {
	repo loanRepo
}

func NewInterestRateUpdater(repo loanRepo) *InterestRateUpdater {
	return &InterestRateUpdater{repo: repo}
}

// Run adjusts each variable-rate loan's interest rate by a random delta in [-1.5%, +1.5%],
// recalculates the monthly installment, and saves.
func (u *InterestRateUpdater) Run() error {
	loans, err := u.repo.FindActiveVariableLoans()
	if err != nil {
		return err
	}

	for i := range loans {
		loan := &loans[i]

		// Random delta in [-1.5, +1.5] percent.
		delta := (rand.Float64()*3.0 - 1.5) // [-1.5, +1.5]
		newRate := math.Max(0, loan.KamatnaStopa+delta)
		loan.KamatnaStopa = newRate
		loan.IznosRate = annuity(loan.Iznos, newRate, loan.Period)

		if err := u.repo.SaveLoan(loan); err != nil {
			slog.Error("Failed to save loan after rate update", "id", loan.ID, "error", err)
			continue
		}
		slog.Info("Interest rate updated", "loan_id", loan.ID, "delta", delta, "new_rate", newRate)
	}

	slog.Info("Interest rate update run complete", "updated", len(loans))
	return nil
}

// annuity computes the monthly annuity payment: M = P * r*(1+r)^n / ((1+r)^n - 1)
// where r = annual rate / 12 / 100.
func annuity(principal, annualRate float64, months int) float64 {
	if annualRate == 0 {
		return principal / float64(months)
	}
	r := annualRate / 12.0 / 100.0
	n := float64(months)
	return principal * r * math.Pow(1+r, n) / (math.Pow(1+r, n) - 1)
}
