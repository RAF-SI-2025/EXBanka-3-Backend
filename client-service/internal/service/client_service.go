package service

import (
	"fmt"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/util"
	"gorm.io/gorm"
)

type ClientService struct {
	cfg        *config.Config
	clientRepo repository.ClientRepositoryInterface
	permRepo   repository.PermissionRepositoryInterface
}

func NewClientService(cfg *config.Config, db *gorm.DB) *ClientService {
	return &ClientService{
		cfg:        cfg,
		clientRepo: repository.NewClientRepository(db),
		permRepo:   repository.NewPermissionRepository(db),
	}
}

// NewClientServiceWithRepos constructs a ClientService with injected repository interfaces,
// allowing mock implementations to be used in unit tests.
func NewClientServiceWithRepos(cfg *config.Config, clientRepo repository.ClientRepositoryInterface, permRepo repository.PermissionRepositoryInterface) *ClientService {
	return &ClientService{
		cfg:        cfg,
		clientRepo: clientRepo,
		permRepo:   permRepo,
	}
}

type CreateClientInput struct {
	Ime            string
	Prezime        string
	DatumRodjenja  int64
	Pol            string
	Email          string
	BrojTelefona   string
	Adresa         string
	PovezaniRacuni string
}

type UpdateClientInput struct {
	Ime            string
	Prezime        string
	DatumRodjenja  int64
	Pol            string
	Email          string
	BrojTelefona   string
	Adresa         string
	PovezaniRacuni string
}

func (s *ClientService) CreateClient(input CreateClientInput) (*models.Client, error) {
	if err := util.ValidatePhoneNumber(input.BrojTelefona); err != nil {
		return nil, err
	}
	if err := util.ValidateEmail(input.Email); err != nil {
		return nil, err
	}
	if err := util.ValidateDateOfBirth(time.Unix(input.DatumRodjenja, 0)); err != nil {
		return nil, err
	}

	emailExists, err := s.clientRepo.EmailExists(input.Email, 0)
	if err != nil {
		return nil, err
	}
	if emailExists {
		return nil, fmt.Errorf("email already in use")
	}

	client := &models.Client{
		Ime:            input.Ime,
		Prezime:        input.Prezime,
		DatumRodjenja:  input.DatumRodjenja,
		Pol:            input.Pol,
		Email:          input.Email,
		BrojTelefona:   input.BrojTelefona,
		Adresa:         input.Adresa,
		Password:       "pending",
		SaltPassword:   "pending",
		PovezaniRacuni: input.PovezaniRacuni,
	}

	if err := s.clientRepo.Create(client); err != nil {
		return nil, err
	}

	return client, nil
}

func (s *ClientService) GetClient(id uint) (*models.Client, error) {
	return s.clientRepo.FindByID(id)
}

func (s *ClientService) ListClients(filter repository.ClientFilter) ([]models.Client, int64, error) {
	return s.clientRepo.List(filter)
}

func (s *ClientService) UpdateClient(id uint, input UpdateClientInput) (*models.Client, error) {
	client, err := s.clientRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("client not found")
	}

	if err := util.ValidatePhoneNumber(input.BrojTelefona); err != nil {
		return nil, err
	}
	if err := util.ValidateEmail(input.Email); err != nil {
		return nil, err
	}
	if err := util.ValidateDateOfBirth(time.Unix(input.DatumRodjenja, 0)); err != nil {
		return nil, err
	}

	if input.Email != client.Email {
		exists, err := s.clientRepo.EmailExists(input.Email, id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("email already in use")
		}
	}

	client.Ime = input.Ime
	client.Prezime = input.Prezime
	client.DatumRodjenja = input.DatumRodjenja
	client.Pol = input.Pol
	client.Email = input.Email
	client.BrojTelefona = input.BrojTelefona
	client.Adresa = input.Adresa
	client.PovezaniRacuni = input.PovezaniRacuni

	if err := s.clientRepo.Update(client); err != nil {
		return nil, err
	}

	return client, nil
}

func (s *ClientService) UpdateClientPermissions(id uint, permissionNames []string) (*models.Client, error) {
	client, err := s.clientRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("client not found")
	}

	perms, err := s.permRepo.FindByNamesForSubject(permissionNames, models.PermissionSubjectClient)
	if err != nil {
		return nil, err
	}
	if len(perms) != len(permissionNames) {
		return nil, fmt.Errorf("clients can only be assigned client permissions")
	}

	if err := s.clientRepo.SetPermissions(client, perms); err != nil {
		return nil, err
	}

	return s.clientRepo.FindByID(id)
}
