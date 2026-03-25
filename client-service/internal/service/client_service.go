package service

import (
	"fmt"
	"log/slog"
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

const clientSetupTokenDurationHours = 24

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

func (s *ClientService) CreateClient(input CreateClientInput) (*models.Client, string, error) {
	if err := util.ValidatePhoneNumber(input.BrojTelefona); err != nil {
		return nil, "", err
	}
	if err := util.ValidateEmail(input.Email); err != nil {
		return nil, "", err
	}
	if err := util.ValidateDateOfBirth(time.Unix(input.DatumRodjenja, 0)); err != nil {
		return nil, "", err
	}

	emailExists, err := s.clientRepo.EmailExists(input.Email, 0)
	if err != nil {
		return nil, "", err
	}
	if emailExists {
		return nil, "", fmt.Errorf("email already in use")
	}

	salt, err := util.GenerateSalt()
	if err != nil {
		return nil, "", err
	}

	placeholderSecret, err := util.GenerateRandomSecret()
	if err != nil {
		return nil, "", err
	}

	hashedPassword, err := util.HashPassword(placeholderSecret, salt)
	if err != nil {
		return nil, "", err
	}

	client := &models.Client{
		Ime:            input.Ime,
		Prezime:        input.Prezime,
		DatumRodjenja:  input.DatumRodjenja,
		Pol:            input.Pol,
		Email:          input.Email,
		BrojTelefona:   input.BrojTelefona,
		Adresa:         input.Adresa,
		Password:       hashedPassword,
		SaltPassword:   salt,
		Aktivan:        false,
		PovezaniRacuni: input.PovezaniRacuni,
	}

	if err := s.clientRepo.Create(client); err != nil {
		return nil, "", err
	}

	// Assign default client permissions (client.basic)
	defaultPerms, err := s.permRepo.FindByNamesForSubject(
		[]string{models.PermClientBasic},
		models.PermissionSubjectClient,
	)
	if err == nil && len(defaultPerms) > 0 {
		if permErr := s.clientRepo.SetPermissions(client, defaultPerms); permErr != nil {
			slog.Warn("failed to assign default permissions to new client", "client_id", client.ID, "error", permErr)
		}
	}

	setupToken, err := util.GenerateClientSetupToken(client.ID, client.Email, s.cfg.JWTSecret, clientSetupTokenDurationHours)
	if err != nil {
		return nil, "", err
	}

	return client, setupToken, nil
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
