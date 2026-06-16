package handler

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

func TestPriceAlertHTTP_CreateAndList(t *testing.T) {
	db := newTestDB(t, "h_alert_create")
	seedExchangeAndListing(t, db, "ALA")
	h := setupAlertHandler(t, db)

	if rec := do(t, h.Collection, http.MethodPost, "/api/v1/price-alerts", clientTokenWithEmail(t), `{"ticker":"ALA","condition":"ABOVE","threshold":100}`); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create alert status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h.Collection, http.MethodGet, "/api/v1/price-alerts", clientToken(t), ""); rec.Code != http.StatusOK {
		t.Errorf("list alerts status=%d", rec.Code)
	}
}

func TestWatchlistHTTP_Flow(t *testing.T) {
	db := newTestDB(t, "h_wl_flow")
	seedExchangeAndListing(t, db, "WLA")
	h := setupWatchlistHandler(t, db)
	tok := clientToken(t)

	if rec := do(t, h.WatchlistsCollection, http.MethodPost, "/api/v1/watchlists", tok, `{"name":"tech"}`); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create watchlist status=%d body=%s", rec.Code, rec.Body.String())
	}
	var wl models.Watchlist
	if err := db.Where("user_id = 100").First(&wl).Error; err != nil {
		t.Fatalf("load watchlist: %v", err)
	}
	base := fmt.Sprintf("/api/v1/watchlists/%d", wl.ID)

	// Add item, list items, remove item, delete watchlist.
	if rec := do(t, h.WatchlistRoutes, http.MethodPost, base+"/items", tok, `{"ticker":"WLA"}`); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Errorf("add item status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h.WatchlistRoutes, http.MethodGet, base+"/items", tok, ""); rec.Code != http.StatusOK {
		t.Errorf("get items status=%d", rec.Code)
	}
	if rec := do(t, h.WatchlistRoutes, http.MethodDelete, base+"/items/WLA", tok, ""); rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Errorf("remove item status=%d", rec.Code)
	}
	if rec := do(t, h.WatchlistRoutes, http.MethodDelete, base, tok, ""); rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Errorf("delete watchlist status=%d", rec.Code)
	}
}

func TestPortfolioHTTP_SetPublic(t *testing.T) {
	db := newTestDB(t, "h_portfolio_setpublic")
	listingExch := models.MarketExchangeRecord{Acronym: "PX", Name: "X", MICCode: "PX1", Polity: "X", Currency: "USD", Timezone: "UTC", WorkingHours: "09-17"}
	db.Create(&listingExch)
	listing := models.MarketListingRecord{Ticker: "PStk", Name: "PStk", Type: "stock", ExchangeID: listingExch.ID, Price: 50, Ask: 51, Bid: 49, Volume: 1, LastRefresh: time.Now().UTC()}
	db.Create(&listing)
	// Holding owned by client 100 (clientToken).
	h0 := models.PortfolioHoldingRecord{UserID: 100, UserType: "client", AssetID: listing.ID, Quantity: 10, AvgBuyPrice: 40, AccountID: 1, CreatedAt: time.Now().UTC()}
	db.Create(&h0)

	h := setupPortfolioHandler(t, db)
	if rec := do(t, h.PortfolioRoutes, http.MethodPut, fmt.Sprintf("/api/v1/portfolio/holdings/%d/public", h0.ID), clientToken(t), `{"publicQuantity":5}`); rec.Code != http.StatusOK {
		t.Fatalf("setPublic status=%d body=%s", rec.Code, rec.Body.String())
	}
}
