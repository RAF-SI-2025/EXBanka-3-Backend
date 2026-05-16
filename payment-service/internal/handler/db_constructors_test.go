package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type mockMobileApprovalSvc struct {
	approved *models.Payment
	code     string
	expires  *time.Time
	approveErr error
	rejected *models.Payment
	rejectErr error
}

func (m *mockMobileApprovalSvc) ApprovePaymentMobile(_ uint, _ string) (*models.Payment, string, *time.Time, error) {
	return m.approved, m.code, m.expires, m.approveErr
}

func (m *mockMobileApprovalSvc) RejectPayment(_ uint) (*models.Payment, error) {
	return m.rejected, m.rejectErr
}

func newTestPaymentDB(t *testing.T, name string) *gorm.DB {
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

func TestNewCreatePaymentHTTPHandler_Constructs(t *testing.T) {
	db := newTestPaymentDB(t, "pay_ctor_create")
	cfg := &config.Config{JWTSecret: "s"}
	if h := handler.NewCreatePaymentHTTPHandler(db, cfg); h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestNewPaymentMobileVerificationHandler_Constructs(t *testing.T) {
	db := newTestPaymentDB(t, "pay_ctor_mobile")
	cfg := &config.Config{JWTSecret: "s"}
	if h := handler.NewPaymentMobileVerificationHandler(db, cfg); h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestNewPaymentHandler_Constructs(t *testing.T) {
	db := newTestPaymentDB(t, "pay_ctor_grpc")
	cfg := &config.Config{JWTSecret: "s"}
	if h := handler.NewPaymentHandler(db, cfg); h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestNewPaymentRecipientHandler_Constructs(t *testing.T) {
	db := newTestPaymentDB(t, "pay_ctor_recipient")
	if h := handler.NewPaymentRecipientHandler(db); h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestNewPrenosHTTPHandler_Constructs(t *testing.T) {
	db := newTestPaymentDB(t, "pay_ctor_prenos")
	cfg := &config.Config{JWTSecret: "s"}
	if h := handler.NewPrenosHTTPHandler(db, cfg); h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func seedOwnedAccount(t *testing.T, db *gorm.DB, clientID uint) *models.Account {
	t.Helper()
	acc := &models.Account{
		BrojRacuna: fmt.Sprintf("acct-%d-%d", clientID, time.Now().UnixNano()),
		ClientID:   &clientID,
		Stanje:     1000,
	}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return acc
}

func TestCreatePaymentHTTPHandler_RecipientNotOwned_Returns403(t *testing.T) {
	db := newTestPaymentDB(t, "pay_create_recip_403")
	cfg := &config.Config{JWTSecret: "ctest-secret"}

	owner := uint(11)
	other := uint(22)
	acc := seedOwnedAccount(t, db, owner)

	recipient := &models.PaymentRecipient{ClientID: other, Naziv: "X", BrojRacuna: "X"}
	if err := db.Create(recipient).Error; err != nil {
		t.Fatalf("seed recipient: %v", err)
	}

	svc := &mockPaymentSvc{created: makePayment(1)}
	h := handler.NewCreatePaymentHTTPHandlerWithService(svc, db, cfg)

	body := fmt.Sprintf(`{"racun_posiljaoca_id":%d,"racun_primaoca_broj":"000000000000000098","iznos":100,"recipient_id":%d}`, acc.ID, recipient.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+makeClientAccessToken(t, cfg.JWTSecret, owner))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreatePaymentHTTPHandler_AccountOwned_Succeeds(t *testing.T) {
	db := newTestPaymentDB(t, "pay_create_owned_ok")
	cfg := &config.Config{JWTSecret: "ctest-secret"}

	owner := uint(33)
	acc := seedOwnedAccount(t, db, owner)

	svc := &mockPaymentSvc{created: makePayment(1)}
	h := handler.NewCreatePaymentHTTPHandlerWithService(svc, db, cfg)

	body := fmt.Sprintf(`{"racun_posiljaoca_id":%d,"racun_primaoca_broj":"000000000000000098","iznos":100}`, acc.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+makeClientAccessToken(t, cfg.JWTSecret, owner))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreatePaymentHTTPHandler_AccountNotOwned_Returns403(t *testing.T) {
	db := newTestPaymentDB(t, "pay_create_notowned")
	cfg := &config.Config{JWTSecret: "ctest-secret"}

	owner := uint(44)
	other := uint(55)
	acc := seedOwnedAccount(t, db, owner)

	svc := &mockPaymentSvc{created: makePayment(1)}
	h := handler.NewCreatePaymentHTTPHandlerWithService(svc, db, cfg)

	body := fmt.Sprintf(`{"racun_posiljaoca_id":%d,"racun_primaoca_broj":"000000000000000098","iznos":100}`, acc.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+makeClientAccessToken(t, cfg.JWTSecret, other))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCreatePaymentHTTPHandler_AccountMissing_Returns500(t *testing.T) {
	db := newTestPaymentDB(t, "pay_create_missing")
	cfg := &config.Config{JWTSecret: "ctest-secret"}

	svc := &mockPaymentSvc{created: makePayment(1)}
	h := handler.NewCreatePaymentHTTPHandlerWithService(svc, db, cfg)

	body := `{"racun_posiljaoca_id":99999,"racun_primaoca_broj":"000000000000000098","iznos":100}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+makeClientAccessToken(t, cfg.JWTSecret, 1))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMobileVerification_PaymentOwnedSuccess(t *testing.T) {
	db := newTestPaymentDB(t, "pay_mobile_owned")
	cfg := &config.Config{JWTSecret: "ctest-secret"}

	owner := uint(77)
	acc := seedOwnedAccount(t, db, owner)
	payment := &models.Payment{
		RacunPosiljaocaID: acc.ID,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             100,
		Status:            "u_obradi",
	}
	if err := db.Create(payment).Error; err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	approved := *payment
	approved.Status = "uspesno"
	svc := &mockMobileApprovalSvc{approved: &approved}
	h := handler.NewPaymentMobileVerificationHandlerWithService(svc, db, cfg)

	url := fmt.Sprintf("/api/v1/payments/%d/approve", payment.ID)
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{"mode":"code"}`))
	req.Header.Set("Authorization", "Bearer "+makeClientAccessToken(t, cfg.JWTSecret, owner))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Approve(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if _, ok := resp["payment"]; !ok {
		t.Fatalf("expected payment in response: %s", rr.Body.String())
	}
}

func TestMobileVerification_PaymentNotOwned_Returns403(t *testing.T) {
	db := newTestPaymentDB(t, "pay_mobile_notowned")
	cfg := &config.Config{JWTSecret: "ctest-secret"}

	owner := uint(88)
	other := uint(99)
	acc := seedOwnedAccount(t, db, owner)
	payment := &models.Payment{RacunPosiljaocaID: acc.ID, RacunPrimaocaBroj: "X", Iznos: 1, Status: "u_obradi"}
	if err := db.Create(payment).Error; err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	svc := &mockMobileApprovalSvc{approved: payment}
	h := handler.NewPaymentMobileVerificationHandlerWithService(svc, db, cfg)

	url := fmt.Sprintf("/api/v1/payments/%d/approve", payment.ID)
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+makeClientAccessToken(t, cfg.JWTSecret, other))
	rr := httptest.NewRecorder()

	h.Approve(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}
