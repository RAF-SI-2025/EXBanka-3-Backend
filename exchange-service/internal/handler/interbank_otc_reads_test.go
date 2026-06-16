package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

// TestInterbankOtcHTTP_Reads covers the cached public-stocks path (readCached)
// and listNegotiations role/includeClosed filtering.
func TestInterbankOtcHTTP_Reads(t *testing.T) {
	h, db := newIBOtcHandlerWithPartner(t, "ib_otc_reads", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})
	now := time.Now().UTC()

	// A cached snapshot row so readCached returns a hit (no live fan-out).
	if err := db.Create(&models.RemotePublicStockSnapshot{
		PartnerRoutingNumber: 444, PayloadJSON: "[]", FetchedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	// A negotiation the caller (client-100) is the buyer of.
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 444, NegotiationID: "neg-read", LocalRole: models.InterbankNegotiationRoleBuyer,
		CounterpartyRoutingNumber: 444, BuyerRoutingNumber: 333, BuyerID: "client-100",
		SellerRoutingNumber: 444, SellerID: "c9", StockTicker: "ACME", Amount: 3,
		PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10, PremiumCurrency: "RSD", PremiumAmount: 5,
		SettlementDate: "2026-12-31", LastModifiedByRoutingNumber: 444, LastModifiedByID: "c9",
		IsOngoing: true, UpdatedAt: now,
	}
	if err := db.Create(neg).Error; err != nil {
		t.Fatalf("seed neg: %v", err)
	}

	tok := clientToken(t)

	// Cached public-stocks read.
	if rec := do(t, h.Routes, http.MethodGet, "/api/v1/interbank-otc/public-stocks", tok, ""); rec.Code != http.StatusOK {
		t.Errorf("public-stocks cached: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Negotiation listing with each filter.
	for _, q := range []string{"", "?role=buyer", "?role=seller", "?includeClosed=true"} {
		if rec := do(t, h.Routes, http.MethodGet, "/api/v1/interbank-otc/negotiations"+q, tok, ""); rec.Code != http.StatusOK {
			t.Errorf("list negotiations %q: status=%d body=%s", q, rec.Code, rec.Body.String())
		}
	}
	// Invalid role -> 400.
	if rec := do(t, h.Routes, http.MethodGet, "/api/v1/interbank-otc/negotiations?role=sideways", tok, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad role: want 400, got %d", rec.Code)
	}
}
