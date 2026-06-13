package interbank

import (
	"context"
	"net/http"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func TestOtcOnNewTx_NotBuyerBankAndTermsMismatch(t *testing.T) {
	ctx := context.Background()
	partner := &PartnerBank{Code: 111}

	// We are not the buyer's bank (neg.BuyerRoutingNumber=999, own=333).
	t.Run("not_buyer_bank", func(t *testing.T) {
		db := openInterbankTestDB(t, "otc_notbuyer")
		neg := &models.InterbankOtcNegotiation{
			NegotiationRoutingNumber: 111, NegotiationID: "neg-nb",
			LocalRole: models.InterbankNegotiationRoleBuyer, CounterpartyRoutingNumber: 111,
			BuyerRoutingNumber: 999, BuyerID: "client-1",
			SellerRoutingNumber: 111, SellerID: "client-9",
			StockTicker: "AAPL", Amount: 4, PricePerUnitCurrency: "USD", PricePerUnitAmount: 10,
			PremiumCurrency: "RSD", PremiumAmount: 4000, SettlementDate: futureSettlement(),
			IsOngoing: true,
		}
		if err := repository.NewInterbankOtcRepository(db).Create(neg); err != nil {
			t.Fatal(err)
		}
		p := newOtcProcessor(db)
		tx := buildOptionAcceptanceTx(111, neg)
		tx.TransactionID = ForeignBankId{RoutingNumber: 111, ID: "t"}
		if v, _ := p.OnNewTx(ctx, partner, &tx); v.Vote != VoteNo {
			t.Fatalf("not-buyer-bank should vote NO, got %+v", v)
		}
	})

	// Terms mismatch (tx premium differs from stored negotiation).
	t.Run("terms_mismatch", func(t *testing.T) {
		db := openInterbankTestDB(t, "otc_terms")
		neg := seedOtcNegotiation(t, db)
		p := newOtcProcessor(db)
		tx := acceptanceTxFor(neg)
		tx.Postings[0].Amount = -3999 // buyer cash leg
		tx.Postings[1].Amount = 3999  // seller cash leg (still balanced)
		if v, _ := p.OnNewTx(ctx, partner, &tx); v.Vote != VoteNo {
			t.Fatalf("terms mismatch should vote NO, got %+v", v)
		}
	})
}

func TestServer_MalformedTypedBody(t *testing.T) {
	srv := newTestServer(t, fakeProcessor{})
	// COMMIT_TX envelope whose body is a JSON array (not a CommitTransaction object).
	body := envelopeBytes(t, 111, "mb1", MessageTypeCommitTx, []int{1, 2, 3})
	rec := postEnvelope(t, srv, "in-key", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed COMMIT_TX body status = %d, want 400", rec.Code)
	}
}

func TestExerciseOnCommit_NonInbound(t *testing.T) {
	db := openInterbankTestDB(t, "ex_noninbound")
	// Insert an outbound pending exercise row.
	row := &models.InterbankPendingExercise{
		TxRoutingNumber: 111, TxID: "out-1", Direction: models.InterbankExerciseDirectionOutbound,
		PartnerRoutingNumber: 111, NegotiationRoutingNumber: 333, NegotiationID: "neg-x",
		StockTicker: "AAPL", StockAmount: 4, PricePerUnitCurrency: "USD", PricePerUnitAmount: 10,
		CashAmount: 40, BuyerRoutingNumber: 111, BuyerID: "emp-1",
		SellerRoutingNumber: 333, SellerID: "client-5",
		Status: models.InterbankExerciseStatusPending,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatal(err)
	}
	p := newExerciseProcessor(db)
	if err := p.OnCommitTx(context.Background(), &PartnerBank{Code: 111}, ForeignBankId{RoutingNumber: 111, ID: "out-1"}); err == nil {
		t.Error("commit on outbound row should error")
	}
	if err := p.OnRollbackTx(context.Background(), &PartnerBank{Code: 111}, ForeignBankId{RoutingNumber: 111, ID: "out-1"}); err == nil {
		t.Error("rollback on outbound row should error")
	}
}
