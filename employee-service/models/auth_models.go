package models

import "time"

type CreateCredentialRequest struct {
	EmployeeID int64  `json:"employee_id"`
	Email      string `json:"email"`
	IsActive   bool   `json:"is_active"`
}

type CreateCredentialResponse struct {
	Message    string                 `json:"message"`
	Credential CredentialResponseData `json:"credential"`
}

type CredentialResponseData struct {
	ID              int64      `json:"id"`
	EmployeeID      int64      `json:"employee_id"`
	Email           string     `json:"email"`
	IsActive        bool       `json:"is_active"`
	ActivationToken *string    `json:"activation_token"`
	CreatedAt       time.Time  `json:"created_at"`
}