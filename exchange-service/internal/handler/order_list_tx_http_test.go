package handler

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

// TestOrderHTTP_SupervisorListAndTransactions covers the supervisor list
// branches (all orders + userId filter) and the order-transactions listing
// (owner success + non-owner 403).
func TestOrderHTTP_SupervisorListAndTransactions(t *testing.T) {
	h, db := setupOrderHandlerWithAccounts(t, "h_order_list_tx")
	now := time.Now().UTC()
	var listingID uint
	db.Table("market_listings").Select("id").Where("ticker = ?", "HORD").Scan(&listingID)

	order := models.OrderRecord{
		UserID: 100, UserType: "client", AssetID: listingID, Direction: "buy",
		OrderType: "market", Quantity: 5, RemainingPortions: 0, ContractSize: 1,
		AccountID: 2, Status: "executed", LastModification: now, CreatedAt: now,
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	if err := db.Create(&models.OrderTransactionRecord{
		OrderID: order.ID, Quantity: 5, PricePerUnit: 100, ExecutedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed tx: %v", err)
	}

	super := supervisorToken(t)

	// Supervisor: all orders.
	if rec := do(t, h.OrdersCollection, http.MethodGet, "/api/v1/orders", super, ""); rec.Code != http.StatusOK {
		t.Errorf("supervisor list all: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Supervisor: narrowed to a specific user.
	if rec := do(t, h.OrdersCollection, http.MethodGet, "/api/v1/orders?userId=100&userType=client", super, ""); rec.Code != http.StatusOK {
		t.Errorf("supervisor list filtered: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Client (non-supervisor) sees only their own.
	if rec := do(t, h.OrdersCollection, http.MethodGet, "/api/v1/orders?status=executed", clientToken(t), ""); rec.Code != http.StatusOK {
		t.Errorf("client list own: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Owner reads the order's transactions.
	txPath := fmt.Sprintf("/api/v1/orders/%d/transactions", order.ID)
	if rec := do(t, h.OrderRoutes, http.MethodGet, txPath, clientToken(t), ""); rec.Code != http.StatusOK {
		t.Errorf("owner transactions: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// A different client is forbidden.
	if rec := do(t, h.OrderRoutes, http.MethodGet, txPath, client2Token(t), ""); rec.Code != http.StatusForbidden {
		t.Errorf("non-owner transactions: want 403, got %d", rec.Code)
	}
	// Transactions for a missing order -> 404.
	if rec := do(t, h.OrderRoutes, http.MethodGet, "/api/v1/orders/99999/transactions", clientToken(t), ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing order transactions: want 404, got %d", rec.Code)
	}
}
