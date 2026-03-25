package service_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/service"
)

// ---- mock client repository ----

type mockClientRepo struct {
	createFn         func(client *models.Client) error
	findByIDFn       func(id uint) (*models.Client, error)
	listFn           func(filter repository.ClientFilter) ([]models.Client, int64, error)
	updateFn         func(client *models.Client) error
	emailExistsFn    func(email string, excludeID uint) (bool, error)
	setPermissionsFn func(client *models.Client, permissions []models.Permission) error
}

func (m *mockClientRepo) Create(client *models.Client) error {
	if m.createFn != nil {
		return m.createFn(client)
	}
	return nil
}

func (m *mockClientRepo) FindByID(id uint) (*models.Client, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockClientRepo) List(filter repository.ClientFilter) ([]models.Client, int64, error) {
	if m.listFn != nil {
		return m.listFn(filter)
	}
	return nil, 0, errors.New("not implemented")
}

func (m *mockClientRepo) Update(client *models.Client) error {
	if m.updateFn != nil {
		return m.updateFn(client)
	}
	return nil
}

func (m *mockClientRepo) EmailExists(email string, excludeID uint) (bool, error) {
	if m.emailExistsFn != nil {
		return m.emailExistsFn(email, excludeID)
	}
	return false, nil
}

func (m *mockClientRepo) SetPermissions(client *models.Client, permissions []models.Permission) error {
	if m.setPermissionsFn != nil {
		return m.setPermissionsFn(client, permissions)
	}
	return nil
}

// ---- mock permission repository ----

type mockPermRepo struct {
	findByNamesForSubjectFn func(names []string, subjectType string) ([]models.Permission, error)
}

func (m *mockPermRepo) FindByNamesForSubject(names []string, subjectType string) ([]models.Permission, error) {
	if m.findByNamesForSubjectFn != nil {
		return m.findByNamesForSubjectFn(names, subjectType)
	}
	return nil, errors.New("not implemented")
}

// ---- compile-time interface checks ----

var _ repository.ClientRepositoryInterface = (*mockClientRepo)(nil)
var _ repository.PermissionRepositoryInterface = (*mockPermRepo)(nil)

// ---- test helper ----

func newTestClientService(clientRepo repository.ClientRepositoryInterface, permRepo repository.PermissionRepositoryInterface) *service.ClientService {
	cfg := &config.Config{}
	return service.NewClientServiceWithRepos(cfg, clientRepo, permRepo)
}

// validCreateClientInput returns a CreateClientInput with all fields valid.
// Clients use regular (non-bank) email addresses.
func validCreateClientInput() service.CreateClientInput {
	return service.CreateClientInput{
		Ime:           "Ana",
		Prezime:       "Anic",
		DatumRodjenja: time.Date(1995, 3, 20, 0, 0, 0, 0, time.UTC).Unix(),
		Pol:           "F",
		Email:         "ana@gmail.com",
		BrojTelefona:  "0651234567",
		Adresa:        "Ulica 2",
	}
}

// ---- tests ----

func TestCreateClient_Success(t *testing.T) {
	clientRepo := &mockClientRepo{
		emailExistsFn: func(email string, excludeID uint) (bool, error) { return false, nil },
		createFn:      func(client *models.Client) error { client.ID = 7; return nil },
	}
	svc := newTestClientService(clientRepo, &mockPermRepo{})

	got, setupToken, err := svc.CreateClient(validCreateClientInput())
	if err != nil {
		t.Fatalf("CreateClient() unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("CreateClient() returned nil client")
	}
	if got.ID != 7 {
		t.Errorf("CreateClient() client.ID = %d, want 7", got.ID)
	}
	if got.Password == "" || got.Password == "pending" {
		t.Error("CreateClient() did not store a secure placeholder password")
	}
	if got.SaltPassword == "" || got.SaltPassword == "pending" {
		t.Error("CreateClient() did not store a secure password salt")
	}
	if got.Aktivan {
		t.Error("CreateClient() should create inactive clients until password setup")
	}
	if setupToken == "" {
		t.Error("CreateClient() did not return a setup token")
	}
}

func TestCreateClient_DuplicateEmail(t *testing.T) {
	clientRepo := &mockClientRepo{
		emailExistsFn: func(email string, excludeID uint) (bool, error) { return true, nil },
	}
	svc := newTestClientService(clientRepo, &mockPermRepo{})

	_, _, err := svc.CreateClient(validCreateClientInput())
	if err == nil {
		t.Fatal("CreateClient() expected error for duplicate email, got nil")
	}
	if !strings.Contains(err.Error(), "email already in use") {
		t.Errorf("CreateClient() error = %q, want contains %q", err.Error(), "email already in use")
	}
}

func TestCreateClient_InvalidEmail(t *testing.T) {
	svc := newTestClientService(&mockClientRepo{}, &mockPermRepo{})

	input := validCreateClientInput()
	input.Email = "not-an-email" // invalid format

	_, _, err := svc.CreateClient(input)
	if err == nil {
		t.Fatal("CreateClient() expected error for invalid email format, got nil")
	}
}

func TestGetClient_Found(t *testing.T) {
	client := &models.Client{ID: 3, Email: "client@bank.com"}
	clientRepo := &mockClientRepo{
		findByIDFn: func(id uint) (*models.Client, error) { return client, nil },
	}
	svc := newTestClientService(clientRepo, &mockPermRepo{})

	got, err := svc.GetClient(3)
	if err != nil {
		t.Fatalf("GetClient() unexpected error: %v", err)
	}
	if got == nil || got.ID != 3 {
		t.Error("GetClient() returned wrong client")
	}
}

func TestUpdateClient_NotFound(t *testing.T) {
	clientRepo := &mockClientRepo{
		findByIDFn: func(id uint) (*models.Client, error) {
			return nil, errors.New("record not found")
		},
	}
	svc := newTestClientService(clientRepo, &mockPermRepo{})

	input := service.UpdateClientInput{
		Ime:           "Ana",
		Prezime:       "Anic",
		DatumRodjenja: time.Date(1995, 3, 20, 0, 0, 0, 0, time.UTC).Unix(),
		Pol:           "F",
		Email:         "ana@bank.com",
		BrojTelefona:  "0651234567",
		Adresa:        "Ulica 2",
	}

	_, err := svc.UpdateClient(999, input)
	if err == nil {
		t.Fatal("UpdateClient() expected error for non-existent client, got nil")
	}
	if !strings.Contains(err.Error(), "client not found") {
		t.Errorf("UpdateClient() error = %q, want contains %q", err.Error(), "client not found")
	}
}

func TestUpdateClient_DuplicateEmail(t *testing.T) {
	existing := &models.Client{
		ID:    4,
		Email: "old@bank.com",
	}
	clientRepo := &mockClientRepo{
		findByIDFn:    func(id uint) (*models.Client, error) { return existing, nil },
		emailExistsFn: func(email string, excludeID uint) (bool, error) { return true, nil },
	}
	svc := newTestClientService(clientRepo, &mockPermRepo{})

	input := service.UpdateClientInput{
		Ime:           "Ana",
		Prezime:       "Anic",
		DatumRodjenja: time.Date(1995, 3, 20, 0, 0, 0, 0, time.UTC).Unix(),
		Pol:           "F",
		Email:         "different@bank.com", // different from current, triggers EmailExists check
		BrojTelefona:  "0651234567",
		Adresa:        "Ulica 2",
	}

	_, err := svc.UpdateClient(4, input)
	if err == nil {
		t.Fatal("UpdateClient() expected error for duplicate email, got nil")
	}
	if !strings.Contains(err.Error(), "email already in use") {
		t.Errorf("UpdateClient() error = %q, want contains %q", err.Error(), "email already in use")
	}
}

func TestCreateClient_AssignsDefaultPermissions(t *testing.T) {
	setPermsCalled := false
	clientRepo := &mockClientRepo{
		emailExistsFn: func(email string, excludeID uint) (bool, error) { return false, nil },
		createFn:      func(client *models.Client) error { client.ID = 5; return nil },
		setPermissionsFn: func(client *models.Client, perms []models.Permission) error {
			setPermsCalled = true
			if len(perms) == 0 {
				t.Error("SetPermissions called with empty permissions slice")
			}
			return nil
		},
	}
	permRepo := &mockPermRepo{
		findByNamesForSubjectFn: func(names []string, subjectType string) ([]models.Permission, error) {
			return []models.Permission{{Name: "client.basic"}}, nil
		},
	}
	svc := newTestClientService(clientRepo, permRepo)

	_, _, err := svc.CreateClient(validCreateClientInput())
	if err != nil {
		t.Fatalf("CreateClient() unexpected error: %v", err)
	}
	if !setPermsCalled {
		t.Error("CreateClient() did not call SetPermissions — default client permissions not assigned")
	}
}

func TestCreateClient_ReturnsClientWithID(t *testing.T) {
	clientRepo := &mockClientRepo{
		emailExistsFn: func(email string, excludeID uint) (bool, error) { return false, nil },
		createFn:      func(client *models.Client) error { client.ID = 42; return nil },
	}
	permRepo := &mockPermRepo{
		findByNamesForSubjectFn: func(names []string, subjectType string) ([]models.Permission, error) {
			return []models.Permission{}, nil
		},
	}
	svc := newTestClientService(clientRepo, permRepo)

	got, _, err := svc.CreateClient(validCreateClientInput())
	if err != nil {
		t.Fatalf("CreateClient() unexpected error: %v", err)
	}
	if got.ID != 42 {
		t.Errorf("CreateClient() returned client.ID = %d, want 42", got.ID)
	}
}

func TestCreateClient_AccountOpeningFlow(t *testing.T) {
	// Simulates employee creating a client during account opening:
	// client is created, gets default permissions, and returns with a usable ID.
	var createdClient *models.Client
	clientRepo := &mockClientRepo{
		emailExistsFn: func(email string, excludeID uint) (bool, error) { return false, nil },
		createFn: func(client *models.Client) error {
			client.ID = 99
			createdClient = client
			return nil
		},
		setPermissionsFn: func(client *models.Client, perms []models.Permission) error {
			client.Permissions = perms
			return nil
		},
	}
	permRepo := &mockPermRepo{
		findByNamesForSubjectFn: func(names []string, subjectType string) ([]models.Permission, error) {
			if subjectType != models.PermissionSubjectClient {
				t.Errorf("FindByNamesForSubject called with subjectType=%q, want %q", subjectType, models.PermissionSubjectClient)
			}
			return []models.Permission{{Name: models.PermClientBasic}}, nil
		},
	}
	svc := newTestClientService(clientRepo, permRepo)

	got, _, err := svc.CreateClient(validCreateClientInput())
	if err != nil {
		t.Fatalf("CreateClient() unexpected error: %v", err)
	}
	if got == nil || got.ID == 0 {
		t.Fatal("CreateClient() returned client without ID")
	}
	if createdClient == nil {
		t.Fatal("client was never passed to Create()")
	}
}

func TestCreateClient_AcceptsNonBankEmail(t *testing.T) {
	clientRepo := &mockClientRepo{
		emailExistsFn: func(email string, excludeID uint) (bool, error) { return false, nil },
		createFn:      func(client *models.Client) error { client.ID = 1; return nil },
	}
	svc := newTestClientService(clientRepo, &mockPermRepo{})

	input := validCreateClientInput()
	input.Email = "user@gmail.com" // regular non-bank email

	got, _, err := svc.CreateClient(input)
	if err != nil {
		t.Fatalf("CreateClient() rejected non-bank email: %v", err)
	}
	if got == nil {
		t.Fatal("CreateClient() returned nil client for valid non-bank email")
	}
}

func TestUpdateClient_SameEmailAllowed(t *testing.T) {
	existing := &models.Client{
		ID:    10,
		Email: "user@gmail.com",
	}
	clientRepo := &mockClientRepo{
		findByIDFn: func(id uint) (*models.Client, error) { return existing, nil },
		updateFn:   func(client *models.Client) error { return nil },
		// EmailExists should NOT be called when email is unchanged
		emailExistsFn: func(email string, excludeID uint) (bool, error) {
			return true, nil // would reject if called
		},
	}
	svc := newTestClientService(clientRepo, &mockPermRepo{})

	input := service.UpdateClientInput{
		Ime:           "Ana",
		Prezime:       "Anic",
		DatumRodjenja: time.Date(1995, 3, 20, 0, 0, 0, 0, time.UTC).Unix(),
		Pol:           "F",
		Email:         "user@gmail.com", // same as existing — should not trigger duplicate check
		BrojTelefona:  "0651234567",
		Adresa:        "Ulica 2",
	}

	_, err := svc.UpdateClient(10, input)
	if err != nil {
		t.Fatalf("UpdateClient() with same email unexpectedly returned error: %v", err)
	}
}

func TestUpdateClient_AcceptsNonBankEmail(t *testing.T) {
	existing := &models.Client{
		ID:    11,
		Email: "old@gmail.com",
	}
	clientRepo := &mockClientRepo{
		findByIDFn:    func(id uint) (*models.Client, error) { return existing, nil },
		emailExistsFn: func(email string, excludeID uint) (bool, error) { return false, nil },
		updateFn:      func(client *models.Client) error { return nil },
	}
	svc := newTestClientService(clientRepo, &mockPermRepo{})

	input := service.UpdateClientInput{
		Ime:           "Ana",
		Prezime:       "Anic",
		DatumRodjenja: time.Date(1995, 3, 20, 0, 0, 0, 0, time.UTC).Unix(),
		Pol:           "F",
		Email:         "new@yahoo.com", // regular non-bank email
		BrojTelefona:  "0651234567",
		Adresa:        "Ulica 2",
	}

	_, err := svc.UpdateClient(11, input)
	if err != nil {
		t.Fatalf("UpdateClient() rejected non-bank email: %v", err)
	}
}

func TestUpdateClientPermissions_WrongSubjectType(t *testing.T) {
	client := &models.Client{ID: 5, Email: "client@bank.com", Permissions: []models.Permission{}}
	clientRepo := &mockClientRepo{
		findByIDFn: func(id uint) (*models.Client, error) { return client, nil },
	}
	// Returns fewer perms than requested — simulating wrong subject type permissions
	permRepo := &mockPermRepo{
		findByNamesForSubjectFn: func(names []string, subjectType string) ([]models.Permission, error) {
			return []models.Permission{{Name: "client.basic"}}, nil
		},
	}
	svc := newTestClientService(clientRepo, permRepo)

	_, err := svc.UpdateClientPermissions(5, []string{"client.basic", "employee.read"})
	if err == nil {
		t.Fatal("UpdateClientPermissions() expected error for wrong subject type, got nil")
	}
	if !strings.Contains(err.Error(), "client permissions") {
		t.Errorf("UpdateClientPermissions() error = %q, want contains %q", err.Error(), "client permissions")
	}
}
