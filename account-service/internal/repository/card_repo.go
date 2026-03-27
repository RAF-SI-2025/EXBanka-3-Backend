package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"gorm.io/gorm"
)

type CardRepository struct {
	db *gorm.DB
}

func NewCardRepository(db *gorm.DB) *CardRepository {
	return &CardRepository{db: db}
}

func (r *CardRepository) Create(card *models.Card) error {
	return r.db.Create(card).Error
}

func (r *CardRepository) FindByID(id uint) (*models.Card, error) {
	var card models.Card
	if err := r.db.First(&card, id).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

func (r *CardRepository) CountByAccountID(accountID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.Card{}).Where("account_id = ?", accountID).Count(&count).Error
	return count, err
}

func (r *CardRepository) CountByClientAndAccount(clientID, accountID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.Card{}).
		Where("client_id = ? AND account_id = ?", clientID, accountID).
		Count(&count).Error
	return count, err
}

func (r *CardRepository) CountByOvlascenoLice(ovlascenoLiceID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.Card{}).
		Where("ovlasceno_lice_id = ?", ovlascenoLiceID).
		Count(&count).Error
	return count, err
}

func (r *CardRepository) ListByAccountID(accountID uint) ([]models.Card, error) {
	var cards []models.Card
	err := r.db.Where("account_id = ?", accountID).Find(&cards).Error
	return cards, err
}

func (r *CardRepository) ListByClientID(clientID uint) ([]models.Card, error) {
	var cards []models.Card
	err := r.db.Where("client_id = ?", clientID).Find(&cards).Error
	return cards, err
}

func (r *CardRepository) Save(card *models.Card) error {
	return r.db.Save(card).Error
}
