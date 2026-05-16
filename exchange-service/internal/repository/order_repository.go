package repository

import (
	"errors"
	"fmt"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// CreateOrder persists a new order record.
func (r *OrderRepository) CreateOrder(order *models.OrderRecord) error {
	return r.db.Create(order).Error
}

// GetOrderByID returns an order with its transactions preloaded.
func (r *OrderRepository) GetOrderByID(id uint) (*models.OrderRecord, error) {
	var record models.OrderRecord
	if err := r.db.Preload("Asset").Preload("Asset.Exchange").Preload("Transactions").
		First(&record, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// FindLatestBuyAccountID returns the account_id of the most recent buy order
// for a given user+asset. Used to backfill legacy portfolio_holdings rows that
// were created before account_id was tracked. Returns 0 if no prior buy exists.
func (r *OrderRepository) FindLatestBuyAccountID(userID uint, userType string, assetID uint) (uint, error) {
	var record models.OrderRecord
	err := r.db.
		Where("user_id = ? AND user_type = ? AND asset_id = ? AND direction = ? AND account_id > 0",
			userID, userType, assetID, "buy").
		Order("created_at DESC").
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return record.AccountID, nil
}

// ListOrdersForUser returns all orders for a given user, optionally filtered by status.
func (r *OrderRepository) ListOrdersForUser(userID uint, userType, statusFilter string) ([]models.OrderRecord, error) {
	q := r.db.Preload("Asset").Preload("Asset.Exchange").
		Where("user_id = ? AND user_type = ?", userID, userType)
	if statusFilter != "" {
		q = q.Where("status = ?", statusFilter)
	}
	var records []models.OrderRecord
	if err := q.Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// ListAllOrders returns all orders across all users, optionally filtered by status.
// Used by supervisors to review orders from all agents.
func (r *OrderRepository) ListAllOrders(statusFilter string) ([]models.OrderRecord, error) {
	q := r.db.Preload("Asset").Preload("Asset.Exchange")
	if statusFilter != "" {
		q = q.Where("status = ?", statusFilter)
	}
	var records []models.OrderRecord
	if err := q.Order("created_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// ListPendingActiveOrders returns all orders that are approved/pending and not done,
// used by the execution engine cron.
func (r *OrderRepository) ListPendingActiveOrders() ([]models.OrderRecord, error) {
	var records []models.OrderRecord
	if err := r.db.Preload("Asset").Preload("Asset.Exchange").
		Where("is_done = false AND status IN ?", []string{"approved", "pending"}).
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// UpdateOrderStatus sets the status and optionally the approved_by field.
func (r *OrderRepository) UpdateOrderStatus(id uint, status string, approvedBy *uint) error {
	updates := map[string]interface{}{
		"status":            status,
		"last_modification": time.Now().UTC(),
	}
	if approvedBy != nil {
		updates["approved_by"] = *approvedBy
	}
	return r.db.Model(&models.OrderRecord{}).Where("id = ?", id).Updates(updates).Error
}

// DecrementRemainingPortions reduces remaining_portions and marks done if 0.
func (r *OrderRepository) DecrementRemainingPortions(id uint, filled int64) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var order models.OrderRecord
		if err := tx.First(&order, id).Error; err != nil {
			return err
		}
		order.RemainingPortions -= filled
		order.LastModification = time.Now().UTC()
		if order.RemainingPortions <= 0 {
			order.RemainingPortions = 0
			order.IsDone = true
			order.Status = "done"
		}
		return tx.Save(&order).Error
	})
}

// CreateOrderTransaction persists a fill event.
func (r *OrderRepository) CreateOrderTransaction(tx *models.OrderTransactionRecord) error {
	return r.db.Create(tx).Error
}

// ListTransactionsForOrder returns all transactions for an order.
func (r *OrderRepository) ListTransactionsForOrder(orderID uint) ([]models.OrderTransactionRecord, error) {
	var records []models.OrderTransactionRecord
	if err := r.db.Where("order_id = ?", orderID).Order("executed_at ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

// --- Actuary helpers (reads from shared DB, employee-service tables) ---

// ActuaryProfile is a lightweight read of the actuary_profiles table.
type ActuaryProfile struct {
	EmployeeID   uint
	Limit        *float64
	UsedLimit    float64
	NeedApproval bool
}

// GetActuaryProfile fetches the actuary profile for an employee directly from the shared DB.
// Returns nil if the employee has no actuary profile (i.e. is not an agent/supervisor).
func (r *OrderRepository) GetActuaryProfile(employeeID uint) (*ActuaryProfile, error) {
	var profile ActuaryProfile
	err := r.db.Table("actuary_profiles").
		Select("employee_id, trading_limit as limit, used_limit, need_approval").
		Where("employee_id = ?", employeeID).
		First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

// IncrementUsedLimit atomically adds delta to used_limit for an employee.
func (r *OrderRepository) IncrementUsedLimit(employeeID uint, delta float64) error {
	return r.db.Table("actuary_profiles").
		Where("employee_id = ?", employeeID).
		UpdateColumn("used_limit", gorm.Expr("used_limit + ?", delta)).Error
}

// FullCancelOrder marks an order as cancelled and fully done.
func (r *OrderRepository) FullCancelOrder(id uint) error {
	return r.db.Model(&models.OrderRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":             "cancelled",
		"is_done":            true,
		"remaining_portions": 0,
		"last_modification":  time.Now().UTC(),
	}).Error
}

// SetRemainingPortions updates remaining_portions for a partial cancel.
func (r *OrderRepository) SetRemainingPortions(id uint, newRemaining int64) error {
	return r.db.Model(&models.OrderRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"remaining_portions": newRemaining,
		"last_modification":  time.Now().UTC(),
	}).Error
}

// RefundToAccount adds the refund amount back to the account's available balance.
func (r *OrderRepository) RefundToAccount(accountID uint, amount float64) error {
	return r.db.Table("accounts").
		Where("id = ?", accountID).
		UpdateColumn("raspolozivo_stanje", gorm.Expr("raspolozivo_stanje + ?", amount)).Error
}

// GetSettlementDate returns the settlement date for a futures or options listing,
// or nil if the asset type has no settlement date (stocks, forex).
func (r *OrderRepository) GetSettlementDate(assetID uint) (*time.Time, error) {
	var futures models.FuturesContractRecord
	if err := r.db.Where("listing_id = ?", assetID).First(&futures).Error; err == nil {
		return &futures.SettlementDate, nil
	}
	var option models.OptionRecord
	if err := r.db.Where("listing_id = ?", assetID).First(&option).Error; err == nil {
		return &option.SettlementDate, nil
	}
	return nil, nil // not a dated instrument
}

// --- Account helpers (reads from shared DB, account-service tables) ---

// UserRSDAccount holds the ID and available balance of a user's RSD account.
type UserRSDAccount struct {
	ID                uint
	RaspolozivoStanje float64
}

// GetUserRSDAccounts returns all active RSD-denominated accounts for a user.
// For clients, matches on client_id; for employees, on zaposleni_id.
func (r *OrderRepository) GetUserRSDAccounts(userID uint, userType string) ([]UserRSDAccount, error) {
	q := r.db.Table("accounts").
		Select("accounts.id, accounts.raspolozivo_stanje").
		Joins("JOIN currencies ON currencies.id = accounts.currency_id").
		Where("currencies.kod = 'RSD' AND accounts.status = 'aktivan'")

	if userType == "client" {
		q = q.Where("accounts.client_id = ?", userID)
	} else {
		q = q.Where("accounts.zaposleni_id = ?", userID)
	}

	rows, err := q.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []UserRSDAccount
	for rows.Next() {
		var a UserRSDAccount
		if err := rows.Scan(&a.ID, &a.RaspolozivoStanje); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// DebitAccount atomically debits amount from an account if sufficient balance exists.
// Returns an error if the balance is insufficient.
func (r *OrderRepository) DebitAccount(accountID uint, amount float64) error {
	result := r.db.Table("accounts").
		Where("id = ? AND raspolozivo_stanje >= ?", accountID, amount).
		Updates(map[string]interface{}{
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje - ?", amount),
			"stanje":             gorm.Expr("stanje - ?", amount),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("insufficient balance in account %d", accountID)
	}
	return nil
}

// CreditAccount adds amount to an account's balance.
func (r *OrderRepository) CreditAccount(accountID uint, amount float64) error {
	return r.db.Table("accounts").
		Where("id = ?", accountID).
		Updates(map[string]interface{}{
			"raspolozivo_stanje": gorm.Expr("raspolozivo_stanje + ?", amount),
			"stanje":             gorm.Expr("stanje + ?", amount),
		}).Error
}

// GetStateTreasuryAccountID returns the ID of the "Republika Srbija" RSD account.
// Returns 0 if no such account exists.
func (r *OrderRepository) GetStateTreasuryAccountID() (uint, error) {
	var id uint
	err := r.db.Table("accounts").
		Select("accounts.id").
		Joins("JOIN currencies ON currencies.id = accounts.currency_id").
		Where("LOWER(accounts.naziv) LIKE '%republika srbija%' AND currencies.kod = 'RSD' AND accounts.status = 'aktivan'").
		Limit(1).
		Scan(&id).Error
	return id, err
}

// GetBankAccountByCurrency returns the ID of EXBanka's (non-state firm) account for the given currency.
// Returns 0 if no such account is found.
func (r *OrderRepository) GetBankAccountByCurrency(currencyKod string) (uint, error) {
	var id uint
	err := r.db.Table("accounts").
		Select("accounts.id").
		Joins("JOIN currencies ON currencies.id = accounts.currency_id").
		Joins("JOIN firmas ON firmas.id = accounts.firma_id").
		Where("currencies.kod = ? AND firmas.is_state = false AND accounts.status = 'aktivan'", currencyKod).
		Limit(1).
		Scan(&id).Error
	return id, err
}

// GetAccountBalance returns the raspolozivo_stanje (available balance) and currency_kod for an account.
func (r *OrderRepository) GetAccountBalance(accountID uint) (balance float64, currencyKod string, err error) {
	row := r.db.Table("accounts").
		Select("accounts.raspolozivo_stanje, currencies.kod").
		Joins("LEFT JOIN currencies ON currencies.id = accounts.currency_id").
		Where("accounts.id = ?", accountID).
		Row()
	err = row.Scan(&balance, &currencyKod)
	return
}
