package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// =====================================================================
// ExchangeHandler (gRPC)
// =====================================================================

func TestNewExchangeHandler_Constructs(t *testing.T) {
	h := NewExchangeHandler()
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

// =====================================================================
// InterbankPaymentHTTPHandler — listPayments + getPayment routes
// =====================================================================

func setupInterbankPaymentHandler(t *testing.T) *InterbankPaymentHTTPHandler {
	return setupInterbankPaymentHandlerNamed(t, "ib_payment_handler")
}

func setupInterbankPaymentHandlerNamed(t *testing.T, name string) *InterbankPaymentHTTPHandler {
	db := newFundTestDB(t, name)
	cfg := &config.Config{JWTSecret: testJWTSecret}
	return NewInterbankPaymentHTTPHandler(cfg, nil, nil, repository.NewInterbankPaymentRepository(db), repository.NewInterbankPaymentWalletRepository(db), db)
}

func TestInterbankPaymentHTTP_Routes_MethodNotAllowed(t *testing.T) {
	h := setupInterbankPaymentHandler(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/payments/cross-bank", nil)
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestInterbankPaymentHTTP_ListPayments_Unauthorized(t *testing.T) {
	h := setupInterbankPaymentHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/cross-bank", nil)
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestInterbankPaymentHTTP_ListPayments_Empty(t *testing.T) {
	h := setupInterbankPaymentHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/cross-bank", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankPaymentHTTP_GetPayment_BadID(t *testing.T) {
	h := setupInterbankPaymentHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/cross-bank/abc", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestInterbankPaymentHTTP_GetPayment_NotFound(t *testing.T) {
	h := setupInterbankPaymentHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/cross-bank/99999", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankPaymentHTTP_Routes_DeepPath_NotFound(t *testing.T) {
	h := setupInterbankPaymentHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/payments/cross-bank/1/extra", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// =====================================================================
// InterbankOtcHTTPHandler — Routes dispatch + simple endpoints
// =====================================================================

func setupInterbankOtcHandler(t *testing.T) *InterbankOtcHTTPHandler {
	return setupInterbankOtcHandlerNamed(t, "ib_otc_handler")
}

func setupInterbankOtcHandlerNamed(t *testing.T, name string) *InterbankOtcHTTPHandler {
	db := newFundTestDB(t, name)
	cfg := &config.Config{JWTSecret: testJWTSecret}
	return NewInterbankOtcHTTPHandler(
		cfg, nil, nil,
		repository.NewInterbankOtcRepository(db),
		nil, // negs handler (interbank.NegotiationsHandler)
		repository.NewRemotePublicStockRepository(db),
		repository.NewInterbankOptionContractRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewInterbankWalletRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
		db,
	)
}

func TestInterbankOtcHTTP_Routes_Unauthorized(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/negotiations", nil)
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// listPublicStocks requires a non-nil interbank registry to fan out; skipped here.

func TestInterbankOtcHTTP_ListNegotiations_AuthorizedEmpty(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/negotiations", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtcHTTP_Routes_BadRoute(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/totally-bogus", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 404 or 405, got %d", rec.Code)
	}
}

func TestInterbankOtcHTTP_GetNegotiation_BadID(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/negotiations/bogus", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 for bogus id, got %d", rec.Code)
	}
}

// =====================================================================
// InterbankOtcHTTPHandler — exercise routes (methods on the OTC handler)
// =====================================================================

func TestInterbankOtcHTTP_ListContracts_AuthorizedEmpty(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/option-contracts", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// =====================================================================
// TaxHTTPHandler routes
// =====================================================================

func setupTaxHandlerExtras(t *testing.T) *TaxHTTPHandler {
	db := newFundTestDB(t, "tax_handler")
	cfg := &config.Config{JWTSecret: testJWTSecret}
	rates := fundRatesProv{}
	taxRepo := repository.NewTaxRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	taxSvc := service.NewTaxService(taxRepo, marketRepo, rates)
	collector := service.NewTaxCollector(taxSvc, orderRepo, taxRepo)
	return NewTaxHTTPHandler(cfg, taxSvc, collector)
}

func TestTaxHTTP_Routes_Unauthorized(t *testing.T) {
	h := setupTaxHandlerExtras(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/records", nil)
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestTaxHTTP_ListRecords_Empty(t *testing.T) {
	h := setupTaxHandlerExtras(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/records", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTaxHTTP_ListRecords_ClientForbidden(t *testing.T) {
	h := setupTaxHandlerExtras(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/records", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestTaxHTTP_TriggerCollection_RequiresSupervisor(t *testing.T) {
	h := setupTaxHandlerExtras(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tax/collect", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected forbidden/unauthorized, got %d", rec.Code)
	}
}

// =====================================================================
// PortfolioHTTPHandler routes
// =====================================================================

func setupPortfolioHandlerNew(t *testing.T) *PortfolioHTTPHandler {
	db := newFundTestDB(t, "portfolio_handler")
	cfg := &config.Config{JWTSecret: testJWTSecret}
	rates := fundRatesProv{}
	taxRepo := repository.NewTaxRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	portRepo := repository.NewPortfolioRepository(db)
	taxSvc := service.NewTaxService(taxRepo, marketRepo, rates)
	portSvc := service.NewPortfolioService(portRepo, taxSvc, marketRepo, orderRepo)
	return NewPortfolioHTTPHandler(cfg, portSvc)
}

func TestPortfolioHTTP_Collection_Unauthorized(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio", nil)
	rec := httptest.NewRecorder()
	h.PortfolioCollection(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestPortfolioHTTP_Collection_Empty(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioCollection(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPortfolioHTTP_Routes_GetHolding_NotFound(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/holdings/9999", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Fatalf("expected 404/403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPortfolioHTTP_Routes_BadID(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/holdings/abc", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPortfolioHTTP_Routes_UnknownPath(t *testing.T) {
	h := setupPortfolioHandlerNew(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/unknown", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestPortfolioHTTP_Round2(t *testing.T) {
	if got := round2(1.005); got < 1.0 || got > 1.01 {
		t.Fatalf("unexpected round2: %v", got)
	}
}

func TestHoldingToResponse(t *testing.T) {
	h := service.HoldingWithPnL{
		Holding: &models.PortfolioHoldingRecord{
			ID: 1, UserID: 2, UserType: "client",
			AssetID: 3, Quantity: 10,
		},
	}
	out := holdingToResponse(h)
	if out.ID != 1 {
		t.Fatalf("expected id=1, got %d", out.ID)
	}
}

// =====================================================================
// OrderHTTPHandler routes
// =====================================================================

func setupOrderHandlerNew(t *testing.T) *OrderHTTPHandler {
	db := newFundTestDB(t, "order_handler")
	cfg := &config.Config{JWTSecret: testJWTSecret}
	rates := fundRatesProv{}
	taxRepo := repository.NewTaxRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	portRepo := repository.NewPortfolioRepository(db)
	taxSvc := service.NewTaxService(taxRepo, marketRepo, rates)
	portSvc := service.NewPortfolioService(portRepo, taxSvc, marketRepo, orderRepo)
	_ = portSvc
	orderSvc := service.NewOrderService(orderRepo, marketRepo, rates)
	return NewOrderHTTPHandler(cfg, orderSvc)
}

func TestOrderHTTP_CreateOrder_BadBody(t *testing.T) {
	h := setupOrderHandlerNew(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", bytes.NewBufferString("not-json"))
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OrdersCollection(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestOrderHTTP_ListOrders_Empty(t *testing.T) {
	h := setupOrderHandlerNew(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OrdersCollection(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_Routes_BadID(t *testing.T) {
	h := setupOrderHandlerNew(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/abc", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestOrderHTTP_GetOrder_NotFound(t *testing.T) {
	h := setupOrderHandlerNew(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/9999", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_CancelOrder_NotFound(t *testing.T) {
	h := setupOrderHandlerNew(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/9999/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_ApproveOrder_NotFound(t *testing.T) {
	h := setupOrderHandlerNew(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/9999/approve", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code < 400 {
		t.Fatalf("expected non-2xx, got %d", rec.Code)
	}
}

// =====================================================================
// summariseNoVote, paymentRowToResponse helpers
// =====================================================================

func TestPaymentRowToResponse(t *testing.T) {
	row := &models.InterbankPayment{
		ID: 1, TxRoutingNumber: 111, TxID: "PAY-1",
		Direction: "outbound", PartnerRoutingNumber: 222,
		SenderAccountNumber: "S", RecipientAccountNumber: "R",
		Currency: "RSD", Amount: 100, Status: "pending",
	}
	out := paymentRowToResponse(row)
	if out["id"].(uint) != 1 {
		t.Fatalf("expected id=1, got %v", out["id"])
	}
	if out["status"].(string) != "pending" {
		t.Fatalf("expected status=pending, got %v", out["status"])
	}
}

func TestPreparePaymentErr_ImplementsError(t *testing.T) {
	e := preparePaymentErr{msg: "hello"}
	var err error = e
	if err.Error() != "hello" {
		t.Fatalf("expected hello, got %s", err.Error())
	}
}

func TestExchangeToResponse_AndListingToResponse(t *testing.T) {
	exch := models.Exchange{ID: 1, Acronym: "X", Name: "X", Currency: "USD"}
	if r := exchangeToResponse(exch); r.Acronym != "X" {
		t.Fatalf("expected acronym X, got %s", r.Acronym)
	}
	summary := models.ExchangeSummary{Acronym: "X"}
	if r := exchangeSummaryToResponse(summary); r.Acronym != "X" {
		t.Fatalf("expected summary X, got %s", r.Acronym)
	}
	listing := models.Listing{Ticker: "AAPL", Price: 100}
	if r := listingToResponse(listing); r.Ticker != "AAPL" {
		t.Fatalf("expected AAPL, got %s", r.Ticker)
	}
}

func TestBlackScholesTheta_NotPanic(t *testing.T) {
	// Just ensure both call paths execute.
	v := blackScholesTheta(100, 100, 0.2, mustParseTime(t, "2030-01-01T00:00:00Z"), "call")
	_ = v
	v = blackScholesTheta(100, 100, 0.2, mustParseTime(t, "2030-01-01T00:00:00Z"), "put")
	_ = v
	v = blackScholesTheta(100, 100, 0.2, mustParseTime(t, "2020-01-01T00:00:00Z"), "call") // already expired
	_ = v
	_ = normalCDF(0.5)
	_ = normalCDF(-0.5)
}

func mustParseTime(t *testing.T, s string) (out time.Time) {
	t.Helper()
	out, _ = time.Parse(time.RFC3339, s)
	return out
}

// Suppress unused-import lints when we trim the test file.
var _ = json.NewEncoder
