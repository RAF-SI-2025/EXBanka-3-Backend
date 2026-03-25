package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"gorm.io/gorm"
)

type TransferRepository struct {
	db *gorm.DB
}

func NewTransferRepository(db *gorm.DB) *TransferRepository {
	return &TransferRepository{db: db}
}

func (r *TransferRepository) Create(transfer *models.Transfer) error {
	return r.db.Create(transfer).Error
}

func (r *TransferRepository) Save(transfer *models.Transfer) error {
	return r.db.Save(transfer).Error
}

func (r *TransferRepository) FindByID(id uint) (*models.Transfer, error) {
	var transfer models.Transfer
	if err := r.db.
		Preload("RacunPosiljaoca.Currency").
		Preload("RacunPrimaoca.Currency").
		First(&transfer, id).Error; err != nil {
		return nil, err
	}
	return &transfer, nil
}

func (r *TransferRepository) ListByAccountID(accountID uint, filter models.TransferFilter) ([]models.Transfer, int64, error) {
	var transfers []models.Transfer
	var total int64

	query := r.db.Model(&models.Transfer{}).
		Where("racun_posiljaoca_id = ? OR racun_primaoca_id = ?", accountID, accountID)

	query = applyTransferFilters(query, filter)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, pageSize, offset := pagination(filter.Page, filter.PageSize)
	_ = page
	if err := query.Offset(offset).Limit(pageSize).
		Order("vreme_transakcije DESC, id DESC").
		Find(&transfers).Error; err != nil {
		return nil, 0, err
	}

	return transfers, total, nil
}

func (r *TransferRepository) ListByClientID(clientID uint, filter models.TransferFilter) ([]models.Transfer, int64, error) {
	var transfers []models.Transfer
	var total int64

	query := r.db.Model(&models.Transfer{}).
		Joins("JOIN accounts sender ON sender.id = transfers.racun_posiljaoca_id").
		Joins("JOIN accounts receiver ON receiver.id = transfers.racun_primaoca_id").
		Where("sender.client_id = ? OR receiver.client_id = ?", clientID, clientID)

	query = applyTransferFilters(query, filter)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, pageSize, offset := pagination(filter.Page, filter.PageSize)
	_ = page
	if err := query.Offset(offset).Limit(pageSize).
		Order("transfers.vreme_transakcije DESC, transfers.id DESC").
		Find(&transfers).Error; err != nil {
		return nil, 0, err
	}

	return transfers, total, nil
}

func applyTransferFilters(query *gorm.DB, filter models.TransferFilter) *gorm.DB {
	if filter.Status != "" {
		query = query.Where("transfers.status = ?", filter.Status)
	}
	if filter.DateFrom != nil {
		query = query.Where("transfers.vreme_transakcije >= ?", filter.DateFrom)
	}
	if filter.DateTo != nil {
		query = query.Where("transfers.vreme_transakcije <= ?", filter.DateTo)
	}
	if filter.MinAmount != nil {
		query = query.Where("transfers.iznos >= ?", *filter.MinAmount)
	}
	if filter.MaxAmount != nil {
		query = query.Where("transfers.iznos <= ?", *filter.MaxAmount)
	}
	return query
}

func pagination(page, pageSize int) (int, int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	return page, pageSize, (page - 1) * pageSize
}
