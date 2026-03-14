package models

import (
	"time"

	"gorm.io/gorm"
)

type Employee struct {
	ID            uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	Ime           string         `gorm:"not null" json:"ime"`
	Prezime       string         `gorm:"not null" json:"prezime"`
	DatumRodjenja time.Time      `json:"datum_rodjenja"`
	Pol           string         `gorm:"not null" json:"pol"`
	Email         string         `gorm:"uniqueIndex;not null" json:"email"`
	BrojTelefona  string         `json:"broj_telefona"`
	Adresa        string         `json:"adresa"`
	Username      string         `gorm:"uniqueIndex;not null" json:"username"`
	Password      string         `gorm:"not null" json:"-"`
	SaltPassword  string         `gorm:"not null;column:salt_password" json:"-"`
	Pozicija      string         `json:"pozicija"`
	Departman     string         `json:"departman"`
	Aktivan       bool           `gorm:"default:true" json:"aktivan"`
	Permissions   []Permission   `gorm:"many2many:employee_permissions;" json:"permissions,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

func (e *Employee) PermissionNames() []string {
	names := make([]string, 0, len(e.Permissions))
	for _, p := range e.Permissions {
		names = append(names, p.Name)
	}
	return names
}
