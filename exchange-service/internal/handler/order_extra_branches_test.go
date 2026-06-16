package handler

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

// TestOrderHTTP_GetOrderBranches covers getOrder owner/forbidden/not-found.
func TestOrderHTTP_GetOrderBranches(t *testing.T) {
	h, db := setupOrderHandlerWithAccounts(t, "h_order_get_branches")
	now := time.Now().UTC()
	var listingID uint
	db.Table("market_listings").Select("id").Where("ticker = ?", "HORD").Scan(&listingID)
	order := models.OrderRecord{
		UserID: 100, UserType: "client", AssetID: listingID, Direction: "buy",
		OrderType: "market", Quantity: 2, ContractSize: 1, AccountID: 2,
		Status: "pending", LastModification: now, CreatedAt: now,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	path := fmt.Sprintf("/api/v1/orders/%d", order.ID)

	if rec := do(t, h.OrderRoutes, http.MethodGet, path, clientToken(t), ""); rec.Code != http.StatusOK {
		t.Errorf("owner getOrder: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h.OrderRoutes, http.MethodGet, path, client2Token(t), ""); rec.Code != http.StatusForbidden {
		t.Errorf("non-owner getOrder: want 403, got %d", rec.Code)
	}
	if rec := do(t, h.OrderRoutes, http.MethodGet, "/api/v1/orders/99999", clientToken(t), ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing getOrder: want 404, got %d", rec.Code)
	}
}

// TestOrderHTTP_CreateOrderBranches covers createOrder bad-body, the fund branch
// without a fund service, and stop-order auto-detection.
func TestOrderHTTP_CreateOrderBranches(t *testing.T) {
	h, _ := setupOrderHandlerWithAccounts(t, "h_order_create_branches")
	tok := clientToken(t)

	// Malformed body -> 400.
	if rec := do(t, h.OrdersCollection, http.MethodPost, "/api/v1/orders", tok, `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad body: want 400, got %d", rec.Code)
	}
	// FundID set but no fund service wired -> 400.
	if rec := do(t, h.OrdersCollection, http.MethodPost, "/api/v1/orders", tok,
		`{"assetTicker":"HORD","orderType":"market","direction":"buy","quantity":1,"contractSize":1,"accountId":2,"fundId":5}`); rec.Code != http.StatusBadRequest {
		t.Errorf("fund without service: want 400, got %d", rec.Code)
	}
	// Stop order via auto-detect (stopValue only).
	if rec := do(t, h.OrdersCollection, http.MethodPost, "/api/v1/orders", tok,
		`{"assetTicker":"HORD","orderType":"market","direction":"buy","quantity":1,"contractSize":1,"accountId":2,"stopValue":80}`); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Errorf("stop auto-detect: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
