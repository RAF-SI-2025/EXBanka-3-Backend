package handler

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

func TestInterbankExercise_StandaloneHelpers(t *testing.T) {
	db := newFundTestDB(t, "ib_ex_helpers")

	// upsertBuyerStockHolding: create then weighted-average update.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := upsertBuyerStockHolding(tx, 1, 5, 10, 3, 100); err != nil {
			return err
		}
		return upsertBuyerStockHolding(tx, 1, 5, 10, 2, 110)
	}); err != nil {
		t.Fatalf("upsertBuyerStockHolding: %v", err)
	}
	var h models.PortfolioHoldingRecord
	if err := db.Where("user_id = 1 AND asset_id = 5").First(&h).Error; err != nil {
		t.Fatalf("load holding: %v", err)
	}
	if h.Quantity != 5 {
		t.Errorf("expected qty 5 after two fills, got %v", h.Quantity)
	}

	// buildExerciseTx: 4-posting option-exercise transaction.
	contract := &models.InterbankOptionContract{
		NegotiationRoutingNumber: 444, NegotiationID: "neg-1", BuyerLocalID: "client-1",
		StockTicker: "ACME", Amount: 3, PricePerUnitCurrency: "RSD",
	}
	tx := buildExerciseTx(interbank.ForeignBankId{RoutingNumber: 444, ID: "x"}, contract, 333, 30)
	if len(tx.Postings) != 4 {
		t.Errorf("expected 4 postings, got %d", len(tx.Postings))
	}

	// parseContractSettlement: RFC3339, date-only, and invalid.
	if _, ok := parseContractSettlement("2026-01-01T00:00:00Z"); !ok {
		t.Error("RFC3339 should parse")
	}
	if _, ok := parseContractSettlement("2026-01-01"); !ok {
		t.Error("date-only should parse")
	}
	if _, ok := parseContractSettlement("garbage"); ok {
		t.Error("invalid should not parse")
	}
}
