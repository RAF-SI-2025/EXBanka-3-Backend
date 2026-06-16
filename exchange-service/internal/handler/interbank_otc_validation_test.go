package handler

import (
	"net/http"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func setupInterbankOtcWithRegistry(t *testing.T, name string) *InterbankOtcHTTPHandler {
	t.Helper()
	db := newFundTestDB(t, name)
	cfg := &config.Config{JWTSecret: testJWTSecret}
	reg, err := interbank.NewRegistryFromJSON(333,
		`[{"code":444,"baseUrl":"http://127.0.0.1:1","outboundKey":"o","inboundKey":"i","displayName":"P"}]`)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return NewInterbankOtcHTTPHandler(
		cfg, reg, interbank.NewClient(reg),
		repository.NewInterbankOtcRepository(db), nil, repository.NewRemotePublicStockRepository(db),
		repository.NewInterbankOptionContractRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewInterbankWalletRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
		db,
	)
}

func TestInterbankOtcHTTP_CreateNegotiation_Validation(t *testing.T) {
	h := setupInterbankOtcWithRegistry(t, "ib_otc_create_val")
	tok := clientToken(t)
	path := "/api/v1/interbank-otc/negotiations"

	cases := map[string]string{
		"bad json":         `{`,
		"missing seller":   `{"stock":{"ticker":"X"},"amount":1}`,
		"own bank seller":  `{"sellerId":{"routingNumber":333,"id":"c1"},"stock":{"ticker":"X"},"amount":1}`,
		"unknown partner":  `{"sellerId":{"routingNumber":999,"id":"c1"},"stock":{"ticker":"X"},"amount":1}`,
		"empty ticker":     `{"sellerId":{"routingNumber":444,"id":"c1"},"stock":{"ticker":""},"amount":1}`,
		"non-positive amt": `{"sellerId":{"routingNumber":444,"id":"c1"},"stock":{"ticker":"X"},"amount":0}`,
	}
	for name, body := range cases {
		rec := do(t, h.Routes, http.MethodPost, path, tok, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d (%s)", name, rec.Code, rec.Body.String())
		}
	}

	// Employee token cannot start interbank negotiations -> 403.
	if rec := do(t, h.Routes, http.MethodPost, path, supervisorToken(t),
		`{"sellerId":{"routingNumber":444,"id":"c1"},"stock":{"ticker":"X"},"amount":1}`); rec.Code != http.StatusForbidden {
		t.Errorf("employee start: expected 403, got %d", rec.Code)
	}
}

func TestInterbankOtcHTTP_ExerciseContract_Validation(t *testing.T) {
	h := setupInterbankOtcWithRegistry(t, "ib_otc_exercise_val")

	// Unauthenticated -> 401.
	if rec := do(t, h.Routes, http.MethodPost, "/api/v1/interbank-otc/option-contracts/1/exercise", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauth: expected 401, got %d", rec.Code)
	}
	// Bad contract id -> 400.
	if rec := do(t, h.Routes, http.MethodPost, "/api/v1/interbank-otc/option-contracts/abc/exercise", clientToken(t), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: expected 400, got %d", rec.Code)
	}
	// Unknown contract -> 404.
	if rec := do(t, h.Routes, http.MethodPost, "/api/v1/interbank-otc/option-contracts/99999/exercise", clientToken(t), ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown: expected 404, got %d (%s)", rec.Code, "")
	}
}

func TestInterbankOtcHTTP_PublicStocks_ReadsCached(t *testing.T) {
	db := newFundTestDB(t, "ib_otc_pubstock")
	cfg := &config.Config{JWTSecret: testJWTSecret}
	reg, _ := interbank.NewRegistryFromJSON(333,
		`[{"code":444,"baseUrl":"http://127.0.0.1:1","outboundKey":"o","inboundKey":"i","displayName":"P"}]`)
	snapRepo := repository.NewRemotePublicStockRepository(db)
	// Seed a cached snapshot so readCached returns data instead of fanning out.
	if err := snapRepo.UpsertPayload(444, `[]`); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	h := NewInterbankOtcHTTPHandler(
		cfg, reg, interbank.NewClient(reg),
		repository.NewInterbankOtcRepository(db), nil, snapRepo,
		repository.NewInterbankOptionContractRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewInterbankWalletRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
		db,
	)

	rec := do(t, h.Routes, http.MethodGet, "/api/v1/interbank-otc/public-stocks", clientToken(t), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("public-stocks status=%d body=%s", rec.Code, rec.Body.String())
	}
}
