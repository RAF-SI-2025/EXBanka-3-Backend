package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	transferv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/gen/proto/transfer/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/util"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newHandlerTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

var seedCounter int64

func seedHandlerAccount(t *testing.T, db *gorm.DB, clientID uint) *models.Account {
	t.Helper()
	seedCounter++
	cid := clientID
	acc := &models.Account{
		BrojRacuna:        fmt.Sprintf("acct-%d-%d-%d", clientID, time.Now().UnixNano(), seedCounter),
		ClientID:          &cid,
		Stanje:            1000,
		RaspolozivoStanje: 1000,
		CurrencyID:        1,
	}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	return acc
}

func TestNewTransferHandler_Constructs(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_h_ctor_grpc")
	if h := NewTransferHandler(db, "", &config.Config{}); h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestNewTransferHTTPHandler_Constructs(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_h_ctor_http")
	if h := NewTransferHTTPHandler(db, "", &config.Config{}); h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestNewTransferMobileVerificationHandler_Constructs(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_h_ctor_mobile")
	if h := NewTransferMobileVerificationHandler(db, &config.Config{}, ""); h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestTransferHandler_AccountOwnedByClient_NoDB_ReturnsTrue(t *testing.T) {
	h := &TransferHandler{}
	owned, err := h.accountOwnedByClient(1, 1)
	if err != nil || !owned {
		t.Fatalf("expected true,nil; got %v, %v", owned, err)
	}
}

func TestTransferHandler_AccountOwnedByClient_DBPath(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_h_ownership")
	owner := uint(101)
	acc := seedHandlerAccount(t, db, owner)
	h := &TransferHandler{db: db}

	owned, err := h.accountOwnedByClient(acc.ID, owner)
	if err != nil || !owned {
		t.Fatalf("expected true, got owned=%v, err=%v", owned, err)
	}

	notOwned, err := h.accountOwnedByClient(acc.ID, 999)
	if err != nil || notOwned {
		t.Fatalf("expected false, got owned=%v, err=%v", notOwned, err)
	}

	if _, err := h.accountOwnedByClient(99999, owner); err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestTransferHTTPHandler_AccountOwnedByClient_DBPath(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_http_ownership")
	owner := uint(202)
	acc := seedHandlerAccount(t, db, owner)
	h := &TransferHTTPHandler{db: db}

	owned, err := h.accountOwnedByClient(acc.ID, owner)
	if err != nil || !owned {
		t.Fatalf("expected true, got owned=%v err=%v", owned, err)
	}
	if _, err := h.accountOwnedByClient(99999, owner); err == nil {
		t.Fatal("expected err for missing acct")
	}
}

func TestEnsureAccountsOwnedByClient_AllOwned(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_http_ensure_ok")
	owner := uint(303)
	a1 := seedHandlerAccount(t, db, owner)
	a2 := seedHandlerAccount(t, db, owner)
	h := &TransferHTTPHandler{db: db}
	w := httptest.NewRecorder()
	if !h.ensureAccountsOwnedByClient(w, owner, a1.ID, a2.ID) {
		t.Fatal("expected success")
	}
}

func TestEnsureAccountsOwnedByClient_SenderNotOwned(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_http_ensure_sender")
	owner := uint(304)
	other := uint(999)
	a1 := seedHandlerAccount(t, db, other)
	a2 := seedHandlerAccount(t, db, owner)
	h := &TransferHTTPHandler{db: db}
	w := httptest.NewRecorder()
	if h.ensureAccountsOwnedByClient(w, owner, a1.ID, a2.ID) {
		t.Fatal("expected false for unowned sender")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestEnsureAccountsOwnedByClient_ReceiverNotOwned(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_http_ensure_receiver")
	owner := uint(305)
	other := uint(998)
	a1 := seedHandlerAccount(t, db, owner)
	a2 := seedHandlerAccount(t, db, other)
	h := &TransferHTTPHandler{db: db}
	w := httptest.NewRecorder()
	if h.ensureAccountsOwnedByClient(w, owner, a1.ID, a2.ID) {
		t.Fatal("expected false for unowned receiver")
	}
}

func TestEnsureAccountsOwnedByClient_SenderMissing_Returns500(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_http_ensure_missing")
	owner := uint(306)
	h := &TransferHTTPHandler{db: db}
	w := httptest.NewRecorder()
	if h.ensureAccountsOwnedByClient(w, owner, 99999, 99998) {
		t.Fatal("expected false for missing acct")
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateTransfer_ClaimSenderNotOwned_403(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_grpc_create_403")
	owner := uint(401)
	other := uint(999)
	sender := seedHandlerAccount(t, db, other) // claim user does NOT own this
	receiver := seedHandlerAccount(t, db, owner)
	h := &TransferHandler{db: db}

	ctx := context.WithValue(context.Background(), middleware.ClaimsKey, &util.Claims{ClientID: owner, TokenSource: "client"})
	_, err := h.CreateTransfer(ctx, &transferv1.CreateTransferRequest{
		RacunPosiljaocaId: uint64(sender.ID), RacunPrimaocaId: uint64(receiver.ID), Iznos: 100,
	})
	if err == nil {
		t.Fatal("expected PermissionDenied")
	}
}

func TestCreateTransfer_ClaimReceiverNotOwned_403(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_grpc_create_recv_403")
	owner := uint(402)
	other := uint(998)
	sender := seedHandlerAccount(t, db, owner)
	receiver := seedHandlerAccount(t, db, other)
	h := &TransferHandler{db: db}

	ctx := context.WithValue(context.Background(), middleware.ClaimsKey, &util.Claims{ClientID: owner, TokenSource: "client"})
	_, err := h.CreateTransfer(ctx, &transferv1.CreateTransferRequest{
		RacunPosiljaocaId: uint64(sender.ID), RacunPrimaocaId: uint64(receiver.ID), Iznos: 100,
	})
	if err == nil {
		t.Fatal("expected PermissionDenied")
	}
}

func TestListTransfersByAccount_ClaimNotOwner_403(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_grpc_list_acct_403")
	owner := uint(501)
	other := uint(999)
	acc := seedHandlerAccount(t, db, other)
	h := &TransferHandler{db: db}

	ctx := context.WithValue(context.Background(), middleware.ClaimsKey, &util.Claims{ClientID: owner, TokenSource: "client"})
	_, err := h.ListTransfersByAccount(ctx, &transferv1.ListTransfersByAccountRequest{AccountId: uint64(acc.ID)})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseFilterFromAccount_AllOptionalFields(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	req := &transferv1.ListTransfersByAccountRequest{
		AccountId: 1, Status: "uspesno", Page: 2, PageSize: 10,
		MinAmount: 100, MaxAmount: 500, DateFrom: now, DateTo: now,
	}
	f := parseFilterFromAccount(req)
	if f.Status != "uspesno" {
		t.Errorf("Status: %v", f.Status)
	}
	if f.MinAmount == nil || *f.MinAmount != 100 {
		t.Error("MinAmount missing")
	}
	if f.MaxAmount == nil || *f.MaxAmount != 500 {
		t.Error("MaxAmount missing")
	}
	if f.DateFrom == nil {
		t.Error("DateFrom missing")
	}
	if f.DateTo == nil {
		t.Error("DateTo missing")
	}
}

func TestParseFilterFromClient_AllOptionalFields(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	req := &transferv1.ListTransfersByClientRequest{
		ClientId: 1, Status: "uspesno", Page: 2, PageSize: 10,
		MinAmount: 100, MaxAmount: 500, DateFrom: now, DateTo: now,
	}
	f := parseFilterFromClient(req)
	if f.MinAmount == nil || f.MaxAmount == nil || f.DateFrom == nil || f.DateTo == nil {
		t.Errorf("expected all filters set: %+v", f)
	}
}

func makeClientHTTPToken(t *testing.T, secret string, clientID uint) string {
	t.Helper()
	claims := jwt.MapClaims{
		"client_id":    clientID,
		"permissions":  []string{"clientBasic"},
		"token_type":   "access",
		"token_source": "client",
		"exp":          time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString([]byte(secret))
	return signed
}

func TestTransferHTTPHandler_Create_AccountNotOwned_Returns403(t *testing.T) {
	db := newHandlerTestDB(t, "xfer_http_create_403")
	owner := uint(601)
	other := uint(999)
	sender := seedHandlerAccount(t, db, other)
	receiver := seedHandlerAccount(t, db, owner)

	cfg := &config.Config{JWTSecret: "secret"}
	svc := &mockHTTPSvc{createResult: makeTestTransfer(1)}
	h := &TransferHTTPHandler{svc: svc, db: db, cfg: cfg}

	body, _ := json.Marshal(map[string]any{
		"racun_posiljaoca_id": sender.ID, "racun_primaoca_id": receiver.ID, "iznos": 100,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+makeClientHTTPToken(t, cfg.JWTSecret, owner))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Create(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}
