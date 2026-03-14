package handler

import (
	"context"
	"time"

	employeev1 "github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/gen/proto/employee/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/repository"
	svc "github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type EmployeeHandler struct {
	employeev1.UnimplementedEmployeeServiceServer
	svc *svc.EmployeeService
}

func NewEmployeeHandler(cfg *config.Config, db *gorm.DB, notifSvc *svc.NotificationService) *EmployeeHandler {
	return &EmployeeHandler{
		svc: svc.NewEmployeeService(cfg, db, notifSvc),
	}
}

func toEmployeeProto(emp *models.Employee) *employeev1.EmployeeProto {
	perms := make([]*employeev1.PermissionProto, 0, len(emp.Permissions))
	for _, p := range emp.Permissions {
		perms = append(perms, &employeev1.PermissionProto{
			Id:          uint64(p.ID),
			Name:        p.Name,
			Description: p.Description,
		})
	}

	return &employeev1.EmployeeProto{
		Id:            uint64(emp.ID),
		Ime:           emp.Ime,
		Prezime:       emp.Prezime,
		DatumRodjenja: timestamppb.New(emp.DatumRodjenja),
		Pol:           emp.Pol,
		Email:         emp.Email,
		BrojTelefona:  emp.BrojTelefona,
		Adresa:        emp.Adresa,
		Username:      emp.Username,
		Pozicija:      emp.Pozicija,
		Departman:     emp.Departman,
		Aktivan:       emp.Aktivan,
		Permissions:   perms,
	}
}

func toEmployeeListItem(emp *models.Employee) *employeev1.EmployeeListItem {
	return &employeev1.EmployeeListItem{
		Id:              uint64(emp.ID),
		Ime:             emp.Ime,
		Prezime:         emp.Prezime,
		Email:           emp.Email,
		Pozicija:        emp.Pozicija,
		BrojTelefona:    emp.BrojTelefona,
		Aktivan:         emp.Aktivan,
		PermissionNames: emp.PermissionNames(),
	}
}

func (h *EmployeeHandler) CreateEmployee(ctx context.Context, req *employeev1.CreateEmployeeRequest) (*employeev1.CreateEmployeeResponse, error) {
	emp, err := h.svc.CreateEmployee(svc.CreateEmployeeInput{
		Ime:           req.Ime,
		Prezime:       req.Prezime,
		DatumRodjenja: time.Unix(req.DatumRodjenja, 0),
		Pol:           req.Pol,
		Email:         req.Email,
		BrojTelefona:  req.BrojTelefona,
		Adresa:        req.Adresa,
		Username:      req.Username,
		Pozicija:      req.Pozicija,
		Departman:     req.Departman,
	})
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &employeev1.CreateEmployeeResponse{
		Employee: toEmployeeProto(emp),
		Message:  "Employee created. Activation email sent.",
	}, nil
}

func (h *EmployeeHandler) GetEmployee(ctx context.Context, req *employeev1.GetEmployeeRequest) (*employeev1.GetEmployeeResponse, error) {
	emp, err := h.svc.GetEmployee(uint(req.Id))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "employee not found")
	}

	return &employeev1.GetEmployeeResponse{Employee: toEmployeeProto(emp)}, nil
}

func (h *EmployeeHandler) ListEmployees(ctx context.Context, req *employeev1.ListEmployeesRequest) (*employeev1.ListEmployeesResponse, error) {
	filter := repository.EmployeeFilter{
		Email:    req.EmailFilter,
		Name:     req.NameFilter,
		Pozicija: req.PozicijaFilter,
		Page:     int(req.Page),
		PageSize: int(req.PageSize),
	}

	employees, total, err := h.svc.ListEmployees(filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list employees")
	}

	items := make([]*employeev1.EmployeeListItem, 0, len(employees))
	for i := range employees {
		items = append(items, toEmployeeListItem(&employees[i]))
	}

	return &employeev1.ListEmployeesResponse{
		Employees: items,
		Total:     total,
		Page:      req.Page,
		PageSize:  req.PageSize,
	}, nil
}

func (h *EmployeeHandler) UpdateEmployee(ctx context.Context, req *employeev1.UpdateEmployeeRequest) (*employeev1.UpdateEmployeeResponse, error) {
	emp, err := h.svc.UpdateEmployee(uint(req.Id), svc.UpdateEmployeeInput{
		Ime:           req.Ime,
		Prezime:       req.Prezime,
		DatumRodjenja: time.Unix(req.DatumRodjenja, 0),
		Pol:           req.Pol,
		Email:         req.Email,
		BrojTelefona:  req.BrojTelefona,
		Adresa:        req.Adresa,
		Username:      req.Username,
		Pozicija:      req.Pozicija,
		Departman:     req.Departman,
		Aktivan:       req.Aktivan,
	})
	if err != nil {
		if err.Error() == "cannot edit an admin employee" {
			return nil, status.Errorf(codes.PermissionDenied, "%s", err.Error())
		}
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	return &employeev1.UpdateEmployeeResponse{Employee: toEmployeeProto(emp)}, nil
}

func (h *EmployeeHandler) SetEmployeeActive(ctx context.Context, req *employeev1.SetEmployeeActiveRequest) (*employeev1.SetEmployeeActiveResponse, error) {
	if err := h.svc.SetEmployeeActive(uint(req.Id), req.Aktivan); err != nil {
		if err.Error() == "cannot deactivate an admin employee" {
			return nil, status.Errorf(codes.PermissionDenied, "%s", err.Error())
		}
		return nil, status.Errorf(codes.NotFound, "%s", err.Error())
	}

	return &employeev1.SetEmployeeActiveResponse{
		Aktivan: req.Aktivan,
		Message: "Employee status updated",
	}, nil
}

func (h *EmployeeHandler) UpdateEmployeePermissions(ctx context.Context, req *employeev1.UpdateEmployeePermissionsRequest) (*employeev1.UpdateEmployeePermissionsResponse, error) {
	emp, err := h.svc.UpdateEmployeePermissions(uint(req.Id), req.PermissionNames)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s", err.Error())
	}

	perms := make([]*employeev1.PermissionProto, 0, len(emp.Permissions))
	for _, p := range emp.Permissions {
		perms = append(perms, &employeev1.PermissionProto{
			Id:          uint64(p.ID),
			Name:        p.Name,
			Description: p.Description,
		})
	}

	return &employeev1.UpdateEmployeePermissionsResponse{
		Permissions: perms,
		Message:     "Permissions updated",
	}, nil
}

func (h *EmployeeHandler) GetAllPermissions(ctx context.Context, req *employeev1.GetAllPermissionsRequest) (*employeev1.GetAllPermissionsResponse, error) {
	perms, err := h.svc.GetAllPermissions()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get permissions")
	}

	result := make([]*employeev1.PermissionProto, 0, len(perms))
	for _, p := range perms {
		result = append(result, &employeev1.PermissionProto{
			Id:          uint64(p.ID),
			Name:        p.Name,
			Description: p.Description,
		})
	}

	return &employeev1.GetAllPermissionsResponse{Permissions: result}, nil
}
