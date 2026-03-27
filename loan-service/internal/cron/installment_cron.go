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

// retryHours defines how many hours the system retries before applying late penalties.
const retryHours = 72

// latePenaltyRate is the interest rate increase (%) applied after retry period expires.
const latePenaltyRate = 0.05

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
				c.markLate(tx, &currentInst)
				*inst = currentInst
				return nil
			}

			if account.Status != "aktivan" || account.RaspolozivoStanje < currentInst.Iznos {
				c.markLate(tx, &currentInst)
				// Check if 72h retry period has expired — apply penalty
				if currentInst.DatumKasnjenja != nil {
					hoursSinceLate := time.Since(*currentInst.DatumKasnjenja).Hours()
					if hoursSinceLate >= retryHours {
						loan.KamatnaStopa += latePenaltyRate
						loan.IznosRate = annuity(loan.Iznos, loan.KamatnaStopa, loan.Period)
						tx.Save(&loan)
						// Reset the late clock so the next penalty doesn't fire until another 72h passes.
						penaltyNow := time.Now().UTC()
						currentInst.DatumKasnjenja = &penaltyNow
						tx.Save(&currentInst)
						slog.Warn("Late penalty applied", "loan_id", loan.ID, "new_rate", loan.KamatnaStopa)
					}
				}
				*inst = currentInst
				return nil
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
			currentInst.DatumKasnjenja = nil
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
		c.markLateFallback(inst)
		return c.repo.Save(inst)
	}

	if account.Status != "aktivan" || account.RaspolozivoStanje < inst.Iznos {
		c.markLateFallback(inst)
		// Check 72h penalty
		if inst.DatumKasnjenja != nil && time.Since(*inst.DatumKasnjenja).Hours() >= retryHours {
			loan.KamatnaStopa += latePenaltyRate
			loan.IznosRate = annuity(loan.Iznos, loan.KamatnaStopa, loan.Period)
			_ = c.loanRepo.SaveLoan(loan)
			// Reset the late clock so the next penalty doesn't fire until another 72h passes.
			penaltyNow := time.Now().UTC()
			inst.DatumKasnjenja = &penaltyNow
			slog.Warn("Late penalty applied", "loan_id", loan.ID, "new_rate", loan.KamatnaStopa)
		}
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
	inst.DatumKasnjenja = nil
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

// markLate sets installment to "kasni" and records when it first became late (for 72h retry).
func (c *InstallmentCollector) markLate(tx *gorm.DB, inst *models.LoanInstallment) {
	inst.Status = "kasni"
	inst.DatumPlacanja = nil
	if inst.DatumKasnjenja == nil {
		now := time.Now().UTC()
		inst.DatumKasnjenja = &now
	}
	tx.Save(inst)
}

// markLateFallback is the non-transaction version of markLate.
func (c *InstallmentCollector) markLateFallback(inst *models.LoanInstallment) {
	inst.Status = "kasni"
	inst.DatumPlacanja = nil
	if inst.DatumKasnjenja == nil {
		now := time.Now().UTC()
		inst.DatumKasnjenja = &now
	}
}

// loanRepo is the subset of LoanRepository used by InterestRateUpdater.
type loanRepo interface {
	FindActiveVariableLoans() ([]models.Loan, error)
	SaveLoan(loan *models.Loan) error
}

// InterestRateUpdater applies a monthly EURIBOR-style random adjustment to variable loans.
type InterestRateUpdater struct {
	repo loanRepo
	db   *gorm.DB
}

func NewInterestRateUpdater(repo loanRepo, db *gorm.DB) *InterestRateUpdater {
	return &InterestRateUpdater{repo: repo, db: db}
}

// Run adjusts each variable-rate loan's interest rate by a random delta in [-1.5%, +1.5%],
// recalculates the monthly installment, and updates pending installments in the schedule.
func (u *InterestRateUpdater) Run() error {
	loans, err := u.repo.FindActiveVariableLoans()
	if err != nil {
		return err
	}

	for i := range loans {
		loan := &loans[i]

		// Random delta in [-1.5, +1.5] percent.
		delta := rand.Float64()*3.0 - 1.5
		newRate := math.Max(0, loan.KamatnaStopa+delta)
		loan.KamatnaStopa = newRate
		loan.IznosRate = annuity(loan.Iznos, newRate, loan.Period)

		if err := u.repo.SaveLoan(loan); err != nil {
			slog.Error("Failed to save loan after rate update", "id", loan.ID, "error", err)
			continue
		}

		// Update all pending installments with the new amount so the daily
		// collection cron picks up the correct value.
		if u.db != nil {
			if err := u.db.Model(&models.LoanInstallment{}).
				Where("loan_id = ? AND status = ?", loan.ID, "ocekuje").
				Update("iznos", loan.IznosRate).Error; err != nil {
				slog.Error("Failed to update installment amounts after rate change", "loan_id", loan.ID, "error", err)
			}
		}

		slog.Info("Interest rate updated", "loan_id", loan.ID, "delta", delta, "new_rate", newRate, "new_iznos_rate", loan.IznosRate)
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
