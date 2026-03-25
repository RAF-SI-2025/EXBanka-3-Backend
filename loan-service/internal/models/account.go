package models

// Account is a shared-table reference model used by loan-service for payout
// and installment collection checks. loan-service does not own this table.
type Account struct {
	ID                uint    `gorm:"primaryKey;autoIncrement" json:"id"`
	BrojRacuna        string  `gorm:"uniqueIndex;size:18;not null" json:"broj_racuna"`
	ClientID          *uint   `json:"client_id"`
	CurrencyID        uint    `gorm:"not null" json:"currency_id"`
	CurrencyKod       string  `gorm:"->;-:migration;column:currency_kod" json:"currency_kod"`
	Stanje            float64 `gorm:"default:0" json:"stanje"`
	RaspolozivoStanje float64 `gorm:"default:0" json:"raspolozivo_stanje"`
	DnevniLimit       float64 `gorm:"default:100000" json:"dnevni_limit"`
	MesecniLimit      float64 `gorm:"default:1000000" json:"mesecni_limit"`
	DnevnaPotrosnja   float64 `gorm:"default:0" json:"dnevna_potrosnja"`
	MesecnaPotrosnja  float64 `gorm:"default:0" json:"mesecna_potrosnja"`
	Status            string  `gorm:"default:'aktivan'" json:"status"`
}
