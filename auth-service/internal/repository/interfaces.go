package repository

import "github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"

// EmployeeRepositoryInterface defines the methods used by AuthService.
type EmployeeRepositoryInterface interface {
	FindByEmail(email string) (*models.Employee, error)
	FindByID(id uint) (*models.Employee, error)
	UpdateFields(id uint, fields map[string]interface{}) error
}

// TokenRepositoryInterface defines the methods used by AuthService.
type TokenRepositoryInterface interface {
	Create(token *models.Token) error
	FindValid(tokenStr, tokenType string) (*models.Token, error)
	InvalidateEmployeeTokens(employeeID uint, tokenType string) error
}

// ClientRepositoryInterface defines the methods used by AuthService for client authentication.
type ClientRepositoryInterface interface {
	FindByEmail(email string) (*models.Client, error)
	FindByID(id uint) (*models.Client, error)
	UpdateFields(id uint, fields map[string]interface{}) error
}

// Compile-time checks: ensure concrete repositories satisfy their interfaces.
var _ EmployeeRepositoryInterface = (*EmployeeRepository)(nil)
var _ TokenRepositoryInterface = (*TokenRepository)(nil)
