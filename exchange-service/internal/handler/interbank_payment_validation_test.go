package handler

import (
	"net/http"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func setupIBPaymentWithRegistry(t *testing.T, name string) *InterbankPaymentHTTPHandler {
	t.Helper()
	db := newFundTestDB(t, name)
	cfg := &config.Config{JWTSecret: testJWTSecret}
	reg, err := interbank.NewRegistryFromJSON(333,
		`[{"code":444,"baseUrl":"http://127.0.0.1:1","outboundKey":"o","inboundKey":"i","displayName":"P"}]`)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return NewInterbankPaymentHTTPHandler(cfg, reg, interbank.NewClient(reg),
		repository.NewInterbankPaymentRepository(db), repository.NewInterbankPaymentWalletRepository(db), db)
}

func TestInterbankPaymentHTTP_CreatePayment_Validation(t *testing.T) {
	h := setupIBPaymentWithRegistry(t, "ib_pay_create_val")
	path := "/api/v1/payments/cross-bank"
	tok := clientToken(t)

	// Employee token cannot initiate cross-bank payments.
	if rec := do(t, h.Routes, http.MethodPost, path, supervisorToken(t),
		`{"senderAccountNumber":"1","recipientAccountNumber":"2","currency":"RSD","amount":10}`); rec.Code != http.StatusForbidden {
		t.Errorf("employee: expected 403, got %d", rec.Code)
	}

	cases := map[string]string{
		"bad json":         `{`,
		"missing accounts": `{"currency":"RSD","amount":10}`,
		"missing currency": `{"senderAccountNumber":"1","recipientAccountNumber":"2","amount":10}`,
		"unknown currency": `{"senderAccountNumber":"1","recipientAccountNumber":"2","currency":"XYZ","amount":10}`,
		"non-positive amt": `{"senderAccountNumber":"1","recipientAccountNumber":"2","currency":"RSD","amount":0}`,
		"same accounts":    `{"senderAccountNumber":"1","recipientAccountNumber":"1","currency":"RSD","amount":10}`,
	}
	for name, body := range cases {
		if rec := do(t, h.Routes, http.MethodPost, path, tok, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d (%s)", name, rec.Code, rec.Body.String())
		}
	}
}
