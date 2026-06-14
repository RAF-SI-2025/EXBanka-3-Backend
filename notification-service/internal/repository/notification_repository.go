package repository

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/models"
	"gorm.io/gorm"
)

type NotificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Create(n *models.Notification) error {
	return r.db.Create(n).Error
}

func (r *NotificationRepository) ListByUser(userID uint, userType string, unreadOnly bool, limit int) ([]models.Notification, error) {
	var out []models.Notification
	q := r.db.Where("user_id = ? AND user_type = ?", userID, userType)
	if unreadOnly {
		q = q.Where("is_read = ?", false)
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	err := q.Order("created_at DESC").Limit(limit).Find(&out).Error
	return out, err
}

func (r *NotificationRepository) UnreadCount(userID uint, userType string) (int64, error) {
	var count int64
	err := r.db.Model(&models.Notification{}).
		Where("user_id = ? AND user_type = ? AND is_read = ?", userID, userType, false).
		Count(&count).Error
	return count, err
}

func (r *NotificationRepository) GetByID(id uint) (*models.Notification, error) {
	var n models.Notification
	err := r.db.First(&n, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *NotificationRepository) MarkRead(id uint) error {
	return r.db.Model(&models.Notification{}).Where("id = ?", id).Update("is_read", true).Error
}

func (r *NotificationRepository) MarkAllRead(userID uint, userType string) error {
	return r.db.Model(&models.Notification{}).
		Where("user_id = ? AND user_type = ? AND is_read = ?", userID, userType, false).
		Update("is_read", true).Error
}

func (r *NotificationRepository) Delete(id uint) error {
	return r.db.Delete(&models.Notification{}, id).Error
}
