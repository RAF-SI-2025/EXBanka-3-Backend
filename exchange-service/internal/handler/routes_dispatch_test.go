package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// =====================================================================
// OtcRoutes — dispatch coverage
// =====================================================================

func TestOtcRoutes_EmptyPath_404(t *testing.T) {
	db := newTestDB(t, "otc_routes_empty")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestOtcRoutes_PublicStocks_MethodNotAllowed(t *testing.T) {
	db := newTestDB(t, "otc_routes_ps_mna")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/otc/public-stocks", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestOtcRoutes_Offers_MethodNotAllowed(t *testing.T) {
	db := newTestDB(t, "otc_routes_offers_mna")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/otc/offers", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestOtcRoutes_Contracts_BadID(t *testing.T) {
	db := newTestDB(t, "otc_routes_contracts_badid")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/contracts/abc", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestOtcRoutes_Contracts_MethodNotAllowed(t *testing.T) {
	db := newTestDB(t, "otc_routes_contracts_mna")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/otc/contracts/1", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestOtcRoutes_Contracts_List(t *testing.T) {
	db := newTestDB(t, "otc_routes_contracts_list")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/contracts", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOtcRoutes_Saga_BadID(t *testing.T) {
	db := newTestDB(t, "otc_routes_saga_badid")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/saga/abc", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestOtcRoutes_Saga_MethodNotAllowed(t *testing.T) {
	db := newTestDB(t, "otc_routes_saga_mna")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/otc/saga/1", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestOtcRoutes_OfferByID_BadID(t *testing.T) {
	db := newTestDB(t, "otc_routes_offer_byid_bad")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/offers/abc", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestOtcRoutes_OfferByID_MethodNotAllowed(t *testing.T) {
	db := newTestDB(t, "otc_routes_offer_byid_mna")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/otc/offers/1", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestOtcRoutes_OfferAction_BadID(t *testing.T) {
	db := newTestDB(t, "otc_routes_offer_action_bad")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/otc/offers/abc/counter", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestOtcRoutes_OfferAction_UnknownAction(t *testing.T) {
	db := newTestDB(t, "otc_routes_offer_unknown")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/otc/offers/1/unknown", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestOtcRoutes_OfferAction_MethodNotAllowed(t *testing.T) {
	db := newTestDB(t, "otc_routes_offer_action_mna")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/offers/1/counter", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestOtcRoutes_DefaultNotFound(t *testing.T) {
	db := newTestDB(t, "otc_routes_default")
	h := setupOtcHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/otc/totally-bogus", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.OtcRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// =====================================================================
// MarketHTTPHandler ListingRoutes — dispatch
// =====================================================================

func TestListingRoutes_EmptyPath_NotFound(t *testing.T) {
	db := newTestDB(t, "listing_routes_root")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/listings/", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ListingRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListingRoutes_MethodNotAllowed(t *testing.T) {
	db := newTestDB(t, "listing_routes_mna")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/listings/AAA", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ListingRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestListingRoutes_TickerNotFound(t *testing.T) {
	db := newTestDB(t, "listing_routes_tk_notfound")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/listings/NOTEXIST", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ListingRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestListingRoutes_TickerFound_Stock(t *testing.T) {
	db := newTestDB(t, "listing_routes_stock")
	_, listingID := seedExchangeAndListing(t, db, "ZZZ")
	// Seed the stock record so the stock branch in ListingRoutes executes
	db.Exec(`INSERT INTO stocks (listing_id, outstanding_shares, dividend_yield) VALUES (?, ?, ?)`, listingID, 1000, 0.02)
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/listings/ZZZ", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ListingRoutes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListingRoutes_HistoryMissing(t *testing.T) {
	db := newTestDB(t, "listing_routes_history_missing")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/listings/MISSING/history", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ListingRoutes(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusOK {
		t.Fatalf("expected 404/200, got %d", rec.Code)
	}
}

func TestListingRoutes_OptionsForNonStock_NotFound(t *testing.T) {
	db := newTestDB(t, "listing_routes_opts_notstock")
	h := setupMarketHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/listings/MISSING/options", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.ListingRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// =====================================================================
// InterbankOtcHTTPHandler Routes — extra dispatch branches
// =====================================================================

func TestInterbankOtc_NegotiationsPathMissing(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty, got %d", rec.Code)
	}
}

func TestInterbankOtc_OptionContracts_List(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/option-contracts", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtc_OptionContracts_BadID(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/option-contracts/abc", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestInterbankOtc_OptionContracts_MethodNotAllowed(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/interbank-otc/option-contracts", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestInterbankOtc_Negotiations_MethodNotAllowed(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/interbank-otc/negotiations", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestInterbankOtc_Negotiations_NotFound(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/negotiations/111/missing-id", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtc_Negotiations_BadRouting(t *testing.T) {
	h := setupInterbankOtcHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/interbank-otc/negotiations/notnumber/N-1", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.Routes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// =====================================================================
// FundRoutes additional branches — listFunds inner error path is hard to hit;
// exercise the empty + sub-path method-not-allowed cases.
// =====================================================================

func TestFundRoutes_Performance_MethodNotAllowed(t *testing.T) {
	db := newFundTestDB(t, "fund_perf_mna")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/funds/1/performance", bytes.NewBufferString("{}"))
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestFundRoutes_Holdings_MethodNotAllowed(t *testing.T) {
	db := newFundTestDB(t, "fund_holdings_mna")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/funds/1/holdings", bytes.NewBufferString("{}"))
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestFundRoutes_Invest_MethodNotAllowed(t *testing.T) {
	db := newFundTestDB(t, "fund_invest_mna")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/funds/1/invest", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestFundRoutes_Withdraw_MethodNotAllowed(t *testing.T) {
	db := newFundTestDB(t, "fund_withdraw_mna")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/funds/1/withdraw", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestFundRoutes_ValidateOrder_MethodNotAllowed(t *testing.T) {
	db := newFundTestDB(t, "fund_validate_mna")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/funds/1/validate-order", nil)
	req.Header.Set("Authorization", "Bearer "+supervisorToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestFundRoutes_PositionsMine_MethodNotAllowed(t *testing.T) {
	db := newFundTestDB(t, "fund_pos_mna")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/funds/positions/mine", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestFundRoutes_GetFund_MethodNotAllowed(t *testing.T) {
	db := newFundTestDB(t, "fund_get_mna")
	h, _ := setupFundHandler(t, db)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/funds/1", nil)
	req.Header.Set("Authorization", "Bearer "+clientToken(t))
	rec := httptest.NewRecorder()
	h.FundRoutes(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
