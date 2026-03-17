package models

import "time"

type Payment struct {
	ID                uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	RacunPosiljaocaID uint      `gorm:"not null" json:"racun_posiljaoca_id"`
	RacunPrimaocaBroj string    `gorm:"not null" json:"racun_primaoca_broj"`
	Iznos             float64   `gorm:"not null" json:"iznos"`
	SifraPlacanja     string    `json:"sifra_placanja"`
	PozivNaBroj       string    `json:"poziv_na_broj"`
	Svrha             string    `json:"svrha"`
	Status            string    `gorm:"default:'u_obradi'" json:"status"` // u_obradi | uspesno | neuspesno | stornirano
	VerifikacioniKod  string    `json:"-"`
	RecipientID       *uint     `json:"recipient_id"`
	VremeTransakcije  time.Time `json:"vreme_transakcije"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
