package handler

import (
	"errors"
	"net/http"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

// =====================================================================
// parseRoutingAndID
// =====================================================================

func TestParseRoutingAndID(t *testing.T) {
	r, id, ok := parseRoutingAndID("123", "abc")
	if !ok || int(r) != 123 || id != "abc" {
		t.Fatalf("happy: routing=%d id=%q ok=%v", r, id, ok)
	}
	if _, _, ok := parseRoutingAndID("", "abc"); ok {
		t.Fatal("empty routing should fail")
	}
	if _, _, ok := parseRoutingAndID("123", ""); ok {
		t.Fatal("empty id should fail")
	}
	if _, _, ok := parseRoutingAndID("notnumeric", "abc"); ok {
		t.Fatal("non-numeric routing should fail")
	}
}

// =====================================================================
// statusFromRemoteError
// =====================================================================

func TestStatusFromRemoteError(t *testing.T) {
	if got := statusFromRemoteError(errors.New("plain")); got != http.StatusBadGateway {
		t.Fatalf("plain: %d", got)
	}
	rerr := &interbank.RemoteError{StatusCode: 404}
	if got := statusFromRemoteError(rerr); got != 404 {
		t.Fatalf("404: %d", got)
	}
	rerr500 := &interbank.RemoteError{StatusCode: 500}
	if got := statusFromRemoteError(rerr500); got != http.StatusBadGateway {
		t.Fatalf("500 should map to BadGateway, got %d", got)
	}
}

// =====================================================================
// localUserIsParty
// =====================================================================

func TestLocalUserIsParty(t *testing.T) {
	buyerRow := &models.InterbankOtcNegotiation{
		LocalRole: models.InterbankNegotiationRoleBuyer,
		BuyerID:   "B-1",
	}
	if !localUserIsParty(buyerRow, "B-1") {
		t.Fatal("buyer with matching id should be party")
	}
	if localUserIsParty(buyerRow, "X") {
		t.Fatal("buyer with non-matching id should not be party")
	}
	sellerRow := &models.InterbankOtcNegotiation{
		LocalRole: models.InterbankNegotiationRoleSeller,
		SellerID:  "S-1",
	}
	if !localUserIsParty(sellerRow, "S-1") {
		t.Fatal("seller with matching id should be party")
	}
	unknownRow := &models.InterbankOtcNegotiation{LocalRole: "unknown"}
	if localUserIsParty(unknownRow, "anything") {
		t.Fatal("unknown role should never be party")
	}
}

// =====================================================================
// summariseNoVote (interbank_payment_http_handler.go)
// =====================================================================

func TestSummariseNoVote_HandlerVariant(t *testing.T) {
	if got := summariseNoVote(nil); got != "partner voted NO without reason" {
		t.Fatalf("nil: %q", got)
	}
	if got := summariseNoVote(&interbank.TransactionVote{}); got != "partner voted NO without reason" {
		t.Fatalf("no reasons: %q", got)
	}
	vote := &interbank.TransactionVote{
		Reasons: []interbank.NoVoteReason{{Reason: "FOO"}, {Reason: "BAR"}},
	}
	got := summariseNoVote(vote)
	if got == "" || got == "partner voted NO without reason" {
		t.Fatalf("unexpected summary: %q", got)
	}
}

// =====================================================================
// parseContractSettlement
// =====================================================================

func TestParseContractSettlement(t *testing.T) {
	if _, ok := parseContractSettlement("2030-01-15T00:00:00Z"); !ok {
		t.Fatal("RFC3339 should parse")
	}
	if _, ok := parseContractSettlement("2030-01-15"); !ok {
		t.Fatal("date-only should parse")
	}
	if _, ok := parseContractSettlement("garbage"); ok {
		t.Fatal("garbage should fail")
	}
}

// =====================================================================
// negotiationRowToResponse / optionContractRowToResponse
// =====================================================================

func TestNegotiationRowToResponse(t *testing.T) {
	row := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 111, NegotiationID: "N-1",
		LocalRole:                 models.InterbankNegotiationRoleBuyer,
		CounterpartyRoutingNumber: 222,
		BuyerRoutingNumber:        111, BuyerID: "B-1",
		SellerRoutingNumber: 222, SellerID: "S-1",
		StockTicker: "AAPL", Amount: 10,
	}
	out := negotiationRowToResponse(row)
	if out["negotiationId"].(string) != "N-1" {
		t.Fatalf("expected N-1, got %v", out["negotiationId"])
	}
	if out["localRole"].(string) != models.InterbankNegotiationRoleBuyer {
		t.Fatalf("expected buyer role")
	}
}

func TestOptionContractRowToResponse(t *testing.T) {
	c := &models.InterbankOptionContract{
		ID: 1, NegotiationRoutingNumber: 111, NegotiationID: "N-1",
		BuyerLocalID:        "B-1",
		SellerRoutingNumber: 222, SellerID: "S-1",
		StockTicker:          "AAPL", Amount: 10,
		PricePerUnitCurrency: "USD", PricePerUnitAmount: 100,
		PremiumCurrency: "USD", PremiumAmount: 5,
		Status: "valid",
	}
	out := optionContractRowToResponse(c)
	if out["id"].(uint) != 1 {
		t.Fatalf("expected id=1, got %v", out["id"])
	}
}

// =====================================================================
// errBadSettlementDate + badRequestError
// =====================================================================

func TestErrBadSettlementDate(t *testing.T) {
	err := errBadSettlementDate()
	if err == nil || err.Error() == "" {
		t.Fatal("expected non-empty error")
	}
}

// =====================================================================
// sagaTransactionToResponse
// =====================================================================

func TestSagaTransactionToResponse(t *testing.T) {
	saga := models.SagaTransactionRecord{
		ID: 1, Type: "otc", Status: "completed", CurrentStep: 2,
		Steps: []models.SagaStepRecord{
			{ID: 1, SagaID: 1, StepNumber: 1, StepName: "reserve", Status: "completed"},
			{ID: 2, SagaID: 1, StepNumber: 2, StepName: "transfer", Status: "completed"},
		},
	}
	out := sagaTransactionToResponse(saga)
	if out.ID != 1 || len(out.Steps) != 2 {
		t.Fatalf("expected 2 steps in id=1, got %+v", out)
	}
}

// =====================================================================
// WithFundService / WithSagaQuerier (single-line wrappers)
// =====================================================================

func TestOrderHTTP_WithFundService_Chains(t *testing.T) {
	h := &OrderHTTPHandler{}
	if got := h.WithFundService(nil); got != h {
		t.Fatal("expected self-return for chaining")
	}
}

func TestOtcHTTP_WithSagaQuerier_Chains(t *testing.T) {
	h := &OtcHTTPHandler{}
	if got := h.WithSagaQuerier(nil); got != h {
		t.Fatal("expected self-return for chaining")
	}
}
