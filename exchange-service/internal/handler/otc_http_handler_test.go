package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
	"gorm.io/gorm"
)

func setupOtcHandler(t *testing.T, db *gorm.DB) *OtcHTTPHandler {
	t.Helper()
	seedOtcHandlerAccounts(t, db)
	cfg := &config.Config{JWTSecret: testJWTSecret}
	otcSvc := service.NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db))
	return NewOtcHTTPHandler(cfg, otcSvc)
}

func seedOtcHandlerAccounts(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS currencies (id integer primary key, kod text not null unique)`).Error; err != nil {
		t.Fatalf("create currencies table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS accounts (
		id integer primary key,
		client_id integer,
		firma_id integer,
		zaposleni_id integer,
		currency_id integer not null,
		stanje real not null default 0,
		raspolozivo_stanje real not null default 0,
		dnevna_potrosnja real not null default 0,
		mesecna_potrosnja real not null default 0,
		status text not null default 'aktivan'
	)`).Error; err != nil {
		t.Fatalf("create accounts table: %v", err)
	}
	if err := db.Exec(`INSERT OR IGNORE INTO currencies (id, kod) VALUES (1, 'USD')`).Error; err != nil {
		t.Fatalf("seed currency: %v", err)
	}
	if err := db.Exec(`INSERT OR IGNORE INTO accounts (id, client_id, currency_id, stanje, raspolozivo_stanje, status) VALUES
		(1, 200, 1, 1000, 1000, 'aktivan'),
		(2, 100, 1, 1000, 1000, 'aktivan')`).Error; err != nil {
		t.Fatalf("seed accounts: %v", err)
	}
}

func clientWithoutTradingToken(t *testing.T) string {
	return makeToken(t, util.Claims{
		ClientID: 100, TokenSource: "client", TokenType: "access",
		Permissions: []string{models.PermClientBasic},
	})
}

func clientTradingToken(t *testing.T, clientID uint) string {
	return makeToken(t, util.Claims{
		ClientID: clientID, TokenSource: "client", TokenType: "access",
		Permissions: []string{models.PermClientTrading, models.PermClientBasic},
	})
}

func TestOtcHTTP_PublicStocks_AuthorizedClient(t *testing.T) {
	db := newTestDB(t, "h_otc_public_stocks")
	_, assetID := seedExchangeAndListing(t, db, "OTC")

	sellerHolding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 5, ReservedQuantity: 2, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	ownHolding := models.PortfolioHoldingRecord{
		UserID: 100, UserType: "client", AssetID: assetID, Quantity: 4,
		PublicQuantity: 4, AvgBuyPrice: 95, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&[]models.PortfolioHoldingRecord{sellerHolding, ownHolding}).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/public-stocks", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()

	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		Count  int `json:"count"`
		Stocks []struct {
			Ticker            string  `json:"ticker"`
			AvailableQuantity float64 `json:"availableQuantity"`
			SellerID          uint    `json:"sellerId"`
		} `json:"stocks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 1 || len(body.Stocks) != 1 {
		t.Fatalf("expected one public stock, got %+v", body)
	}
	if body.Stocks[0].Ticker != "OTC" || body.Stocks[0].AvailableQuantity != 3 || body.Stocks[0].SellerID != 200 {
		t.Fatalf("unexpected public stock response: %+v", body.Stocks[0])
	}
}

func TestOtcHTTP_PublicStocks_RequiresTradingPermission(t *testing.T) {
	db := newTestDB(t, "h_otc_public_forbidden")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/public-stocks", nil)
	req.Header.Set("Authorization", "Bearer "+clientWithoutTradingToken(t))
	rec := httptest.NewRecorder()

	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOtcHTTP_OffersRequireTradingPermission(t *testing.T) {
	db := newTestDB(t, "h_otc_offers_forbidden")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/offers", nil)
	req.Header.Set("Authorization", "Bearer "+clientWithoutTradingToken(t))
	rec := httptest.NewRecorder()

	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOtcHTTP_PublicStocks_WrongMethod(t *testing.T) {
	db := newTestDB(t, "h_otc_public_method")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/otc/public-stocks", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()

	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if body := rec.Body.String(); body == "" {
		t.Fatal("expected non-empty method-not-allowed response")
	}
}

func TestOtcHTTP_CreateOffer(t *testing.T) {
	db := newTestDB(t, "h_otc_create_offer")
	_, assetID := seedExchangeAndListing(t, db, "OFR")

	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 6, ReservedQuantity: 1, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/otc/offers",
		strings.NewReader(fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":3,"pricePerStock":105,"settlementDate":"2026-12-31","premium":25}`, holding.ID)),
	)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()

	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		Offer struct {
			ID             uint    `json:"id"`
			Ticker         string  `json:"ticker"`
			Amount         float64 `json:"amount"`
			PricePerStock  float64 `json:"pricePerStock"`
			CurrentPrice   float64 `json:"currentPrice"`
			DeviationPct   float64 `json:"deviationPct"`
			Status         string  `json:"status"`
			BuyerID        uint    `json:"buyerId"`
			SellerID       uint    `json:"sellerId"`
			SettlementDate string  `json:"settlementDate"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Offer.ID == 0 || body.Offer.Ticker != "OFR" || body.Offer.Status != models.OtcOfferStatusPending {
		t.Fatalf("unexpected offer response: %+v", body.Offer)
	}
	if body.Offer.Amount != 3 || body.Offer.PricePerStock != 105 || body.Offer.BuyerID != 100 || body.Offer.SellerID != 200 {
		t.Fatalf("unexpected offer fields: %+v", body.Offer)
	}
	if body.Offer.CurrentPrice != 100 || body.Offer.DeviationPct != 5 {
		t.Fatalf("unexpected offer market fields: %+v", body.Offer)
	}
}

func TestOtcHTTP_CreateOffer_RejectsExcessAmount(t *testing.T) {
	db := newTestDB(t, "h_otc_create_offer_excess")
	_, assetID := seedExchangeAndListing(t, db, "EXC")

	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 4, ReservedQuantity: 1, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/otc/offers",
		strings.NewReader(fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":4,"pricePerStock":105,"settlementDate":"2026-12-31","premium":25}`, holding.ID)),
	)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()

	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOtcHTTP_ListAndGetOffers(t *testing.T) {
	db := newTestDB(t, "h_otc_list_get_offers")
	_, assetID := seedExchangeAndListing(t, db, "LGO")

	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 6, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/otc/offers",
		strings.NewReader(fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":3,"pricePerStock":105,"settlementDate":"2026-12-31","premium":25}`, holding.ID)),
	)
	createReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	createRec := httptest.NewRecorder()
	h.OtcRoutes(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		Offer struct {
			ID uint `json:"id"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/otc/offers?status=pending", nil)
	listReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	listRec := httptest.NewRecorder()
	h.OtcRoutes(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var listed struct {
		Count  int `json:"count"`
		Offers []struct {
			ID     uint   `json:"id"`
			Ticker string `json:"ticker"`
			Status string `json:"status"`
		} `json:"offers"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Count != 1 || listed.Offers[0].ID != created.Offer.ID || listed.Offers[0].Ticker != "LGO" {
		t.Fatalf("unexpected offers list: %+v", listed)
	}

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/otc/offers/%d", created.Offer.ID), nil)
	getReq.Header.Set("Authorization", "Bearer "+clientTradingToken(t, 200))
	getRec := httptest.NewRecorder()
	h.OtcRoutes(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRec.Code, getRec.Body.String())
	}
}

func TestOtcHTTP_GetOffer_HidesFromUnrelatedParticipant(t *testing.T) {
	db := newTestDB(t, "h_otc_get_offer_unrelated")
	_, assetID := seedExchangeAndListing(t, db, "HID")

	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 6, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/otc/offers",
		strings.NewReader(fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":3,"pricePerStock":105,"settlementDate":"2026-12-31","premium":25}`, holding.ID)),
	)
	createReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	createRec := httptest.NewRecorder()
	h.OtcRoutes(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Offer struct {
			ID uint `json:"id"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/otc/offers/%d", created.Offer.ID), nil)
	getReq.Header.Set("Authorization", "Bearer "+clientTradingToken(t, 999))
	getRec := httptest.NewRecorder()
	h.OtcRoutes(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unrelated participant, got %d body=%s", getRec.Code, getRec.Body.String())
	}
}

func TestOtcHTTP_CounterOffer(t *testing.T) {
	db := newTestDB(t, "h_otc_counter_offer")
	_, assetID := seedExchangeAndListing(t, db, "CTR")

	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 6, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/otc/offers",
		strings.NewReader(fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":3,"pricePerStock":105,"settlementDate":"2026-12-31","premium":25}`, holding.ID)),
	)
	createReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	createRec := httptest.NewRecorder()
	h.OtcRoutes(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Offer struct {
			ID uint `json:"id"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	counterReq := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/api/v1/otc/offers/%d/counter", created.Offer.ID),
		strings.NewReader(`{"amount":4,"pricePerStock":108,"settlementDate":"2027-01-31","premium":30}`),
	)
	counterReq.Header.Set("Authorization", "Bearer "+clientTradingToken(t, 200))
	counterRec := httptest.NewRecorder()
	h.OtcRoutes(counterRec, counterReq)
	if counterRec.Code != http.StatusOK {
		t.Fatalf("counter status=%d body=%s", counterRec.Code, counterRec.Body.String())
	}

	var body struct {
		Offer struct {
			ID             uint    `json:"id"`
			Amount         float64 `json:"amount"`
			PricePerStock  float64 `json:"pricePerStock"`
			CurrentPrice   float64 `json:"currentPrice"`
			DeviationPct   float64 `json:"deviationPct"`
			Premium        float64 `json:"premium"`
			ModifiedByID   uint    `json:"modifiedById"`
			ModifiedByType string  `json:"modifiedByType"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(counterRec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Offer.ID != created.Offer.ID || body.Offer.Amount != 4 || body.Offer.PricePerStock != 108 || body.Offer.Premium != 30 {
		t.Fatalf("unexpected counter response: %+v", body.Offer)
	}
	if body.Offer.CurrentPrice != 100 || body.Offer.DeviationPct != 8 {
		t.Fatalf("unexpected counter market fields: %+v", body.Offer)
	}
	if body.Offer.ModifiedByID != 200 || body.Offer.ModifiedByType != "client" {
		t.Fatalf("modifier not updated: %+v", body.Offer)
	}
}

func TestOtcHTTP_CounterOffer_HidesFromUnrelatedParticipant(t *testing.T) {
	db := newTestDB(t, "h_otc_counter_unrelated")
	_, assetID := seedExchangeAndListing(t, db, "CUR")

	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 6, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/otc/offers",
		strings.NewReader(fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":3,"pricePerStock":105,"settlementDate":"2026-12-31","premium":25}`, holding.ID)),
	)
	createReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	createRec := httptest.NewRecorder()
	h.OtcRoutes(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Offer struct {
			ID uint `json:"id"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	counterReq := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/api/v1/otc/offers/%d/counter", created.Offer.ID),
		strings.NewReader(`{"amount":4,"pricePerStock":108,"settlementDate":"2027-01-31","premium":30}`),
	)
	counterReq.Header.Set("Authorization", "Bearer "+clientTradingToken(t, 999))
	counterRec := httptest.NewRecorder()
	h.OtcRoutes(counterRec, counterReq)
	if counterRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got status=%d body=%s", counterRec.Code, counterRec.Body.String())
	}
}

func TestOtcHTTP_DeclineOffer(t *testing.T) {
	db := newTestDB(t, "h_otc_decline_offer")
	_, assetID := seedExchangeAndListing(t, db, "DCL")

	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 6, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/otc/offers",
		strings.NewReader(fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":3,"pricePerStock":105,"settlementDate":"2026-12-31","premium":25}`, holding.ID)),
	)
	createReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	createRec := httptest.NewRecorder()
	h.OtcRoutes(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Offer struct {
			ID uint `json:"id"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	declineReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/otc/offers/%d/decline", created.Offer.ID), nil)
	declineReq.Header.Set("Authorization", "Bearer "+clientTradingToken(t, 200))
	declineRec := httptest.NewRecorder()
	h.OtcRoutes(declineRec, declineReq)
	if declineRec.Code != http.StatusOK {
		t.Fatalf("decline status=%d body=%s", declineRec.Code, declineRec.Body.String())
	}
	var body struct {
		Offer struct {
			Status       string `json:"status"`
			ModifiedByID uint   `json:"modifiedById"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(declineRec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Offer.Status != models.OtcOfferStatusDeclined || body.Offer.ModifiedByID != 200 {
		t.Fatalf("unexpected decline response: %+v", body.Offer)
	}
}

func TestOtcHTTP_CancelOffer(t *testing.T) {
	db := newTestDB(t, "h_otc_cancel_offer")
	_, assetID := seedExchangeAndListing(t, db, "CNL")

	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 6, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/otc/offers",
		strings.NewReader(fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":3,"pricePerStock":105,"settlementDate":"2026-12-31","premium":25}`, holding.ID)),
	)
	createReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	createRec := httptest.NewRecorder()
	h.OtcRoutes(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Offer struct {
			ID uint `json:"id"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	cancelReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/otc/offers/%d/cancel", created.Offer.ID), nil)
	cancelReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	cancelRec := httptest.NewRecorder()
	h.OtcRoutes(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", cancelRec.Code, cancelRec.Body.String())
	}
	var body struct {
		Offer struct {
			Status       string `json:"status"`
			ModifiedByID uint   `json:"modifiedById"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(cancelRec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Offer.Status != models.OtcOfferStatusCancelled || body.Offer.ModifiedByID != 100 {
		t.Fatalf("unexpected cancel response: %+v", body.Offer)
	}
}

func TestOtcHTTP_AcceptOffer(t *testing.T) {
	db := newTestDB(t, "h_otc_accept_offer")
	_, assetID := seedExchangeAndListing(t, db, "ACP")

	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 6, ReservedQuantity: 1, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/otc/offers",
		strings.NewReader(fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":3,"pricePerStock":105,"settlementDate":"2026-12-31","premium":25}`, holding.ID)),
	)
	createReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	createRec := httptest.NewRecorder()
	h.OtcRoutes(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Offer struct {
			ID uint `json:"id"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/otc/offers/%d/accept", created.Offer.ID), nil)
	acceptReq.Header.Set("Authorization", "Bearer "+clientTradingToken(t, 200))
	acceptRec := httptest.NewRecorder()
	h.OtcRoutes(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("accept status=%d body=%s", acceptRec.Code, acceptRec.Body.String())
	}

	var body struct {
		Contract struct {
			ID           uint    `json:"id"`
			OfferID      uint    `json:"offerId"`
			Ticker       string  `json:"ticker"`
			Amount       float64 `json:"amount"`
			StrikePrice  float64 `json:"strikePrice"`
			CurrentPrice float64 `json:"currentPrice"`
			Premium      float64 `json:"premium"`
			Profit       float64 `json:"profit"`
			Status       string  `json:"status"`
			BuyerID      uint    `json:"buyerId"`
			SellerID     uint    `json:"sellerId"`
		} `json:"contract"`
	}
	if err := json.Unmarshal(acceptRec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Contract.ID == 0 || body.Contract.OfferID != created.Offer.ID || body.Contract.Status != models.OtcContractStatusValid {
		t.Fatalf("unexpected contract response: %+v", body.Contract)
	}
	if body.Contract.Ticker != "ACP" || body.Contract.Amount != 3 || body.Contract.StrikePrice != 105 || body.Contract.Premium != 25 {
		t.Fatalf("unexpected contract terms: %+v", body.Contract)
	}
	if body.Contract.CurrentPrice != 100 || body.Contract.Profit != -40 {
		t.Fatalf("unexpected contract market/profit fields: %+v", body.Contract)
	}

	var updatedHolding models.PortfolioHoldingRecord
	if err := db.First(&updatedHolding, holding.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedHolding.ReservedQuantity != 4 {
		t.Fatalf("expected reserved quantity 4, got %.2f", updatedHolding.ReservedQuantity)
	}
}

func TestOtcHTTP_ListContracts(t *testing.T) {
	db := newTestDB(t, "h_otc_list_contracts")
	_, assetID := seedExchangeAndListing(t, db, "CON")

	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 6, ReservedQuantity: 1, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/otc/offers",
		strings.NewReader(fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":3,"pricePerStock":105,"settlementDate":"2026-12-31","premium":25}`, holding.ID)),
	)
	createReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	createRec := httptest.NewRecorder()
	h.OtcRoutes(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Offer struct {
			ID uint `json:"id"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/otc/offers/%d/accept", created.Offer.ID), nil)
	acceptReq.Header.Set("Authorization", "Bearer "+clientTradingToken(t, 200))
	acceptRec := httptest.NewRecorder()
	h.OtcRoutes(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("accept status=%d body=%s", acceptRec.Code, acceptRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/otc/contracts?status=valid", nil)
	listReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	listRec := httptest.NewRecorder()
	h.OtcRoutes(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var listed struct {
		Count     int    `json:"count"`
		Status    string `json:"status"`
		Contracts []struct {
			ID           uint    `json:"id"`
			Ticker       string  `json:"ticker"`
			Amount       float64 `json:"amount"`
			StrikePrice  float64 `json:"strikePrice"`
			CurrentPrice float64 `json:"currentPrice"`
			Profit       float64 `json:"profit"`
			Status       string  `json:"status"`
			BuyerID      uint    `json:"buyerId"`
			SellerID     uint    `json:"sellerId"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Count != 1 || listed.Status != models.OtcContractStatusValid || len(listed.Contracts) != 1 {
		t.Fatalf("unexpected contracts list: %+v", listed)
	}
	if listed.Contracts[0].Ticker != "CON" || listed.Contracts[0].Amount != 3 || listed.Contracts[0].StrikePrice != 105 {
		t.Fatalf("unexpected contract response: %+v", listed.Contracts[0])
	}
	if listed.Contracts[0].CurrentPrice != 100 || listed.Contracts[0].Profit != -40 {
		t.Fatalf("unexpected contract market/profit fields: %+v", listed.Contracts[0])
	}
	if listed.Contracts[0].BuyerID != 100 || listed.Contracts[0].SellerID != 200 || listed.Contracts[0].Status != models.OtcContractStatusValid {
		t.Fatalf("unexpected participant/status fields: %+v", listed.Contracts[0])
	}
}

func TestOtcHTTP_ListContracts_RejectsInvalidStatus(t *testing.T) {
	db := newTestDB(t, "h_otc_list_contracts_bad_status")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/contracts?status=pending", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()

	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOtcHTTP_ListContracts_RequiresTradingPermission(t *testing.T) {
	db := newTestDB(t, "h_otc_list_contracts_forbidden")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/contracts", nil)
	req.Header.Set("Authorization", "Bearer "+clientWithoutTradingToken(t))
	rec := httptest.NewRecorder()

	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOtcHTTP_AcceptOffer_RejectsOwnCounter(t *testing.T) {
	db := newTestDB(t, "h_otc_accept_own_counter")
	_, assetID := seedExchangeAndListing(t, db, "ABY")

	holding := models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: assetID, Quantity: 10,
		PublicQuantity: 6, AvgBuyPrice: 90, AccountID: 1,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	h := setupOtcHandler(t, db)
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/otc/offers",
		strings.NewReader(fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":3,"pricePerStock":105,"settlementDate":"2026-12-31","premium":25}`, holding.ID)),
	)
	createReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	createRec := httptest.NewRecorder()
	h.OtcRoutes(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Offer struct {
			ID uint `json:"id"`
		} `json:"offer"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/otc/offers/%d/accept", created.Offer.ID), nil)
	acceptReq.Header.Set("Authorization", "Bearer "+clientToken(t))
	acceptRec := httptest.NewRecorder()
	h.OtcRoutes(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusBadRequest {
		t.Fatalf("expected own-counter accept to fail with 400, got status=%d body=%s", acceptRec.Code, acceptRec.Body.String())
	}
}
