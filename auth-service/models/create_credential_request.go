package models

type CreateCredentialRequest struct {
	EmployeeID int64  `json:"employee_id"`
	Email      string `json:"email"`
	IsActive   bool   `json:"is_active"`
}