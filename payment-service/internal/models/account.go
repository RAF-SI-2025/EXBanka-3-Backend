package models

// Account is a read-only reference used for balance checks in payment-service.
type Account struct {
	ID                uint    `gorm:"primaryKey;autoIncrement"`
	BrojRacuna        string  `gorm:"uniqueIndex;size:18;not null"`
	ClientID          *uint
	RaspolozivoStanje float64 `gorm:"default:0"`
	Stanje            float64 `gorm:"default:0"`
	DnevniLimit       float64 `gorm:"default:100000"`
	Status            string  `gorm:"default:'aktivan'"`
}
