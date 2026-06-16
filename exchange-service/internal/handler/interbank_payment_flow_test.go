package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func TestInterbankPaymentHTTP_CreatePayment_Success(t *testing.T) {
	db := newFundTestDB(t, "ib_pay_create_ok")
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (1, 'RSD')`)
	// Sender account: prefix 333 = this bank, owned by client 100 (clientToken).
	db.Exec(`INSERT INTO accounts (id, broj_racuna, currency_id, status, client_id, stanje, raspolozivo_stanje) VALUES (1, '333000000000000001', 1, 'aktivan', 100, 5000, 5000)`)

	// Partner stub: YES vote for NEW_TX; harmless 200 for COMMIT_TX.
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

	body := `{"senderAccountNumber":"333000000000000001","recipientAccountNumber":"444000000000000002","currency":"RSD","amount":100}`
	rec := do(t, h.Routes, http.MethodPost, "/api/v1/payments/cross-bank", clientToken(t), body)
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("expected 2xx after YES vote, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankPaymentHTTP_CreatePayment_RejectedVote(t *testing.T) {
	db := newFundTestDB(t, "ib_pay_create_no")
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (1, 'RSD')`)
	db.Exec(`INSERT INTO accounts (id, broj_racuna, currency_id, status, client_id, stanje, raspolozivo_stanje) VALUES (1, '333000000000000001', 1, 'aktivan', 100, 5000, 5000)`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(interbank.TransactionVote{Vote: interbank.VoteNo})
	}))
	defer srv.Close()

	cfg := &config.Config{JWTSecret: testJWTSecret}
	reg, _ := interbank.NewRegistryFromJSON(333, fmt.Sprintf(
		`[{"code":444,"baseUrl":"%s","outboundKey":"o","inboundKey":"i","displayName":"P"}]`, srv.URL))
	h := NewInterbankPaymentHTTPHandler(cfg, reg, interbank.NewClient(reg),
		repository.NewInterbankPaymentRepository(db), repository.NewInterbankPaymentWalletRepository(db), db)

	body := `{"senderAccountNumber":"333000000000000001","recipientAccountNumber":"444000000000000002","currency":"RSD","amount":100}`
	rec := do(t, h.Routes, http.MethodPost, "/api/v1/payments/cross-bank", clientToken(t), body)
	// A NO vote is a completed (non-5xx) outcome: the reservation is released and
	// the payment marked rejected.
	if rec.Code >= 500 {
		t.Fatalf("rejected vote should not 5xx, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrderHTTP_WithNotifier(t *testing.T) {
	db := newTestDB(t, "h_order_notifier")
	h := setupOrderHandler(t, db)
	if h.WithNotifier(notify.NewClient("", "")) == nil {
		t.Error("WithNotifier returned nil")
	}
}
