package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"gorm.io/gorm"
)

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) Create(account *models.Account) error {
	return r.db.Create(account).Error
}

func (r *AccountRepository) FindByID(id uint) (*models.Account, error) {
	var account models.Account
	if err := r.db.Preload("Currency").Preload("Client").Preload("Firma").First(&account, id).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepository) FindByBrojRacuna(broj string) (*models.Account, error) {
	var account models.Account
	if err := r.db.Preload("Currency").Where("broj_racuna = ?", broj).First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *AccountRepository) ListByClientID(clientID uint) ([]models.Account, error) {
	var accounts []models.Account
	if err := r.db.Preload("Currency").Preload("Firma").Where("client_id = ?", clientID).Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *AccountRepository) ListAll(filter models.AccountFilter) ([]models.Account, int64, error) {
	var accounts []models.Account
	var total int64

	query := r.db.Model(&models.Account{}).
		Joins("LEFT JOIN clients ON clients.id = accounts.client_id").
		Preload("Currency").
		Preload("Client").
		Preload("Firma")

	if filter.Tip != "" {
		query = query.Where("tip = ?", filter.Tip)
	}
	if filter.Vrsta != "" {
		query = query.Where("vrsta = ?", filter.Vrsta)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.CurrencyID != nil {
		query = query.Where("currency_id = ?", *filter.CurrencyID)
	}
	if filter.AccountNumber != "" {
		query = query.Where("accounts.broj_racuna ILIKE ?", "%"+filter.AccountNumber+"%")
	}
	if filter.ClientName != "" {
		query = query.Where(
			"(clients.ime ILIKE ? OR clients.prezime ILIKE ?)",
			"%"+filter.ClientName+"%",
			"%"+filter.ClientName+"%",
		)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	if err := query.
		Order("clients.prezime ASC NULLS LAST, clients.ime ASC NULLS LAST, accounts.id ASC").
		Offset(offset).
		Limit(pageSize).
		Find(&accounts).Error; err != nil {
		return nil, 0, err
	}

	return accounts, total, nil
}

func (r *AccountRepository) UpdateFields(id uint, fields map[string]interface{}) error {
	return r.db.Model(&models.Account{}).Where("id = ?", id).Updates(fields).Error
}

func (r *AccountRepository) ExistsByNameForClient(clientID uint, naziv string, excludeID uint) (bool, error) {
	var count int64
	query := r.db.Model(&models.Account{}).Where("client_id = ? AND naziv = ?", clientID, naziv)
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
