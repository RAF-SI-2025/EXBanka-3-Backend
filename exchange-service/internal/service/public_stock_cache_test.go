package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func TestPublicStockCacheRunner_Run(t *testing.T) {
	db := openSagaTestDB(t, "public_stock_cache")
	repo := repository.NewRemotePublicStockRepository(db)

	// Empty registry -> Run returns immediately.
	emptyReg, err := interbank.NewRegistryFromJSON(333, "[]")
	if err != nil {
		t.Fatalf("empty registry: %v", err)
	}
	NewPublicStockCacheRunner(emptyReg, interbank.NewClient(emptyReg), repo).Run()

	// Live partner returning an (empty) stock list -> UpsertPayload path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	reg, err := interbank.NewRegistryFromJSON(333, fmt.Sprintf(
		`[{"code":444,"baseUrl":"%s","outboundKey":"o","inboundKey":"i","displayName":"P"}]`, srv.URL))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	NewPublicStockCacheRunner(reg, interbank.NewClient(reg), repo).Run()

	// Dead partner -> fetch fails -> UpsertError (stale) path.
	deadReg, err := interbank.NewRegistryFromJSON(333,
		`[{"code":445,"baseUrl":"http://127.0.0.1:1","outboundKey":"o","inboundKey":"i","displayName":"Dead"}]`)
	if err != nil {
		t.Fatalf("dead registry: %v", err)
	}
	NewPublicStockCacheRunner(deadReg, interbank.NewClient(deadReg), repo).Run()
}
