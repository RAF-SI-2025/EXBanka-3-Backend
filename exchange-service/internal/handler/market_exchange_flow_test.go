package handler

import (
	"net/http"
	"testing"
)

// TestMarketHTTP_ListingSuffixesAndExchange covers the history/options suffix
// branches of ListingRoutes plus getExchangeStatus and toggleExchangeTime.
func TestMarketHTTP_ListingSuffixesAndExchange(t *testing.T) {
	db := newTestDB(t, "h_market_exchange_flow")
	seedExchangeAndListing(t, db, "MEX") // exchange acronym "X", stock "MEX"
	h := setupMarketHandler(t, db)
	tok := clientToken(t)

	// Stock detail (stock subtype attach).
	if rec := do(t, h.ListingRoutes, http.MethodGet, "/api/v1/listings/MEX", tok, ""); rec.Code != http.StatusOK {
		t.Errorf("listing detail: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// History suffix.
	if rec := do(t, h.ListingRoutes, http.MethodGet, "/api/v1/listings/MEX/history", tok, ""); rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Errorf("history: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Options suffix.
	if rec := do(t, h.ListingRoutes, http.MethodGet, "/api/v1/listings/MEX/options", tok, ""); rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Errorf("options: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Exchange status (acronym "X" from the seed helper).
	if rec := do(t, h.ExchangeRoutes, http.MethodGet, "/api/v1/exchanges/X/status", tok, ""); rec.Code != http.StatusOK {
		t.Errorf("exchange status: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Unknown exchange -> 404.
	if rec := do(t, h.ExchangeRoutes, http.MethodGet, "/api/v1/exchanges/NOPE/status", tok, ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown exchange: want 404, got %d", rec.Code)
	}

	// Toggle as supervisor.
	if rec := do(t, h.ExchangeRoutes, http.MethodPost, "/api/v1/exchanges/X/toggle", supervisorToken(t), `{"useManualTime":true,"manualTimeOpen":true}`); rec.Code != http.StatusOK {
		t.Errorf("toggle: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Toggle as client -> 403.
	if rec := do(t, h.ExchangeRoutes, http.MethodPost, "/api/v1/exchanges/X/toggle", tok, `{"useManualTime":true}`); rec.Code != http.StatusForbidden {
		t.Errorf("toggle as client: want 403, got %d", rec.Code)
	}
	// Toggle bad body -> 400.
	if rec := do(t, h.ExchangeRoutes, http.MethodPost, "/api/v1/exchanges/X/toggle", supervisorToken(t), `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("toggle bad body: want 400, got %d", rec.Code)
	}
}
