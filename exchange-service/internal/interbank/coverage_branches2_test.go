package interbank

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func TestExerciseOnNewTx_TermMismatches(t *testing.T) {
	mk := func(t *testing.T) *ExerciseTxProcessor {
		db := openInterbankTestDB(t, "ex_mismatch_"+t.Name())
		seedExerciseNegotiation(t, db) // neg-x: buyer 111/emp-1, AAPL, amount 4, USD 10
		seedListing(t, db, "AAPL")
		return newExerciseProcessor(db)
	}
	partner := &PartnerBank{Code: 111}
	ctx := context.Background()

	// Wrong buyer id.
	t.Run("wrong_buyer", func(t *testing.T) {
		p := mk(t)
		tx := exerciseTxFor()
		other := ForeignBankId{RoutingNumber: 111, ID: "emp-99"}
		tx.Postings[1].Account.ID = &other
		tx.Postings[3].Account.ID = &other
		if v, err := p.OnNewTx(ctx, partner, &tx); err != nil || v.Vote != VoteNo {
			t.Fatalf("v=%+v err=%v", v, err)
		}
	})

	// Wrong ticker.
	t.Run("wrong_ticker", func(t *testing.T) {
		p := mk(t)
		tx := exerciseTxFor()
		tx.Postings[2].Asset.Stock = &StockDescription{Ticker: "MSFT"}
		tx.Postings[3].Asset.Stock = &StockDescription{Ticker: "MSFT"}
		if v, _ := p.OnNewTx(ctx, partner, &tx); v.Vote != VoteNo {
			t.Fatalf("v=%+v", v)
		}
	})

	// Wrong stock amount.
	t.Run("wrong_amount", func(t *testing.T) {
		p := mk(t)
		tx := exerciseTxFor()
		tx.Postings[2].Amount = -5
		tx.Postings[3].Amount = 5
		if v, _ := p.OnNewTx(ctx, partner, &tx); v.Vote != VoteNo {
			t.Fatalf("v=%+v", v)
		}
	})

	// Wrong cash currency.
	t.Run("wrong_currency", func(t *testing.T) {
		p := mk(t)
		tx := exerciseTxFor()
		tx.Postings[0].Asset.Monas = &MonetaryAsset{Currency: "EUR"}
		tx.Postings[1].Asset.Monas = &MonetaryAsset{Currency: "EUR"}
		if v, _ := p.OnNewTx(ctx, partner, &tx); v.Vote != VoteNo {
			t.Fatalf("v=%+v", v)
		}
	})

	// Wrong cash amount (price*amount mismatch).
	t.Run("wrong_cash", func(t *testing.T) {
		p := mk(t)
		tx := exerciseTxFor()
		tx.Postings[0].Amount = 999
		tx.Postings[1].Amount = -999
		if v, _ := p.OnNewTx(ctx, partner, &tx); v.Vote != VoteNo {
			t.Fatalf("v=%+v", v)
		}
	})

	// Already exercised.
	t.Run("already_exercised", func(t *testing.T) {
		db := openInterbankTestDB(t, "ex_already")
		seedExerciseNegotiation(t, db)
		seedListing(t, db, "AAPL")
		// Insert a committed exercise row for the same negotiation.
		committed := &models.InterbankPendingExercise{
			TxRoutingNumber: 111, TxID: "prev", Direction: models.InterbankExerciseDirectionInbound,
			PartnerRoutingNumber: 111, NegotiationRoutingNumber: 333, NegotiationID: "neg-x",
			StockTicker: "AAPL", StockAmount: 4, PricePerUnitCurrency: "USD", PricePerUnitAmount: 10,
			CashAmount: 40, BuyerRoutingNumber: 111, BuyerID: "emp-1",
			SellerRoutingNumber: 333, SellerID: "client-5",
			Status: models.InterbankExerciseStatusCommitted, CreatedAt: time.Now().UTC(),
		}
		if err := db.Create(committed).Error; err != nil {
			t.Fatal(err)
		}
		p := newExerciseProcessor(db)
		tx := exerciseTxFor()
		if v, _ := p.OnNewTx(ctx, partner, &tx); v.Vote != VoteNo {
			t.Fatalf("already-exercised should vote NO, got %+v", v)
		}
	})

	// Expired settlement.
	t.Run("expired", func(t *testing.T) {
		db := openInterbankTestDB(t, "ex_expired")
		neg := &models.InterbankOtcNegotiation{
			NegotiationRoutingNumber: 333, NegotiationID: "neg-x",
			LocalRole: models.InterbankNegotiationRoleSeller, CounterpartyRoutingNumber: 111,
			BuyerRoutingNumber: 111, BuyerID: "emp-1", SellerRoutingNumber: 333, SellerID: "client-5",
			StockTicker: "AAPL", Amount: 4, PricePerUnitCurrency: "USD", PricePerUnitAmount: 10,
			PremiumCurrency: "USD", PremiumAmount: 5,
			SettlementDate: time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339),
			IsOngoing:      false, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		if err := repository.NewInterbankOtcRepository(db).Create(neg); err != nil {
			t.Fatal(err)
		}
		seedListing(t, db, "AAPL")
		p := newExerciseProcessor(db)
		tx := exerciseTxFor()
		if v, _ := p.OnNewTx(ctx, partner, &tx); v.Vote != VoteNo {
			t.Fatalf("expired should vote NO, got %+v", v)
		}
	})
}

func TestNegotiations_NoPartnerBranches(t *testing.T) {
	db := openInterbankTestDB(t, "neg_nopartner")
	h := newNegHandler(t, db, nil)
	neg := seedOtcNegotiationSeller(t, db, "neg-np")
	path := "/negotiations/333/" + neg.NegotiationID

	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.Item(rec, authedReq(m, path, nil, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without partner = %d, want 401", m, rec.Code)
		}
	}
}

func TestNegotiations_DeleteAlreadyClosed(t *testing.T) {
	db := openInterbankTestDB(t, "neg_delclosed")
	h := newNegHandler(t, db, nil)
	buyer := &PartnerBank{Code: 111}
	neg := seedOtcNegotiationSeller(t, db, "neg-dc")
	_ = repository.NewInterbankOtcRepository(db).MarkClosed(333, neg.NegotiationID)

	rec := httptest.NewRecorder()
	h.Item(rec, authedReq(http.MethodDelete, "/negotiations/333/"+neg.NegotiationID, buyer, nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("delete already-closed = %d, want 204", rec.Code)
	}
}
