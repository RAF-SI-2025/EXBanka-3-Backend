package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"gorm.io/gorm"
)

type PaymentRepository struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func (r *PaymentRepository) Create(p *models.Payment) error {
	return r.db.Create(p).Error
}

func (r *PaymentRepository) FindByID(id uint) (*models.Payment, error) {
	var p models.Payment
	if err := r.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PaymentRepository) Save(p *models.Payment) error {
	return r.db.Save(p).Error
}

func (r *PaymentRepository) ListByAccountID(accountID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	query := r.db.Model(&models.Payment{}).Where("racun_posiljaoca_id = ?", accountID)
	query = applyPaymentFilters(query, filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.Order("vreme_transakcije DESC").Order("id DESC")
	query = applyPagination(query, filter.Page, filter.PageSize)
	var payments []models.Payment
	if err := query.Find(&payments).Error; err != nil {
		return nil, 0, err
	}
	return payments, total, nil
}

func (r *PaymentRepository) ListByClientID(clientID uint, filter models.PaymentFilter) ([]models.Payment, int64, error) {
	subQuery := r.db.Model(&models.Account{}).Select("id").Where("client_id = ?", clientID)
	query := r.db.Model(&models.Payment{}).Where("racun_posiljaoca_id IN (?)", subQuery)
	query = applyPaymentFilters(query, filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.Order("vreme_transakcije DESC").Order("id DESC")
	query = applyPagination(query, filter.Page, filter.PageSize)
	var payments []models.Payment
	if err := query.Find(&payments).Error; err != nil {
		return nil, 0, err
	}
	return payments, total, nil
}

func applyPaymentFilters(query *gorm.DB, filter models.PaymentFilter) *gorm.DB {
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.DateFrom != nil {
		query = query.Where("vreme_transakcije >= ?", filter.DateFrom)
	}
	if filter.DateTo != nil {
		query = query.Where("vreme_transakcije <= ?", filter.DateTo)
	}
	if filter.MinAmount != nil {
		query = query.Where("iznos >= ?", *filter.MinAmount)
	}
	if filter.MaxAmount != nil {
		query = query.Where("iznos <= ?", *filter.MaxAmount)
	}
	return query
}

func applyPagination(query *gorm.DB, page, pageSize int) *gorm.DB {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	return query.Offset((page - 1) * pageSize).Limit(pageSize)
}
