package models

import "time"

// Notification is a single in-app (and optionally emailed) message addressed to
// one user. Ownership follows the same (UserID, UserType) pattern used by
// watchlists and price alerts across the platform.
type Notification struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"            json:"id"`
	UserID    uint      `gorm:"not null;index:idx_notif_user"       json:"user_id"`
	UserType  string    `gorm:"not null;index:idx_notif_user"       json:"user_type"` // "employee" | "client"
	Type      string    `gorm:"not null"                            json:"type"`      // e.g. "ORDER_CREATED"
	Title     string    `gorm:"not null"                            json:"title"`
	Body      string    `gorm:"type:text"                           json:"body"`
	Link      string    `                                           json:"link"` // optional deep-link path in the SPA
	IsRead    bool      `gorm:"not null;default:false;index:idx_notif_read" json:"is_read"`
	CreatedAt time.Time `gorm:"index"                               json:"created_at"`
}
