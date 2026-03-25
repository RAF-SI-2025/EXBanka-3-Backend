package handler_test

import (
	"context"
	"errors"
	"testing"

	clientv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/gen/proto/client/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/repository"
	svc "github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/service"
)

// --- mock service ---

type mockClientService struct {
	createResult      *models.Client
	createSetupToken  string
	createErr         error
	getResult         *models.Client
	getErr            error
	listResult        []models.Client
	listTotal         int64
	updateResult      *models.Client
	updateErr         error
	updatePermsResult *models.Client
	updatePermsErr    error
}

func (m *mockClientService) CreateClient(input svc.CreateClientInput) (*models.Client, string, error) {
	return m.createResult, m.createSetupToken, m.createErr
}
func (m *mockClientService) GetClient(id uint) (*models.Client, error) {
	return m.getResult, m.getErr
}
func (m *mockClientService) ListClients(filter repository.ClientFilter) ([]models.Client, int64, error) {
	return m.listResult, m.listTotal, nil
}
func (m *mockClientService) UpdateClient(id uint, input svc.UpdateClientInput) (*models.Client, error) {
	return m.updateResult, m.updateErr
}
func (m *mockClientService) UpdateClientPermissions(id uint, permissionNames []string) (*models.Client, error) {
	return m.updatePermsResult, m.updatePermsErr
}

// --- tests ---

func TestCreateClient_Success(t *testing.T) {
	client := &models.Client{ID: 1, Ime: "Ana", Prezime: "Jovic", Email: "ana@test.com"}
	h := handler.NewClientHandlerWithService(&mockClientService{createResult: client, createSetupToken: "setup-token"})

	resp, err := h.CreateClient(context.Background(), &clientv1.CreateClientRequest{
		Ime:     "Ana",
		Prezime: "Jovic",
		Email:   "ana@test.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Client.Id != 1 {
		t.Errorf("expected client ID=1, got %d", resp.Client.Id)
	}
	if resp.Client.Email != "ana@test.com" {
		t.Errorf("expected email=ana@test.com, got %q", resp.Client.Email)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
	if resp.Message != "Client created. Complete initial password setup with token: setup-token" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestCreateClient_ServiceError_ReturnsInvalidArgument(t *testing.T) {
	h := handler.NewClientHandlerWithService(&mockClientService{createErr: errors.New("email already in use")})

	_, err := h.CreateClient(context.Background(), &clientv1.CreateClientRequest{Email: "dup@test.com"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetClient_Success(t *testing.T) {
	client := &models.Client{ID: 5, Ime: "Marko", Email: "marko@test.com"}
	h := handler.NewClientHandlerWithService(&mockClientService{getResult: client})

	resp, err := h.GetClient(context.Background(), &clientv1.GetClientRequest{Id: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Client.Id != 5 {
		t.Errorf("expected client ID=5, got %d", resp.Client.Id)
	}
}

func TestGetClient_NotFound_ReturnsNotFound(t *testing.T) {
	h := handler.NewClientHandlerWithService(&mockClientService{getErr: errors.New("record not found")})

	_, err := h.GetClient(context.Background(), &clientv1.GetClientRequest{Id: 999})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListClients_ReturnsPaginatedResults(t *testing.T) {
	clients := []models.Client{
		{ID: 1, Ime: "Ana"},
		{ID: 2, Ime: "Marko"},
	}
	h := handler.NewClientHandlerWithService(&mockClientService{listResult: clients, listTotal: 2})

	resp, err := h.ListClients(context.Background(), &clientv1.ListClientsRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Clients) != 2 {
		t.Errorf("expected 2 clients, got %d", len(resp.Clients))
	}
	if resp.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Total)
	}
	if resp.Page != 1 || resp.PageSize != 10 {
		t.Errorf("expected page=1 pageSize=10, got %d/%d", resp.Page, resp.PageSize)
	}
}

func TestUpdateClient_Success(t *testing.T) {
	updated := &models.Client{ID: 3, Ime: "Novi", Prezime: "Naziv", Email: "novi@test.com"}
	h := handler.NewClientHandlerWithService(&mockClientService{updateResult: updated})

	resp, err := h.UpdateClient(context.Background(), &clientv1.UpdateClientRequest{
		Id:      3,
		Ime:     "Novi",
		Prezime: "Naziv",
		Email:   "novi@test.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Client.Id != 3 {
		t.Errorf("expected client ID=3, got %d", resp.Client.Id)
	}
	if resp.Client.Ime != "Novi" {
		t.Errorf("expected Ime=Novi, got %q", resp.Client.Ime)
	}
}

func TestUpdateClient_ServiceError_ReturnsInvalidArgument(t *testing.T) {
	h := handler.NewClientHandlerWithService(&mockClientService{updateErr: errors.New("client not found")})

	_, err := h.UpdateClient(context.Background(), &clientv1.UpdateClientRequest{Id: 999})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdateClientPermissions_Success(t *testing.T) {
	client := &models.Client{
		ID: 4,
		Permissions: []models.Permission{
			{ID: 1, Name: "client.basic", Description: "Basic client role"},
		},
	}
	h := handler.NewClientHandlerWithService(&mockClientService{updatePermsResult: client})

	resp, err := h.UpdateClientPermissions(context.Background(), &clientv1.UpdateClientPermissionsRequest{
		Id:              4,
		PermissionNames: []string{"client.basic"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Permissions) != 1 {
		t.Errorf("expected 1 permission, got %d", len(resp.Permissions))
	}
	if resp.Permissions[0].Name != "client.basic" {
		t.Errorf("expected permission name=client.basic, got %q", resp.Permissions[0].Name)
	}
}

func TestUpdateClientPermissions_ServiceError_ReturnsInvalidArgument(t *testing.T) {
	h := handler.NewClientHandlerWithService(&mockClientService{updatePermsErr: errors.New("invalid permission")})

	_, err := h.UpdateClientPermissions(context.Background(), &clientv1.UpdateClientPermissionsRequest{
		Id:              4,
		PermissionNames: []string{"admin"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
