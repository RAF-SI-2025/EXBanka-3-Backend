package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
)

// PriceAlertHTTPHandler serves /api/v1/price-alerts routes.
type PriceAlertHTTPHandler struct {
	cfg  *config.Config
	repo *repository.PriceAlertRepository
}

func NewPriceAlertHTTPHandler(cfg *config.Config, repo *repository.PriceAlertRepository) *PriceAlertHTTPHandler {
	return &PriceAlertHTTPHandler{cfg: cfg, repo: repo}
}

// alertCaller returns per-user identity for alert ownership — same pattern as watchlistCaller.
func alertCaller(claims *util.Claims) (userID uint, userType string) {
	if claims.TokenSource == "employee" {
		return claims.EmployeeID, "employee"
	}
	return claims.ClientID, "client"
}

// Collection handles:
//
//	GET  /api/v1/price-alerts  — list caller's active alerts
//	POST /api/v1/price-alerts  — create a new alert
func (h *PriceAlertHTTPHandler) Collection(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireMarketReadAccessHTTP(w, claims) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.list(w, claims)
	case http.MethodPost:
		h.create(w, r, claims)
	default:
		http.NotFound(w, r)
	}
}

// Routes handles DELETE /api/v1/price-alerts/{id}
func (h *PriceAlertHTTPHandler) Routes(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireMarketReadAccessHTTP(w, claims) {
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/price-alerts/")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid alert id"})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		h.delete(w, claims, uint(id))
	default:
		http.NotFound(w, r)
	}
}

// GET /api/v1/price-alerts
func (h *PriceAlertHTTPHandler) list(w http.ResponseWriter, claims *util.Claims) {
	uid, utype := alertCaller(claims)
	alerts, err := h.repo.ListByUser(uid, utype)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

// POST /api/v1/price-alerts
func (h *PriceAlertHTTPHandler) create(w http.ResponseWriter, r *http.Request, claims *util.Claims) {
	var body struct {
		Ticker    string  `json:"ticker"`
		Condition string  `json:"condition"`
		Threshold float64 `json:"threshold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}
	defer r.Body.Close()

	if strings.TrimSpace(body.Ticker) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "ticker is required"})
		return
	}
	if body.Condition != "ABOVE" && body.Condition != "BELOW" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "condition must be ABOVE or BELOW"})
		return
	}
	if body.Threshold <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "threshold must be positive"})
		return
	}
	if claims.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "user has no email in token"})
		return
	}

	uid, utype := alertCaller(claims)
	alert := &models.PriceAlert{
		UserID:            uid,
		UserType:          utype,
		Ticker:            strings.TrimSpace(body.Ticker),
		Condition:         body.Condition,
		Threshold:         body.Threshold,
		NotificationEmail: claims.Email, // saved from JWT — cron uses this directly
		IsActive:          true,
	}
	if err := h.repo.Create(alert); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	writeJSON(w, http.StatusCreated, alert)
}

// DELETE /api/v1/price-alerts/{id}
func (h *PriceAlertHTTPHandler) delete(w http.ResponseWriter, claims *util.Claims, id uint) {
	alert, err := h.repo.GetByID(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	if alert == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "alert not found"})
		return
	}
	ownerID, ownerType := alertCaller(claims)
	if alert.UserID != ownerID || alert.UserType != ownerType {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "access denied"})
		return
	}
	if err := h.repo.Deactivate(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
