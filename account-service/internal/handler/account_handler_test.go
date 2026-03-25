package handler_test

import (
	"context"
	"errors"
	"testing"

	accountv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/gen/proto/account/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/service"
)

// --- mock service ---

type mockAccountService struct {
	createResult     *models.Account
	createErr        error
	getResult        *models.Account
	getErr           error
	listClientResult []models.Account
	listAllResult    []models.Account
	listAllTotal     int64
	capturedFilter   models.AccountFilter
	updateNameErr    error
	updateLimitsErr  error
}

func (m *mockAccountService) CreateAccount(input service.CreateAccountInput) (*models.Account, error) {
	return m.createResult, m.createErr
}
func (m *mockAccountService) GetAccount(id uint) (*models.Account, error) {
	return m.getResult, m.getErr
}
func (m *mockAccountService) ListAccountsByClient(clientID uint) ([]models.Account, error) {
	return m.listClientResult, nil
}
func (m *mockAccountService) ListAllAccounts(filter models.AccountFilter) ([]models.Account, int64, error) {
	m.capturedFilter = filter
	return m.listAllResult, m.listAllTotal, nil
}
func (m *mockAccountService) UpdateAccountName(id uint, naziv string) error {
	return m.updateNameErr
}
func (m *mockAccountService) UpdateAccountLimits(id uint, clientID uint, dnevniLimit, mesecniLimit float64) error {
	return m.updateLimitsErr
}

// --- tests ---

func TestCreateAccount_Success(t *testing.T) {
	acc := &models.Account{ID: 1, BrojRacuna: "000100000000000001", Tip: "tekuci", Vrsta: "licni", Status: "aktivan"}
	h := handler.NewAccountHandlerWithService(&mockAccountService{createResult: acc})

	resp, err := h.CreateAccount(context.Background(), &accountv1.CreateAccountRequest{
		ClientId:   1,
		CurrencyId: 1,
		Tip:        "tekuci",
		Vrsta:      "licni",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Account.Id != 1 {
		t.Errorf("expected account ID=1, got %d", resp.Account.Id)
	}
	if resp.Account.BrojRacuna != "000100000000000001" {
		t.Errorf("expected BrojRacuna set, got %q", resp.Account.BrojRacuna)
	}
}

func TestCreateAccount_ServiceError_ReturnsInvalidArgument(t *testing.T) {
	h := handler.NewAccountHandlerWithService(&mockAccountService{createErr: errors.New("invalid account type")})

	_, err := h.CreateAccount(context.Background(), &accountv1.CreateAccountRequest{Tip: "invalid"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetAccount_Success(t *testing.T) {
	acc := &models.Account{ID: 5, BrojRacuna: "000100000000000002", Status: "aktivan"}
	h := handler.NewAccountHandlerWithService(&mockAccountService{getResult: acc})

	resp, err := h.GetAccount(context.Background(), &accountv1.GetAccountRequest{Id: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Account.Id != 5 {
		t.Errorf("expected account ID=5, got %d", resp.Account.Id)
	}
}

func TestGetAccount_NotFound_ReturnsNotFound(t *testing.T) {
	h := handler.NewAccountHandlerWithService(&mockAccountService{getErr: errors.New("record not found")})

	_, err := h.GetAccount(context.Background(), &accountv1.GetAccountRequest{Id: 999})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListClientAccounts_ReturnsAccounts(t *testing.T) {
	accounts := []models.Account{{ID: 1}, {ID: 2}}
	h := handler.NewAccountHandlerWithService(&mockAccountService{listClientResult: accounts})

	resp, err := h.ListClientAccounts(context.Background(), &accountv1.ListClientAccountsRequest{ClientId: 7})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Accounts) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(resp.Accounts))
	}
}

func TestListAllAccounts_ReturnsPaginatedResults(t *testing.T) {
	accounts := []models.Account{{ID: 1}, {ID: 2}, {ID: 3}}
	h := handler.NewAccountHandlerWithService(&mockAccountService{listAllResult: accounts, listAllTotal: 3})

	resp, err := h.ListAllAccounts(context.Background(), &accountv1.ListAllAccountsRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Accounts) != 3 {
		t.Errorf("expected 3 accounts, got %d", len(resp.Accounts))
	}
	if resp.Total != 3 {
		t.Errorf("expected total=3, got %d", resp.Total)
	}
}

func TestUpdateAccountName_Success(t *testing.T) {
	h := handler.NewAccountHandlerWithService(&mockAccountService{})

	resp, err := h.UpdateAccountName(context.Background(), &accountv1.UpdateAccountNameRequest{Id: 3, Naziv: "Novi naziv"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestUpdateAccountLimits_Success(t *testing.T) {
	h := handler.NewAccountHandlerWithService(&mockAccountService{})

	resp, err := h.UpdateAccountLimits(context.Background(), &accountv1.UpdateAccountLimitsRequest{
		Id: 4, DnevniLimit: 50000, MesecniLimit: 500000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestUpdateAccountLimits_ServiceError_ReturnsInvalidArgument(t *testing.T) {
	h := handler.NewAccountHandlerWithService(&mockAccountService{updateLimitsErr: errors.New("dnevni limit cannot be negative")})

	_, err := h.UpdateAccountLimits(context.Background(), &accountv1.UpdateAccountLimitsRequest{
		Id: 4, DnevniLimit: -1,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
