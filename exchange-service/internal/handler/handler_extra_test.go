package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"gorm.io/gorm"
)

// --- order handler: approve / decline / cancel / listTransactions ---

func TestOrderHTTP_Approve_ForbiddenForNonSupervisor(t *testing.T) {
	db := newTestDB(t, "h_orders_approve_forbid")
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/1/approve", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t)) // agent, not supervisor
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_Approve_OrderNotFound(t *testing.T) {
	db := newTestDB(t, "h_orders_approve_nf")
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/9999/approve", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	// service returns "order not found", handler maps to 400
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_Decline_ForbiddenForNonSupervisor(t *testing.T) {
	db := newTestDB(t, "h_orders_decline_forbid")
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/1/decline", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestOrderHTTP_Decline_OrderNotFound(t *testing.T) {
	db := newTestDB(t, "h_orders_decline_nf")
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/9999/decline", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_Cancel_OrderNotFound(t *testing.T) {
	db := newTestDB(t, "h_orders_cancel_nf")
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/9999/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestOrderHTTP_Cancel_ForbiddenForNonOwner(t *testing.T) {
	db := newTestDB(t, "h_orders_cancel_forbid")
	_, assetID := seedExchangeAndListing(t, db, "CXL")
	// Create an order owned by client 100; bankToken is an employee agent (non-supervisor),
	// callerIdentity returns (0,"bank") — not the owner.
	order := models.OrderRecord{
		UserID: 100, UserType: "client", AssetID: assetID, OrderType: "limit",
		Direction: "buy", Quantity: 1, ContractSize: 1, PricePerUnit: 100,
		Status: "approved", RemainingPortions: 1,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/orders/%d/cancel", order.ID), strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_ListTransactions_OrderNotFound(t *testing.T) {
	db := newTestDB(t, "h_orders_tx_nf")
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/9999/transactions", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestOrderHTTP_ListTransactions_OK(t *testing.T) {
	db := newTestDB(t, "h_orders_tx_ok")
	_, assetID := seedExchangeAndListing(t, db, "TXR")
	// Order owned by the bank — bankToken caller maps to (0,"bank") and is the owner.
	order := models.OrderRecord{
		UserID: 0, UserType: "bank", AssetID: assetID, OrderType: "limit",
		Direction: "buy", Quantity: 1, ContractSize: 1, PricePerUnit: 100,
		Status: "approved", RemainingPortions: 1,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/orders/%d/transactions", order.ID), nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// --- market handler: GetPortfolio + toggleExchangeTime ---

func TestMarketHTTP_GetPortfolio_WrongMethod(t *testing.T) {
	db := newTestDB(t, "h_get_portfolio_method")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/portfolio", nil)
	rec := httptest.NewRecorder()
	h.GetPortfolio(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestMarketHTTP_GetPortfolio_OKForBank(t *testing.T) {
	db := newTestDB(t, "h_get_portfolio_ok")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.GetPortfolio(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExchangeRoutes_ToggleForbiddenForNonSupervisor(t *testing.T) {
	db := newTestDB(t, "h_exch_toggle_forbid")
	exch := models.MarketExchangeRecord{
		Acronym: "TGL", Name: "TGL", MICCode: "T1", Polity: "X", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatal(err)
	}
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exchanges/TGL/toggle", strings.NewReader(`{"useManualTime":true,"manualTimeOpen":true}`))
	req.Header.Set("Authorization", "Bearer "+bankToken(t)) // agent, not supervisor
	rec := httptest.NewRecorder()
	h.ExchangeRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExchangeRoutes_ToggleSuccess(t *testing.T) {
	db := newTestDB(t, "h_exch_toggle_ok")
	exch := models.MarketExchangeRecord{
		Acronym: "TGS", Name: "TGS", MICCode: "T2", Polity: "X", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatal(err)
	}
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exchanges/TGS/toggle", strings.NewReader(`{"useManualTime":true,"manualTimeOpen":false}`))
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.ExchangeRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["useManualTime"] != true {
		t.Errorf("body=%+v", body)
	}
}

func TestExchangeRoutes_ToggleBadJSON(t *testing.T) {
	db := newTestDB(t, "h_exch_toggle_badjson")
	h := setupMarketHandler(t, db)
	// Body is empty — json decode fails
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exchanges/X/toggle", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.ExchangeRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestExchangeRoutes_UnknownSubpath(t *testing.T) {
	db := newTestDB(t, "h_exch_unknown_sub")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exchanges/X/unknown", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ExchangeRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestExchangeRoutes_EmptyAcronym(t *testing.T) {
	db := newTestDB(t, "h_exch_empty")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exchanges/", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ExchangeRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- tax handler ---

func setupTaxHandler(t *testing.T, db *gorm.DB) *TaxHTTPHandler {
	cfg := &config.Config{JWTSecret: testJWTSecret}
	taxRepo := repository.NewTaxRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	taxSvc := service.NewTaxService(taxRepo, marketRepo, &fakeRates{})
	collector := service.NewTaxCollector(taxSvc, orderRepo, taxRepo)
	return NewTaxHTTPHandler(cfg, taxSvc, collector, nil)
}

func TestTaxHTTP_UnknownRoute_404(t *testing.T) {
	db := newTestDB(t, "h_tax_unknown")
	h := setupTaxHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/somethingelse", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTaxHTTP_Collect_ForbiddenForNonSupervisor(t *testing.T) {
	db := newTestDB(t, "h_tax_collect_forbid")
	h := setupTaxHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tax/collect", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTaxHTTP_Collect_Unauthorized(t *testing.T) {
	db := newTestDB(t, "h_tax_collect_unauth")
	h := setupTaxHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tax/collect", strings.NewReader(`{"period":"2025-12"}`))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestTaxHTTP_Collect_SupervisorSucceeds(t *testing.T) {
	db := newTestDB(t, "h_tax_collect_ok")
	h := setupTaxHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tax/collect", strings.NewReader(`{"period":"2025-12"}`))
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["period"] != "2025-12" {
		t.Errorf("body=%+v", body)
	}
}

func TestTaxHTTP_Collect_DefaultsPeriodWhenBodyEmpty(t *testing.T) {
	db := newTestDB(t, "h_tax_collect_defp")
	h := setupTaxHandler(t, db)
	// Empty body — collector should fall back to PreviousMonthPeriod
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tax/collect", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTaxHTTP_Records_ForbiddenForNonSupervisor(t *testing.T) {
	db := newTestDB(t, "h_tax_records_forbid")
	h := setupTaxHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/records", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTaxHTTP_Records_OKEmpty(t *testing.T) {
	db := newTestDB(t, "h_tax_records_ok")
	h := setupTaxHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/records", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTaxHTTP_Records_FilterByUserID(t *testing.T) {
	db := newTestDB(t, "h_tax_records_uid")
	h := setupTaxHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/records?userId=42&userType=client", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestTaxHTTP_Records_BadUserID(t *testing.T) {
	db := newTestDB(t, "h_tax_records_baduid")
	h := setupTaxHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/records?userId=not-a-num", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTaxHTTP_Summary_BadUserID(t *testing.T) {
	db := newTestDB(t, "h_tax_sum_baduid")
	h := setupTaxHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/summary/abc", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTaxHTTP_Summary_ForbiddenForOtherClient(t *testing.T) {
	db := newTestDB(t, "h_tax_sum_forbid")
	h := setupTaxHandler(t, db)
	// clientToken is for clientID 100; ask about user 999
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/summary/999?userType=client", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTaxHTTP_Summary_SelfClientOK(t *testing.T) {
	db := newTestDB(t, "h_tax_sum_self")
	h := setupTaxHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/summary/100?userType=client", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTaxHTTP_Summary_SupervisorOK(t *testing.T) {
	db := newTestDB(t, "h_tax_sum_super")
	h := setupTaxHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tax/summary/777?userType=client&period=2025-12", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.TaxRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["user_id"].(float64) != 777 {
		t.Errorf("body=%+v", body)
	}
}
