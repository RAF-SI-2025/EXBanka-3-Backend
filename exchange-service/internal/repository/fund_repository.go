package repository

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FundRepository struct {
	db *gorm.DB
}

func NewFundRepository(db *gorm.DB) *FundRepository {
	return &FundRepository{db: db}
}

// FundAccountRef mirrors the shape OtcAccountReference exposes, but is scoped
// to operations the fund service needs.
type FundAccountRef struct {
	ID                uint    `gorm:"column:id"`
	BrojRacuna        string  `gorm:"column:broj_racuna"`
	ClientID          *uint   `gorm:"column:client_id"`
	FirmaID           *uint   `gorm:"column:firma_id"`
	ZaposleniID       *uint   `gorm:"column:zaposleni_id"`
	CurrencyID        uint    `gorm:"column:currency_id"`
	CurrencyCode      string  `gorm:"column:currency_kod"`
	Status            string  `gorm:"column:status"`
	Podvrsta          string  `gorm:"column:podvrsta"`
	FirmaIsState      bool    `gorm:"column:firma_is_state"`
	Stanje            float64 `gorm:"column:stanje"`
	RaspolozivoStanje float64 `gorm:"column:raspolozivo_stanje"`
}

// IsFundAccount reports whether the account is an investment fund's own RSD
// account (`podvrsta='fondacija'`). Such accounts have no firma and are NOT
// bank-owned by the new definition; this helper exists for the defence-in-depth
// guard in fund_service.go.
func (a FundAccountRef) IsFundAccount() bool {
	return a.Podvrsta == "fondacija"
}

// IsBankOwned reports whether the account belongs to the bank itself.
// Convention used across loan-service, transfer-service and order-repository:
// firma_id is set, no client_id, and the firma is NOT the state firma
// (Republika Srbija). Fund accounts (no firma_id) are excluded automatically.
func (a FundAccountRef) IsBankOwned() bool {
	return a.ClientID == nil && a.FirmaID != nil && !a.FirmaIsState
}

func (a FundAccountRef) BelongsToClient(clientID uint) bool {
	return a.ClientID != nil && *a.ClientID == clientID
}

func (r *FundRepository) DB() *gorm.DB { return r.db }

// CreditAccount credits the given account by amount (no questions asked) and
// is exposed for the fund-service auto-liquidation flow.
func (r *FundRepository) CreditAccount(accountID uint, amount float64) error {
	return creditFundAccount(r.db, accountID, amount)
}

func (r *FundRepository) GetAccountByID(accountID uint) (*FundAccountRef, error) {
	return getFundAccountRef(r.db, accountID, false)
}

func (r *FundRepository) findOrCreateRSDCurrencyID(tx *gorm.DB) (uint, error) {
	var id uint
	err := tx.Table("currencies").Select("id").Where("kod = ?", "RSD").Limit(1).Scan(&id).Error
	if err != nil {
		return 0, err
	}
	if id != 0 {
		return id, nil
	}
	// Seed RSD if missing — fund creation should not fail because the row was
	// never inserted.
	now := time.Now().UTC()
	result := tx.Exec(`INSERT INTO currencies (kod, naziv, simbol, drzava, aktivan, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"RSD", "Srpski dinar", "RSD", "Srbija", true, now, now)
	if result.Error != nil {
		return 0, result.Error
	}
	if err := tx.Table("currencies").Select("id").Where("kod = ?", "RSD").Limit(1).Scan(&id).Error; err != nil {
		return 0, err
	}
	return id, nil
}

// CreateFundWithAccount creates a fund and its dedicated bank-owned RSD account
// inside a single transaction.
func (r *FundRepository) CreateFundWithAccount(naziv, opis string, minimalniUlog float64, managerID uint) (*models.InvestmentFundRecord, error) {
	var createdFund models.InvestmentFundRecord
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Duplicate-name check first to return a friendly error.
		var existing int64
		if err := tx.Model(&models.InvestmentFundRecord{}).Where("naziv = ?", naziv).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return fmt.Errorf("fond sa nazivom %q vec postoji", naziv)
		}

		currencyID, err := r.findOrCreateRSDCurrencyID(tx)
		if err != nil {
			return fmt.Errorf("RSD currency lookup failed: %w", err)
		}

		now := time.Now().UTC()
		expires := now.AddDate(50, 0, 0)
		accountNumber := generateFundAccountNumber()

		// Insert directly: account-service is the canonical owner of the
		// `accounts` schema but the column set is stable. We mark the fund's
		// account as bank-owned (no client/firma/zaposleni FK).
		insert := tx.Exec(`INSERT INTO accounts
			(broj_racuna, currency_id, tip, vrsta, podvrsta, stanje, raspolozivo_stanje,
			 dnevni_limit, mesecni_limit, dnevna_potrosnja, mesecna_potrosnja,
			 datum_isteka, odrzavanje_racuna, naziv, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			accountNumber, currencyID, "tekuci", "poslovni", "fondacija",
			0.0, 0.0, 1_000_000_000.0, 10_000_000_000.0, 0.0, 0.0,
			expires, 0.0, "Fond: "+naziv, "aktivan", now, now,
		)
		if insert.Error != nil {
			return fmt.Errorf("failed to create fund account: %w", insert.Error)
		}

		var accountID uint
		if err := tx.Table("accounts").Select("id").Where("broj_racuna = ?", accountNumber).Limit(1).Scan(&accountID).Error; err != nil {
			return fmt.Errorf("failed to resolve fund account id: %w", err)
		}

		fund := models.InvestmentFundRecord{
			Naziv:          naziv,
			Opis:           opis,
			MinimalniUlog:  minimalniUlog,
			ManagerID:      managerID,
			AccountID:      accountID,
			DatumKreiranja: now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(&fund).Error; err != nil {
			return err
		}
		createdFund = fund
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &createdFund, nil
}

func (r *FundRepository) GetFundByID(id uint) (*models.InvestmentFundRecord, error) {
	var fund models.InvestmentFundRecord
	if err := r.db.First(&fund, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &fund, nil
}

func (r *FundRepository) ListFunds() ([]models.InvestmentFundRecord, error) {
	var funds []models.InvestmentFundRecord
	if err := r.db.Order("naziv ASC").Find(&funds).Error; err != nil {
		return nil, err
	}
	return funds, nil
}

func (r *FundRepository) ListFundsByManager(managerID uint) ([]models.InvestmentFundRecord, error) {
	var funds []models.InvestmentFundRecord
	if err := r.db.Where("manager_id = ?", managerID).Order("naziv ASC").Find(&funds).Error; err != nil {
		return nil, err
	}
	return funds, nil
}

// --- positions & transactions ---

func (r *FundRepository) GetPosition(clientID uint, clientType string, fundID uint) (*models.ClientFundPositionRecord, error) {
	var pos models.ClientFundPositionRecord
	if err := r.db.Where("client_id = ? AND client_type = ? AND fund_id = ?", clientID, clientType, fundID).
		First(&pos).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &pos, nil
}

func (r *FundRepository) ListPositionsForClient(clientID uint, clientType string) ([]models.ClientFundPositionRecord, error) {
	var positions []models.ClientFundPositionRecord
	if err := r.db.Where("client_id = ? AND client_type = ? AND ukupan_ulozeni_iznos > 0", clientID, clientType).
		Order("updated_at DESC").Find(&positions).Error; err != nil {
		return nil, err
	}
	return positions, nil
}

func (r *FundRepository) ListPositionsForFund(fundID uint) ([]models.ClientFundPositionRecord, error) {
	var positions []models.ClientFundPositionRecord
	if err := r.db.Where("fund_id = ? AND ukupan_ulozeni_iznos > 0", fundID).
		Order("updated_at DESC").Find(&positions).Error; err != nil {
		return nil, err
	}
	return positions, nil
}

func (r *FundRepository) TotalInvestedInFund(fundID uint) (float64, error) {
	var total float64
	err := r.db.Model(&models.ClientFundPositionRecord{}).
		Where("fund_id = ?", fundID).
		Select("COALESCE(SUM(ukupan_ulozeni_iznos), 0)").
		Scan(&total).Error
	return total, err
}

// RecordInvestment debits the source account, credits the fund's account,
// inserts a ClientFundTransactionRecord (is_inflow=true) and upserts the
// ClientFundPositionRecord — all in a single DB transaction.
func (r *FundRepository) RecordInvestment(
	clientID uint, clientType string,
	fundID uint, sourceAccountID, fundAccountID uint, amount float64,
) (*models.ClientFundTransactionRecord, error) {
	var txRecord models.ClientFundTransactionRecord
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := debitFundAccount(tx, sourceAccountID, amount); err != nil {
			return err
		}
		if err := creditFundAccount(tx, fundAccountID, amount); err != nil {
			return err
		}
		now := time.Now().UTC()
		txRecord = models.ClientFundTransactionRecord{
			ClientID:   clientID,
			ClientType: clientType,
			FundID:     fundID,
			AccountID:  sourceAccountID,
			Iznos:      amount,
			Status:     models.FundTransactionStatusCompleted,
			IsInflow:   true,
			Timestamp:  now,
			CreatedAt:  now,
		}
		if err := tx.Create(&txRecord).Error; err != nil {
			return err
		}
		return upsertPosition(tx, clientID, clientType, fundID, amount)
	})
	if err != nil {
		return nil, err
	}
	return &txRecord, nil
}

// RecordWithdrawal credits the destination account from the fund's account and
// reduces the client's position by the cost-basis fraction that was withdrawn.
//
// Money flow:
//   fund_account  -= netToClient   (commission stays in the fund — option B from FUND-1)
//   destination   += netToClient
//
// `amount` is the GROSS withdrawal value (share × fundValue) — recorded in the
// transaction row for audit. `positionReduction` is the cost-basis decrement to
// apply to UkupanUlozeniIznos (see FUND-2: must be proportional, not gross).
func (r *FundRepository) RecordWithdrawal(
	clientID uint, clientType string,
	fundID uint, fundAccountID, destinationAccountID uint, amount float64, commission float64, positionReduction float64,
) (*models.ClientFundTransactionRecord, error) {
	var txRecord models.ClientFundTransactionRecord
	err := r.db.Transaction(func(tx *gorm.DB) error {
		rec, err := RecordWithdrawalTx(tx, clientID, clientType, fundID, fundAccountID, destinationAccountID, amount, commission, positionReduction)
		if err != nil {
			return err
		}
		txRecord = *rec
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &txRecord, nil
}

// RecordWithdrawalTx is the transaction-aware variant of RecordWithdrawal. It
// runs inside the caller-provided `tx`, allowing fund-service to combine
// liquidation and withdrawal in a single atomic transaction (FUND-3).
func RecordWithdrawalTx(
	tx *gorm.DB,
	clientID uint, clientType string,
	fundID uint, fundAccountID, destinationAccountID uint, amount float64, commission float64, positionReduction float64,
) (*models.ClientFundTransactionRecord, error) {
	netToClient := amount - commission
	if netToClient < 0 {
		netToClient = 0
	}
	// FUND-1: only debit the fund by what actually leaves it. Commission stays
	// inside the fund's account, which preserves total bookkeeping value.
	if err := debitFundAccount(tx, fundAccountID, netToClient); err != nil {
		return nil, err
	}
	if err := creditFundAccount(tx, destinationAccountID, netToClient); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	txRecord := models.ClientFundTransactionRecord{
		ClientID:   clientID,
		ClientType: clientType,
		FundID:     fundID,
		AccountID:  destinationAccountID,
		Iznos:      amount,
		Status:     models.FundTransactionStatusCompleted,
		IsInflow:   false,
		Timestamp:  now,
		CreatedAt:  now,
	}
	if err := tx.Create(&txRecord).Error; err != nil {
		return nil, err
	}
	if err := reducePosition(tx, clientID, clientType, fundID, positionReduction); err != nil {
		return nil, err
	}
	return &txRecord, nil
}

func (r *FundRepository) ListTransactionsForFund(fundID uint) ([]models.ClientFundTransactionRecord, error) {
	var txs []models.ClientFundTransactionRecord
	if err := r.db.Where("fund_id = ?", fundID).Order("timestamp DESC").Find(&txs).Error; err != nil {
		return nil, err
	}
	return txs, nil
}

func (r *FundRepository) ListTransactionsForClient(clientID uint, clientType string) ([]models.ClientFundTransactionRecord, error) {
	var txs []models.ClientFundTransactionRecord
	if err := r.db.Where("client_id = ? AND client_type = ?", clientID, clientType).
		Order("timestamp DESC").Find(&txs).Error; err != nil {
		return nil, err
	}
	return txs, nil
}

// --- performance ---

func (r *FundRepository) SavePerformanceSnapshot(fundID uint, snapshotDate time.Time, value float64) error {
	now := time.Now().UTC()
	rec := models.FundPerformanceHistoryRecord{
		FundID:    fundID,
		Date:      snapshotDate,
		FundValue: value,
		CreatedAt: now,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "fund_id"}, {Name: "date"}},
		DoUpdates: clause.AssignmentColumns([]string{"fund_value"}),
	}).Create(&rec).Error
}

func (r *FundRepository) ListPerformance(fundID uint, from, to time.Time) ([]models.FundPerformanceHistoryRecord, error) {
	var recs []models.FundPerformanceHistoryRecord
	if err := r.db.Where("fund_id = ? AND date BETWEEN ? AND ?", fundID, from, to).
		Order("date ASC").Find(&recs).Error; err != nil {
		return nil, err
	}
	return recs, nil
}

// --- helpers ---

func getFundAccountRef(db *gorm.DB, accountID uint, lock bool) (*FundAccountRef, error) {
	if accountID == 0 {
		return nil, nil
	}
	q := db.Table("accounts").
		Select("accounts.id, accounts.broj_racuna, accounts.client_id, accounts.firma_id, accounts.zaposleni_id, accounts.currency_id, currencies.kod AS currency_kod, accounts.status, accounts.podvrsta, COALESCE(firmas.is_state, false) AS firma_is_state, accounts.stanje, accounts.raspolozivo_stanje").
		Joins("LEFT JOIN currencies ON currencies.id = accounts.currency_id").
		Joins("LEFT JOIN firmas ON firmas.id = accounts.firma_id").
		Where("accounts.id = ?", accountID)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "accounts"}})
	}
	var ref FundAccountRef
	if err := q.First(&ref).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ref, nil
}

func debitFundAccount(tx *gorm.DB, accountID uint, amount float64) error {
	if amount <= 0 {
		return nil
	}
	res := tx.Table("accounts").
		Where("id = ? AND raspolozivo_stanje >= ?", accountID, amount).
		Updates(map[string]interface{}{
			"stanje":             gorm.Expr("stanje - ?", amount),
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje - ?", amount),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("nedovoljno sredstava na racunu")
	}
	return nil
}

func creditFundAccount(tx *gorm.DB, accountID uint, amount float64) error {
	if amount <= 0 {
		return nil
	}
	return tx.Table("accounts").Where("id = ?", accountID).
		Updates(map[string]interface{}{
			"stanje":             gorm.Expr("stanje + ?", amount),
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje + ?", amount),
		}).Error
}

func upsertPosition(tx *gorm.DB, clientID uint, clientType string, fundID uint, delta float64) error {
	var pos models.ClientFundPositionRecord
	err := tx.Where("client_id = ? AND client_type = ? AND fund_id = ?", clientID, clientType, fundID).
		First(&pos).Error
	now := time.Now().UTC()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		pos = models.ClientFundPositionRecord{
			ClientID:              clientID,
			ClientType:            clientType,
			FundID:                fundID,
			UkupanUlozeniIznos:    delta,
			DatumPoslednjePromene: now,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		return tx.Create(&pos).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&pos).Updates(map[string]interface{}{
		"ukupan_ulozeni_iznos":    pos.UkupanUlozeniIznos + delta,
		"datum_poslednje_promene": now,
		"updated_at":              now,
	}).Error
}

func reducePosition(tx *gorm.DB, clientID uint, clientType string, fundID uint, amount float64) error {
	var pos models.ClientFundPositionRecord
	err := tx.Where("client_id = ? AND client_type = ? AND fund_id = ?", clientID, clientType, fundID).
		First(&pos).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	newValue := pos.UkupanUlozeniIznos - amount
	if newValue < 0 {
		newValue = 0
	}
	now := time.Now().UTC()
	return tx.Model(&pos).Updates(map[string]interface{}{
		"ukupan_ulozeni_iznos":    newValue,
		"datum_poslednje_promene": now,
		"updated_at":              now,
	}).Error
}

// generateFundAccountNumber produces an 18-digit bank-owned account number in
// the same family the account-service uses (333 / 0001 / random / check / 12).
// The format is independent of the account-service util to avoid a cross-module
// import.
func generateFundAccountNumber() string {
	const bankCode = "333"
	const branchCode = "0001"
	const typeCode = "12"
	for {
		random := fmt.Sprintf("%08d", rand.Int63n(100_000_000))
		prefix := bankCode + branchCode + random
		sum := digitSum(prefix) + digitSum(typeCode)
		check := (11 - (sum % 11)) % 11
		if check == 10 {
			continue
		}
		return prefix + fmt.Sprintf("%d", check) + typeCode
	}
}

func digitSum(s string) int {
	sum := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			sum += int(c - '0')
		}
	}
	return sum
}
