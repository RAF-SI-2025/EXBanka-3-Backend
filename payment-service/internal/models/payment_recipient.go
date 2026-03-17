package models

import "time"

type PaymentRecipient struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ClientID   uint      `gorm:"not null" json:"client_id"`
	Naziv      string    `gorm:"not null" json:"naziv"`
	BrojRacuna string    `gorm:"not null" json:"broj_racuna"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
