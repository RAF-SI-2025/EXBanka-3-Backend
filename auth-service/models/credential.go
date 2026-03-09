package models

import "time"

type Credential struct {
	ID                int64      `json:"id"`
	EmployeeID        int64      `json:"employee_id"`
	Email             string     `json:"email"`
	PasswordHash      string     `json:"-"`
	SaltPassword      string     `json:"-"`
	IsActive          bool       `json:"is_active"`
	ActivationToken   *string    `json:"-"`
	ResetToken        *string    `json:"-"`
	ResetTokenExpires *time.Time `json:"-"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}