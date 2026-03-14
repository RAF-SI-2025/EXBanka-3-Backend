package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
	"gorm.io/gorm"
)

type EmployeeRepository struct {
	db *gorm.DB
}

func NewEmployeeRepository(db *gorm.DB) *EmployeeRepository {
	return &EmployeeRepository{db: db}
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

func (r *EmployeeRepository) UpdateFields(id uint, fields map[string]interface{}) error {
	return r.db.Model(&models.Employee{}).Where("id = ?", id).Updates(fields).Error
}
