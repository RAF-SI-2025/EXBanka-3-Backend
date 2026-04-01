package models

type Permission struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string     `gorm:"uniqueIndex;not null" json:"name"`
	Description string     `json:"description"`
	SubjectType string     `gorm:"not null;default:employee;index" json:"subject_type"`
	Employees   []Employee `gorm:"many2many:employee_permissions;" json:"-"`
}

const (
	PermissionSubjectEmployee = "employee"
	PermissionSubjectClient   = "client"
)

// 4 employee roles (hierarchical: Admin > Supervisor > Agent > Basic)
const (
	PermEmployeeBasic      = "employeeBasic"
	PermEmployeeAgent      = "employeeAgent"
	PermEmployeeSupervisor = "employeeSupervisor"
	PermEmployeeAdmin      = "employeeAdmin"
)

// 2 client roles
const (
	PermClientBasic   = "clientBasic"
	PermClientTrading = "clientTrading"
)

// Spec-compatible aliases (Sprint 4 backlog names)
const (
	PermCanTrade    = PermClientTrading     // canTrade → clientTrading
	PermIsAgent     = PermEmployeeAgent     // isAgent → employeeAgent
	PermIsSupervisor = PermEmployeeSupervisor // isSupervisor → employeeSupervisor
)

var DefaultPermissions = []Permission{
	{Name: PermEmployeeBasic, Description: "Osnovno poslovanje banke, upravljanje klijentima", SubjectType: PermissionSubjectEmployee},
	{Name: PermEmployeeAgent, Description: "Osnovno poslovanje banke, upravljanje klijentima, trgovina hartijama sa berze uz limite", SubjectType: PermissionSubjectEmployee},
	{Name: PermEmployeeSupervisor, Description: "Osnovno poslovanje banke, upravljanje klijentima, trgovina hartijama sa berze bez limita, OTC, upravljanje fondovima i agentima", SubjectType: PermissionSubjectEmployee},
	{Name: PermEmployeeAdmin, Description: "Osnovno poslovanje banke, upravljanje klijentima, trgovina hartijama sa berze bez limita, OTC, upravljanje fondovima i agentima, upravlja svim zaposlenima", SubjectType: PermissionSubjectEmployee},
	{Name: PermClientBasic, Description: "Osnovno poslovanje banke", SubjectType: PermissionSubjectClient},
	{Name: PermClientTrading, Description: "Trgovina hartijama sa berze, OTC, investiranje u fondove", SubjectType: PermissionSubjectClient},
}

// EmployeeRoleLevel returns the hierarchy level (higher = more access).
func EmployeeRoleLevel(role string) int {
	switch role {
	case PermEmployeeAdmin:
		return 4
	case PermEmployeeSupervisor:
		return 3
	case PermEmployeeAgent:
		return 2
	case PermEmployeeBasic:
		return 1
	default:
		return 0
	}
}

// HasEmployeeRole checks if the user's permissions include the required role or higher.
func HasEmployeeRole(userPermissions []string, requiredRole string) bool {
	requiredLevel := EmployeeRoleLevel(requiredRole)
	for _, p := range userPermissions {
		if EmployeeRoleLevel(p) >= requiredLevel {
			return true
		}
	}
	return false
}
