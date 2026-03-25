package handler

import (
	"encoding/json"
	"net/http"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/repository"
)

type CurrencyHTTPHandler struct {
	repo *repository.CurrencyRepository
	cfg  *config.Config
}

func NewCurrencyHTTPHandler(repo *repository.CurrencyRepository, cfg *config.Config) *CurrencyHTTPHandler {
	return &CurrencyHTTPHandler{repo: repo, cfg: cfg}
}

func (h *CurrencyHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeBasic) {
		return
	}

	currencies, err := h.repo.FindAll()
	if err != nil {
		http.Error(w, `{"error":"failed to list currencies"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"currencies": currencies,
	})
}
