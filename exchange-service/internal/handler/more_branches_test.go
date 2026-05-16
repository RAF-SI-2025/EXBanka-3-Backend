package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// =====================================================================
// PortfolioHTTPHandler.exerciseOption — all 3 branches
// =====================================================================

func TestPortfolioHTTP_ExerciseOption_Unauthorized(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/portfolio/holdings/1/exercise", nil)
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestPortfolioHTTP_ExerciseOption_ClientForbidden(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/portfolio/holdings/1/exercise", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestPortfolioHTTP_ExerciseOption_AgentMissingHolding(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/portfolio/holdings/99999/exercise", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPortfolioHTTP_SetPublic_BadBody(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/portfolio/holdings/1/public", bytes.NewBufferString("not-json"))
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest && rec.Code != http.StatusForbidden {
		t.Fatalf("expected 4xx, got %d", rec.Code)
	}
}

// =====================================================================
// OtcHTTPHandler.getContract / exerciseContract / getSagaStatus — not-found / forbidden paths
// =====================================================================

func TestOtcHTTP_GetContract_Unauthorized(t *testing.T) {
	db := newTestDB(t, "otc_get_contract_unauth")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/contracts/1", nil)
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestOtcHTTP_GetContract_ClientNoTrading(t *testing.T) {
	db := newTestDB(t, "otc_get_contract_notrading")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/contracts/1", nil)
	req.Header.Set("Authorization", "Bearer "+clientWithoutTradingToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestOtcHTTP_GetContract_NotFound(t *testing.T) {
	db := newTestDB(t, "otc_get_contract_notfound")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/contracts/99999", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOtcHTTP_ExerciseContract_NotFound(t *testing.T) {
	db := newTestDB(t, "otc_exercise_notfound")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/otc/contracts/99999/exercise", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code < 400 {
		t.Fatalf("expected 4xx, got %d", rec.Code)
	}
}

func TestOtcHTTP_GetSagaStatus_NoQuerier(t *testing.T) {
	db := newTestDB(t, "otc_saga_noquerier")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/saga/1", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	// sagaRepo is nil → service returns a specific error path
	if rec.Code < 400 {
		t.Fatalf("expected 4xx, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// =====================================================================
// InterbankOtcHTTPHandler — getOptionContract, exerciseOptionContract
// =====================================================================

func TestInterbankOtcHTTP_GetOptionContract_NotFound(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/option-contracts/9999", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtcHTTP_ExerciseOptionContract_BadID(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/interbank-otc/option-contracts/abc/exercise", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestInterbankOtcHTTP_ExerciseOptionContract_NotFound(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/interbank-otc/option-contracts/9999/exercise", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtcHTTP_ExerciseOptionContract_MethodNotAllowed(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/option-contracts/1/exercise", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// =====================================================================
// InterbankOtcHTTPHandler - createNegotiation bad-body / put-update / delete
// =====================================================================

func TestInterbankOtcHTTP_CreateNegotiation_BadBody(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/interbank-otc/negotiations", bytes.NewBufferString("not-json"))
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtcHTTP_UpdateNegotiation_BadBody(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/interbank-otc/negotiations/111/N-1", bytes.NewBufferString("not-json"))
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code < 400 {
		t.Fatalf("expected 4xx, got %d", rec.Code)
	}
}

func TestInterbankOtcHTTP_CloseNegotiation_NotFound(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/interbank-otc/negotiations/111/N-1", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code < 400 {
		t.Fatalf("expected 4xx, got %d", rec.Code)
	}
}

// acceptNegotiation panics with a nil registry; skipped.

func TestInterbankOtcHTTP_AcceptNegotiation_MethodNotAllowed(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/negotiations/111/N-1/accept", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
