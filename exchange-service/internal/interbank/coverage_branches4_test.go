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

func TestPaymentProcessor_NonInboundAndResolvedStates(t *testing.T) {
	db := openInterbankTestDB(t, "pay_states")
	// Outbound row → commit/rollback must error (dispatched to wrong side).
	outbound := &models.InterbankPayment{
		TxRoutingNumber: 111, TxID: "out", Direction: models.InterbankPaymentDirectionOutbound,
		PartnerRoutingNumber: 111, SenderAccountNumber: "333000777", RecipientAccountNumber: "111000111",
		Currency: "RSD", Amount: 100, Status: models.InterbankPaymentStatusPending, CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(outbound).Error; err != nil {
		t.Fatal(err)
	}
	// Committed inbound row → rollback must error.
	committed := &models.InterbankPayment{
		TxRoutingNumber: 111, TxID: "done", Direction: models.InterbankPaymentDirectionInbound,
		PartnerRoutingNumber: 111, SenderAccountNumber: "111000111", RecipientAccountNumber: "333000777",
		Currency: "RSD", Amount: 100, Status: models.InterbankPaymentStatusCommitted, CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(committed).Error; err != nil {
		t.Fatal(err)
	}

	p := newPaymentProcessor(db)
	partner := &PartnerBank{Code: 111}
	ctx := context.Background()
	if err := p.OnCommitTx(ctx, partner, ForeignBankId{RoutingNumber: 111, ID: "out"}); err == nil {
		t.Error("commit on outbound should error")
	}
	if err := p.OnRollbackTx(ctx, partner, ForeignBankId{RoutingNumber: 111, ID: "out"}); err == nil {
		t.Error("rollback on outbound should error")
	}
	if err := p.OnRollbackTx(ctx, partner, ForeignBankId{RoutingNumber: 111, ID: "done"}); err == nil {
		t.Error("rollback on committed should error")
	}
	// Committed → commit again is idempotent no-op.
	if err := p.OnCommitTx(ctx, partner, ForeignBankId{RoutingNumber: 111, ID: "done"}); err != nil {
		t.Errorf("commit replay on committed: %v", err)
	}
}

func TestOtcProcessor_CommitRollbackErrorStates(t *testing.T) {
	db := openInterbankTestDB(t, "otc_states")
	p := newOtcProcessor(db)
	partner := &PartnerBank{Code: 111}
	ctx := context.Background()

	// Unknown commit → error.
	if err := p.OnCommitTx(ctx, partner, ForeignBankId{RoutingNumber: 111, ID: "ghost"}); err == nil {
		t.Error("unknown otc commit should error")
	}

	// Committed pending row → rollback must error.
	committed := &models.InterbankPendingTx{
		TxRoutingNumber: 111, TxID: "c", PartnerRoutingNumber: 111,
		NegotiationRoutingNumber: 111, NegotiationID: "neg-1",
		ReservedFromLocalID: "client-1", ReservedCurrency: "RSD", ReservedAmount: 100,
		StockTicker: "AAPL", OptionAmount: 4, PricePerUnitCurrency: "USD", PricePerUnitAmount: 10,
		SettlementDate: futureSettlement(), BuyerRoutingNumber: 333, BuyerID: "client-1",
		SellerRoutingNumber: 111, SellerID: "client-9",
		Status: models.InterbankPendingTxStatusCommitted, CreatedAt: time.Now().UTC(),
	}
	if err := repository.NewInterbankPendingTxRepository(db).Create(committed); err != nil {
		t.Fatal(err)
	}
	if err := p.OnRollbackTx(ctx, partner, ForeignBankId{RoutingNumber: 111, ID: "c"}); err == nil {
		t.Error("rollback on committed otc should error")
	}
	// Commit on committed → idempotent.
	if err := p.OnCommitTx(ctx, partner, ForeignBankId{RoutingNumber: 111, ID: "c"}); err != nil {
		t.Errorf("commit replay: %v", err)
	}

	// Rolled-back row → commit must error, rollback is idempotent.
	rolled := &models.InterbankPendingTx{
		TxRoutingNumber: 111, TxID: "rb", PartnerRoutingNumber: 111,
		NegotiationRoutingNumber: 111, NegotiationID: "neg-1",
		ReservedFromLocalID: "client-1", ReservedCurrency: "RSD", ReservedAmount: 100,
		StockTicker: "AAPL", OptionAmount: 4, PricePerUnitCurrency: "USD", PricePerUnitAmount: 10,
		SettlementDate: futureSettlement(), BuyerRoutingNumber: 333, BuyerID: "client-1",
		SellerRoutingNumber: 111, SellerID: "client-9",
		Status: models.InterbankPendingTxStatusRolledBack, CreatedAt: time.Now().UTC(),
	}
	if err := repository.NewInterbankPendingTxRepository(db).Create(rolled); err != nil {
		t.Fatal(err)
	}
	if err := p.OnCommitTx(ctx, partner, ForeignBankId{RoutingNumber: 111, ID: "rb"}); err == nil {
		t.Error("commit on rolled-back should error")
	}
	if err := p.OnRollbackTx(ctx, partner, ForeignBankId{RoutingNumber: 111, ID: "rb"}); err != nil {
		t.Errorf("rollback replay on rolled-back: %v", err)
	}
}

func TestNegotiations_UpdateLastModifiedMismatch(t *testing.T) {
	db := openInterbankTestDB(t, "neg_lm")
	h := newNegHandler(t, db, nil)
	buyer := &PartnerBank{Code: 111}
	neg := seedOtcNegotiationSeller(t, db, "neg-lm")
	// Flip last mover to seller so the buyer may counter.
	_ = repository.NewInterbankOtcRepository(db).MarkOngoing(333, neg.NegotiationID, 333, "client-5")

	// Counter-offer whose LastModifiedBy routing doesn't match the caller → 403.
	offer := OtcOffer{
		Stock: StockDescription{Ticker: "AAPL"}, SettlementDate: futureSettlement(),
		PricePerUnit: MonetaryValue{Currency: "USD", Amount: 10},
		Premium:      MonetaryValue{Currency: "RSD", Amount: 3000},
		BuyerID:      ForeignBankId{RoutingNumber: 111, ID: "emp-1"},
		SellerID:     ForeignBankId{RoutingNumber: 333, ID: "client-5"},
		Amount:       4, LastModifiedBy: ForeignBankId{RoutingNumber: 222, ID: "x"},
	}
	rec := httptest.NewRecorder()
	h.Item(rec, authedReq(http.MethodPut, "/negotiations/333/"+neg.NegotiationID, buyer, offer))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
