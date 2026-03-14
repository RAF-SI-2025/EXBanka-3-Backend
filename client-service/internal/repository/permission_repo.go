package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/models"
	"gorm.io/gorm"
)

type PermissionRepository struct {
	db *gorm.DB
}

func NewPermissionRepository(db *gorm.DB) *PermissionRepository {
	return &PermissionRepository{db: db}
}

func (r *PermissionRepository) FindByNamesForSubject(names []string, subjectType string) ([]models.Permission, error) {
	var perms []models.Permission
	err := r.db.Where("name IN ? AND subject_type = ?", names, subjectType).Find(&perms).Error
	return perms, err
}
