package interbank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"gorm.io/gorm"
)

// acceptHandlerWithBank builds a NegotiationsHandler whose outbound client
// talks to the given fake-bank handler (the counterparty).
func acceptHandlerWithBank(t *testing.T, db *gorm.DB, bank http.Handler) *NegotiationsHandler {
	t.Helper()
	srv := httptest.NewServer(bank)
	t.Cleanup(srv.Close)
	reg := testRegistry(t, 333, 111, srv.URL)
	client := NewClient(reg, WithHTTPClient(srv.Client()),
		WithRetryPolicy([]time.Duration{time.Millisecond}),
		WithSleepFunc(func(time.Duration) {}))
	return NewNegotiationsHandler(reg,
		repository.NewInterbankOtcRepository(db), client, db,
		repository.NewInterbankWalletRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
	)
}

func seedSellerHoldingAndAccount(t *testing.T, db *gorm.DB) {
	t.Helper()
	assetID := seedListing(t, db, "AAPL")
	usd := seedCurrency(t, db, "USD")
	seedClientAccount(t, db, 5, usd, "333000555", 0, 0)
	holding := models.PortfolioHoldingRecord{
		UserID: 5, UserType: "client", AssetID: assetID,
		Quantity: 10, PublicQuantity: 10, IsPublic: true, AvgBuyPrice: 8, AccountID: 1,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}
}

func TestAccept_NoVote_ReopensAndReleases(t *testing.T) {
	db := openInterbankTestDB(t, "accept_novote")
	h := acceptHandlerWithBank(t, db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env Message
		_ = json.NewDecoder(r.Body).Decode(&env)
		if env.MessageType == MessageTypeNewTx {
			_ = json.NewEncoder(w).Encode(TransactionVote{Vote: VoteNo, Reasons: []NoVoteReason{{Reason: ReasonInsufficientAsset}}})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	neg := seedOtcNegotiationSeller(t, db, "neg-no")
	seedSellerHoldingAndAccount(t, db)

	outcome, status, _ := h.AcceptForLocalSeller(context.Background(), 333, neg.NegotiationID, "client-5")
	if status != 0 || outcome.Vote == nil || outcome.Vote.Vote != VoteNo {
		t.Fatalf("outcome=%+v status=%d", outcome, status)
	}
	// Negotiation reopened and reservation released.
	got, _ := repository.NewInterbankOtcRepository(db).Get(333, neg.NegotiationID)
	if !got.IsOngoing {
		t.Error("negotiation should be reopened after NO vote")
	}
	var reserved float64
	db.Raw(`SELECT reserved_quantity FROM portfolio_holdings WHERE user_id=5`).Scan(&reserved)
	if reserved != 0 {
		t.Errorf("reserved = %v, want 0 (released)", reserved)
	}
}

func TestAccept_DispatchFailure_Reopens(t *testing.T) {
	db := openInterbankTestDB(t, "accept_dispatchfail")
	h := acceptHandlerWithBank(t, db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	neg := seedOtcNegotiationSeller(t, db, "neg-fail")
	seedSellerHoldingAndAccount(t, db)

	outcome, status, _ := h.AcceptForLocalSeller(context.Background(), 333, neg.NegotiationID, "client-5")
	if status != 0 || outcome.DispatchErr == nil {
		t.Fatalf("expected dispatch error, got %+v status=%d", outcome, status)
	}
	got, _ := repository.NewInterbankOtcRepository(db).Get(333, neg.NegotiationID)
	if !got.IsOngoing {
		t.Error("negotiation should be reopened after dispatch failure")
	}
}

func TestAccept_CommitFailure_StaysClosed(t *testing.T) {
	db := openInterbankTestDB(t, "accept_commitfail")
	h := acceptHandlerWithBank(t, db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env Message
		_ = json.NewDecoder(r.Body).Decode(&env)
		if env.MessageType == MessageTypeNewTx {
			_ = json.NewEncoder(w).Encode(TransactionVote{Vote: VoteYes})
			return
		}
		// Fail the COMMIT_TX.
		http.Error(w, "commit boom", http.StatusInternalServerError)
	}))
	neg := seedOtcNegotiationSeller(t, db, "neg-commitfail")
	seedSellerHoldingAndAccount(t, db)

	outcome, status, _ := h.AcceptForLocalSeller(context.Background(), 333, neg.NegotiationID, "client-5")
	if status != 0 || outcome.Vote == nil || outcome.Vote.Vote != VoteYes || outcome.CommitErr == nil {
		t.Fatalf("expected YES + commit error, got %+v status=%d", outcome, status)
	}
	got, _ := repository.NewInterbankOtcRepository(db).Get(333, neg.NegotiationID)
	if got.IsOngoing {
		t.Error("negotiation should stay closed after YES + commit failure")
	}
}

// Buyer-role accept (we are the buyer's bank) exercises closeAndReserveBuyerPremium
// and FinaliseBuyerAcceptCommit.
func TestAccept_BuyerRole_HappyPath(t *testing.T) {
	db := openInterbankTestDB(t, "accept_buyer")
	h := acceptHandlerWithBank(t, db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env Message
		_ = json.NewDecoder(r.Body).Decode(&env)
		if env.MessageType == MessageTypeNewTx {
			_ = json.NewEncoder(w).Encode(TransactionVote{Vote: VoteYes})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// Buyer-role negotiation: we (333) are the buyer's bank; seller is 111.
	// Last mover is us (buyer 333) so the calling partner (seller 111) may accept.
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 111, NegotiationID: "neg-buyer",
		LocalRole: models.InterbankNegotiationRoleBuyer, CounterpartyRoutingNumber: 111,
		BuyerRoutingNumber: 333, BuyerID: "client-1",
		SellerRoutingNumber: 111, SellerID: "emp-9",
		StockTicker: "AAPL", Amount: 4,
		PricePerUnitCurrency: "USD", PricePerUnitAmount: 10,
		PremiumCurrency: "RSD", PremiumAmount: 4000,
		SettlementDate:              futureSettlement(),
		LastModifiedByRoutingNumber: 333, LastModifiedByID: "client-1",
		IsOngoing: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repository.NewInterbankOtcRepository(db).Create(neg); err != nil {
		t.Fatal(err)
	}
	rsd := seedCurrency(t, db, "RSD")
	seedClientAccount(t, db, 1, rsd, "333000111", 10000, 10000)

	seller := &PartnerBank{Code: 111}
	rec := httptest.NewRecorder()
	h.accept(rec, authedReq(http.MethodGet, "/x", seller, nil), 111, "neg-buyer")
	if rec.Code != http.StatusOK {
		t.Fatalf("buyer accept status = %d body=%s", rec.Code, rec.Body.String())
	}
	// Buyer premium debited (stanje 10000-4000) and option contract created.
	var stanje float64
	db.Raw(`SELECT stanje FROM accounts WHERE client_id=1`).Scan(&stanje)
	if stanje != 6000 {
		t.Fatalf("buyer stanje = %v, want 6000", stanje)
	}
	if c, _ := repository.NewInterbankOptionContractRepository(db).Get(111, "neg-buyer"); c == nil {
		t.Fatal("buyer option contract not created")
	}
}

func TestAccept_BuyerRole_InsufficientPremium(t *testing.T) {
	db := openInterbankTestDB(t, "accept_buyer_insufficient")
	h := acceptHandlerWithBank(t, db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(TransactionVote{Vote: VoteYes})
	}))
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 111, NegotiationID: "neg-poor",
		LocalRole: models.InterbankNegotiationRoleBuyer, CounterpartyRoutingNumber: 111,
		BuyerRoutingNumber: 333, BuyerID: "client-1",
		SellerRoutingNumber: 111, SellerID: "emp-9",
		StockTicker: "AAPL", Amount: 4,
		PricePerUnitCurrency: "USD", PricePerUnitAmount: 10,
		PremiumCurrency: "RSD", PremiumAmount: 4000,
		SettlementDate:              futureSettlement(),
		LastModifiedByRoutingNumber: 333, LastModifiedByID: "client-1",
		IsOngoing: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repository.NewInterbankOtcRepository(db).Create(neg); err != nil {
		t.Fatal(err)
	}
	rsd := seedCurrency(t, db, "RSD")
	seedClientAccount(t, db, 1, rsd, "333000111", 100, 100) // not enough to reserve 4000

	seller := &PartnerBank{Code: 111}
	rec := httptest.NewRecorder()
	h.accept(rec, authedReq(http.MethodGet, "/x", seller, nil), 111, "neg-poor")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (reserve failed before dispatch)", rec.Code)
	}
}

// Finalise helpers are silent no-ops with nil db/negRepo.
func TestFinaliseHelpers_NilNoop(t *testing.T) {
	neg := &models.InterbankOtcNegotiation{NegotiationRoutingNumber: 1, NegotiationID: "x"}
	if err := FinaliseAcceptCommit(nil, nil, nil, neg); err != nil {
		t.Error(err)
	}
	if err := FinaliseBuyerAcceptCommit(nil, nil, nil, neg); err != nil {
		t.Error(err)
	}
}
