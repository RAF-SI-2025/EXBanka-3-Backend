package handler

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// deadRegistry points the partner at a refused port so outbound calls fail.
func deadRegistry(t *testing.T) *interbank.Registry {
	t.Helper()
	reg, err := interbank.NewRegistryFromJSON(333,
		`[{"code":444,"baseUrl":"http://127.0.0.1:1","outboundKey":"o","inboundKey":"i","displayName":"P"}]`)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return reg
}

func TestInterbankOtc_CreateNegotiation_PartnerUnreachable(t *testing.T) {
	h := setupInterbankOtcWithRegistry(t, "ib_otc_create_dead") // 127.0.0.1:1
	body := `{"sellerId":{"routingNumber":444,"id":"c9"},"stock":{"ticker":"ACME"},"amount":3,"pricePerUnit":{"currency":"RSD","amount":10},"premium":{"currency":"RSD","amount":5},"settlementDate":"2026-12-31"}`
	rec := do(t, h.Routes, http.MethodPost, "/api/v1/interbank-otc/negotiations", clientToken(t), body)
	if rec.Code < 400 {
		t.Fatalf("expected a 4xx/5xx when the partner is unreachable, got %d", rec.Code)
	}
}

func TestInterbankPayment_CreatePayment_TransportFailure(t *testing.T) {
	db := newFundTestDB(t, "ib_pay_transport")
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (1, 'RSD')`)
	db.Exec(`INSERT INTO accounts (id, broj_racuna, currency_id, status, client_id, stanje, raspolozivo_stanje) VALUES (1, '333000000000000001', 1, 'aktivan', 100, 5000, 5000)`)

	cfg := &config.Config{JWTSecret: testJWTSecret}
	reg := deadRegistry(t)
	h := NewInterbankPaymentHTTPHandler(cfg, reg, interbank.NewClient(reg),
		repository.NewInterbankPaymentRepository(db), repository.NewInterbankPaymentWalletRepository(db), db)

	body := `{"senderAccountNumber":"333000000000000001","recipientAccountNumber":"444000000000000002","currency":"RSD","amount":100}`
	rec := do(t, h.Routes, http.MethodPost, "/api/v1/payments/cross-bank", clientToken(t), body)
	// Funds reserved then released via finaliseTransportFailure; the endpoint
	// reports the failure (>=400) but doesn't 5xx-panic.
	if rec.Code < 400 {
		t.Fatalf("expected a failure status on transport error, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInterbankOtc_Exercise_TransportFailure(t *testing.T) {
	h, db := newIBOtcHandlerWithPartner(t, "ib_ex_transport", func(w http.ResponseWriter, r *http.Request) {
		// Hijack the connection to simulate a transport failure mid-request.
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	})
	db.Exec(`INSERT OR IGNORE INTO currencies (id, kod) VALUES (1, 'RSD')`)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, stanje, raspolozivo_stanje) VALUES (1, 1, 'aktivan', 100, 5000, 5000)`)
	now := time.Now().UTC()
	exch := models.MarketExchangeRecord{Acronym: "EXT", Name: "X", MICCode: "EXT1", Polity: "X", Currency: "RSD", Timezone: "UTC", WorkingHours: "09-17"}
	db.Create(&exch)
	db.Create(&models.MarketListingRecord{Ticker: "ACME", Name: "ACME", Type: "stock", ExchangeID: exch.ID, Price: 10, Ask: 10, Bid: 10, Volume: 1, LastRefresh: now})
	ct := &models.InterbankOptionContract{
		NegotiationRoutingNumber: 444, NegotiationID: "neg", BuyerLocalID: "client-100",
		SellerRoutingNumber: 444, SellerID: "c9", StockTicker: "ACME", Amount: 3,
		PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10, Status: models.InterbankOptionContractStatusValid, SettlementDate: "2026-12-31",
	}
	db.Create(ct)

	rec := do(t, h.Routes, http.MethodPost, fmt.Sprintf("/api/v1/interbank-otc/option-contracts/%d/exercise", ct.ID), clientToken(t), "")
	if rec.Code < 400 {
		t.Fatalf("expected a failure status on exercise transport error, got %d", rec.Code)
	}
}
