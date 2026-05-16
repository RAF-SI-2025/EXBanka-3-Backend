package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	accountv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/gen/proto/account/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
)

func ctxWithClaims(c *util.Claims) context.Context {
	return context.WithValue(context.Background(), middleware.ClaimsKey, c)
}

func TestNewAccountHandler_Constructs(t *testing.T) {
	db := newTestAccountDB(t, "acct_ctor_new")
	if h := handler.NewAccountHandler(db, &config.Config{}); h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestCreateAccount_WithEmployeeClaimsAndFirma_Succeeds(t *testing.T) {
	acc := &models.Account{ID: 7, BrojRacuna: "X", FirmaID: ptrUint(3)}
	svc := &mockAccountService{createResult: acc}
	h := handler.NewAccountHandlerWithService(svc)
	ctx := ctxWithClaims(&util.Claims{EmployeeID: 9, TokenSource: "employee"})

	_, err := h.CreateAccount(ctx, &accountv1.CreateAccountRequest{
		ClientId: 5, FirmaId: 3, CurrencyId: 1, Tip: "tekuci", Vrsta: "licni",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestGetAccount_ClientOwnershipMismatch_Returns403(t *testing.T) {
	otherClient := uint(99)
	acc := &models.Account{ID: 5, ClientID: &otherClient}
	h := handler.NewAccountHandlerWithService(&mockAccountService{getResult: acc})
	ctx := ctxWithClaims(&util.Claims{ClientID: 1, TokenSource: "client"})

	_, err := h.GetAccount(ctx, &accountv1.GetAccountRequest{Id: 5})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetAccount_ClientOwned_Succeeds(t *testing.T) {
	owner := uint(1)
	acc := &models.Account{ID: 5, ClientID: &owner}
	h := handler.NewAccountHandlerWithService(&mockAccountService{getResult: acc})
	ctx := ctxWithClaims(&util.Claims{ClientID: 1, TokenSource: "client"})

	resp, err := h.GetAccount(ctx, &accountv1.GetAccountRequest{Id: 5})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Account.Id != 5 {
		t.Errorf("expected ID 5, got %d", resp.Account.Id)
	}
}

func TestListClientAccounts_OtherClient_Returns403(t *testing.T) {
	h := handler.NewAccountHandlerWithService(&mockAccountService{})
	ctx := ctxWithClaims(&util.Claims{ClientID: 1, TokenSource: "client"})

	_, err := h.ListClientAccounts(ctx, &accountv1.ListClientAccountsRequest{ClientId: 99})
	if err == nil {
		t.Fatal("expected permission denied")
	}
}

func TestListAllAccounts_WithFilters(t *testing.T) {
	mock := &mockAccountService{listAllResult: []models.Account{{ID: 1}}, listAllTotal: 1}
	h := handler.NewAccountHandlerWithService(mock)

	_, err := h.ListAllAccounts(context.Background(), &accountv1.ListAllAccountsRequest{
		ClientName: "ivo", Tip: "tekuci", Vrsta: "licni", Status: "aktivan",
		CurrencyId: 2, Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if mock.capturedFilter.CurrencyID == nil || *mock.capturedFilter.CurrencyID != 2 {
		t.Fatal("expected currency filter to be applied")
	}
}

func TestUpdateAccountName_ClientNotOwner_Returns403(t *testing.T) {
	otherClient := uint(99)
	acc := &models.Account{ID: 5, ClientID: &otherClient}
	h := handler.NewAccountHandlerWithService(&mockAccountService{getResult: acc})
	ctx := ctxWithClaims(&util.Claims{ClientID: 1, TokenSource: "client"})

	_, err := h.UpdateAccountName(ctx, &accountv1.UpdateAccountNameRequest{Id: 5, Naziv: "X"})
	if err == nil {
		t.Fatal("expected permission denied")
	}
}

func TestUpdateAccountName_ClientOwnerAcctNotFound_ReturnsNotFound(t *testing.T) {
	h := handler.NewAccountHandlerWithService(&mockAccountService{getErr: errors.New("missing")})
	ctx := ctxWithClaims(&util.Claims{ClientID: 1, TokenSource: "client"})

	_, err := h.UpdateAccountName(ctx, &accountv1.UpdateAccountNameRequest{Id: 5, Naziv: "X"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateAccountName_ServiceError_ReturnsInternal(t *testing.T) {
	h := handler.NewAccountHandlerWithService(&mockAccountService{updateNameErr: errors.New("db down")})
	_, err := h.UpdateAccountName(context.Background(), &accountv1.UpdateAccountNameRequest{Id: 5, Naziv: "X"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestToAccountProto_AllOptionalFields(t *testing.T) {
	owner := uint(7)
	firma := uint(11)
	acc := &models.Account{
		ID: 1, BrojRacuna: "X", CurrencyID: 2, Tip: "tekuci", Vrsta: "licni", Status: "aktivan",
		ClientID: &owner, FirmaID: &firma,
		Currency: models.Currency{Kod: "EUR"},
	}
	svc := &mockAccountService{getResult: acc}
	h := handler.NewAccountHandlerWithService(svc)

	resp, err := h.GetAccount(context.Background(), &accountv1.GetAccountRequest{Id: 1})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if resp.Account.ClientId != 7 || resp.Account.FirmaId != 11 || resp.Account.CurrencyKod != "EUR" {
		t.Fatalf("expected all optional fields set: %+v", resp.Account)
	}
}

func ptrUint(v uint) *uint { return &v }

// --- card handler extra branches ---

func TestCardHandler_HandleGet_ClientNotOwner_Returns403(t *testing.T) {
	svc := &mockCardSvc{foundCard: &models.Card{ID: 5, ClientID: 99}}
	h := handler.NewCardHTTPHandlerWithConfig(svc, &config.Config{JWTSecret: accountTestJWTSecret})

	req := authedReq(http.MethodGet, "/api/v1/cards/5", clientToken(t, 1), "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCardHandler_HandleGet_CardNotFound(t *testing.T) {
	svc := &mockCardSvc{foundCard: nil}
	h := handler.NewCardHTTPHandlerWithConfig(svc, &config.Config{JWTSecret: accountTestJWTSecret})

	req := authedReq(http.MethodGet, "/api/v1/cards/5", employeeToken(t, 1), "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCardHandler_HandleGet_ServiceError(t *testing.T) {
	svc := &mockCardSvc{err: errors.New("db down")}
	h := handler.NewCardHTTPHandlerWithConfig(svc, &config.Config{JWTSecret: accountTestJWTSecret})

	req := authedReq(http.MethodGet, "/api/v1/cards/5", employeeToken(t, 1), "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCardHandler_HandleBlock_ClientForbids_OtherClientID(t *testing.T) {
	svc := &mockCardSvc{foundCard: &models.Card{ID: 1}}
	h := handler.NewCardHTTPHandlerWithConfig(svc, &config.Config{JWTSecret: accountTestJWTSecret})

	body, _ := json.Marshal(map[string]any{"clientId": 99})
	req := authedReq(http.MethodPut, "/api/v1/cards/1/block", clientToken(t, 1), string(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCardHandler_HandleBlock_ClientServiceError(t *testing.T) {
	svc := &mockCardSvc{err: errors.New("blocked")}
	h := handler.NewCardHTTPHandlerWithConfig(svc, &config.Config{JWTSecret: accountTestJWTSecret})

	req := authedReq(http.MethodPut, "/api/v1/cards/1/block", clientToken(t, 1), `{}`)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCardHandler_HandleListByClient_AsClient_OK(t *testing.T) {
	svc := &mockCardSvc{cards: []models.Card{{ID: 1, ClientID: 1}}}
	h := handler.NewCardHTTPHandlerWithConfig(svc, &config.Config{JWTSecret: accountTestJWTSecret})

	req := authedReq(http.MethodGet, "/api/v1/cards/client/1", clientToken(t, 1), "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCardHandler_HandleListByClient_AsClient_OtherClient_403(t *testing.T) {
	svc := &mockCardSvc{}
	h := handler.NewCardHTTPHandlerWithConfig(svc, &config.Config{JWTSecret: accountTestJWTSecret})

	req := authedReq(http.MethodGet, "/api/v1/cards/client/99", clientToken(t, 1), "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestCardHandler_HandleListByClient_ServiceError(t *testing.T) {
	svc := &mockCardSvc{err: errors.New("db")}
	h := handler.NewCardHTTPHandlerWithConfig(svc, &config.Config{JWTSecret: accountTestJWTSecret})

	req := authedReq(http.MethodGet, "/api/v1/cards/client/1", clientToken(t, 1), "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCardHandler_HandleDeactivate_InvalidID(t *testing.T) {
	svc := &mockCardSvc{}
	h := handler.NewCardHTTPHandlerWithConfig(svc, &config.Config{JWTSecret: accountTestJWTSecret})

	req := authedReq(http.MethodPut, "/api/v1/cards/abc/deactivate", employeeToken(t, 1), "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCardHandler_HandleUnblock_ServiceError(t *testing.T) {
	svc := &mockCardSvc{err: errors.New("bad")}
	h := handler.NewCardHTTPHandlerWithConfig(svc, &config.Config{JWTSecret: accountTestJWTSecret})

	req := authedReq(http.MethodPut, "/api/v1/cards/1/unblock", employeeToken(t, 1), `{}`)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCardHandler_NotFoundRoute(t *testing.T) {
	svc := &mockCardSvc{}
	h := handler.NewCardHTTPHandlerWithConfig(svc, &config.Config{JWTSecret: accountTestJWTSecret})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cards/foo/bar/baz/qux", bytes.NewBufferString(""))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
