package handler

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

// TestInterbankExercise_OptionContract_ErrorBranches covers the early guards of
// exerciseOptionContract: non-client 403, missing 404, not-owner 403, and a
// non-valid status 409.
func TestInterbankExercise_OptionContract_ErrorBranches(t *testing.T) {
	h, db := newIBOtcHandlerWithPartner(t, "ib_ex_exercise_errs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	const p = "/api/v1/interbank-otc/option-contracts/%d/exercise"

	// Non-client (employee) token -> 403.
	if rec := do(t, h.Routes, http.MethodPost, fmt.Sprintf(p, 1), bankToken(t), ""); rec.Code != http.StatusForbidden {
		t.Errorf("non-client: want 403, got %d", rec.Code)
	}
	// Missing contract -> 404.
	if rec := do(t, h.Routes, http.MethodPost, fmt.Sprintf(p, 99999), clientToken(t), ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing: want 404, got %d", rec.Code)
	}

	// A contract owned by a different local buyer -> 403.
	other := &models.InterbankOptionContract{
		NegotiationRoutingNumber: 444, NegotiationID: "n1", BuyerLocalID: "client-777",
		SellerRoutingNumber: 444, SellerID: "c9", StockTicker: "ACME", Amount: 1,
		PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10,
		Status: models.InterbankOptionContractStatusValid, SettlementDate: "2026-12-31",
	}
	db.Create(other)
	if rec := do(t, h.Routes, http.MethodPost, fmt.Sprintf(p, other.ID), clientToken(t), ""); rec.Code != http.StatusForbidden {
		t.Errorf("not owner: want 403, got %d", rec.Code)
	}

	// A contract we own but already exercised -> 409.
	done := &models.InterbankOptionContract{
		NegotiationRoutingNumber: 444, NegotiationID: "n2", BuyerLocalID: "client-100",
		SellerRoutingNumber: 444, SellerID: "c9", StockTicker: "ACME", Amount: 1,
		PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10,
		Status: models.InterbankOptionContractStatusExercised, SettlementDate: "2026-12-31",
	}
	db.Create(done)
	if rec := do(t, h.Routes, http.MethodPost, fmt.Sprintf(p, done.ID), clientToken(t), ""); rec.Code != http.StatusConflict {
		t.Errorf("already exercised: want 409, got %d", rec.Code)
	}
}
