package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
)

type listClientAccountsRepo interface {
	ListByClientID(clientID uint) ([]models.Account, error)
}

type ListClientAccountsHTTPHandler struct {
	repo listClientAccountsRepo
	cfg  *config.Config
}

func NewListClientAccountsHTTPHandler(repo listClientAccountsRepo) *ListClientAccountsHTTPHandler {
	return &ListClientAccountsHTTPHandler{repo: repo}
}

func NewListClientAccountsHTTPHandlerWithConfig(repo listClientAccountsRepo, cfg *config.Config) *ListClientAccountsHTTPHandler {
	return &ListClientAccountsHTTPHandler{repo: repo, cfg: cfg}
}

// clientIDFromPath extracts the last path segment from e.g. "/api/v1/accounts/client/42"
func clientIDFromPath(path string) (uint, error) {
	trimmed := strings.TrimRight(path, "/")
	parts := strings.Split(trimmed, "/")
	raw := parts[len(parts)-1]
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}

func (h *ListClientAccountsHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clientID, err := clientIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, `{"error":"invalid client id"}`, http.StatusBadRequest)
		return
	}

	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireClientOrEmployeeHTTP(w, claims, clientID, models.PermClientBasic, models.PermEmployeeBasic) {
		return
	}

	accounts, err := h.repo.ListByClientID(clientID)
	if err != nil {
		http.Error(w, `{"error":"failed to list accounts"}`, http.StatusInternalServerError)
		return
	}

	result := make([]accountHTTPJSON, 0, len(accounts))
	for _, a := range accounts {
		result = append(result, toAccountHTTPJSON(&a))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
