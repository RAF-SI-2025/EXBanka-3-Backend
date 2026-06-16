package handler

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func TestOrderHTTP_GetCancelApproveDecline(t *testing.T) {
	db := newTestDB(t, "h_order_routes")
	seedOtcHandlerAccounts(t, db) // accounts 1 (client 200), 2 (client 100)
	_, listingID := seedExchangeAndListing(t, db, "HOR")
	orderRepo := repository.NewOrderRepository(db)
	now := time.Now().UTC()
	mk := func(status string, qty int64) *models.OrderRecord {
		o := &models.OrderRecord{
			UserID: 100, UserType: "client", AssetID: listingID, OrderType: "market", Direction: "buy",
			Quantity: qty, ContractSize: 1, PricePerUnit: 100, CurrencyRate: 1, Commission: 5,
			Status: status, RemainingPortions: qty, AccountID: 2, LastModification: now, CreatedAt: now,
		}
		if err := orderRepo.CreateOrder(o); err != nil {
			t.Fatalf("seed order: %v", err)
		}
		return o
	}

	h := setupOrderHandler(t, db)
	client, super := clientToken(t), supervisorToken(t)

	// Owner reads an order.
	o1 := mk("approved", 4)
	if rec := do(t, h.OrderRoutes, http.MethodGet, fmt.Sprintf("/api/v1/orders/%d", o1.ID), client, ""); rec.Code != http.StatusOK {
		t.Errorf("getOrder status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Owner partial-cancels.
	if rec := do(t, h.OrderRoutes, http.MethodPost, fmt.Sprintf("/api/v1/orders/%d/cancel", o1.ID), client, `{"newRemaining":2}`); rec.Code != http.StatusOK {
		t.Errorf("cancel status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Supervisor approves a pending order.
	o2 := mk("pending", 2)
	if rec := do(t, h.OrderRoutes, http.MethodPost, fmt.Sprintf("/api/v1/orders/%d/approve", o2.ID), super, ""); rec.Code != http.StatusOK {
		t.Errorf("approve status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Supervisor declines another pending order (refunds).
	o3 := mk("pending", 2)
	if rec := do(t, h.OrderRoutes, http.MethodPost, fmt.Sprintf("/api/v1/orders/%d/decline", o3.ID), super, ""); rec.Code != http.StatusOK {
		t.Errorf("decline status=%d body=%s", rec.Code, rec.Body.String())
	}
}
