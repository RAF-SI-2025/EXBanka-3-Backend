package models

import "time"

// AuditLog records who did what and when for privileged operations.
type AuditLog struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
	Action       string    `gorm:"not null;index" json:"action"`
	ActorID      uint      `gorm:"not null;index" json:"actor_id"`
	ResourceID   *uint     `json:"resource_id,omitempty"`
	ResourceType string    `json:"resource_type,omitempty"`
	OldValue     string    `gorm:"type:text" json:"old_value,omitempty"`
	NewValue     string    `gorm:"type:text" json:"new_value,omitempty"`
}
