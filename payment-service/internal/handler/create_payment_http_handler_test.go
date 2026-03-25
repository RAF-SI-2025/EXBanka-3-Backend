package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/golang-jwt/jwt/v5"
)

func makeClientAccessToken(t *testing.T, secret string, clientID uint) string {
	t.Helper()

	claims := jwt.MapClaims{
		"client_id":    clientID,
		"permissions":  []string{"clientBasic"},
		"token_type":   "access",
		"token_source": "client",
		"exp":          time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func TestCreatePaymentHTTPHandler_OmitsVerificationCode(t *testing.T) {
	svc := &mockPaymentSvc{
		created: &models.Payment{
			ID:                1,
			RacunPosiljaocaID: 2,
			RacunPrimaocaBroj: "333000000000000001",
			Iznos:             150,
			SifraPlacanja:     "289",
			PozivNaBroj:       "TEST",
			Svrha:             "HTTP create",
			Status:            "u_obradi",
			VremeTransakcije:  time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC),
		},
	}
	cfg := &config.Config{JWTSecret: "test-secret"}
	h := handler.NewCreatePaymentHTTPHandlerWithService(svc, nil, cfg)

	body := bytes.NewBufferString(`{"racun_posiljaoca_id":2,"racun_primaoca_broj":"333000000000000001","iznos":150,"sifra_placanja":"289","poziv_na_broj":"TEST","svrha":"HTTP create"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", body)
	req.Header.Set("Authorization", "Bearer "+makeClientAccessToken(t, cfg.JWTSecret, 7))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, exists := payload["verificationCode"]; exists {
		t.Fatalf("verificationCode should be omitted, got %s", rr.Body.String())
	}
	if _, exists := payload["verification_code"]; exists {
		t.Fatalf("verification_code should be omitted, got %s", rr.Body.String())
	}
	if _, exists := payload["payment"]; !exists {
		t.Fatalf("payment payload missing: %s", rr.Body.String())
	}
}

func TestCreatePaymentHTTPHandler_RejectsMissingAuth(t *testing.T) {
	svc := &mockPaymentSvc{}
	cfg := &config.Config{JWTSecret: "test-secret"}
	h := handler.NewCreatePaymentHTTPHandlerWithService(svc, nil, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr.Code, rr.Body.String())
	}
}
