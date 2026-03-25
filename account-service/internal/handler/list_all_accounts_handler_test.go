package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
)

func TestListAllAccountsHTTP_ParsesFiltersAndReturnsAccounts(t *testing.T) {
	clientID := uint(7)
	svc := &mockAccountService{
		listAllResult: []models.Account{
			{
				ID:         10,
				BrojRacuna: "265000000000000001",
				ClientID:   &clientID,
				Tip:        "tekuci",
				Vrsta:      "licni",
				Podvrsta:   "standardni",
				CurrencyID: 1,
				Currency:   models.Currency{Kod: "RSD"},
				Status:     "aktivan",
			},
		},
		listAllTotal: 1,
	}
	h := handler.NewListAllAccountsHTTPHandler(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?client_name=Petar&account_number=265000&tip=tekuci&vrsta=licni&status=aktivan&currency_id=1&page=2&page_size=5", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	if svc.capturedFilter.ClientName != "Petar" {
		t.Fatalf("expected client_name filter to be captured, got %q", svc.capturedFilter.ClientName)
	}
	if svc.capturedFilter.AccountNumber != "265000" {
		t.Fatalf("expected account_number filter to be captured, got %q", svc.capturedFilter.AccountNumber)
	}
	if svc.capturedFilter.Page != 2 || svc.capturedFilter.PageSize != 5 {
		t.Fatalf("expected pagination 2/5, got %d/%d", svc.capturedFilter.Page, svc.capturedFilter.PageSize)
	}
	if svc.capturedFilter.CurrencyID == nil || *svc.capturedFilter.CurrencyID != 1 {
		t.Fatalf("expected currency_id filter 1, got %+v", svc.capturedFilter.CurrencyID)
	}

	var resp struct {
		Accounts []map[string]interface{} `json:"accounts"`
		Total    float64                  `json:"total"`
		Page     float64                  `json:"page"`
		PageSize float64                  `json:"pageSize"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(resp.Accounts))
	}
	if resp.Accounts[0]["podvrsta"] != "standardni" {
		t.Fatalf("expected podvrsta in response, got %v", resp.Accounts[0]["podvrsta"])
	}
	if resp.Total != 1 || resp.Page != 2 || resp.PageSize != 5 {
		t.Fatalf("unexpected pagination payload: %+v", resp)
	}
}

func TestListAllAccountsHTTP_InvalidCurrencyID_ReturnsBadRequest(t *testing.T) {
	h := handler.NewListAllAccountsHTTPHandler(&mockAccountService{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?currency_id=bad", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}
