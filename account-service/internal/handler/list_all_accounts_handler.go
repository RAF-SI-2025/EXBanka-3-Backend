package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
)

type ListAllAccountsHTTPHandler struct {
	svc AccountServiceInterface
	cfg *config.Config
}

func NewListAllAccountsHTTPHandler(svc AccountServiceInterface, cfg *config.Config) *ListAllAccountsHTTPHandler {
	return &ListAllAccountsHTTPHandler{svc: svc, cfg: cfg}
}

func parseOptionalInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseOptionalUint(raw string) (*uint, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	id := uint(value)
	return &id, nil
}

func (h *ListAllAccountsHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	currencyID, err := parseOptionalUint(r.URL.Query().Get("currency_id"))
	if err != nil {
		http.Error(w, `{"error":"invalid currency_id"}`, http.StatusBadRequest)
		return
	}

	filter := models.AccountFilter{
		ClientName:    r.URL.Query().Get("client_name"),
		AccountNumber: r.URL.Query().Get("account_number"),
		Tip:           r.URL.Query().Get("tip"),
		Vrsta:         r.URL.Query().Get("vrsta"),
		Status:        r.URL.Query().Get("status"),
		CurrencyID:    currencyID,
		Page:          parseOptionalInt(r.URL.Query().Get("page"), 1),
		PageSize:      parseOptionalInt(r.URL.Query().Get("page_size"), 20),
	}

	accounts, total, err := h.svc.ListAllAccounts(filter)
	if err != nil {
		http.Error(w, `{"error":"failed to list accounts"}`, http.StatusInternalServerError)
		return
	}

	items := make([]accountHTTPJSON, 0, len(accounts))
	for i := range accounts {
		items = append(items, toAccountHTTPJSON(&accounts[i]))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accounts": items,
		"total":    total,
		"page":     filter.Page,
		"pageSize": filter.PageSize,
	})
}
