package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// TestInterbankPaymentHTTP_ListAndGet covers listPayments and getPayment
// (client success + non-client 403, missing 404, bad id 400).
func TestInterbankPaymentHTTP_ListAndGet(t *testing.T) {
	db := newFundTestDB(t, "ib_pay_reads")
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (1, 'RSD')`)
	db.Exec(`INSERT INTO accounts (id, broj_racuna, currency_id, status, client_id, stanje, raspolozivo_stanje) VALUES (1, '333000000000000001', 1, 'aktivan', 100, 5000, 5000)`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(interbank.TransactionVote{Vote: interbank.VoteYes})
	}))
	defer srv.Close()

	cfg := &config.Config{JWTSecret: testJWTSecret}
	reg, _ := interbank.NewRegistryFromJSON(333, fmt.Sprintf(
		`[{"code":444,"baseUrl":"%s","outboundKey":"o","inboundKey":"i","displayName":"P"}]`, srv.URL))
	h := NewInterbankPaymentHTTPHandler(cfg, reg, interbank.NewClient(reg),
		repository.NewInterbankPaymentRepository(db), repository.NewInterbankPaymentWalletRepository(db), db)

	// Create a payment so a row exists for client 100.
	body := `{"senderAccountNumber":"333000000000000001","recipientAccountNumber":"444000000000000002","currency":"RSD","amount":100}`
	if rec := do(t, h.Routes, http.MethodPost, "/api/v1/payments/cross-bank", clientToken(t), body); rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("create payment: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// List as the owning client.
	rec := do(t, h.Routes, http.MethodGet, "/api/v1/payments/cross-bank?limit=10", clientToken(t), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Payments []struct {
			ID uint `json:"id"`
		} `json:"payments"`
		Count int `json:"count"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if listed.Count == 0 || len(listed.Payments) == 0 {
		t.Fatalf("expected at least one listed payment, got %+v", listed)
	}
	pid := listed.Payments[0].ID

	// Get that payment as the owner.
	if rec := do(t, h.Routes, http.MethodGet, fmt.Sprintf("/api/v1/payments/cross-bank/%d", pid), clientToken(t), ""); rec.Code != http.StatusOK {
		t.Errorf("get own payment: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// List as a non-client (employee) -> 403.
	if rec := do(t, h.Routes, http.MethodGet, "/api/v1/payments/cross-bank", bankToken(t), ""); rec.Code != http.StatusForbidden {
		t.Errorf("list as employee: want 403, got %d", rec.Code)
	}
	// Get a missing payment -> 404.
	if rec := do(t, h.Routes, http.MethodGet, "/api/v1/payments/cross-bank/99999", clientToken(t), ""); rec.Code != http.StatusNotFound {
		t.Errorf("get missing: want 404, got %d", rec.Code)
	}
	// Bad payment id -> 400.
	if rec := do(t, h.Routes, http.MethodGet, "/api/v1/payments/cross-bank/abc", clientToken(t), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: want 400, got %d", rec.Code)
	}

	tok := clientToken(t)
	const p = "/api/v1/payments/cross-bank"
	// Non-client initiate -> 403.
	if rec := do(t, h.Routes, http.MethodPost, p, bankToken(t), `{}`); rec.Code != http.StatusForbidden {
		t.Errorf("non-client create: want 403, got %d", rec.Code)
	}
	// Malformed body -> 400.
	if rec := do(t, h.Routes, http.MethodPost, p, tok, `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad body: want 400, got %d", rec.Code)
	}
	// Missing account numbers -> 400.
	if rec := do(t, h.Routes, http.MethodPost, p, tok, `{"currency":"RSD","amount":10}`); rec.Code != http.StatusBadRequest {
		t.Errorf("missing accounts: want 400, got %d", rec.Code)
	}
	// Missing currency -> 400.
	if rec := do(t, h.Routes, http.MethodPost, p, tok, `{"senderAccountNumber":"333000000000000001","recipientAccountNumber":"444000000000000002","amount":10}`); rec.Code != http.StatusBadRequest {
		t.Errorf("missing currency: want 400, got %d", rec.Code)
	}
	// Unknown currency -> 400.
	if rec := do(t, h.Routes, http.MethodPost, p, tok, `{"senderAccountNumber":"333000000000000001","recipientAccountNumber":"444000000000000002","currency":"ZZZ","amount":10}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown currency: want 400, got %d", rec.Code)
	}
	// Non-positive amount -> 400.
	if rec := do(t, h.Routes, http.MethodPost, p, tok, `{"senderAccountNumber":"333000000000000001","recipientAccountNumber":"444000000000000002","currency":"RSD","amount":0}`); rec.Code != http.StatusBadRequest {
		t.Errorf("zero amount: want 400, got %d", rec.Code)
	}
	// Sender == recipient -> 400.
	if rec := do(t, h.Routes, http.MethodPost, p, tok, `{"senderAccountNumber":"333000000000000001","recipientAccountNumber":"333000000000000001","currency":"RSD","amount":10}`); rec.Code != http.StatusBadRequest {
		t.Errorf("same account: want 400, got %d", rec.Code)
	}
}
