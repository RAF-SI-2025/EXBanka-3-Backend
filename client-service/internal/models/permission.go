package models

type Permission struct {
	ID          uint     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string   `gorm:"uniqueIndex;not null" json:"name"`
	Description string   `json:"description"`
	SubjectType string   `gorm:"not null;default:employee;index" json:"subject_type"`
	Clients     []Client `gorm:"many2many:client_permissions;" json:"-"`
}

const (
	PermissionSubjectEmployee = "employee"
	PermissionSubjectClient   = "client"
)

const (
	PermAdmin               = "admin"
	PermEmployeeCreate      = "employee.create"
	PermEmployeeRead        = "employee.read"
	PermEmployeeUpdate      = "employee.update"
	PermEmployeeActivate    = "employee.activate"
	PermEmployeePermissions = "employee.permissions"
	PermClientBasic         = "client.basic"
	PermClientTrading       = "client.trading"
)

var DefaultPermissions = []Permission{
	{Name: PermAdmin, Description: "Full administrative access", SubjectType: PermissionSubjectEmployee},
	{Name: PermEmployeeCreate, Description: "Can create new employees", SubjectType: PermissionSubjectEmployee},
	{Name: PermEmployeeRead, Description: "Can read employee data", SubjectType: PermissionSubjectEmployee},
	{Name: PermEmployeeUpdate, Description: "Can update employee data (non-admin targets only)", SubjectType: PermissionSubjectEmployee},
	{Name: PermEmployeeActivate, Description: "Can activate/deactivate employees", SubjectType: PermissionSubjectEmployee},
	{Name: PermEmployeePermissions, Description: "Can manage employee permissions", SubjectType: PermissionSubjectEmployee},
	{Name: PermClientBasic, Description: "Basic client role", SubjectType: PermissionSubjectClient},
	{Name: PermClientTrading, Description: "Trading-enabled client role", SubjectType: PermissionSubjectClient},
}
