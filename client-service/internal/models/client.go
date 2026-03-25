package models

import "time"

type Client struct {
	ID             uint         `gorm:"primaryKey;autoIncrement" json:"id"`
	Ime            string       `gorm:"not null" json:"ime"`
	Prezime        string       `gorm:"not null" json:"prezime"`
	DatumRodjenja  int64        `json:"datum_rodjenja"`
	Pol            string       `json:"pol"`
	Email          string       `gorm:"uniqueIndex;not null" json:"email"`
	BrojTelefona   string       `json:"broj_telefona"`
	Adresa         string       `json:"adresa"`
	Password       string       `gorm:"not null" json:"-"`
	SaltPassword   string       `gorm:"not null;column:salt_password" json:"-"`
	Aktivan        bool         `gorm:"not null;default:false" json:"aktivan"`
	PovezaniRacuni string       `json:"povezani_racuni"`
	Permissions    []Permission `gorm:"many2many:client_permissions;" json:"permissions,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

func (c *Client) PermissionNames() []string {
	names := make([]string, 0, len(c.Permissions))
	for _, p := range c.Permissions {
		names = append(names, p.Name)
	}
	return names
}
