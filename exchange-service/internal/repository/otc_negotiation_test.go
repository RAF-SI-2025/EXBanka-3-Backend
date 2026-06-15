package repository

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

func TestOtcRepository_NegotiationEntries(t *testing.T) {
	db := openRepoTestDB(t, "otc_neg_entries")
	r := NewOtcRepository(db)

	// Minimal offer row (no preloads needed for entry tests).
	offer := &models.OtcOfferRecord{
		StockListingID: 1, SellerHoldingID: 1, Amount: 10, PricePerStock: 100,
		SettlementDate: time.Now().Add(72 * time.Hour).UTC(), Premium: 5,
		LastModified: time.Now().UTC(), ModifiedByID: 2, ModifiedByType: "client",
		Status: models.OtcOfferStatusPending,
		BuyerID: 2, BuyerType: "client", BuyerAccountID: 1,
		SellerID: 3, SellerType: "client", SellerAccountID: 2,
	}
	if err := r.CreateOffer(offer); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	prevAmount := 10.0
	entries := []*models.OtcNegotiationEntryRecord{
		{OfferID: offer.ID, Action: models.OtcNegotiationActionCreated, ActorID: 2, ActorType: "client",
			Amount: 10, PricePerStock: 100, Premium: 5, SettlementDate: offer.SettlementDate, CreatedAt: time.Now().Add(-2 * time.Hour)},
		{OfferID: offer.ID, Action: models.OtcNegotiationActionCountered, ActorID: 3, ActorType: "client",
			Amount: 12, PricePerStock: 95, Premium: 6, SettlementDate: offer.SettlementDate,
			PrevAmount: &prevAmount, CreatedAt: time.Now().Add(-1 * time.Hour)},
	}
	for _, e := range entries {
		if err := r.AppendNegotiationEntry(e); err != nil {
			t.Fatalf("AppendNegotiationEntry: %v", err)
		}
	}

	got, err := r.ListNegotiationEntries(offer.ID)
	if err != nil {
		t.Fatalf("ListNegotiationEntries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	// Oldest first.
	if got[0].Action != models.OtcNegotiationActionCreated || got[1].Action != models.OtcNegotiationActionCountered {
		t.Errorf("entries out of order: %s, %s", got[0].Action, got[1].Action)
	}
	if got[1].PrevAmount == nil || *got[1].PrevAmount != 10 {
		t.Errorf("expected prev amount 10 on counter entry, got %v", got[1].PrevAmount)
	}
}

func TestOtcRepository_ListNegotiations_Filters(t *testing.T) {
	db := openRepoTestDB(t, "otc_neg_list")
	r := NewOtcRepository(db)

	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	mk := func(buyerID, sellerID uint, status string, modified time.Time) uint {
		o := &models.OtcOfferRecord{
			StockListingID: 1, SellerHoldingID: 1, Amount: 10, PricePerStock: 100,
			SettlementDate: base.Add(72 * time.Hour), Premium: 1, LastModified: modified,
			ModifiedByID: buyerID, ModifiedByType: "client", Status: status,
			BuyerID: buyerID, BuyerType: "client", BuyerAccountID: 1,
			SellerID: sellerID, SellerType: "client", SellerAccountID: 2,
		}
		if err := r.CreateOffer(o); err != nil {
			t.Fatalf("CreateOffer: %v", err)
		}
		// CreateOffer stamps last_modified to now when zero; force our value.
		db.Model(o).Update("last_modified", modified)
		return o.ID
	}

	// user 5 is buyer against seller 9 (accepted) and seller 11 (declined).
	mk(5, 9, models.OtcOfferStatusAccepted, base)
	mk(5, 11, models.OtcOfferStatusDeclined, base.Add(48*time.Hour))
	// user 5 is seller against buyer 9 (cancelled) — counterparty 9 again.
	mk(9, 5, models.OtcOfferStatusCancelled, base.Add(24*time.Hour))

	all, err := r.ListNegotiations(5, "client", NegotiationFilter{})
	if err != nil {
		t.Fatalf("ListNegotiations: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 negotiations for user 5, got %d", len(all))
	}

	// Status filter.
	accepted, _ := r.ListNegotiations(5, "client", NegotiationFilter{Status: models.OtcOfferStatusAccepted})
	if len(accepted) != 1 {
		t.Errorf("expected 1 accepted, got %d", len(accepted))
	}

	// Counterparty filter: party 9 appears twice (once as seller, once as buyer).
	cp9, _ := r.ListNegotiations(5, "client", NegotiationFilter{CounterpartyID: 9})
	if len(cp9) != 2 {
		t.Errorf("expected 2 negotiations with counterparty 9, got %d", len(cp9))
	}

	// Date range filter: only offers modified strictly after base+12h.
	from := base.Add(12 * time.Hour)
	recent, _ := r.ListNegotiations(5, "client", NegotiationFilter{From: &from})
	if len(recent) != 2 {
		t.Errorf("expected 2 negotiations after %v, got %d", from, len(recent))
	}
}
