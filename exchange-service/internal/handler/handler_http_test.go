package handler

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/provider"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

const testJWTSecret = "test-secret"

func newTestDB(t *testing.T, name string) *gorm.DB {
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

func makeToken(t *testing.T, claims util.Claims) string {
	t.Helper()
	claims.RegisteredClaims = jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour))}
	tk := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	signed, err := tk.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func clientToken(t *testing.T) string {
	return makeToken(t, util.Claims{
		ClientID: 100, TokenSource: "client", TokenType: "access",
		Permissions: []string{models.PermClientTrading, models.PermClientBasic},
	})
}

func bankToken(t *testing.T) string {
	return makeToken(t, util.Claims{
		EmployeeID: 5, TokenSource: "employee", TokenType: "access",
		Permissions: []string{models.PermEmployeeAgent},
	})
}

func supervisorToken(t *testing.T) string {
	return makeToken(t, util.Claims{
		EmployeeID: 6, TokenSource: "employee", TokenType: "access",
		Permissions: []string{models.PermEmployeeSupervisor},
	})
}

func seedExchangeAndListing(t *testing.T, db *gorm.DB, ticker string) (uint, uint) {
	t.Helper()
	exch := models.MarketExchangeRecord{
		Acronym: "X", Name: "X Exchange", MICCode: "X1", Polity: "X", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatal(err)
	}
	listing := models.MarketListingRecord{
		Ticker: ticker, Name: ticker, Type: "stock",
		ExchangeID: exch.ID, Price: 100, Ask: 101, Bid: 99, Volume: 1000,
	}
	if err := db.Create(&listing).Error; err != nil {
		t.Fatal(err)
	}
	return exch.ID, listing.ID
}

func setupMarketHandler(t *testing.T, db *gorm.DB) *MarketHTTPHandler {
	repo := repository.NewMarketRepository(db)
	prov := provider.NewDatabaseMarketProvider(repo)
	svc := service.NewMarketService(prov)
	cfg := &config.Config{JWTSecret: testJWTSecret}
	return NewMarketHTTPHandler(cfg, svc, repo)
}

func TestMarketHTTP_ListExchanges_Authorized(t *testing.T) {
	db := newTestDB(t, "h_list_exchanges")
	seedExchangeAndListing(t, db, "AAA")
	h := setupMarketHandler(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/exchanges", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()

	h.ListExchanges(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["count"].(float64) != 1 {
		t.Errorf("count=%v", body["count"])
	}
}

func TestMarketHTTP_ListExchanges_Unauthenticated(t *testing.T) {
	db := newTestDB(t, "h_list_exch_unauth")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exchanges", nil)
	rec := httptest.NewRecorder()
	h.ListExchanges(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMarketHTTP_ListExchanges_WrongMethod(t *testing.T) {
	db := newTestDB(t, "h_list_exch_method")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exchanges", nil)
	rec := httptest.NewRecorder()
	h.ListExchanges(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestMarketHTTP_ListListings_Filter(t *testing.T) {
	db := newTestDB(t, "h_list_listings")
	seedExchangeAndListing(t, db, "ZZZ")
	h := setupMarketHandler(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/listings?q=ZZ&type=stock", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()

	h.ListListings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["count"].(float64) != 1 {
		t.Errorf("count=%v", body["count"])
	}
}

func TestMarketHTTP_ListListings_TypeFilterExcludes(t *testing.T) {
	db := newTestDB(t, "h_list_listings_typefilter")
	seedExchangeAndListing(t, db, "STK") // stock
	h := setupMarketHandler(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/listings?type=option", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()

	h.ListListings(rec, req)
	var body map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["count"].(float64) != 0 {
		t.Errorf("type filter should exclude stock when type=option")
	}
}

func TestMarketHTTP_ListingRoutes_GetListing(t *testing.T) {
	db := newTestDB(t, "h_listing_routes_get")
	seedExchangeAndListing(t, db, "FOO")
	h := setupMarketHandler(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/listings/FOO", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ListingRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMarketHTTP_ListingRoutes_NotFound(t *testing.T) {
	db := newTestDB(t, "h_listing_routes_nf")
	h := setupMarketHandler(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/listings/NOPE", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ListingRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestMarketHTTP_ListingRoutes_EmptyPath(t *testing.T) {
	db := newTestDB(t, "h_listing_routes_empty")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/listings/", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ListingRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- exchange status routes ---

func TestExchangeRoutes_StatusEndpoint(t *testing.T) {
	db := newTestDB(t, "h_exch_status")
	exch := models.MarketExchangeRecord{
		Acronym: "NASDAQ", Name: "NASDAQ", MICCode: "X", Polity: "X", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
		UseManualTime: true, ManualTimeOpen: true,
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatal(err)
	}
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exchanges/NASDAQ/status", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ExchangeRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExchangeRoutes_NotFound(t *testing.T) {
	db := newTestDB(t, "h_exch_status_nf")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exchanges/NOPE/status", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ExchangeRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- portfolio handler HTTP ---

func setupPortfolioHandler(t *testing.T, db *gorm.DB) *PortfolioHTTPHandler {
	cfg := &config.Config{JWTSecret: testJWTSecret}
	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &fakeRates{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	return NewPortfolioHTTPHandler(cfg, psvc)
}

type fakeRates struct{}

func (fakeRates) GetRate(from, to string) (float64, error) {
	if from == to {
		return 1, nil
	}
	return 1, nil
}
func (fakeRates) GetAllRates() []service.ExchangeRate { return nil }

func TestPortfolioHTTP_Collection_EmptyForBank(t *testing.T) {
	db := newTestDB(t, "h_pf_empty")
	h := setupPortfolioHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioCollection(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPortfolioHTTP_Collection_WrongMethod(t *testing.T) {
	db := newTestDB(t, "h_pf_method")
	h := setupPortfolioHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/portfolio", nil)
	rec := httptest.NewRecorder()
	h.PortfolioCollection(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestPortfolioHTTP_Routes_HoldingsList(t *testing.T) {
	db := newTestDB(t, "h_pf_holdings")
	h := setupPortfolioHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/holdings", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPortfolioHTTP_Routes_BadHoldingID(t *testing.T) {
	db := newTestDB(t, "h_pf_bad_id")
	h := setupPortfolioHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/holdings/not-a-num", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPortfolioHTTP_Routes_HoldingNotFound(t *testing.T) {
	db := newTestDB(t, "h_pf_not_found")
	h := setupPortfolioHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/holdings/9999", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestPortfolioHTTP_Routes_SetPublicQuantity(t *testing.T) {
	db := newTestDB(t, "h_pf_public_quantity")
	_, assetID := seedExchangeAndListing(t, db, "PUB")
	holding := models.PortfolioHoldingRecord{
		UserID: 0, UserType: "bank", AssetID: assetID, Quantity: 10, AvgBuyPrice: 100, AccountID: 7,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupPortfolioHandler(t, db)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/portfolio/holdings/%d/public", holding.ID), strings.NewReader(`{"publicQuantity":4}`))
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["publicQuantity"].(float64) != 4 || body["availableForOtc"].(float64) != 4 {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestPortfolioHTTP_Routes_SetPublicRejectsNonStock(t *testing.T) {
	db := newTestDB(t, "h_pf_public_non_stock")
	exch := models.MarketExchangeRecord{
		Acronym: "FX", Name: "FX Exchange", MICCode: "FX1", Polity: "X", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatal(err)
	}
	listing := models.MarketListingRecord{
		Ticker: "EUR/USD", Name: "Euro Dollar", Type: "forex",
		ExchangeID: exch.ID, Price: 1.1, Ask: 1.11, Bid: 1.09, Volume: 1000,
	}
	if err := db.Create(&listing).Error; err != nil {
		t.Fatal(err)
	}
	holding := models.PortfolioHoldingRecord{
		UserID: 0, UserType: "bank", AssetID: listing.ID, Quantity: 10, AvgBuyPrice: 1.1, AccountID: 7,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupPortfolioHandler(t, db)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/portfolio/holdings/%d/public", holding.ID), strings.NewReader(`{"publicQuantity":4}`))
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPortfolioHTTP_Routes_UnknownPathPrefix(t *testing.T) {
	db := newTestDB(t, "h_pf_unknown")
	h := setupPortfolioHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/foo", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.PortfolioRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- order handler HTTP ---

func setupOrderHandler(t *testing.T, db *gorm.DB) *OrderHTTPHandler {
	cfg := &config.Config{JWTSecret: testJWTSecret}
	osvc := service.NewOrderService(
		repository.NewOrderRepository(db),
		repository.NewMarketRepository(db),
		&fakeRates{},
	)
	return NewOrderHTTPHandler(cfg, osvc, nil)
}

func TestOrderHTTP_OrdersCollection_GetEmpty(t *testing.T) {
	db := newTestDB(t, "h_orders_get_empty")
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.OrdersCollection(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_OrdersCollection_GetSupervisorAll(t *testing.T) {
	db := newTestDB(t, "h_orders_get_super")
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.OrdersCollection(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_OrdersCollection_BadJSONOnPost(t *testing.T) {
	db := newTestDB(t, "h_orders_bad_json")
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.OrdersCollection(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing body, got %d", rec.Code)
	}
}

func TestOrderHTTP_OrderRoutes_BadID(t *testing.T) {
	db := newTestDB(t, "h_orders_bad_id")
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/abc", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestOrderHTTP_OrderRoutes_NotFound(t *testing.T) {
	db := newTestDB(t, "h_orders_nf")
	h := setupOrderHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/9999", nil)
	req.Header.Set("Authorization", "Bearer "+bankToken(t))
	rec := httptest.NewRecorder()
	h.OrderRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- pure response builders ---

func TestExchangeToResponse(t *testing.T) {
	r := exchangeToResponse(models.Exchange{ID: 1, Name: "X", Acronym: "X", Currency: "USD", Enabled: true})
	if r.Currency != "USD" || !r.Enabled {
		t.Errorf("got %+v", r)
	}
}

func TestListingToResponse(t *testing.T) {
	r := listingToResponse(models.Listing{Ticker: "AAPL", Price: 100, Ask: 101, Bid: 99, Type: models.ListingTypeStock})
	if r.Ticker != "AAPL" || r.Type != "stock" {
		t.Errorf("got %+v", r)
	}
}

func TestHistoryToResponse(t *testing.T) {
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := historyToResponse(models.ListingDailyPriceInfo{Date: d, Price: 100})
	if r.Date != "2026-01-01" || r.Price != 100 {
		t.Errorf("got %+v", r)
	}
}

func TestPortfolioToResponse(t *testing.T) {
	p := models.Portfolio{
		OwnerID: 5, OwnerType: models.PortfolioOwnerTypeClient,
		EstimatedValue: 1000, PositionCount: 1, ReadOnly: true,
		Items: []models.PortfolioItem{{Ticker: "AAPL", Quantity: 10}},
	}
	r := portfolioToResponse(p)
	if r.OwnerID != "5" || r.OwnerType != "client" || len(r.Items) != 1 {
		t.Errorf("got %+v", r)
	}
}

func TestExchangeSummaryToResponse(t *testing.T) {
	r := exchangeSummaryToResponse(models.ExchangeSummary{Name: "N", Acronym: "A", Currency: "USD"})
	if r.Currency != "USD" {
		t.Errorf("got %+v", r)
	}
}

// --- pure math ---

func TestNormalCDF_Centered(t *testing.T) {
	if v := normalCDF(0); math.Abs(v-0.5) > 1e-9 {
		t.Errorf("normalCDF(0)=%v, want 0.5", v)
	}
}

func TestBlackScholesTheta_ExpiredReturnsZero(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	if v := blackScholesTheta(100, 100, 0.2, past, "call"); v != 0 {
		t.Errorf("expected 0 for expired, got %v", v)
	}
}

func TestBlackScholesTheta_InvalidInputs(t *testing.T) {
	future := time.Now().Add(30 * 24 * time.Hour)
	if v := blackScholesTheta(0, 100, 0.2, future, "call"); v != 0 {
		t.Errorf("expected 0 for stockPrice=0, got %v", v)
	}
	if v := blackScholesTheta(100, 0, 0.2, future, "call"); v != 0 {
		t.Errorf("expected 0 for strike=0, got %v", v)
	}
	if v := blackScholesTheta(100, 100, 0, future, "call"); v != 0 {
		t.Errorf("expected 0 for vol=0, got %v", v)
	}
}

func TestBlackScholesTheta_CallAndPut(t *testing.T) {
	future := time.Now().Add(30 * 24 * time.Hour)
	c := blackScholesTheta(100, 100, 0.2, future, "call")
	p := blackScholesTheta(100, 100, 0.2, future, "put")
	if c >= 0 {
		t.Errorf("call theta should be negative, got %v", c)
	}
	if p == c {
		t.Errorf("put and call theta should differ")
	}
}
