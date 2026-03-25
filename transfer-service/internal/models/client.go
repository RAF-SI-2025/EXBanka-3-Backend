package models

type Client struct {
	ID      uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Ime     string `gorm:"not null" json:"ime"`
	Prezime string `gorm:"not null" json:"prezime"`
	Email   string `gorm:"uniqueIndex;not null" json:"email"`
}
