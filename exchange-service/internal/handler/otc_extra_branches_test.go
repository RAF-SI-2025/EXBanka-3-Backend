package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// TestOtcHTTP_SagaStatusBranches covers getSagaStatus 404 (wired querier, missing
// saga) and 500 (querier not wired).
func TestOtcHTTP_SagaStatusBranches(t *testing.T) {
	db := newTestDB(t, "h_otc_saga_branches")
	cfg := &config.Config{JWTSecret: testJWTSecret}
	otcSvc := service.NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db))

	// Querier wired -> missing saga returns 404.
	wired := NewOtcHTTPHandler(cfg, otcSvc).WithSagaQuerier(repository.NewSagaRepository(db))
	if rec := do(t, wired.OtcRoutes, http.MethodGet, "/api/v1/otc/saga/99999", clientToken(t), ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing saga: want 404, got %d", rec.Code)
	}

	// No querier wired -> 500.
	bare := NewOtcHTTPHandler(cfg, otcSvc)
	if rec := do(t, bare.OtcRoutes, http.MethodGet, "/api/v1/otc/saga/1", clientToken(t), ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("no querier: want 500, got %d", rec.Code)
	}
}

// TestOtcHTTP_CounterAndHistoryBranches covers counterOffer's bad-body and
// bad-settlement 400s plus negotiationHistory (success + missing).
func TestOtcHTTP_CounterAndHistoryBranches(t *testing.T) {
	db := newTestDB(t, "h_otc_counter_hist")
	_, listingID := seedExchangeAndListing(t, db, "HCH")
	now := time.Now().UTC()
	db.Create(&models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: listingID, Quantity: 10,
		PublicQuantity: 8, AvgBuyPrice: 90, AccountID: 1, CreatedAt: now,
	})
	var holding models.PortfolioHoldingRecord
	db.Where("user_id = 200").First(&holding)

	h := setupOtcHandler(t, db)
	buyer := clientToken(t)

	// Counter with malformed JSON -> 400.
	if rec := do(t, h.OtcRoutes, http.MethodPost, "/api/v1/otc/offers/1/counter", buyer, `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("counter bad body: want 400, got %d", rec.Code)
	}
	// Counter with bad settlement date -> 400.
	if rec := do(t, h.OtcRoutes, http.MethodPost, "/api/v1/otc/offers/1/counter", buyer,
		`{"amount":1,"pricePerStock":1,"settlementDate":"nope","premium":0}`); rec.Code != http.StatusBadRequest {
		t.Errorf("counter bad settlement: want 400, got %d", rec.Code)
	}

	// Create an offer, then read its negotiation history.
	createBody := fmt.Sprintf(`{"sellerHoldingId":%d,"buyerAccountId":2,"amount":2,"pricePerStock":100,"settlementDate":"2026-12-31","premium":3}`, holding.ID)
	rec := do(t, h.OtcRoutes, http.MethodPost, "/api/v1/otc/offers", buyer, createBody)
	var created struct {
		Offer struct {
			ID uint `json:"id"`
		} `json:"offer"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	if rec := do(t, h.OtcRoutes, http.MethodGet, fmt.Sprintf("/api/v1/otc/offers/%d/history", created.Offer.ID), buyer, ""); rec.Code != http.StatusOK {
		t.Errorf("negotiation history: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// History for a missing offer -> 404.
	if rec := do(t, h.OtcRoutes, http.MethodGet, "/api/v1/otc/offers/99999/history", buyer, ""); rec.Code != http.StatusNotFound {
		t.Errorf("history missing: want 404, got %d", rec.Code)
	}
}
