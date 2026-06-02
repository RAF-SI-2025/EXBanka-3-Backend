package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
)

// WatchlistHTTPHandler serves /api/v1/watchlists routes.
type WatchlistHTTPHandler struct {
	cfg  *config.Config
	repo *repository.WatchlistRepository
}

func NewWatchlistHTTPHandler(cfg *config.Config, repo *repository.WatchlistRepository) *WatchlistHTTPHandler {
	return &WatchlistHTTPHandler{cfg: cfg, repo: repo}
}

// watchlistCaller returns the per-user identity for watchlist ownership.
// Unlike callerIdentity (which collapses all employees to "bank"),
// this returns each employee's own EmployeeID so watchlists are per-employee.
func watchlistCaller(claims *util.Claims) (userID uint, userType string) {
	if claims.TokenSource == "employee" {
		return claims.EmployeeID, "employee"
	}
	return claims.ClientID, "client"
}

// WatchlistsCollection handles:
//
//	GET  /api/v1/watchlists  — list caller's watchlists
//	POST /api/v1/watchlists  — create a new watchlist
func (h *WatchlistHTTPHandler) WatchlistsCollection(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireMarketReadAccessHTTP(w, claims) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.listWatchlists(w, claims)
	case http.MethodPost:
		h.createWatchlist(w, r, claims)
	default:
		http.NotFound(w, r)
	}
}

// WatchlistRoutes handles paths under /api/v1/watchlists/{id}:
//
//	DELETE /api/v1/watchlists/{id}
//	GET    /api/v1/watchlists/{id}/items
//	POST   /api/v1/watchlists/{id}/items
//	DELETE /api/v1/watchlists/{id}/items/{ticker}  (ticker may contain "/")
func (h *WatchlistHTTPHandler) WatchlistRoutes(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireMarketReadAccessHTTP(w, claims) {
		return
	}

	// Strip the /api/v1/watchlists/ prefix, leaving "{id}" or "{id}/items[/ticker]"
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/watchlists/")

	// Split on the first "/" to separate the watchlist ID from the sub-path.
	var idStr, subpath string
	if i := strings.Index(rest, "/"); i < 0 {
		idStr = rest
	} else {
		idStr = rest[:i]
		subpath = rest[i+1:] // "items" or "items/EUR/USD"
	}

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid watchlist id"})
		return
	}
	wlID := uint(id)

	switch {
	case subpath == "" && r.Method == http.MethodDelete:
		h.deleteWatchlist(w, claims, wlID)
	case subpath == "items" && r.Method == http.MethodGet:
		h.getItems(w, claims, wlID)
	case subpath == "items" && r.Method == http.MethodPost:
		h.addItem(w, r, claims, wlID)
	case strings.HasPrefix(subpath, "items/") && r.Method == http.MethodDelete:
		ticker := strings.TrimPrefix(subpath, "items/")
		h.removeItem(w, claims, wlID, ticker)
	default:
		http.NotFound(w, r)
	}
}

// loadAndVerifyOwner fetches the watchlist and checks that the caller owns it.
// Writes 404 or 403 and returns false if the check fails.
func (h *WatchlistHTTPHandler) loadAndVerifyOwner(w http.ResponseWriter, claims *util.Claims, wlID uint) (*models.Watchlist, bool) {
	wl, err := h.repo.GetByID(wlID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return nil, false
	}
	if wl == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "watchlist not found"})
		return nil, false
	}
	ownerID, ownerType := watchlistCaller(claims)
	if wl.UserID != ownerID || wl.UserType != ownerType {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "access denied"})
		return nil, false
	}
	return wl, true
}

// GET /api/v1/watchlists
func (h *WatchlistHTTPHandler) listWatchlists(w http.ResponseWriter, claims *util.Claims) {
	uid, utype := watchlistCaller(claims)
	lists, err := h.repo.ListByUser(uid, utype)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, lists)
}

// POST /api/v1/watchlists
func (h *WatchlistHTTPHandler) createWatchlist(w http.ResponseWriter, r *http.Request, claims *util.Claims) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "name is required"})
		return
	}
	defer r.Body.Close()

	uid, utype := watchlistCaller(claims)
	wl := &models.Watchlist{
		UserID:   uid,
		UserType: utype,
		Name:     strings.TrimSpace(body.Name),
	}
	if err := h.repo.Create(wl); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, wl)
}

// DELETE /api/v1/watchlists/{id}
func (h *WatchlistHTTPHandler) deleteWatchlist(w http.ResponseWriter, claims *util.Claims, wlID uint) {
	if _, ok := h.loadAndVerifyOwner(w, claims, wlID); !ok {
		return
	}
	if err := h.repo.Delete(wlID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/watchlists/{id}/items
func (h *WatchlistHTTPHandler) getItems(w http.ResponseWriter, claims *util.Claims, wlID uint) {
	if _, ok := h.loadAndVerifyOwner(w, claims, wlID); !ok {
		return
	}
	items, err := h.repo.GetItems(wlID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// POST /api/v1/watchlists/{id}/items
func (h *WatchlistHTTPHandler) addItem(w http.ResponseWriter, r *http.Request, claims *util.Claims, wlID uint) {
	if _, ok := h.loadAndVerifyOwner(w, claims, wlID); !ok {
		return
	}
	var body struct {
		Ticker string `json:"ticker"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Ticker) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "ticker is required"})
		return
	}
	defer r.Body.Close()

	item, err := h.repo.AddItem(wlID, strings.TrimSpace(body.Ticker))
	if errors.Is(err, repository.ErrTickerNotFound) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "ticker not found"})
		return
	}
	if errors.Is(err, repository.ErrDuplicateItem) {
		writeJSON(w, http.StatusConflict, map[string]string{"message": "ticker already on watchlist"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

// DELETE /api/v1/watchlists/{id}/items/{ticker}
// Note: ticker is extracted as the full remainder after "items/" to support
// forex tickers that contain "/" (e.g. "EUR/USD").
func (h *WatchlistHTTPHandler) removeItem(w http.ResponseWriter, claims *util.Claims, wlID uint, ticker string) {
	if _, ok := h.loadAndVerifyOwner(w, claims, wlID); !ok {
		return
	}
	if err := h.repo.RemoveItem(wlID, ticker); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
