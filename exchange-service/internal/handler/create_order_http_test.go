package handler

import (
	"net/http"
	"testing"

	"gorm.io/gorm"
)

func setupOrderHandlerWithAccounts(t *testing.T, name string) (*OrderHTTPHandler, *gorm.DB) {
	t.Helper()
	db := newTestDB(t, name)
	seedOtcHandlerAccounts(t, db)                  // accounts 1 (client 200), 2 (client 100), USD
	seedExchangeAndListing(t, db, "HORD")          // USD stock "HORD" @ 100
	return setupOrderHandler(t, db), db
}

func TestOrderHTTP_CreateOrder_ClientMarketBuy(t *testing.T) {
	h, _ := setupOrderHandlerWithAccounts(t, "h_create_market")
	body := `{"assetTicker":"HORD","orderType":"market","direction":"buy","quantity":2,"contractSize":1,"accountId":2}`
	rec := do(t, h.OrdersCollection, http.MethodPost, "/api/v1/orders", clientToken(t), body)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_CreateOrder_LimitAutoDetect(t *testing.T) {
	h, _ := setupOrderHandlerWithAccounts(t, "h_create_limit")
	// orderType "market" + a limit value -> auto-detected as a limit order.
	body := `{"assetTicker":"HORD","orderType":"market","direction":"buy","quantity":1,"contractSize":1,"accountId":2,"limitValue":120}`
	rec := do(t, h.OrdersCollection, http.MethodPost, "/api/v1/orders", clientToken(t), body)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_CreateOrder_StopAutoDetectAndSell(t *testing.T) {
	h, _ := setupOrderHandlerWithAccounts(t, "h_create_stop")
	// Stop value only -> "stop"; sell direction doesn't debit.
	body := `{"assetTicker":"HORD","orderType":"market","direction":"sell","quantity":1,"contractSize":1,"accountId":2,"stopValue":90}`
	rec := do(t, h.OrdersCollection, http.MethodPost, "/api/v1/orders", clientToken(t), body)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_CreateOrder_ValidationError(t *testing.T) {
	h, _ := setupOrderHandlerWithAccounts(t, "h_create_invalid")
	// Unknown asset -> service rejects with 400.
	body := `{"assetTicker":"NOPE","orderType":"market","direction":"buy","quantity":1,"contractSize":1,"accountId":2}`
	rec := do(t, h.OrdersCollection, http.MethodPost, "/api/v1/orders", clientToken(t), body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown asset, got %d", rec.Code)
	}
}
