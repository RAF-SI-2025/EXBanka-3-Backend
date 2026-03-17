package models

import "time"

type Currency struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Kod       string    `gorm:"uniqueIndex;size:3;not null" json:"kod"`
	Naziv     string    `gorm:"not null" json:"naziv"`
	Simbol    string    `json:"simbol"`
	Drzava    string    `json:"drzava"`
	Aktivan   bool      `gorm:"default:true" json:"aktivan"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
