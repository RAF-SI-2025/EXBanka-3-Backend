package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
	"gorm.io/gorm"
)

// --- helpers ---

func watchlistTestCfg() *config.Config {
	return &config.Config{JWTSecret: testJWTSecret}
}

// client2Token is a second client (ID=200) used to test 403 cross-user access.
func client2Token(t *testing.T) string {
	t.Helper()
	return makeToken(t, util.Claims{
		ClientID: 200, TokenSource: "client", TokenType: "access",
		Permissions: []string{models.PermClientTrading, models.PermClientBasic},
	})
}

func setupWatchlistHandler(t *testing.T, db *gorm.DB) *WatchlistHTTPHandler {
	t.Helper()
	repo := repository.NewWatchlistRepository(db)
	return NewWatchlistHTTPHandler(watchlistTestCfg(), repo)
}

// doRequest fires an HTTP request against the handler, routing to the correct method.
func doWatchlistRequest(t *testing.T, h *WatchlistHTTPHandler, method, path, token string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()

	// Route to the right sub-handler.
	if path == "/api/v1/watchlists" {
		h.WatchlistsCollection(rr, req)
	} else {
		h.WatchlistRoutes(rr, req)
	}
	return rr
}

// --- tests ---

func TestWatchlist_Unauthenticated(t *testing.T) {
	db := newTestDB(t, "wl_unauth")
	h := setupWatchlistHandler(t, db)

	rr := doWatchlistRequest(t, h, http.MethodGet, "/api/v1/watchlists", "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestWatchlist_CreateAndList(t *testing.T) {
	db := newTestDB(t, "wl_create_list")
	h := setupWatchlistHandler(t, db)
	tok := clientToken(t)

	// Create a watchlist
	rr := doWatchlistRequest(t, h, http.MethodPost, "/api/v1/watchlists", tok, map[string]string{"name": "My List"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var created models.Watchlist
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Name != "My List" {
		t.Errorf("name: got %q, want %q", created.Name, "My List")
	}
	if created.UserID != 100 || created.UserType != "client" {
		t.Errorf("owner: got (%d,%s), want (100,client)", created.UserID, created.UserType)
	}

	// List watchlists — should contain the one we just created
	rr = doWatchlistRequest(t, h, http.MethodGet, "/api/v1/watchlists", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", rr.Code)
	}
	var lists []models.Watchlist
	if err := json.NewDecoder(rr.Body).Decode(&lists); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(lists) != 1 {
		t.Fatalf("list length: want 1, got %d", len(lists))
	}
	if lists[0].ID != created.ID {
		t.Errorf("listed watchlist id mismatch: %d vs %d", lists[0].ID, created.ID)
	}
}

func TestWatchlist_CreateRequiresName(t *testing.T) {
	db := newTestDB(t, "wl_create_name")
	h := setupWatchlistHandler(t, db)
	tok := clientToken(t)

	rr := doWatchlistRequest(t, h, http.MethodPost, "/api/v1/watchlists", tok, map[string]string{"name": ""})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for empty name, got %d", rr.Code)
	}
}

func TestWatchlist_ForbiddenForOtherUser(t *testing.T) {
	db := newTestDB(t, "wl_forbidden")
	h := setupWatchlistHandler(t, db)

	// client1 creates a watchlist
	tok1 := clientToken(t) // ClientID=100
	rr := doWatchlistRequest(t, h, http.MethodPost, "/api/v1/watchlists", tok1, map[string]string{"name": "Private"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", rr.Code)
	}
	var wl models.Watchlist
	_ = json.NewDecoder(rr.Body).Decode(&wl)

	// client2 tries to GET items from client1's watchlist → 403
	tok2 := client2Token(t) // ClientID=200
	path := "/api/v1/watchlists/" + uintToStr(wl.ID) + "/items"
	rr = doWatchlistRequest(t, h, http.MethodGet, path, tok2, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestWatchlist_AddItem_UnknownTicker(t *testing.T) {
	db := newTestDB(t, "wl_unknown_ticker")
	h := setupWatchlistHandler(t, db)
	tok := clientToken(t)

	// Create watchlist
	rr := doWatchlistRequest(t, h, http.MethodPost, "/api/v1/watchlists", tok, map[string]string{"name": "L"})
	var wl models.Watchlist
	_ = json.NewDecoder(rr.Body).Decode(&wl)

	// Add a ticker that doesn't exist in market_listings → 400
	path := "/api/v1/watchlists/" + uintToStr(wl.ID) + "/items"
	rr = doWatchlistRequest(t, h, http.MethodPost, path, tok, map[string]string{"ticker": "DOES_NOT_EXIST"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for unknown ticker, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestWatchlist_AddItem_Duplicate_Returns409(t *testing.T) {
	db := newTestDB(t, "wl_dup_ticker")
	h := setupWatchlistHandler(t, db)
	tok := clientToken(t)

	seedExchangeAndListing(t, db, "AAPL")

	// Create watchlist
	rr := doWatchlistRequest(t, h, http.MethodPost, "/api/v1/watchlists", tok, map[string]string{"name": "L"})
	var wl models.Watchlist
	_ = json.NewDecoder(rr.Body).Decode(&wl)

	path := "/api/v1/watchlists/" + uintToStr(wl.ID) + "/items"

	// First add → 201
	rr = doWatchlistRequest(t, h, http.MethodPost, path, tok, map[string]string{"ticker": "AAPL"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("first add: want 201, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// Second add (same ticker) → 409
	rr = doWatchlistRequest(t, h, http.MethodPost, path, tok, map[string]string{"ticker": "AAPL"})
	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate add: want 409, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestWatchlist_GetItems_ReturnsPrice(t *testing.T) {
	db := newTestDB(t, "wl_get_items_price")
	h := setupWatchlistHandler(t, db)
	tok := clientToken(t)

	_, listingID := seedExchangeAndListing(t, db, "MSFT")

	// Seed a daily price history record with a known Change value
	history := models.MarketListingDailyPriceInfoRecord{
		ListingID: listingID,
		Date:      time.Now().UTC(),
		Price:     150,
		High:      155,
		Low:       145,
		Change:    3.14,
		Volume:    5000,
	}
	if err := db.Create(&history).Error; err != nil {
		t.Fatalf("seed history: %v", err)
	}

	// Create watchlist and add MSFT
	rr := doWatchlistRequest(t, h, http.MethodPost, "/api/v1/watchlists", tok, map[string]string{"name": "Tech"})
	var wl models.Watchlist
	_ = json.NewDecoder(rr.Body).Decode(&wl)

	itemsPath := "/api/v1/watchlists/" + uintToStr(wl.ID) + "/items"
	rr = doWatchlistRequest(t, h, http.MethodPost, itemsPath, tok, map[string]string{"ticker": "MSFT"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("add item: want 201, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// GET items
	rr = doWatchlistRequest(t, h, http.MethodGet, itemsPath, tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get items: want 200, got %d", rr.Code)
	}
	var items []repository.WatchlistItemView
	if err := json.NewDecoder(rr.Body).Decode(&items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items count: want 1, got %d", len(items))
	}
	it := items[0]
	if it.Ticker != "MSFT" {
		t.Errorf("ticker: got %q", it.Ticker)
	}
	if it.Price != 100 {
		t.Errorf("price: want 100, got %f", it.Price)
	}
	if it.Change != 3.14 {
		t.Errorf("change: want 3.14, got %f", it.Change)
	}
}

func TestWatchlist_RemoveItem(t *testing.T) {
	db := newTestDB(t, "wl_remove_item")
	h := setupWatchlistHandler(t, db)
	tok := clientToken(t)

	seedExchangeAndListing(t, db, "GOOG")

	// Create and populate watchlist
	rr := doWatchlistRequest(t, h, http.MethodPost, "/api/v1/watchlists", tok, map[string]string{"name": "L"})
	var wl models.Watchlist
	_ = json.NewDecoder(rr.Body).Decode(&wl)

	itemsBase := "/api/v1/watchlists/" + uintToStr(wl.ID) + "/items"
	doWatchlistRequest(t, h, http.MethodPost, itemsBase, tok, map[string]string{"ticker": "GOOG"})

	// Remove it
	rr = doWatchlistRequest(t, h, http.MethodDelete, itemsBase+"/GOOG", tok, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("remove: want 204, got %d", rr.Code)
	}

	// Items list should be empty
	rr = doWatchlistRequest(t, h, http.MethodGet, itemsBase, tok, nil)
	var items []repository.WatchlistItemView
	_ = json.NewDecoder(rr.Body).Decode(&items)
	if len(items) != 0 {
		t.Errorf("after remove: want 0 items, got %d", len(items))
	}
}

func TestWatchlist_CascadeDelete_ItemsGone(t *testing.T) {
	db := newTestDB(t, "wl_cascade_del")
	h := setupWatchlistHandler(t, db)
	tok := clientToken(t)

	seedExchangeAndListing(t, db, "TSLA")

	// Create watchlist and add TSLA
	rr := doWatchlistRequest(t, h, http.MethodPost, "/api/v1/watchlists", tok, map[string]string{"name": "EV"})
	var wl models.Watchlist
	_ = json.NewDecoder(rr.Body).Decode(&wl)

	itemsPath := "/api/v1/watchlists/" + uintToStr(wl.ID) + "/items"
	rr = doWatchlistRequest(t, h, http.MethodPost, itemsPath, tok, map[string]string{"ticker": "TSLA"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("add item: want 201, got %d", rr.Code)
	}

	// Delete the watchlist
	rr = doWatchlistRequest(t, h, http.MethodDelete, "/api/v1/watchlists/"+uintToStr(wl.ID), tok, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete watchlist: want 204, got %d", rr.Code)
	}

	// Verify items are gone in the database
	var count int64
	db.Model(&models.WatchlistItem{}).Where("watchlist_id = ?", wl.ID).Count(&count)
	if count != 0 {
		t.Errorf("cascade delete: want 0 items remaining, got %d", count)
	}

	// GET items now returns 404 (watchlist is gone)
	rr = doWatchlistRequest(t, h, http.MethodGet, itemsPath, tok, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("after delete get items: want 404, got %d", rr.Code)
	}
}

func TestWatchlist_ForexTickerWithSlash(t *testing.T) {
	db := newTestDB(t, "wl_forex_slash")
	h := setupWatchlistHandler(t, db)
	tok := clientToken(t)

	seedExchangeAndListing(t, db, "EUR/USD")

	// Create watchlist
	rr := doWatchlistRequest(t, h, http.MethodPost, "/api/v1/watchlists", tok, map[string]string{"name": "FX"})
	var wl models.Watchlist
	_ = json.NewDecoder(rr.Body).Decode(&wl)

	itemsBase := "/api/v1/watchlists/" + uintToStr(wl.ID) + "/items"
	rr = doWatchlistRequest(t, h, http.MethodPost, itemsBase, tok, map[string]string{"ticker": "EUR/USD"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("add EUR/USD: want 201, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// DELETE items/EUR/USD — ticker with "/" must be preserved intact
	rr = doWatchlistRequest(t, h, http.MethodDelete, itemsBase+"/EUR/USD", tok, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("remove EUR/USD: want 204, got %d", rr.Code)
	}

	// Verify it's gone
	rr = doWatchlistRequest(t, h, http.MethodGet, itemsBase, tok, nil)
	var items []repository.WatchlistItemView
	_ = json.NewDecoder(rr.Body).Decode(&items)
	if len(items) != 0 {
		t.Errorf("after forex remove: want 0 items, got %d", len(items))
	}
}

func TestWatchlist_DeleteNotFound(t *testing.T) {
	db := newTestDB(t, "wl_del_notfound")
	h := setupWatchlistHandler(t, db)
	tok := clientToken(t)

	rr := doWatchlistRequest(t, h, http.MethodDelete, "/api/v1/watchlists/9999", tok, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 for non-existent watchlist, got %d", rr.Code)
	}
}

func TestWatchlist_EmployeeHasOwnWatchlist(t *testing.T) {
	db := newTestDB(t, "wl_employee_own")
	h := setupWatchlistHandler(t, db)
	empTok := bankToken(t) // EmployeeID=5

	// Employee creates a watchlist
	rr := doWatchlistRequest(t, h, http.MethodPost, "/api/v1/watchlists", empTok, map[string]string{"name": "Agents List"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("employee create: want 201, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var wl models.Watchlist
	_ = json.NewDecoder(rr.Body).Decode(&wl)

	if wl.UserID != 5 || wl.UserType != "employee" {
		t.Errorf("employee watchlist owner: got (%d,%s), want (5,employee)", wl.UserID, wl.UserType)
	}

	// Client cannot access employee's watchlist → 403
	tok := clientToken(t)
	path := "/api/v1/watchlists/" + uintToStr(wl.ID) + "/items"
	rr = doWatchlistRequest(t, h, http.MethodGet, path, tok, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("client accessing employee watchlist: want 403, got %d", rr.Code)
	}
}

// uintToStr converts a uint to its decimal string representation.
func uintToStr(n uint) string {
	return strconv.FormatUint(uint64(n), 10)
}
