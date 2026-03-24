package models

import "time"

// ValidInstallmentStatuses returns the allowed values for LoanInstallment.Status.
// Exported so tests can verify the business data without a DB.
func ValidInstallmentStatuses() []string {
	return []string{"ocekuje", "placena", "kasni"}
}

// LoanInstallment represents one monthly installment of a loan.
type LoanInstallment struct {
	ID                  uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	LoanID              uint       `gorm:"not null" json:"loan_id"`
	RedniBroj           int        `gorm:"not null" json:"redni_broj"`              // 1, 2, 3…
	Iznos               float64    `gorm:"not null" json:"iznos"`
	KamataStopaSnapshot float64    `gorm:"not null" json:"kamata_stopa_snapshot"`   // rate at the time of payment
	DatumDospeca        time.Time  `gorm:"not null" json:"datum_dospeca"`
	DatumPlacanja       *time.Time `json:"datum_placanja"`                          // nil until paid
	Status              string     `gorm:"not null;default:'ocekuje'" json:"status"` // ocekuje | placena | kasni
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`

	Loan Loan `gorm:"foreignKey:LoanID" json:"loan,omitempty"`
}
