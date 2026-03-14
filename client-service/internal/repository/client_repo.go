package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/models"
	"gorm.io/gorm"
)

type ClientRepository struct {
	db *gorm.DB
}

func NewClientRepository(db *gorm.DB) *ClientRepository {
	return &ClientRepository{db: db}
}

type ClientFilter struct {
	Email    string
	Name     string
	Page     int
	PageSize int
}

func (r *ClientRepository) Create(client *models.Client) error {
	return r.db.Create(client).Error
}

func (r *ClientRepository) FindByID(id uint) (*models.Client, error) {
	var client models.Client
	err := r.db.Preload("Permissions").First(&client, id).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

func (r *ClientRepository) List(filter ClientFilter) ([]models.Client, int64, error) {
	var clients []models.Client
	var total int64

	query := r.db.Model(&models.Client{}).Preload("Permissions")

	if filter.Email != "" {
		query = query.Where("email ILIKE ?", "%"+filter.Email+"%")
	}
	if filter.Name != "" {
		query = query.Where("ime ILIKE ? OR prezime ILIKE ?", "%"+filter.Name+"%", "%"+filter.Name+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	err := query.Limit(pageSize).Offset(offset).Find(&clients).Error
	return clients, total, err
}

func (r *ClientRepository) Update(client *models.Client) error {
	return r.db.Save(client).Error
}

func (r *ClientRepository) EmailExists(email string, excludeID uint) (bool, error) {
	var count int64
	err := r.db.Model(&models.Client{}).
		Where("email = ? AND id != ?", email, excludeID).
		Count(&count).Error
	return count > 0, err
}

func (r *ClientRepository) SetPermissions(client *models.Client, permissions []models.Permission) error {
	return r.db.Model(client).Association("Permissions").Replace(permissions)
}
