package repository

import (
	"errors"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"gorm.io/gorm"
)

type ActuaryProfileRepository struct {
	db *gorm.DB
}

func NewActuaryProfileRepository(db *gorm.DB) *ActuaryProfileRepository {
	return &ActuaryProfileRepository{db: db}
}

func (r *ActuaryProfileRepository) FindByEmployeeID(employeeID uint) (*models.ActuaryProfile, error) {
	var profile models.ActuaryProfile
	if err := r.db.Where("employee_id = ?", employeeID).First(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &profile, nil
}

func (r *ActuaryProfileRepository) Upsert(profile *models.ActuaryProfile) error {
	return r.db.Where("employee_id = ?", profile.EmployeeID).
		Assign(profile).
		FirstOrCreate(profile).Error
}

func (r *ActuaryProfileRepository) DeleteByEmployeeID(employeeID uint) error {
	return r.db.Where("employee_id = ?", employeeID).Delete(&models.ActuaryProfile{}).Error
}

func (r *ActuaryProfileRepository) UpdateLimit(employeeID uint, limit *float64) error {
	return r.db.Model(&models.ActuaryProfile{}).
		Where("employee_id = ?", employeeID).
		Update("trading_limit", limit).Error
}

func (r *ActuaryProfileRepository) ResetUsedLimit(employeeID uint) error {
	return r.db.Model(&models.ActuaryProfile{}).
		Where("employee_id = ?", employeeID).
		Update("used_limit", 0).Error
}

func (r *ActuaryProfileRepository) SetNeedApproval(employeeID uint, needApproval bool) error {
	return r.db.Model(&models.ActuaryProfile{}).
		Where("employee_id = ?", employeeID).
		Update("need_approval", needApproval).Error
}

// ResetAllAgentUsedLimits resets used_limit to 0 for all agents (those with a non-null limit).
// Supervisors have NULL limit and are excluded.
func (r *ActuaryProfileRepository) ResetAllAgentUsedLimits() (int64, error) {
	result := r.db.Model(&models.ActuaryProfile{}).
		Where("trading_limit IS NOT NULL").
		Update("used_limit", 0)
	return result.RowsAffected, result.Error
}
