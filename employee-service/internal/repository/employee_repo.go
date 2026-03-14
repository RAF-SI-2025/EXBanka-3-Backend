package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"gorm.io/gorm"
)

type EmployeeRepository struct {
	db *gorm.DB
}

func NewEmployeeRepository(db *gorm.DB) *EmployeeRepository {
	return &EmployeeRepository{db: db}
}

type EmployeeFilter struct {
	Email    string
	Name     string
	Pozicija string
	Page     int
	PageSize int
}

func (r *EmployeeRepository) Create(emp *models.Employee) error {
	return r.db.Create(emp).Error
}

func (r *EmployeeRepository) FindByID(id uint) (*models.Employee, error) {
	var emp models.Employee
	if err := r.db.Preload("Permissions").First(&emp, id).Error; err != nil {
		return nil, err
	}
	return &emp, nil
}

func (r *EmployeeRepository) FindByEmail(email string) (*models.Employee, error) {
	var emp models.Employee
	if err := r.db.Preload("Permissions").Where("email = ?", email).First(&emp).Error; err != nil {
		return nil, err
	}
	return &emp, nil
}

func (r *EmployeeRepository) FindByUsername(username string) (*models.Employee, error) {
	var emp models.Employee
	if err := r.db.Preload("Permissions").Where("username = ?", username).First(&emp).Error; err != nil {
		return nil, err
	}
	return &emp, nil
}

func (r *EmployeeRepository) List(filter EmployeeFilter) ([]models.Employee, int64, error) {
	var employees []models.Employee
	var total int64

	query := r.db.Model(&models.Employee{}).Preload("Permissions")

	if filter.Email != "" {
		query = query.Where("email ILIKE ?", "%"+filter.Email+"%")
	}
	if filter.Name != "" {
		query = query.Where("ime ILIKE ? OR prezime ILIKE ?", "%"+filter.Name+"%", "%"+filter.Name+"%")
	}
	if filter.Pozicija != "" {
		query = query.Where("pozicija ILIKE ?", "%"+filter.Pozicija+"%")
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

	err := query.Limit(pageSize).Offset(offset).Find(&employees).Error
	return employees, total, err
}

func (r *EmployeeRepository) Update(emp *models.Employee) error {
	return r.db.Save(emp).Error
}

func (r *EmployeeRepository) UpdateFields(id uint, fields map[string]interface{}) error {
	return r.db.Model(&models.Employee{}).Where("id = ?", id).Updates(fields).Error
}

func (r *EmployeeRepository) SetPermissions(emp *models.Employee, permissions []models.Permission) error {
	return r.db.Model(emp).Association("Permissions").Replace(permissions)
}

func (r *EmployeeRepository) EmailExists(email string, excludeID uint) (bool, error) {
	var count int64
	err := r.db.Model(&models.Employee{}).
		Where("email = ? AND id != ?", email, excludeID).
		Count(&count).Error
	return count > 0, err
}

func (r *EmployeeRepository) UsernameExists(username string, excludeID uint) (bool, error) {
	var count int64
	err := r.db.Model(&models.Employee{}).
		Where("username = ? AND id != ?", username, excludeID).
		Count(&count).Error
	return count > 0, err
}
