package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
	"gorm.io/gorm"
)

type ClientRepository struct {
	db *gorm.DB
}

func NewClientRepository(db *gorm.DB) *ClientRepository {
	return &ClientRepository{db: db}
}

func (r *ClientRepository) FindByEmail(email string) (*models.Client, error) {
	var client models.Client
	err := r.db.Preload("Permissions").Where("email = ?", email).First(&client).Error
	if err != nil {
		return nil, err
	}
	return &client, nil
}

var _ ClientRepositoryInterface = (*ClientRepository)(nil)
