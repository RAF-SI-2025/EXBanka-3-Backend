package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
)

func clientToken200(t *testing.T) string {
	return makeToken(t, util.Claims{
		ClientID: 200, TokenSource: "client", TokenType: "access",
		Permissions: []string{models.PermClientTrading, models.PermClientBasic},
	})
}

// TestOtcHTTP_OfferFlow drives create -> counter -> accept through the HTTP
// layer, covering createOffer / counterOffer / acceptOffer.
func TestOtcHTTP_OfferFlow(t *testing.T) {
	db := newTestDB(t, "h_otc_flow")
	_, listingID := seedExchangeAndListing(t, db, "HOF")
	now := time.Now().UTC()
	if err := db.Create(&models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: listingID, Quantity: 10,
		PublicQuantity: 6, ReservedQuantity: 0, AvgBuyPrice: 90, AccountID: 1, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed holding: %v", err)
	}
	var holding models.PortfolioHoldingRecord
	db.Where("user_id = 200").First(&holding)

	h := setupOtcHandler(t, db) // seeds accounts 1 (client 200), 2 (client 100)
	buyer := clientToken(t)     // client 100
	seller := clientToken200(t) // client 200

	// Buyer creates an offer.
	createBody := fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":3,"pricePerStock":100,"settlementDate":"2026-12-31","premium":5}`, holding.ID)
	rec := do(t, h.OtcRoutes, http.MethodPost, "/api/v1/otc/offers", buyer, createBody)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create offer status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Offer struct {
			ID uint `json:"id"`
		} `json:"offer"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	offerID := created.Offer.ID
	path := fmt.Sprintf("/api/v1/otc/offers/%d", offerID)

	// Seller counters.
	if rec := do(t, h.OtcRoutes, http.MethodPost, path+"/counter", seller, `{"amount":3,"pricePerStock":105,"settlementDate":"2026-12-31","premium":6}`); rec.Code != http.StatusOK {
		t.Fatalf("counter status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Get the offer + listing (read paths).
	if rec := do(t, h.OtcRoutes, http.MethodGet, path, buyer, ""); rec.Code != http.StatusOK {
		t.Errorf("get offer status=%d", rec.Code)
	}
	// Buyer accepts (not the last modifier -> allowed).
	if rec := do(t, h.OtcRoutes, http.MethodPost, path+"/accept", buyer, ""); rec.Code != http.StatusOK {
		t.Fatalf("accept status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestOtcHTTP_ErrorBranches covers createOffer's bad-settlement (parseSettlementDate
// error) and updateOfferStatus on a missing offer.
func TestOtcHTTP_ErrorBranches(t *testing.T) {
	db := newTestDB(t, "h_otc_errors")
	h := setupOtcHandler(t, db)
	tok := clientToken(t)

	// Bad settlement date format -> 400.
	if rec := do(t, h.OtcRoutes, http.MethodPost, "/api/v1/otc/offers", tok,
		`{"sellerHoldingId":1,"buyerAccountId":2,"amount":3,"pricePerStock":100,"settlementDate":"garbage","premium":5}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad settlement: expected 400, got %d", rec.Code)
	}
	// Decline / counter a non-existent offer -> 404.
	if rec := do(t, h.OtcRoutes, http.MethodPost, "/api/v1/otc/offers/99999/decline", tok, ""); rec.Code != http.StatusNotFound {
		t.Errorf("decline missing: expected 404, got %d", rec.Code)
	}
	if rec := do(t, h.OtcRoutes, http.MethodPost, "/api/v1/otc/offers/99999/counter", tok,
		`{"amount":1,"pricePerStock":1,"settlementDate":"2026-12-31","premium":0}`); rec.Code != http.StatusNotFound {
		t.Errorf("counter missing: expected 404, got %d", rec.Code)
	}
	// Bad offer id -> 400.
	if rec := do(t, h.OtcRoutes, http.MethodGet, "/api/v1/otc/offers/abc", tok, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad offer id: expected 400, got %d", rec.Code)
	}
}

// TestOtcHTTP_DeclineAndCancel covers the updateOfferStatus decline/cancel paths.
func TestOtcHTTP_DeclineAndCancel(t *testing.T) {
	db := newTestDB(t, "h_otc_decline")
	_, listingID := seedExchangeAndListing(t, db, "HOD")
	now := time.Now().UTC()
	db.Create(&models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: listingID, Quantity: 10,
		PublicQuantity: 8, AvgBuyPrice: 90, AccountID: 1, CreatedAt: now,
	})
	var holding models.PortfolioHoldingRecord
	db.Where("user_id = 200").First(&holding)
	h := setupOtcHandler(t, db)
	buyer, seller := clientToken(t), clientToken200(t)
	createBody := fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":2,"pricePerStock":100,"settlementDate":"2026-12-31","premium":3}`, holding.ID)

	// Offer 1: seller declines.
	rec := do(t, h.OtcRoutes, http.MethodPost, "/api/v1/otc/offers", buyer, createBody)
	var c1 struct{ Offer struct{ ID uint `json:"id"` } `json:"offer"` }
	_ = json.Unmarshal(rec.Body.Bytes(), &c1)
	if rec := do(t, h.OtcRoutes, http.MethodPost, fmt.Sprintf("/api/v1/otc/offers/%d/decline", c1.Offer.ID), seller, ""); rec.Code != http.StatusOK {
		t.Errorf("decline status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Offer 2: buyer cancels.
	rec2 := do(t, h.OtcRoutes, http.MethodPost, "/api/v1/otc/offers", buyer, createBody)
	var c2 struct{ Offer struct{ ID uint `json:"id"` } `json:"offer"` }
	_ = json.Unmarshal(rec2.Body.Bytes(), &c2)
	if rec := do(t, h.OtcRoutes, http.MethodPost, fmt.Sprintf("/api/v1/otc/offers/%d/cancel", c2.Offer.ID), buyer, ""); rec.Code != http.StatusOK {
		t.Errorf("cancel status=%d body=%s", rec.Code, rec.Body.String())
	}
}
