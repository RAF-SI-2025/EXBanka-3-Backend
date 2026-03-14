package models

import "time"

type Token struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	EmployeeID uint      `gorm:"not null;index" json:"employee_id"`
	Token      string    `gorm:"uniqueIndex;not null" json:"token"`
	Type       string    `gorm:"not null" json:"type"`
	ExpiresAt  time.Time `gorm:"not null" json:"expires_at"`
	Used       bool      `gorm:"default:false" json:"used"`
	CreatedAt  time.Time `json:"created_at"`

	Employee Employee `gorm:"foreignKey:EmployeeID" json:"-"`
}

const (
	TokenTypeActivation = "activation"
	TokenTypeReset      = "reset"
)
