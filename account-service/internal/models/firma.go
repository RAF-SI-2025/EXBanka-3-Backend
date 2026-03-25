package models

import "time"

type Firma struct {
	ID                uint            `gorm:"primaryKey;autoIncrement" json:"id"`
	Naziv             string          `gorm:"not null" json:"naziv"`
	MaticniBroj       string          `gorm:"uniqueIndex;not null" json:"maticni_broj"`
	PIB               string          `gorm:"uniqueIndex;not null" json:"pib"`
	SifraDelatnostiID *uint           `json:"sifra_delatnosti_id,omitempty"`
	SifraDelatnosti   SifraDelatnosti `gorm:"foreignKey:SifraDelatnostiID" json:"sifra_delatnosti,omitempty"`
	Adresa            string          `json:"adresa"`
	VlasnikID         *uint           `json:"vlasnik_id"`
	Client            *Client         `gorm:"foreignKey:VlasnikID" json:"vlasnik,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}
