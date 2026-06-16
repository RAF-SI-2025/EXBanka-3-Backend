package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// makeOtcOffer creates a pending offer (buyer 100 -> seller 200's holding).
func makeOtcOffer(t *testing.T, svc *service.OtcService, holdingID uint) uint {
	t.Helper()
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID: 100, BuyerType: "client", BuyerAccountID: 2,
		SellerHoldingID: holdingID, Amount: 3, PricePerStock: 105,
		SettlementDate: time.Now().UTC().AddDate(0, 1, 0), Premium: 25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	return offer.ID
}

func TestOtcService_ListNegotiations_Filters(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_neg_list")
	makeOtcOffer(t, svc, holding.ID)

	// All negotiations for the buyer.
	all, err := svc.ListNegotiations(100, "client", "", nil, nil, 0)
	if err != nil || len(all) != 1 {
		t.Fatalf("ListNegotiations all: %d err=%v", len(all), err)
	}
	// Status filter.
	if pend, _ := svc.ListNegotiations(100, "client", "pending", nil, nil, 0); len(pend) != 1 {
		t.Errorf("pending filter: %d", len(pend))
	}
	// Counterparty match / mismatch.
	if cp, _ := svc.ListNegotiations(100, "client", "", nil, nil, 200); len(cp) != 1 {
		t.Errorf("counterparty 200: %d", len(cp))
	}
	if cp, _ := svc.ListNegotiations(100, "client", "", nil, nil, 999); len(cp) != 0 {
		t.Errorf("counterparty 999: %d", len(cp))
	}
	// Date filter: nothing after a future cutoff.
	future := time.Now().UTC().Add(time.Hour)
	if rec, _ := svc.ListNegotiations(100, "client", "", &future, nil, 0); len(rec) != 0 {
		t.Errorf("from-future filter: %d", len(rec))
	}
	// Invalid status -> error.
	if _, err := svc.ListNegotiations(100, "client", "bogus", nil, nil, 0); err == nil {
		t.Error("expected invalid-status error")
	}
}

func TestOtcService_GetNegotiationHistory(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_neg_hist")
	offerID := makeOtcOffer(t, svc, holding.ID)

	// Seller counters -> a second entry.
	if _, err := svc.CounterOffer(service.CounterOtcOfferInput{
		OfferID: offerID, ModifiedByID: 200, ModifiedByType: "client",
		Amount: 4, PricePerStock: 110, SettlementDate: time.Now().UTC().AddDate(0, 1, 0), Premium: 30,
	}); err != nil {
		t.Fatalf("CounterOffer: %v", err)
	}

	offer, entries, err := svc.GetNegotiationHistory(offerID, 100, "client")
	if err != nil || offer == nil {
		t.Fatalf("GetNegotiationHistory: %v", err)
	}
	if len(entries) < 2 {
		t.Errorf("expected >=2 entries (created + countered), got %d", len(entries))
	}
	if entries[0].Action != "created" {
		t.Errorf("first entry should be 'created', got %q", entries[0].Action)
	}

	// Non-participant and unknown offer -> not found.
	if _, _, err := svc.GetNegotiationHistory(offerID, 999, "client"); !errors.Is(err, service.ErrOtcOfferNotFound) {
		t.Errorf("non-participant: want ErrOtcOfferNotFound, got %v", err)
	}
	if _, _, err := svc.GetNegotiationHistory(99999, 100, "client"); !errors.Is(err, service.ErrOtcOfferNotFound) {
		t.Errorf("unknown offer: want ErrOtcOfferNotFound, got %v", err)
	}
}

func TestOtcService_SendExpiryReminders(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_reminders")
	// Offer settling in ~2 days; seller accepts to create the contract.
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID: 100, BuyerType: "client", BuyerAccountID: 2,
		SellerHoldingID: holding.ID, Amount: 2, PricePerStock: 105,
		SettlementDate: time.Now().UTC().Add(48 * time.Hour), Premium: 10,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if _, err := svc.AcceptOffer(offer.ID, 200, "client"); err != nil {
		t.Fatalf("AcceptOffer: %v", err)
	}

	// Within a 5-day window -> one reminder; a second call is deduped to zero.
	reminded, err := svc.SendExpiryReminders(time.Now().UTC(), 5)
	if err != nil {
		t.Fatalf("SendExpiryReminders: %v", err)
	}
	if reminded != 1 {
		t.Errorf("expected 1 contract reminded, got %d", reminded)
	}
	if again, _ := svc.SendExpiryReminders(time.Now().UTC(), 5); again != 0 {
		t.Errorf("expected dedup (0) on second run, got %d", again)
	}
}
