package models

import "time"

type Transfer struct {
	ID                    uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	RacunPosiljaocaID     uint       `gorm:"not null" json:"racun_posiljaoca_id"`
	RacunPrimaocaID       uint       `gorm:"not null" json:"racun_primaoca_id"`
	Iznos                 float64    `gorm:"not null" json:"iznos"`
	ValutaIznosa          string     `json:"valuta_iznosa"`
	KonvertovaniIznos     float64    `json:"konvertovani_iznos"`
	Kurs                  float64    `json:"kurs"`
	Provizija             float64    `json:"provizija"`
	ProvizijaProcent      float64    `json:"provizija_procent"`
	Svrha                 string     `json:"svrha"`
	Status                string     `gorm:"default:'u_obradi'" json:"status"` // uspesno | neuspesno | u_obradi
	VerifikacioniKod      string     `json:"-"`
	VerificationExpiresAt *time.Time `json:"verification_expires_at,omitempty"`
	BrojPokusaja          int        `gorm:"default:0" json:"broj_pokusaja"`
	VremeTransakcije      time.Time  `json:"vreme_transakcije"`
	RacunPosiljaoca       Account    `gorm:"foreignKey:RacunPosiljaocaID" json:"racun_posiljaoca,omitempty"`
	RacunPrimaoca         Account    `gorm:"foreignKey:RacunPrimaocaID" json:"racun_primaoca,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}
