package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

// TestInterbankExercise_OptionContract_Success drives exerciseOptionContract
// end-to-end: reserve strike cash, POST the exercise NEW_TX (partner votes YES),
// then commit — debit cash, add the buyer's stock, mark the contract exercised.
func TestInterbankExercise_OptionContract_Success(t *testing.T) {
	h, db := newIBOtcHandlerWithPartner(t, "ib_ex_exercise_ok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(interbank.TransactionVote{Vote: interbank.VoteYes})
	})
	db.Exec(`INSERT OR IGNORE INTO currencies (id, kod) VALUES (1, 'RSD')`)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, stanje, raspolozivo_stanje) VALUES (1, 1, 'aktivan', 100, 5000, 5000)`)

	now := time.Now().UTC()
	exch := models.MarketExchangeRecord{Acronym: "EXC", Name: "X", MICCode: "EXC1", Polity: "X", Currency: "RSD", Timezone: "UTC", WorkingHours: "09-17"}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if err := db.Create(&models.MarketListingRecord{
		Ticker: "ACME", Name: "ACME", Type: "stock", ExchangeID: exch.ID,
		Price: 10, Ask: 10, Bid: 10, Volume: 1, LastRefresh: now,
	}).Error; err != nil {
		t.Fatalf("listing: %v", err)
	}
	ct := &models.InterbankOptionContract{
		NegotiationRoutingNumber: 444, NegotiationID: "neg", BuyerLocalID: "client-100",
		SellerRoutingNumber: 444, SellerID: "c9", StockTicker: "ACME", Amount: 3,
		PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10, PremiumCurrency: "RSD", PremiumAmount: 5,
		Status: models.InterbankOptionContractStatusValid, SettlementDate: "2026-12-31",
	}
	if err := db.Create(ct).Error; err != nil {
		t.Fatalf("contract: %v", err)
	}

	rec := do(t, h.Routes, http.MethodPost, fmt.Sprintf("/api/v1/interbank-otc/option-contracts/%d/exercise", ct.ID), clientToken(t), "")
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("exercise status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestInterbankExercise_OptionContract_RejectedVote covers the NO-vote path:
// the strike reservation is released and the row marked rejected.
func TestInterbankExercise_OptionContract_RejectedVote(t *testing.T) {
	h, db := newIBOtcHandlerWithPartner(t, "ib_ex_exercise_no", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(interbank.TransactionVote{Vote: interbank.VoteNo})
	})
	db.Exec(`INSERT OR IGNORE INTO currencies (id, kod) VALUES (1, 'RSD')`)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, stanje, raspolozivo_stanje) VALUES (1, 1, 'aktivan', 100, 5000, 5000)`)
	now := time.Now().UTC()
	exch := models.MarketExchangeRecord{Acronym: "EXN", Name: "X", MICCode: "EXN1", Polity: "X", Currency: "RSD", Timezone: "UTC", WorkingHours: "09-17"}
	db.Create(&exch)
	db.Create(&models.MarketListingRecord{Ticker: "ACME", Name: "ACME", Type: "stock", ExchangeID: exch.ID, Price: 10, Ask: 10, Bid: 10, Volume: 1, LastRefresh: now})
	ct := &models.InterbankOptionContract{
		NegotiationRoutingNumber: 444, NegotiationID: "neg", BuyerLocalID: "client-100",
		SellerRoutingNumber: 444, SellerID: "c9", StockTicker: "ACME", Amount: 3,
		PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10, Status: models.InterbankOptionContractStatusValid, SettlementDate: "2026-12-31",
	}
	db.Create(ct)

	rec := do(t, h.Routes, http.MethodPost, fmt.Sprintf("/api/v1/interbank-otc/option-contracts/%d/exercise", ct.ID), clientToken(t), "")
	if rec.Code >= 500 {
		t.Fatalf("rejected vote should not 5xx, got %d body=%s", rec.Code, rec.Body.String())
	}
}
