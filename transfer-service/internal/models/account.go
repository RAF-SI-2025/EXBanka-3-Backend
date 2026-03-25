package models

import "time"

// Account is a read-only reference type used by transfer-service
// to look up account details without owning the account table.
type Account struct {
	ID                uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	BrojRacuna        string    `gorm:"uniqueIndex;size:18;not null" json:"broj_racuna"`
	ClientID          *uint     `json:"client_id"`
	FirmaID           *uint     `json:"firma_id"`
	CurrencyID        uint      `gorm:"not null" json:"currency_id"`
	Tip               string    `gorm:"not null" json:"tip"`
	Vrsta             string    `gorm:"not null" json:"vrsta"`
	Stanje            float64   `gorm:"default:0" json:"stanje"`
	RaspolozivoStanje float64   `gorm:"default:0" json:"raspolozivo_stanje"`
	DnevniLimit       float64   `gorm:"default:100000" json:"dnevni_limit"`
	MesecniLimit      float64   `gorm:"default:1000000" json:"mesecni_limit"`
	DnevnaPotrosnja   float64   `gorm:"default:0" json:"dnevna_potrosnja"`
	MesecnaPotrosnja  float64   `gorm:"default:0" json:"mesecna_potrosnja"`
	Naziv             string    `json:"naziv"`
	Status            string    `gorm:"default:'aktivan'" json:"status"`
	Client            *Client   `gorm:"foreignKey:ClientID" json:"client,omitempty"`
	Currency          Currency  `gorm:"foreignKey:CurrencyID" json:"currency"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
