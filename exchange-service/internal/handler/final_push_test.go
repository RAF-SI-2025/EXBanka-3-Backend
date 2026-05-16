package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
)

// =====================================================================
// FundHTTPHandler.listMyPositions — supervisor + agent branches
// =====================================================================

func TestFundHTTP_ListMyPositions_Supervisor(t *testing.T) {
	db := newFundTestDB(t, "fh_pos_supervisor")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/funds/positions/mine", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFundHTTP_ListMyPositions_AgentEmpty(t *testing.T) {
	db := newFundTestDB(t, "fh_pos_agent")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/funds/positions/mine", nil)
	// bankToken is an agent (PermEmployeeAgent only, no supervisor) — empty list path
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// =====================================================================
// OrderHTTP createOrder — body branches
// =====================================================================

func TestOrderHTTP_CreateOrder_NoTradingPermission(t *testing.T) {
	h := setupOrderHandlerNew(t)
	// Build a token without canTrade.
	tok := makeToken(t, util.Claims{ClientID: 100, TokenSource: "client", TokenType: "access", Permissions: []string{models.PermClientBasic}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.OrdersCollection(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestOrderHTTP_CreateOrder_FundOrder_NoFundService(t *testing.T) {
	h := setupOrderHandlerNew(t)
	body, _ := json.Marshal(map[string]interface{}{
		"fundId":      1,
		"assetTicker": "AAPL",
		"orderType":   "market",
		"direction":   "buy",
		"quantity":    1,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.OrdersCollection(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (no fundSvc), got %d body=%s", rec.Code, rec.Body.String())
	}
}

// =====================================================================
// InterbankOtc — listOptionContracts requires CLIENT token (employee gets 403)
// =====================================================================

func TestInterbankOtcHTTP_ListOptionContracts_EmployeeForbidden(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/option-contracts", nil)
	// supervisorToken has employee source — should get 403 from localParticipantIDFromClaims
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtcHTTP_GetOptionContract_EmployeeForbidden(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/option-contracts/1", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtcHTTP_ListNegotiations_EmployeeForbidden(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/negotiations", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtcHTTP_GetNegotiation_EmployeeForbidden(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/negotiations/111/N-1", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// =====================================================================
// localParticipantIDFromClaims branches
// =====================================================================

func TestLocalParticipantIDFromClaims_EmployeeReturnsFalse(t *testing.T) {
	claims := &util.Claims{TokenSource: "employee", EmployeeID: 5}
	if _, ok := localParticipantIDFromClaims(claims); ok {
		t.Fatal("expected false for employee token")
	}
}

func TestLocalParticipantIDFromClaims_ZeroClientReturnsFalse(t *testing.T) {
	claims := &util.Claims{TokenSource: "client", ClientID: 0}
	if _, ok := localParticipantIDFromClaims(claims); ok {
		t.Fatal("expected false for zero clientID")
	}
}

func TestLocalParticipantIDFromClaims_ClientReturnsID(t *testing.T) {
	claims := &util.Claims{TokenSource: "client", ClientID: 42}
	id, ok := localParticipantIDFromClaims(claims)
	if !ok || id == "" {
		t.Fatalf("expected ok+id, got %q ok=%v", id, ok)
	}
}

// =====================================================================
// PortfolioHTTP — listHoldings extra branches
// =====================================================================

func TestPortfolioHTTP_Routes_HoldingsList_Extra(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/holdings", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPortfolioHTTP_Routes_HoldingsList_MethodNotAllowed(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/portfolio/holdings", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestPortfolioHTTP_Routes_GetHolding_MethodNotAllowed(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/portfolio/holdings/1", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
