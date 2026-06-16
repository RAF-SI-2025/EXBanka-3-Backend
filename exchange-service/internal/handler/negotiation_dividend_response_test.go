package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

// TestOtcNegotiationHistory_WithEntries seeds a real negotiation thread so the
// history endpoint exercises negotiationEntryToResponse + parseHistoryDate.
func TestOtcNegotiationHistory_WithEntries(t *testing.T) {
	db := newTestDB(t, "h_neg_hist")
	seedOtcHandlerAccounts(t, db)
	_, listingID := seedExchangeAndListing(t, db, "HNH")

	now := time.Now().UTC()
	settle := now.AddDate(0, 1, 0)
	offer := &models.OtcOfferRecord{
		StockListingID: listingID, SellerHoldingID: 1, Amount: 3, PricePerStock: 100,
		SettlementDate: settle, Premium: 5, LastModified: now, ModifiedByID: 200, ModifiedByType: "client",
		Status: models.OtcOfferStatusDeclined,
		BuyerID: 100, BuyerType: "client", BuyerAccountID: 2,
		SellerID: 200, SellerType: "client", SellerAccountID: 1,
	}
	if err := db.Create(offer).Error; err != nil {
		t.Fatalf("seed offer: %v", err)
	}
	prevAmt := 3.0
	prevSettle := settle
	for _, e := range []*models.OtcNegotiationEntryRecord{
		{OfferID: offer.ID, Action: "created", ActorID: 100, ActorType: "client", Amount: 3, PricePerStock: 100, Premium: 5, SettlementDate: settle, CreatedAt: now.Add(-2 * time.Hour)},
		{OfferID: offer.ID, Action: "countered", ActorID: 200, ActorType: "client", Amount: 4, PricePerStock: 110, Premium: 6, SettlementDate: settle, PrevAmount: &prevAmt, PrevSettlementDate: &prevSettle, CreatedAt: now.Add(-1 * time.Hour)},
	} {
		if err := db.Create(e).Error; err != nil {
			t.Fatalf("seed entry: %v", err)
		}
	}

	h := setupOtcHandler(t, db)

	// History endpoint -> populated entries (covers negotiationEntryToResponse).
	rec := do(t, h.OtcRoutes, http.MethodGet, "/api/v1/otc/offers/"+strconv.FormatUint(uint64(offer.ID), 10)+"/history", clientToken(t), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Count != 2 {
		t.Errorf("expected 2 history entries, got %d", resp.Count)
	}

	// Negotiations list with date + status filters (covers parseHistoryDate).
	rec2 := do(t, h.OtcRoutes, http.MethodGet,
		"/api/v1/otc/negotiations?status=declined&from=2026-01-01&to=2027-12-31&counterparty=200",
		clientToken(t), "")
	if rec2.Code != http.StatusOK {
		t.Fatalf("negotiations status=%d body=%s", rec2.Code, rec2.Body.String())
	}
}

// TestPortfolioDividends_WithData seeds a payout so the list endpoint exercises
// dividendToResponse.
func TestPortfolioDividends_WithData(t *testing.T) {
	db := newFundTestDB(t, "h_div_data")
	h := setupPortfolioWithDividends(t, db)

	now := time.Now().UTC()
	if err := db.Create(&models.DividendPayoutRecord{
		AssetID: 1, Ticker: "AAPL", UserID: 100, UserType: "client", AccountID: 2,
		Quantity: 10, PricePerShare: 200, DividendYield: 0.01, Currency: "USD",
		GrossAmount: 20, CreditedAmount: 20, CreditedCurrency: "USD", TaxRSD: 300,
		Period: "2026-Q2", PaidAt: now,
	}).Error; err != nil {
		t.Fatalf("seed payout: %v", err)
	}

	rec := do(t, h.PortfolioRoutes, http.MethodGet, "/api/v1/portfolio/dividends", clientToken(t), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Count != 1 {
		t.Errorf("expected 1 dividend, got %d", resp.Count)
	}
}
