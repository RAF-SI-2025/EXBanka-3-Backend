package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

const accountTestJWTSecret = "acct-test-secret"

func newTestAccountDB(t *testing.T, name string) *gorm.DB {
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

func makeAccountToken(t *testing.T, claims util.Claims) string {
	t.Helper()
	claims.RegisteredClaims = jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims).SignedString([]byte(accountTestJWTSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func employeeToken(t *testing.T, employeeID uint) string {
	return makeAccountToken(t, util.Claims{
		EmployeeID: employeeID, TokenSource: "employee", TokenType: "access",
		Permissions: []string{"employeeBasic"},
	})
}

func clientToken(t *testing.T, clientID uint) string {
	return makeAccountToken(t, util.Claims{
		ClientID: clientID, TokenSource: "client", TokenType: "access",
		Permissions: []string{"clientBasic"},
	})
}

func authedReq(method, path, token, body string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

// --- CurrencyHTTPHandler ---

func TestCurrencyHandler_WrongMethod(t *testing.T) {
	db := newTestAccountDB(t, "cur_method")
	h := handler.NewCurrencyHTTPHandler(repository.NewCurrencyRepository(db), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/currencies", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestCurrencyHandler_NoAuth_401(t *testing.T) {
	db := newTestAccountDB(t, "cur_unauth")
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCurrencyHTTPHandler(repository.NewCurrencyRepository(db), cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/currencies", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestCurrencyHandler_RejectsClient(t *testing.T) {
	db := newTestAccountDB(t, "cur_client")
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCurrencyHTTPHandler(repository.NewCurrencyRepository(db), cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/currencies", clientToken(t, 1), ""))
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestCurrencyHandler_EmployeeOK(t *testing.T) {
	db := newTestAccountDB(t, "cur_ok")
	if err := db.Create(&models.Currency{Kod: "RSD", Naziv: "Dinar", Drzava: "RS"}).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCurrencyHTTPHandler(repository.NewCurrencyRepository(db), cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/currencies", employeeToken(t, 1), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if got, ok := body["currencies"].([]interface{}); !ok || len(got) != 1 {
		t.Errorf("currencies = %+v", body["currencies"])
	}
}

// --- FirmaHandler ---

func TestFirmaHandler_Create_WrongMethod(t *testing.T) {
	db := newTestAccountDB(t, "firma_method")
	h := handler.NewFirmaHandler(db, nil)
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodGet, "/api/v1/firme", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestFirmaHandler_Create_BadBody(t *testing.T) {
	db := newTestAccountDB(t, "firma_bad")
	h := handler.NewFirmaHandler(db, nil)
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/v1/firme", bytes.NewBufferString("not-json")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestFirmaHandler_Create_MissingRequired(t *testing.T) {
	db := newTestAccountDB(t, "firma_missing")
	h := handler.NewFirmaHandler(db, nil)
	rec := httptest.NewRecorder()
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/v1/firme", bytes.NewBufferString(`{"naziv":"x"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestFirmaHandler_Create_Success(t *testing.T) {
	db := newTestAccountDB(t, "firma_ok")
	h := handler.NewFirmaHandler(db, nil)
	rec := httptest.NewRecorder()
	body := `{"naziv":"Test d.o.o.","maticniBroj":"12345678","pib":"987654321","adresa":"Bul 1"}`
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/v1/firme", bytes.NewBufferString(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFirmaHandler_Create_Conflict(t *testing.T) {
	db := newTestAccountDB(t, "firma_conflict")
	if err := db.Create(&models.Firma{Naziv: "X", MaticniBroj: "12345678", PIB: "987654321"}).Error; err != nil {
		t.Fatal(err)
	}
	h := handler.NewFirmaHandler(db, nil)
	rec := httptest.NewRecorder()
	body := `{"naziv":"Y","maticniBroj":"12345678","pib":"987654321"}`
	h.Create(rec, httptest.NewRequest(http.MethodPost, "/api/v1/firme", bytes.NewBufferString(body)))
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestFirmaHandler_Create_RejectsClient(t *testing.T) {
	db := newTestAccountDB(t, "firma_client")
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewFirmaHandler(db, cfg)
	rec := httptest.NewRecorder()
	body := `{"naziv":"Y","maticniBroj":"22","pib":"33"}`
	h.Create(rec, authedReq(http.MethodPost, "/api/v1/firme", clientToken(t, 1), body))
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestFirmaHandler_ListSifreDelatnosti_WrongMethod(t *testing.T) {
	db := newTestAccountDB(t, "sif_method")
	h := handler.NewFirmaHandler(db, nil)
	rec := httptest.NewRecorder()
	h.ListSifreDelatnosti(rec, httptest.NewRequest(http.MethodPost, "/api/v1/sifre", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestFirmaHandler_ListSifreDelatnosti_OK(t *testing.T) {
	db := newTestAccountDB(t, "sif_ok")
	if err := db.Create(&models.SifraDelatnosti{Sifra: "62.01", Naziv: "Programiranje"}).Error; err != nil {
		t.Fatal(err)
	}
	h := handler.NewFirmaHandler(db, nil)
	rec := httptest.NewRecorder()
	h.ListSifreDelatnosti(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sifre", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// --- CreateAccountHTTPHandler ---

func TestCreateAccountHTTP_WrongMethod(t *testing.T) {
	db := newTestAccountDB(t, "create_method")
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCreateAccountHTTPHandler(db, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestCreateAccountHTTP_NoAuth_401(t *testing.T) {
	db := newTestAccountDB(t, "create_noauth")
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCreateAccountHTTPHandler(db, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewBufferString(`{}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestCreateAccountHTTP_BadBody(t *testing.T) {
	db := newTestAccountDB(t, "create_bad")
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCreateAccountHTTPHandler(db, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/accounts", employeeToken(t, 1), "not-json"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCreateAccountHTTP_RejectsClient(t *testing.T) {
	db := newTestAccountDB(t, "create_clientreject")
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCreateAccountHTTPHandler(db, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/accounts", clientToken(t, 1), `{}`))
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestCreateAccountHTTP_ServiceError(t *testing.T) {
	db := newTestAccountDB(t, "create_svcerr")
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCreateAccountHTTPHandler(db, cfg)
	// No currency seeded → service should complain about missing/invalid currency
	body := `{"clientId":1,"currencyId":999,"tip":"tekuci","vrsta":"licni"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/accounts", employeeToken(t, 1), body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// --- card_handler client request/verify ---

func TestCardHandler_ClientRequest_BadBody(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCardHTTPHandlerWithConfig(&mockCardSvc{}, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/cards/request", clientToken(t, 5), "not-json"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCardHandler_ClientRequest_Success(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	svc := &fullCardSvc{cardReq: &models.CardRequest{ID: 99, ExpiresAt: time.Now().Add(time.Hour)}}
	h := handler.NewCardHTTPHandlerWithConfig(svc, cfg)
	body := `{"accountId":1,"vrstaKartice":"visa","clientEmail":"x@y.z","clientName":"X Y"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/cards/request", clientToken(t, 5), body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCardHandler_ClientRequest_ServiceError(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	svc := &fullCardSvc{err: errors.New("svc fail")}
	h := handler.NewCardHTTPHandlerWithConfig(svc, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/cards/request", clientToken(t, 5), `{"accountId":1}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCardHandler_ClientVerify_BadID(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCardHTTPHandlerWithConfig(&mockCardSvc{}, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/cards/request/abc/verify", clientToken(t, 5), `{"code":"1234"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCardHandler_ClientVerify_BadBody(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCardHTTPHandlerWithConfig(&mockCardSvc{}, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/cards/request/5/verify", clientToken(t, 5), "not-json"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCardHandler_ClientVerify_Success(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	svc := &mockCardSvc{foundCard: &models.Card{ID: 1, Status: "aktivna"}}
	h := handler.NewCardHTTPHandlerWithConfig(svc, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/cards/request/5/verify", clientToken(t, 5), `{"code":"1234"}`))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCardHandler_ClientVerify_ServiceError(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	svc := &mockCardSvc{err: errors.New("invalid code")}
	h := handler.NewCardHTTPHandlerWithConfig(svc, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/cards/request/5/verify", clientToken(t, 5), `{"code":"wrong"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// --- http_auth: extra branches ---

func TestAuth_NotBearer(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCurrencyHTTPHandler(repository.NewCurrencyRepository(newTestAccountDB(t, "auth_nb")), cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/currencies", nil)
	req.Header.Set("Authorization", "Basic abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_EmptyBearer(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCurrencyHTTPHandler(repository.NewCurrencyRepository(newTestAccountDB(t, "auth_eb")), cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/currencies", nil)
	req.Header.Set("Authorization", "Bearer  ")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_RefreshToken_401(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCurrencyHTTPHandler(repository.NewCurrencyRepository(newTestAccountDB(t, "auth_rt")), cfg)
	tok := makeAccountToken(t, util.Claims{
		EmployeeID: 1, TokenSource: "employee", TokenType: "refresh",
		Permissions: []string{"employeeBasic"},
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/currencies", tok, ""))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_BadToken_401(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCurrencyHTTPHandler(repository.NewCurrencyRepository(newTestAccountDB(t, "auth_bt")), cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/currencies", "garbage-token", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_EmployeeMissingPermission(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	h := handler.NewCurrencyHTTPHandler(repository.NewCurrencyRepository(newTestAccountDB(t, "auth_emp")), cfg)
	tok := makeAccountToken(t, util.Claims{
		EmployeeID: 1, TokenSource: "employee", TokenType: "access",
		Permissions: []string{}, // no employeeBasic
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/currencies", tok, ""))
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

// requireClientOrEmployeeHTTP via list_client_accounts handler
func TestListClientAccountsHTTP_ClientCanAccessOwn(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	repo := &mockListClientAccountsRepo{accounts: []models.Account{{ID: 1}}}
	h := handler.NewListClientAccountsHTTPHandlerWithConfig(repo, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/accounts/client/5", clientToken(t, 5), ""))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListClientAccountsHTTP_ClientCannotAccessOther(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	repo := &mockListClientAccountsRepo{}
	h := handler.NewListClientAccountsHTTPHandlerWithConfig(repo, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/accounts/client/9", clientToken(t, 5), ""))
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestListClientAccountsHTTP_EmployeeCanAccessAny(t *testing.T) {
	cfg := &config.Config{JWTSecret: accountTestJWTSecret}
	repo := &mockListClientAccountsRepo{accounts: []models.Account{{ID: 1}}}
	h := handler.NewListClientAccountsHTTPHandlerWithConfig(repo, cfg)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/accounts/client/42", employeeToken(t, 1), ""))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// --- card handler: unknown route ---

func TestCardHandler_UnknownRoute_404(t *testing.T) {
	h := handler.NewCardHTTPHandler(&mockCardSvc{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/cards/foo/bar/baz", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- helpers ---

// fullCardSvc is a complete mock that supports overriding RequestCardClient
// (the smaller mockCardSvc returns nil unconditionally).
type fullCardSvc struct {
	createdCard  *models.Card
	foundCard    *models.Card
	cards        []models.Card
	cardReq      *models.CardRequest
	err          error
}

func (m *fullCardSvc) CreateCard(_ service.CreateCardInput) (*models.Card, error) {
	return m.createdCard, m.err
}
func (m *fullCardSvc) GetCard(_ uint) (*models.Card, error)            { return m.foundCard, m.err }
func (m *fullCardSvc) ListByAccount(_ uint) ([]models.Card, error)     { return m.cards, m.err }
func (m *fullCardSvc) ListByClient(_ uint) ([]models.Card, error)      { return m.cards, m.err }
func (m *fullCardSvc) BlockCard(_, _ uint) (*models.Card, error)       { return m.foundCard, m.err }
func (m *fullCardSvc) BlockCardWithNotify(_, _ uint, _ *service.CardStatusNotifyInfo) (*models.Card, error) {
	return m.foundCard, m.err
}
func (m *fullCardSvc) UnblockCard(_ uint) (*models.Card, error) { return m.foundCard, m.err }
func (m *fullCardSvc) UnblockCardWithNotify(_ uint, _ *service.CardStatusNotifyInfo) (*models.Card, error) {
	return m.foundCard, m.err
}
func (m *fullCardSvc) DeactivateCard(_ uint) (*models.Card, error) { return m.foundCard, m.err }
func (m *fullCardSvc) DeactivateCardWithNotify(_ uint, _ *service.CardStatusNotifyInfo) (*models.Card, error) {
	return m.foundCard, m.err
}
func (m *fullCardSvc) RequestCardClient(_ service.ClientCardRequestInput) (*models.CardRequest, error) {
	return m.cardReq, m.err
}
func (m *fullCardSvc) VerifyCardRequest(_ uint, _ string) (*models.Card, error) {
	return m.foundCard, m.err
}
