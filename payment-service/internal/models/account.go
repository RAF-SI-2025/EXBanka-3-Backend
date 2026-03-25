package models

// Account is a read-only reference used for balance checks in payment-service.
type Account struct {
	ID                uint   `gorm:"primaryKey;autoIncrement"`
	BrojRacuna        string `gorm:"uniqueIndex;size:18;not null"`
	ClientID          *uint
	CurrencyID        uint
	CurrencyKod       string  `gorm:"->;-:migration;column:currency_kod"`
	RaspolozivoStanje float64 `gorm:"default:0"`
	Stanje            float64 `gorm:"default:0"`
	DnevniLimit       float64 `gorm:"default:100000"`
	MesecniLimit      float64 `gorm:"default:1000000"`
	DnevnaPotrosnja   float64 `gorm:"default:0"`
	MesecnaPotrosnja  float64 `gorm:"default:0"`
	Status            string  `gorm:"default:'aktivan'"`
}
