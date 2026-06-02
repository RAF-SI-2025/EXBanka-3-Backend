package models

import "time"

// PriceAlert stores a user-defined threshold notification.
// NotificationEmail is captured from JWT claims at creation time so
// the cron job can send the email without calling any other service.
type PriceAlert struct {
	ID                uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID            uint      `gorm:"not null;index:idx_alert_user"  json:"user_id"`
	UserType          string    `gorm:"not null;index:idx_alert_user"  json:"user_type"` // "employee" | "client"
	Ticker            string    `gorm:"not null"                        json:"ticker"`
	Condition         string    `gorm:"not null"                        json:"condition"` // "ABOVE" | "BELOW"
	Threshold         float64   `gorm:"not null"                        json:"threshold"`
	NotificationEmail string    `gorm:"not null"                        json:"notification_email"`
	IsActive          bool      `gorm:"not null;default:true"           json:"is_active"`
	CreatedAt         time.Time `                                        json:"created_at"`
}
