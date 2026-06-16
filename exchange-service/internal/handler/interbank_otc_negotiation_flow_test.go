package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"gorm.io/gorm"
)

// newIBOtcHandlerWithPartner wires the OTC handler against a live partner stub.
func newIBOtcHandlerWithPartner(t *testing.T, name string, partner http.HandlerFunc) (*InterbankOtcHTTPHandler, *gorm.DB) {
	t.Helper()
	db := newFundTestDB(t, name)
	srv := httptest.NewServer(partner)
	t.Cleanup(srv.Close)
	cfg := &config.Config{JWTSecret: testJWTSecret}
	reg, err := interbank.NewRegistryFromJSON(333, fmt.Sprintf(
		`[{"code":444,"baseUrl":"%s","outboundKey":"o","inboundKey":"i","displayName":"P"}]`, srv.URL))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	client := interbank.NewClient(reg)
	negRepo := repository.NewInterbankOtcRepository(db)
	walletRepo := repository.NewInterbankWalletRepository(db)
	portfolioRepo := repository.NewPortfolioRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	negsHandler := interbank.NewNegotiationsHandler(reg, negRepo, client, db, walletRepo, portfolioRepo, marketRepo)
	h := NewInterbankOtcHTTPHandler(
		cfg, reg, client,
		negRepo, negsHandler, repository.NewRemotePublicStockRepository(db),
		repository.NewInterbankOptionContractRepository(db),
		repository.NewInterbankExerciseRepository(db),
		walletRepo, portfolioRepo, marketRepo, db,
	)
	return h, db
}

func TestInterbankOtcHTTP_CreateNegotiation_Success(t *testing.T) {
	h, _ := newIBOtcHandlerWithPartner(t, "ib_otc_create_ok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"routingNumber":444,"id":"neg-1"}`))
	})
	body := `{"sellerId":{"routingNumber":444,"id":"c9"},"stock":{"ticker":"ACME"},"amount":3,"pricePerUnit":{"currency":"RSD","amount":10},"premium":{"currency":"RSD","amount":5},"settlementDate":"2026-12-31"}`
	rec := do(t, h.Routes, http.MethodPost, "/api/v1/interbank-otc/negotiations", clientToken(t), body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtcHTTP_UpdateNegotiation_Success(t *testing.T) {
	h, db := newIBOtcHandlerWithPartner(t, "ib_otc_update_ok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`)) // partner echoes back an OtcNegotiation
	})
	now := time.Now().UTC()
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 444, NegotiationID: "neg-upd", LocalRole: models.InterbankNegotiationRoleBuyer,
		CounterpartyRoutingNumber: 444, BuyerRoutingNumber: 333, BuyerID: "client-100",
		SellerRoutingNumber: 444, SellerID: "c9", StockTicker: "ACME", Amount: 3,
		PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10, PremiumCurrency: "RSD", PremiumAmount: 5,
		SettlementDate: "2026-12-31", LastModifiedByRoutingNumber: 444, LastModifiedByID: "c9",
		IsOngoing: true, UpdatedAt: now,
	}
	if err := db.Create(neg).Error; err != nil {
		t.Fatalf("seed neg: %v", err)
	}
	body := `{"settlementDate":"2026-12-31","pricePerUnit":{"currency":"RSD","amount":12},"premium":{"currency":"RSD","amount":6},"amount":4}`
	rec := do(t, h.Routes, http.MethodPut, "/api/v1/interbank-otc/negotiations/444/neg-upd", clientToken(t), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtcHTTP_AcceptNegotiation_Reachable(t *testing.T) {
	// Partner stub ACKs everything; the negsHandler is now wired so accept is
	// reachable. With no matching local seller negotiation it returns a non-OK
	// outcome (covers the early + error-response branches of acceptNegotiation).
	h, _ := newIBOtcHandlerWithPartner(t, "ib_otc_accept", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	rec := do(t, h.Routes, http.MethodPost, "/api/v1/interbank-otc/negotiations/444/nope/accept", clientToken(t), "")
	if rec.Code == http.StatusOK {
		t.Fatalf("expected a non-OK accept outcome for a missing negotiation, got %d", rec.Code)
	}
}

func TestInterbankOtcHTTP_Negotiation_ErrorBranches(t *testing.T) {
	h, db := newIBOtcHandlerWithPartner(t, "ib_otc_neg_errors", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	tok := clientToken(t)
	now := time.Now().UTC()

	// Update / close / accept a missing negotiation -> 404.
	for _, m := range []struct {
		method, suffix, body string
	}{
		{http.MethodPut, "", `{"amount":1,"pricePerUnit":{"currency":"RSD","amount":1},"premium":{"currency":"RSD","amount":0},"settlementDate":"2026-12-31"}`},
		{http.MethodDelete, "", ""},
	} {
		if rec := do(t, h.Routes, m.method, "/api/v1/interbank-otc/negotiations/444/missing"+m.suffix, tok, m.body); rec.Code != http.StatusNotFound {
			t.Errorf("%s missing: expected 404, got %d", m.method, rec.Code)
		}
	}

	// A negotiation where client 100 is NOT a party -> 403 on update.
	other := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 444, NegotiationID: "neg-other", LocalRole: models.InterbankNegotiationRoleBuyer,
		CounterpartyRoutingNumber: 444, BuyerRoutingNumber: 333, BuyerID: "client-777",
		SellerRoutingNumber: 444, SellerID: "c9", StockTicker: "ACME", Amount: 3,
		PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10, PremiumCurrency: "RSD", PremiumAmount: 5,
		SettlementDate: "2026-12-31", LastModifiedByRoutingNumber: 444, LastModifiedByID: "c9",
		IsOngoing: true, UpdatedAt: now,
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("seed neg: %v", err)
	}
	if rec := do(t, h.Routes, http.MethodPut, "/api/v1/interbank-otc/negotiations/444/neg-other", tok,
		`{"amount":1,"pricePerUnit":{"currency":"RSD","amount":1},"premium":{"currency":"RSD","amount":0},"settlementDate":"2026-12-31"}`); rec.Code != http.StatusForbidden {
		t.Errorf("non-party update: expected 403, got %d", rec.Code)
	}

	// A no-longer-ongoing negotiation we ARE a party to -> 409 on update.
	closed := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 444, NegotiationID: "neg-closed", LocalRole: models.InterbankNegotiationRoleBuyer,
		CounterpartyRoutingNumber: 444, BuyerRoutingNumber: 333, BuyerID: "client-100",
		SellerRoutingNumber: 444, SellerID: "c9", StockTicker: "ACME", Amount: 3,
		PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10, PremiumCurrency: "RSD", PremiumAmount: 5,
		SettlementDate: "2026-12-31", LastModifiedByRoutingNumber: 444, LastModifiedByID: "c9",
		IsOngoing: false, UpdatedAt: now,
	}
	if err := db.Create(closed).Error; err != nil {
		t.Fatalf("seed closed: %v", err)
	}
	db.Model(&models.InterbankOtcNegotiation{}).Where("negotiation_id = ?", "neg-closed").UpdateColumn("is_ongoing", false)
	if rec := do(t, h.Routes, http.MethodPut, "/api/v1/interbank-otc/negotiations/444/neg-closed", tok,
		`{"amount":1,"pricePerUnit":{"currency":"RSD","amount":1},"premium":{"currency":"RSD","amount":0},"settlementDate":"2026-12-31"}`); rec.Code != http.StatusConflict {
		t.Errorf("closed update: expected 409, got %d", rec.Code)
	}
	// Bad body on a valid ongoing negotiation we're a party to -> 400.
	ongoing := *closed
	ongoing.ID = 0
	ongoing.NegotiationID = "neg-ongoing"
	ongoing.IsOngoing = true
	if err := db.Create(&ongoing).Error; err != nil {
		t.Fatalf("seed ongoing: %v", err)
	}
	if rec := do(t, h.Routes, http.MethodPut, "/api/v1/interbank-otc/negotiations/444/neg-ongoing", tok, `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad body: expected 400, got %d", rec.Code)
	}
}

func TestInterbankOtcHTTP_CloseNegotiation_Success(t *testing.T) {
	h, db := newIBOtcHandlerWithPartner(t, "ib_otc_close_ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent) // partner ACKs the DELETE/close
	})
	// Seed a local negotiation the caller (client 100) is the buyer of.
	now := time.Now().UTC()
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 444, NegotiationID: "neg-close", LocalRole: models.InterbankNegotiationRoleBuyer,
		CounterpartyRoutingNumber: 444, BuyerRoutingNumber: 333, BuyerID: "client-100",
		SellerRoutingNumber: 444, SellerID: "c9", StockTicker: "ACME", Amount: 3,
		PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10, PremiumCurrency: "RSD", PremiumAmount: 5,
		SettlementDate: "2026-12-31", LastModifiedByRoutingNumber: 333, LastModifiedByID: "client-100",
		IsOngoing: true, UpdatedAt: now,
	}
	if err := db.Create(neg).Error; err != nil {
		t.Fatalf("seed neg: %v", err)
	}
	rec := do(t, h.Routes, http.MethodDelete, "/api/v1/interbank-otc/negotiations/444/neg-close", clientToken(t), "")
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("close status=%d body=%s", rec.Code, rec.Body.String())
	}
}
