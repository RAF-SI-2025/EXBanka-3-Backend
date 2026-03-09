package services

import (
	"errors"
	"strings"

	"auth-service/models"
	"auth-service/repository"
)

type CredentialService struct {
	CredentialRepo *repository.CredentialRepository
}

func NewCredentialService(credentialRepo *repository.CredentialRepository) *CredentialService {
	return &CredentialService{
		CredentialRepo: credentialRepo,
	}
}

func (s *CredentialService) CreateCredential(req *models.CreateCredentialRequest) (*models.Credential, error) {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.EmployeeID <= 0 {
		return nil, errors.New("employee_id must be greater than 0")
	}

	if req.Email == "" {
		return nil, errors.New("email is required")
	}

	if !strings.Contains(req.Email, "@") {
		return nil, errors.New("invalid email format")
	}

	exists, err := s.CredentialRepo.ExistsByEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("email already exists")
	}

	existingByEmployee, err := s.CredentialRepo.GetByEmployeeID(req.EmployeeID)
	if err == nil && existingByEmployee != nil {
		return nil, errors.New("credential for this employee already exists")
	}

	activationToken, err := GenerateRandomToken(32)
	if err != nil {
		return nil, err
	}

	credential := &models.Credential{
		EmployeeID:      req.EmployeeID,
		Email:           req.Email,
		IsActive:        req.IsActive,
		ActivationToken: &activationToken,
	}

	err = s.CredentialRepo.Create(credential)
	if err != nil {
		return nil, err
	}

	return credential, nil
}