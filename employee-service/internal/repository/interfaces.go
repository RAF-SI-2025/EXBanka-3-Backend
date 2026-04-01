package repository

import "github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"

// EmployeeRepositoryInterface defines the methods used by EmployeeService.
type EmployeeRepositoryInterface interface {
	Create(emp *models.Employee) error
	FindByID(id uint) (*models.Employee, error)
	FindByEmail(email string) (*models.Employee, error)
	ListAll() ([]models.Employee, error)
	List(filter EmployeeFilter) ([]models.Employee, int64, error)
	Update(emp *models.Employee) error
	UpdateFields(id uint, fields map[string]interface{}) error
	SetPermissions(emp *models.Employee, permissions []models.Permission) error
	EmailExists(email string, excludeID uint) (bool, error)
	UsernameExists(username string, excludeID uint) (bool, error)
}

// PermissionRepositoryInterface defines the methods used by EmployeeService.
type PermissionRepositoryInterface interface {
	FindAllBySubject(subjectType string) ([]models.Permission, error)
	FindByNamesForSubject(names []string, subjectType string) ([]models.Permission, error)
}

// TokenRepositoryInterface defines the methods used by EmployeeService.
type TokenRepositoryInterface interface {
	Create(token *models.Token) error
	FindValid(tokenStr, tokenType string) (*models.Token, error)
	InvalidateEmployeeTokens(employeeID uint, tokenType string) error
}

type ActuaryProfileRepositoryInterface interface {
	FindByEmployeeID(employeeID uint) (*models.ActuaryProfile, error)
	Upsert(profile *models.ActuaryProfile) error
	DeleteByEmployeeID(employeeID uint) error
	UpdateLimit(employeeID uint, limit *float64) error
	ResetUsedLimit(employeeID uint) error
	SetNeedApproval(employeeID uint, needApproval bool) error
	ResetAllAgentUsedLimits() (int64, error)
}

var _ EmployeeRepositoryInterface = (*EmployeeRepository)(nil)
var _ PermissionRepositoryInterface = (*PermissionRepository)(nil)
var _ TokenRepositoryInterface = (*TokenRepository)(nil)
var _ ActuaryProfileRepositoryInterface = (*ActuaryProfileRepository)(nil)
