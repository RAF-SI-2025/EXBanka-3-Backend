package interbank

import (
	"bytes"
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

// authedReq builds a request with a *PartnerBank already attached to context,
// as AuthMiddleware would.
func authedReq(method, path string, partner *PartnerBank, body any) *http.Request {
	var r *http.Request
	if body != nil {
		raw, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(raw))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if partner != nil {
		r = r.WithContext(context.WithValue(r.Context(), partnerContextKey{}, partner))
	}
	return r
}

func newNegHandler(t *testing.T, db *gorm.DB, client *Client) *NegotiationsHandler {
	t.Helper()
	reg := testRegistry(t, 333, 111, "http://partner")
	return NewNegotiationsHandler(
		reg,
		repository.NewInterbankOtcRepository(db),
		client,
		db,
		repository.NewInterbankWalletRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
	)
}

func TestNegotiations_CreateReadUpdateDelete(t *testing.T) {
	db := openInterbankTestDB(t, "neg_crud")
	h := newNegHandler(t, db, nil)
	buyer := &PartnerBank{Code: 111}

	offer := OtcOffer{
		Stock:          StockDescription{Ticker: "AAPL"},
		SettlementDate: futureSettlement(),
		PricePerUnit:   MonetaryValue{Currency: "USD", Amount: 10},
		Premium:        MonetaryValue{Currency: "RSD", Amount: 4000},
		BuyerID:        ForeignBankId{RoutingNumber: 111, ID: "emp-1"},
		SellerID:       ForeignBankId{RoutingNumber: 333, ID: "client-5"},
		Amount:         4,
		LastModifiedBy: ForeignBankId{RoutingNumber: 111, ID: "emp-1"},
	}

	// Create.
	rec := httptest.NewRecorder()
	h.Collection(rec, authedReq(http.MethodPost, "/negotiations", buyer, offer))
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created ForeignBankId
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.RoutingNumber != 333 || created.ID == "" {
		t.Fatalf("created id = %+v", created)
	}
	path := "/negotiations/333/" + created.ID

	// Read.
	rec = httptest.NewRecorder()
	h.Item(rec, authedReq(http.MethodGet, path, buyer, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("read status = %d", rec.Code)
	}

	// Delete (close).
	rec = httptest.NewRecorder()
	h.Item(rec, authedReq(http.MethodDelete, path, buyer, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
}

func TestNegotiations_CreateValidation(t *testing.T) {
	db := openInterbankTestDB(t, "neg_create_val")
	h := newNegHandler(t, db, nil)
	buyer := &PartnerBank{Code: 111}

	base := OtcOffer{
		Stock: StockDescription{Ticker: "AAPL"}, SettlementDate: futureSettlement(),
		PricePerUnit: MonetaryValue{Currency: "USD", Amount: 10},
		Premium:      MonetaryValue{Currency: "RSD", Amount: 4000},
		BuyerID:      ForeignBankId{RoutingNumber: 111, ID: "emp-1"},
		SellerID:     ForeignBankId{RoutingNumber: 333, ID: "client-5"},
		Amount:       4, LastModifiedBy: ForeignBankId{RoutingNumber: 111, ID: "emp-1"},
	}

	// No partner in context.
	rec := httptest.NewRecorder()
	h.create(rec, authedReq(http.MethodPost, "/negotiations", nil, base))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no partner status = %d", rec.Code)
	}

	// Seller routing not us.
	bad := base
	bad.SellerID.RoutingNumber = 999
	rec = httptest.NewRecorder()
	h.Collection(rec, authedReq(http.MethodPost, "/negotiations", buyer, bad))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("wrong seller routing status = %d", rec.Code)
	}

	// Buyer routing != partner.
	bad = base
	bad.BuyerID.RoutingNumber = 222
	rec = httptest.NewRecorder()
	h.Collection(rec, authedReq(http.MethodPost, "/negotiations", buyer, bad))
	if rec.Code != http.StatusForbidden {
		t.Errorf("wrong buyer routing status = %d", rec.Code)
	}

	// LastModifiedBy != partner.
	bad = base
	bad.LastModifiedBy.RoutingNumber = 222
	rec = httptest.NewRecorder()
	h.Collection(rec, authedReq(http.MethodPost, "/negotiations", buyer, bad))
	if rec.Code != http.StatusForbidden {
		t.Errorf("wrong lastmodified status = %d", rec.Code)
	}

	// Malformed JSON.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/negotiations", bytes.NewReader([]byte("not json")))
	req = req.WithContext(context.WithValue(req.Context(), partnerContextKey{}, buyer))
	h.create(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed body status = %d", rec.Code)
	}

	// Wrong method on collection.
	rec = httptest.NewRecorder()
	h.Collection(rec, authedReq(http.MethodGet, "/negotiations", buyer, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET collection status = %d", rec.Code)
	}
}

func TestNegotiations_ItemRoutingAndAuthz(t *testing.T) {
	db := openInterbankTestDB(t, "neg_item")
	h := newNegHandler(t, db, nil)
	buyer := &PartnerBank{Code: 111}
	stranger := &PartnerBank{Code: 222}

	// Seed a negotiation owned by 333 between buyer 111 and seller 333.
	neg := seedOtcNegotiationSeller(t, db, "neg-i")

	// Not found.
	rec := httptest.NewRecorder()
	h.Item(rec, authedReq(http.MethodGet, "/negotiations/333/missing", buyer, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing status = %d", rec.Code)
	}

	// Stranger forbidden.
	rec = httptest.NewRecorder()
	h.Item(rec, authedReq(http.MethodGet, "/negotiations/333/"+neg.NegotiationID, stranger, nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("stranger status = %d", rec.Code)
	}

	// Bad path shapes.
	for _, p := range []string{"/wrong/333/x", "/negotiations/abc/x", "/negotiations/333/x/bogus", "/negotiations/333"} {
		rec = httptest.NewRecorder()
		h.Item(rec, authedReq(http.MethodGet, p, buyer, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("path %q unexpectedly OK", p)
		}
	}

	// Unsupported method on item.
	rec = httptest.NewRecorder()
	h.Item(rec, authedReq(http.MethodPatch, "/negotiations/333/"+neg.NegotiationID, buyer, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("PATCH status = %d", rec.Code)
	}
}

func TestNegotiations_UpdateTurnAndClosed(t *testing.T) {
	db := openInterbankTestDB(t, "neg_update")
	h := newNegHandler(t, db, nil)
	buyer := &PartnerBank{Code: 111}

	neg := seedOtcNegotiationSeller(t, db, "neg-u")
	path := "/negotiations/333/" + neg.NegotiationID
	offer := OtcOffer{
		Stock: StockDescription{Ticker: "AAPL"}, SettlementDate: futureSettlement(),
		PricePerUnit: MonetaryValue{Currency: "USD", Amount: 10},
		Premium:      MonetaryValue{Currency: "RSD", Amount: 3000},
		BuyerID:      ForeignBankId{RoutingNumber: 111, ID: "emp-1"},
		SellerID:     ForeignBankId{RoutingNumber: 333, ID: "client-5"},
		Amount:       4, LastModifiedBy: ForeignBankId{RoutingNumber: 111, ID: "emp-1"},
	}

	// neg seeded with LastModifiedBy=111 (buyer). Buyer countering again → not their turn → 409.
	rec := httptest.NewRecorder()
	h.Item(rec, authedReq(http.MethodPut, path, buyer, offer))
	if rec.Code != http.StatusConflict {
		t.Fatalf("out-of-turn update status = %d", rec.Code)
	}

	// Flip last mover to the seller (333); now the buyer may counter → 200.
	if err := repository.NewInterbankOtcRepository(db).MarkOngoing(333, neg.NegotiationID, 333, "client-5"); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.Item(rec, authedReq(http.MethodPut, path, buyer, offer))
	if rec.Code != http.StatusOK {
		t.Fatalf("in-turn update status = %d body=%s", rec.Code, rec.Body.String())
	}

	// Close it, then update → 409 not ongoing.
	_ = repository.NewInterbankOtcRepository(db).MarkClosed(333, neg.NegotiationID)
	rec = httptest.NewRecorder()
	h.Item(rec, authedReq(http.MethodPut, path, buyer, offer))
	if rec.Code != http.StatusConflict {
		t.Fatalf("closed update status = %d", rec.Code)
	}
}

// seedOtcNegotiationSeller seeds a seller-role negotiation (we are the
// seller's bank, 333) with the given id. LastModifiedBy is the buyer (111).
func seedOtcNegotiationSeller(t *testing.T, db *gorm.DB, id string) *models.InterbankOtcNegotiation {
	t.Helper()
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber:  333,
		NegotiationID:             id,
		LocalRole:                 models.InterbankNegotiationRoleSeller,
		CounterpartyRoutingNumber: 111,
		BuyerRoutingNumber:        111,
		BuyerID:                   "emp-1",
		SellerRoutingNumber:       333,
		SellerID:                  "client-5",
		StockTicker:               "AAPL",
		Amount:                    4,
		PricePerUnitCurrency:      "USD",
		PricePerUnitAmount:        10,
		PremiumCurrency:           "USD",
		PremiumAmount:             5,
		SettlementDate:            futureSettlement(),
		LastModifiedByRoutingNumber: 111,
		LastModifiedByID:          "emp-1",
		IsOngoing:                 true,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	}
	if err := repository.NewInterbankOtcRepository(db).Create(neg); err != nil {
		t.Fatalf("seed seller neg: %v", err)
	}
	return neg
}

// ---------------------------------------------------------------------------
// accept() / AcceptForLocalSeller — full dispatch against a fake buyer bank
// ---------------------------------------------------------------------------

func TestAcceptForLocalSeller_HappyPath(t *testing.T) {
	db := openInterbankTestDB(t, "accept_happy")

	// Fake buyer's bank: votes YES on NEW_TX, 204 on COMMIT_TX.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env Message
		_ = json.NewDecoder(r.Body).Decode(&env)
		switch env.MessageType {
		case MessageTypeNewTx:
			_ = json.NewEncoder(w).Encode(TransactionVote{Vote: VoteYes})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)

	reg := testRegistry(t, 333, 111, srv.URL)
	client := NewClient(reg, WithHTTPClient(srv.Client()), WithSleepFunc(func(time.Duration) {}))
	h := NewNegotiationsHandler(reg,
		repository.NewInterbankOtcRepository(db), client, db,
		repository.NewInterbankWalletRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
	)

	neg := seedOtcNegotiationSeller(t, db, "neg-accept")
	assetID := seedListing(t, db, "AAPL")
	usd := seedCurrency(t, db, "USD")
	seedClientAccount(t, db, 5, usd, "333000555", 0, 0)
	holding := models.PortfolioHoldingRecord{UserID: 5, UserType: "client", AssetID: assetID, Quantity: 10, PublicQuantity: 10, IsPublic: true, AvgBuyPrice: 8, AccountID: 1}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	outcome, status, msg := h.AcceptForLocalSeller(context.Background(), 333, neg.NegotiationID, "client-5")
	if status != 0 {
		t.Fatalf("precondition failed: status=%d msg=%s", status, msg)
	}
	if outcome.Vote == nil || outcome.Vote.Vote != VoteYes {
		t.Fatalf("vote = %+v", outcome.Vote)
	}
	if outcome.DispatchErr != nil || outcome.CommitErr != nil {
		t.Fatalf("dispatch=%v commit=%v", outcome.DispatchErr, outcome.CommitErr)
	}
	// Seller credited the premium (USD 5).
	var stanje float64
	db.Raw(`SELECT stanje FROM accounts WHERE client_id=5`).Scan(&stanje)
	if stanje != 5 {
		t.Fatalf("seller stanje = %v, want 5", stanje)
	}
	// Stock reserved (4) by closeAndReserveSeller.
	var reserved float64
	db.Raw(`SELECT reserved_quantity FROM portfolio_holdings WHERE user_id=5`).Scan(&reserved)
	if reserved != 4 {
		t.Fatalf("reserved = %v, want 4", reserved)
	}
}

func TestAcceptForLocalSeller_Preconditions(t *testing.T) {
	db := openInterbankTestDB(t, "accept_pre")
	h := newNegHandler(t, db, nil)

	// Not found.
	if _, status, _ := h.AcceptForLocalSeller(context.Background(), 333, "ghost", "client-5"); status != http.StatusNotFound {
		t.Errorf("missing neg status = %d", status)
	}

	neg := seedOtcNegotiationSeller(t, db, "neg-pre")
	// Wrong seller id.
	if _, status, _ := h.AcceptForLocalSeller(context.Background(), 333, neg.NegotiationID, "client-99"); status != http.StatusForbidden {
		t.Errorf("wrong seller status = %d", status)
	}
	// Closed.
	_ = repository.NewInterbankOtcRepository(db).MarkClosed(333, neg.NegotiationID)
	if _, status, _ := h.AcceptForLocalSeller(context.Background(), 333, neg.NegotiationID, "client-5"); status != http.StatusConflict {
		t.Errorf("closed status = %d", status)
	}

	// Buyer-role negotiation can't be accepted locally as seller.
	buyerNeg := seedOtcNegotiation(t, db) // LocalRole=buyer
	if _, status, _ := h.AcceptForLocalSeller(context.Background(), 111, buyerNeg.NegotiationID, "client-1"); status != http.StatusForbidden {
		t.Errorf("buyer-role status = %d", status)
	}
}

func TestAccept_PartnerTriggeredAuthz(t *testing.T) {
	db := openInterbankTestDB(t, "accept_partner")
	h := newNegHandler(t, db, nil)
	buyer := &PartnerBank{Code: 111}

	// no partner.
	rec := httptest.NewRecorder()
	h.accept(rec, authedReq(http.MethodGet, "/x", nil, nil), 333, "x")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no partner status = %d", rec.Code)
	}

	// not found.
	rec = httptest.NewRecorder()
	h.accept(rec, authedReq(http.MethodGet, "/x", buyer, nil), 333, "ghost")
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing status = %d", rec.Code)
	}

	// closed → 409.
	neg := seedOtcNegotiationSeller(t, db, "neg-pa")
	_ = repository.NewInterbankOtcRepository(db).MarkClosed(333, neg.NegotiationID)
	rec = httptest.NewRecorder()
	h.accept(rec, authedReq(http.MethodGet, "/x", buyer, nil), 333, neg.NegotiationID)
	if rec.Code != http.StatusConflict {
		t.Errorf("closed accept status = %d", rec.Code)
	}

	// ongoing but buyer made last move → not their turn → 409.
	neg2 := seedOtcNegotiationSeller(t, db, "neg-pa2")
	rec = httptest.NewRecorder()
	h.accept(rec, authedReq(http.MethodGet, "/x", buyer, nil), 333, neg2.NegotiationID)
	if rec.Code != http.StatusConflict {
		t.Errorf("out-of-turn accept status = %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// otc_handler.go
// ---------------------------------------------------------------------------

func TestEncodeDecodeLocalParticipantID(t *testing.T) {
	if got := EncodeLocalParticipantID(LocalParticipantClient, 7); got != "client-7" {
		t.Errorf("encode = %q", got)
	}
	pt, id, err := DecodeLocalParticipantID("client-7")
	if err != nil || pt != LocalParticipantClient || id != 7 {
		t.Errorf("decode = %v %d %v", pt, id, err)
	}
	if _, _, err := DecodeLocalParticipantID("nodash"); err == nil {
		t.Error("missing dash should error")
	}
	if _, _, err := DecodeLocalParticipantID("client-xyz"); err == nil {
		t.Error("non-numeric should error")
	}
}

func TestStubDisplayNameResolver(t *testing.T) {
	r := StubDisplayNameResolver{}
	if n, _ := r.ResolveDisplayName(LocalParticipantClient, 3); n != "Client #3" {
		t.Errorf("client = %q", n)
	}
	if n, _ := r.ResolveDisplayName(LocalParticipantBank, 0); n != "EXBanka" {
		t.Errorf("bank = %q", n)
	}
	if _, err := r.ResolveDisplayName("weird", 0); err == nil {
		t.Error("unknown type should error")
	}
}

func TestOTCHandler_PublicStock(t *testing.T) {
	db := openInterbankTestDB(t, "otc_public")
	assetID := seedListing(t, db, "AAPL")
	holding := models.PortfolioHoldingRecord{
		UserID: 5, UserType: "client", AssetID: assetID,
		Quantity: 10, PublicQuantity: 6, ReservedQuantity: 1, AvgBuyPrice: 8, AccountID: 1,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}
	reg := testRegistry(t, 333, 111, "http://partner")
	h := NewOTCHandler(reg, repository.NewPortfolioRepository(db), StubDisplayNameResolver{})

	rec := httptest.NewRecorder()
	h.PublicStock(rec, httptest.NewRequest(http.MethodGet, "/public-stock", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out PublicStocksResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out) != 1 || out[0].Stock.Ticker != "AAPL" {
		t.Fatalf("out = %+v", out)
	}
	if out[0].Sellers[0].Amount != 5 { // 6 public - 1 reserved
		t.Fatalf("amount = %v, want 5", out[0].Sellers[0].Amount)
	}

	// Wrong method.
	rec = httptest.NewRecorder()
	h.PublicStock(rec, httptest.NewRequest(http.MethodPost, "/public-stock", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d", rec.Code)
	}
}

func TestOTCHandler_UserInfo(t *testing.T) {
	db := openInterbankTestDB(t, "otc_userinfo")
	reg := testRegistry(t, 333, 111, "http://partner")
	h := NewOTCHandler(reg, repository.NewPortfolioRepository(db), StubDisplayNameResolver{})

	// Happy path.
	rec := httptest.NewRecorder()
	h.UserInfo(rec, httptest.NewRequest(http.MethodGet, "/user/333/client-7", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var info UserInformation
	_ = json.Unmarshal(rec.Body.Bytes(), &info)
	if info.BankDisplayName != "EXBanka" || info.DisplayName != "Client #7" {
		t.Fatalf("info = %+v", info)
	}

	// Wrong method.
	rec = httptest.NewRecorder()
	h.UserInfo(rec, httptest.NewRequest(http.MethodPost, "/user/333/client-7", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d", rec.Code)
	}

	// Bad path.
	rec = httptest.NewRecorder()
	h.UserInfo(rec, httptest.NewRequest(http.MethodGet, "/user/333", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad path status = %d", rec.Code)
	}

	// Non-numeric routing.
	rec = httptest.NewRecorder()
	h.UserInfo(rec, httptest.NewRequest(http.MethodGet, "/user/abc/client-7", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric routing status = %d", rec.Code)
	}

	// Foreign routing → 404.
	rec = httptest.NewRecorder()
	h.UserInfo(rec, httptest.NewRequest(http.MethodGet, "/user/111/client-7", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("foreign routing status = %d", rec.Code)
	}

	// Bad id → 400.
	rec = httptest.NewRecorder()
	h.UserInfo(rec, httptest.NewRequest(http.MethodGet, "/user/333/nodash", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad id status = %d", rec.Code)
	}
}

func TestBankDisplayNameForRouting(t *testing.T) {
	reg := testRegistry(t, 333, 111, "http://partner")
	if got := bankDisplayNameForRouting(reg, 333); got != "EXBanka" {
		t.Errorf("own = %q", got)
	}
	if got := bankDisplayNameForRouting(reg, 111); got != "Partner 111" {
		t.Errorf("partner = %q", got)
	}
	if got := bankDisplayNameForRouting(reg, 999); got != "Bank 999" {
		t.Errorf("unknown = %q", got)
	}
}

func TestPathPair(t *testing.T) {
	if a, b, ok := pathPair("/user/333/client-7", "/user/"); !ok || a != "333" || b != "client-7" {
		t.Errorf("got %q %q %v", a, b, ok)
	}
	if _, _, ok := pathPair("/other/x/y", "/user/"); ok {
		t.Error("wrong prefix should fail")
	}
	if _, _, ok := pathPair("/user/onlyone", "/user/"); ok {
		t.Error("single segment should fail")
	}
}
