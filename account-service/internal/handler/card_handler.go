package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
)

// cardServiceInterface is the subset of CardService used by the HTTP handler.
type cardServiceInterface interface {
	CreateCard(input service.CreateCardInput) (*models.Card, error)
	GetCard(id uint) (*models.Card, error)
	ListByAccount(accountID uint) ([]models.Card, error)
	ListByClient(clientID uint) ([]models.Card, error)
	BlockCard(cardID, clientID uint) (*models.Card, error)
	UnblockCard(cardID uint) (*models.Card, error)
	DeactivateCard(cardID uint) (*models.Card, error)
}

// CardHTTPHandler handles all /api/v1/cards/* routes.
type CardHTTPHandler struct {
	svc cardServiceInterface
	cfg *config.Config
}

func NewCardHTTPHandler(svc cardServiceInterface) *CardHTTPHandler {
	return &CardHTTPHandler{svc: svc}
}

func NewCardHTTPHandlerWithConfig(svc cardServiceInterface, cfg *config.Config) *CardHTTPHandler {
	return &CardHTTPHandler{
		svc: svc,
		cfg: cfg,
	}
}

// ServeHTTP dispatches to the appropriate sub-handler based on path and method.
//
// Routes:
//
//	POST   /api/v1/cards                  → CreateCard
//	GET    /api/v1/cards/{id}             → GetCard
//	GET    /api/v1/cards/account/{id}     → ListByAccount
//	GET    /api/v1/cards/client/{id}      → ListByClient
//	PUT    /api/v1/cards/{id}/block       → BlockCard
//	PUT    /api/v1/cards/{id}/unblock     → UnblockCard
//	PUT    /api/v1/cards/{id}/deactivate  → DeactivateCard
func (h *CardHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Strip prefix and clean trailing slash.
	path := strings.TrimRight(r.URL.Path, "/")
	const prefix = "/api/v1/cards"
	rest := strings.TrimPrefix(path, prefix)

	// POST /api/v1/cards
	if rest == "" && r.Method == http.MethodPost {
		h.handleCreate(w, r)
		return
	}

	parts := strings.Split(strings.TrimPrefix(rest, "/"), "/")

	switch {
	// GET /api/v1/cards/account/{id}
	case len(parts) == 2 && parts[0] == "account" && r.Method == http.MethodGet:
		h.handleListByAccount(w, r, parts[1])

	// GET /api/v1/cards/client/{id}
	case len(parts) == 2 && parts[0] == "client" && r.Method == http.MethodGet:
		h.handleListByClient(w, r, parts[1])

	// PUT /api/v1/cards/{id}/block
	case len(parts) == 2 && parts[1] == "block" && r.Method == http.MethodPut:
		h.handleBlock(w, r, parts[0])

	// PUT /api/v1/cards/{id}/unblock
	case len(parts) == 2 && parts[1] == "unblock" && r.Method == http.MethodPut:
		h.handleUnblock(w, r, parts[0])

	// PUT /api/v1/cards/{id}/deactivate
	case len(parts) == 2 && parts[1] == "deactivate" && r.Method == http.MethodPut:
		h.handleDeactivate(w, r, parts[0])

	// GET /api/v1/cards/{id}
	case len(parts) == 1 && parts[0] != "" && r.Method == http.MethodGet:
		h.handleGet(w, r, parts[0])

	default:
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseID(raw string) (uint, error) {
	id, err := strconv.ParseUint(raw, 10, 64)
	return uint(id), err
}

// POST /api/v1/cards
func (h *CardHTTPHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeBasic) {
		return
	}

	var req struct {
		AccountID    uint   `json:"accountId"`
		ClientID     uint   `json:"clientId"`
		VrstaKartice string `json:"vrstaKartice"`
		NazivKartice string `json:"nazivKartice"`
		ClientEmail  string `json:"clientEmail"`
		ClientName   string `json:"clientName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	card, err := h.svc.CreateCard(service.CreateCardInput{
		AccountID:    req.AccountID,
		ClientID:     req.ClientID,
		VrstaKartice: req.VrstaKartice,
		NazivKartice: req.NazivKartice,
		ClientEmail:  req.ClientEmail,
		ClientName:   req.ClientName,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, card)
}

// GET /api/v1/cards/{id}
func (h *CardHTTPHandler) handleGet(w http.ResponseWriter, r *http.Request, rawID string) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}

	id, err := parseID(rawID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid card id")
		return
	}

	card, err := h.svc.GetCard(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if card == nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	if claims != nil && (claims.ClientID != 0 || claims.TokenSource == "client") {
		if card.ClientID != claims.ClientID {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		if !util.HasPermission(claims, models.PermClientBasic) {
			writeError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
	} else if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeBasic) {
		return
	}

	writeJSON(w, http.StatusOK, card)
}

// GET /api/v1/cards/account/{id}
func (h *CardHTTPHandler) handleListByAccount(w http.ResponseWriter, r *http.Request, rawID string) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeBasic) {
		return
	}

	id, err := parseID(rawID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	cards, err := h.svc.ListByAccount(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, cards)
}

// GET /api/v1/cards/client/{id}
func (h *CardHTTPHandler) handleListByClient(w http.ResponseWriter, r *http.Request, rawID string) {
	id, err := parseID(rawID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid client id")
		return
	}

	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireClientOrEmployeeHTTP(w, claims, id, models.PermClientBasic, models.PermEmployeeBasic) {
		return
	}

	cards, err := h.svc.ListByClient(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, cards)
}

// PUT /api/v1/cards/{id}/block
func (h *CardHTTPHandler) handleBlock(w http.ResponseWriter, r *http.Request, rawID string) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}

	cardID, err := parseID(rawID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid card id")
		return
	}

	var req struct {
		ClientID uint `json:"clientId"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}
	if claims != nil && (claims.ClientID != 0 || claims.TokenSource == "client") {
		if claims.ClientID == 0 || !util.HasPermission(claims, models.PermClientBasic) {
			writeError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
		if req.ClientID != 0 && req.ClientID != claims.ClientID {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		req.ClientID = claims.ClientID
	} else if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeBasic) {
		return
	}

	card, err := h.svc.BlockCard(cardID, req.ClientID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, card)
}

// PUT /api/v1/cards/{id}/unblock
func (h *CardHTTPHandler) handleUnblock(w http.ResponseWriter, r *http.Request, rawID string) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeBasic) {
		return
	}

	cardID, err := parseID(rawID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid card id")
		return
	}

	card, err := h.svc.UnblockCard(cardID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, card)
}

// PUT /api/v1/cards/{id}/deactivate
func (h *CardHTTPHandler) handleDeactivate(w http.ResponseWriter, r *http.Request, rawID string) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeBasic) {
		return
	}

	cardID, err := parseID(rawID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid card id")
		return
	}

	card, err := h.svc.DeactivateCard(cardID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, card)
}
