package handler_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/util"
	"github.com/golang-jwt/jwt/v5"
)

const testJWTSecret = "loan-test-secret"

func makeLoanToken(t *testing.T, claims util.Claims) string {
	t.Helper()
	claims.RegisteredClaims = jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	tk := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	signed, err := tk.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func employeeBasicToken(t *testing.T, employeeID uint) string {
	return makeLoanToken(t, util.Claims{
		EmployeeID:  employeeID,
		TokenSource: "employee",
		TokenType:   "access",
		Permissions: []string{"employeeBasic"},
	})
}

func clientBasicToken(t *testing.T, clientID uint) string {
	return makeLoanToken(t, util.Claims{
		ClientID:    clientID,
		TokenSource: "client",
		TokenType:   "access",
		Permissions: []string{"clientBasic"},
	})
}

func newAuthHandler(svc handler.LoanServiceInterface) http.Handler {
	cfg := &config.Config{JWTSecret: testJWTSecret}
	return handler.NewLoanHandlerWithConfig(svc, cfg, nil)
}

func authedGET(h http.Handler, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func authedPOST(h http.Handler, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// --- routing edge cases ---

func TestServeHTTP_EmptyPath_404(t *testing.T) {
	h := newHandler(&mockLoanService{})
	w := getRequest(h, "/api/v1/loans")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestServeHTTP_UnknownRoute_404(t *testing.T) {
	h := newHandler(&mockLoanService{})
	w := getRequest(h, "/api/v1/loans/unknown/path/here")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- auth: parseHTTPClaims ---

func TestAuth_NoToken_401(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	w := getRequest(h, "/api/v1/loans/requests")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuth_NotBearer_401(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/loans/requests", nil)
	req.Header.Set("Authorization", "Basic abc")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuth_EmptyBearer_401(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/loans/requests", nil)
	req.Header.Set("Authorization", "Bearer  ")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuth_BadToken_401(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	w := authedGET(h, "/api/v1/loans/requests", "garbage")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuth_RefreshTokenRejected_401(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	tok := makeLoanToken(t, util.Claims{
		EmployeeID: 1, TokenSource: "employee", TokenType: "refresh",
		Permissions: []string{"employeeBasic"},
	})
	w := authedGET(h, "/api/v1/loans/requests", tok)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for refresh token, got %d", w.Code)
	}
}

// --- requireEmployeePermissionHTTP ---

func TestEmployeeOnlyEndpoint_RejectsClient(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	w := authedGET(h, "/api/v1/loans/requests", clientBasicToken(t, 5))
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestEmployeeOnlyEndpoint_RejectsMissingPermission(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	tok := makeLoanToken(t, util.Claims{
		EmployeeID: 1, TokenSource: "employee", TokenType: "access",
		Permissions: []string{}, // no employeeBasic
	})
	w := authedGET(h, "/api/v1/loans/requests", tok)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestEmployeeOnlyEndpoint_AcceptsEmployee(t *testing.T) {
	svc := &mockLoanService{loans: []models.Loan{{ID: 1}}}
	h := newAuthHandler(svc)
	w := authedGET(h, "/api/v1/loans/requests", employeeBasicToken(t, 7))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestListAll_RequiresEmployee(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	w := authedGET(h, "/api/v1/loans/all", clientBasicToken(t, 1))
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// --- handleListByClient: client-based access control ---

func TestListByClient_ClientCanAccessOwn(t *testing.T) {
	svc := &mockLoanService{loans: []models.Loan{{ID: 1}}}
	h := newAuthHandler(svc)
	w := authedGET(h, "/api/v1/loans/client/5", clientBasicToken(t, 5))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestListByClient_ClientCannotAccessOther(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	w := authedGET(h, "/api/v1/loans/client/9", clientBasicToken(t, 5))
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestListByClient_EmployeeCanAccessAny(t *testing.T) {
	svc := &mockLoanService{loans: []models.Loan{{ID: 1}}}
	h := newAuthHandler(svc)
	w := authedGET(h, "/api/v1/loans/client/42", employeeBasicToken(t, 1))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestListByClient_ServiceError_Returns500(t *testing.T) {
	svc := &mockLoanService{err: errors.New("db down")}
	h := newAuthHandler(svc)
	w := authedGET(h, "/api/v1/loans/client/42", employeeBasicToken(t, 1))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- handleGetByID: client ownership check via DB ---

func TestGetByID_ClientWithoutDB_500(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	w := authedGET(h, "/api/v1/loans/3", clientBasicToken(t, 5))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestGetByID_EmployeeCanAccessAny(t *testing.T) {
	svc := &mockLoanService{loan: &models.Loan{ID: 1}}
	h := newAuthHandler(svc)
	w := authedGET(h, "/api/v1/loans/1", employeeBasicToken(t, 1))
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetByID_ServiceError_404(t *testing.T) {
	svc := &mockLoanService{err: errors.New("not found")}
	h := newAuthHandler(svc)
	w := authedGET(h, "/api/v1/loans/77", employeeBasicToken(t, 1))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- handleListInstallments: same shape as GetByID ---

func TestListInstallments_InvalidID_400(t *testing.T) {
	w := getRequest(newHandler(&mockLoanService{}), "/api/v1/loans/abc/installments")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListInstallments_ClientWithoutDB_500(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	w := authedGET(h, "/api/v1/loans/3/installments", clientBasicToken(t, 5))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestListInstallments_EmployeeServiceError_500(t *testing.T) {
	svc := &mockLoanService{err: errors.New("db")}
	h := newAuthHandler(svc)
	w := authedGET(h, "/api/v1/loans/1/installments", employeeBasicToken(t, 1))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- approve / reject auth + body handling ---

func TestApprove_RejectsClient(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	w := authedPOST(h, "/api/v1/loans/1/approve", clientBasicToken(t, 5), `{}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestApprove_RejectsZaposleniMismatch(t *testing.T) {
	h := newAuthHandler(&mockLoanService{loan: &models.Loan{ID: 1}})
	// employee 7 sends body with zaposleni_id=99 → mismatch → 403
	w := authedPOST(h, "/api/v1/loans/1/approve", employeeBasicToken(t, 7), `{"zaposleni_id":99}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestApprove_AcceptsMatchingZaposleni(t *testing.T) {
	svc := &mockLoanService{loan: &models.Loan{ID: 1, Status: "aktivan"}}
	h := newAuthHandler(svc)
	w := authedPOST(h, "/api/v1/loans/1/approve", employeeBasicToken(t, 7), `{"zaposleni_id":7}`)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestApprove_ServiceError_400(t *testing.T) {
	svc := &mockLoanService{err: errors.New("already approved")}
	h := newAuthHandler(svc)
	w := authedPOST(h, "/api/v1/loans/1/approve", employeeBasicToken(t, 7), `{}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestReject_RejectsClient(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	w := authedPOST(h, "/api/v1/loans/1/reject", clientBasicToken(t, 5), `{}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestReject_InvalidID_400(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	w := authedPOST(h, "/api/v1/loans/abc/reject", employeeBasicToken(t, 7), `{}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestReject_ZaposleniMismatch_403(t *testing.T) {
	h := newAuthHandler(&mockLoanService{loan: &models.Loan{ID: 1}})
	w := authedPOST(h, "/api/v1/loans/1/reject", employeeBasicToken(t, 7), `{"zaposleni_id":99}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestReject_ServiceError_400(t *testing.T) {
	svc := &mockLoanService{err: errors.New("cannot reject")}
	h := newAuthHandler(svc)
	w := authedPOST(h, "/api/v1/loans/1/reject", employeeBasicToken(t, 7), `{}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- request: client-perm gating with auth ---

func TestRequest_ClientPermitsOwnRequest(t *testing.T) {
	svc := &mockLoanService{loan: &models.Loan{ID: 1, Status: "zahtev"}}
	h := newAuthHandler(svc)
	w := authedPOST(h, "/api/v1/loans/request", clientBasicToken(t, 5), `{"vrsta":"gotovinski","iznos":10000,"period":12,"client_id":5}`)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestRequest_ClientCannotImpersonate(t *testing.T) {
	h := newAuthHandler(&mockLoanService{})
	w := authedPOST(h, "/api/v1/loans/request", clientBasicToken(t, 5), `{"client_id":99}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestRequest_ServiceFailsNonValidationError_500(t *testing.T) {
	svc := &mockLoanService{err: errors.New("db down")}
	h := newAuthHandler(svc)
	w := authedPOST(h, "/api/v1/loans/request", clientBasicToken(t, 5), `{"vrsta":"x","client_id":5}`)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- listAll service error ---

func TestListAll_ServiceError_500(t *testing.T) {
	svc := &mockLoanService{err: errors.New("oops")}
	h := newAuthHandler(svc)
	w := authedGET(h, "/api/v1/loans/all", employeeBasicToken(t, 1))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestListRequests_ServiceError_500(t *testing.T) {
	svc := &mockLoanService{err: errors.New("oops")}
	h := newAuthHandler(svc)
	w := authedGET(h, "/api/v1/loans/requests", employeeBasicToken(t, 1))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
