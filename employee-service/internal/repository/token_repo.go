package repository

import (
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"gorm.io/gorm"
)

type TokenRepository struct {
	db *gorm.DB
}

func NewTokenRepository(db *gorm.DB) *TokenRepository {
	return &TokenRepository{db: db}
}

func (r *TokenRepository) Create(token *models.Token) error {
	return r.db.Create(token).Error
}

func (r *TokenRepository) FindValid(tokenStr, tokenType string) (*models.Token, error) {
	var token models.Token
	if err := r.db.Preload("Employee").
		Where("token = ? AND type = ? AND used = false AND expires_at > ?",
			tokenStr, tokenType, time.Now()).
		First(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *TokenRepository) InvalidateEmployeeTokens(employeeID uint, tokenType string) error {
	return r.db.Model(&models.Token{}).
		Where("employee_id = ? AND type = ? AND used = false", employeeID, tokenType).
		Update("used", true).Error
}
