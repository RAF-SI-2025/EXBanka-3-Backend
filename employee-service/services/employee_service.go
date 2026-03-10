package services

import (
	"errors"
	"strings"

	"employee-service/models"
	"employee-service/repository"
)

type EmployeeService struct {
	EmployeeRepo *repository.EmployeeRepository
	AuthClient   *AuthClient
}

func NewEmployeeService(repo *repository.EmployeeRepository, authClient *AuthClient) *EmployeeService {
	return &EmployeeService{
		EmployeeRepo: repo,
		AuthClient:   authClient,
	}
}

func (s *EmployeeService) CreateEmployee(req *models.CreateEmployeeRequest) (*models.Employee, *models.CreateCredentialResponse, error) {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Email == "" {
		return nil, nil, errors.New("email is required")
	}

	if req.FirstName == "" || req.LastName == "" {
		return nil, nil, errors.New("first name and last name are required")
	}

	employee := &models.Employee{
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		DateOfBirth: req.DateOfBirth,
		Gender:      req.Gender,
		Email:       req.Email,
		PhoneNumber: req.PhoneNumber,
		Address:     req.Address,
		Username:    req.Username,
		Position:    req.Position,
		Department:  req.Department,
		Active:      true,
	}

	err := s.EmployeeRepo.Create(employee)
	if err != nil {
		return nil, nil, err
	}

	credentialResponse, err := s.AuthClient.CreateCredential(employee.ID, employee.Email, false)
	if err != nil {
		return nil, nil, err
	}

	return employee, credentialResponse, nil
}

func (s *EmployeeService) GetAllEmployees(filter *models.EmployeeFilter) ([]models.Employee, error) {
	return s.EmployeeRepo.GetAll(filter)
}