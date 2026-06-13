package interbank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	reg := testRegistry(t, 333, 111, srv.URL)
	c := NewClient(reg,
		WithHTTPClient(srv.Client()),
		WithRetryPolicy([]time.Duration{time.Millisecond, time.Millisecond}),
		WithSleepFunc(func(time.Duration) {}),
	)
	return c, srv
}

func TestClient_NewIdempotenceKey(t *testing.T) {
	reg := testRegistry(t, 333, 111, "http://x")
	c := NewClient(reg)
	k := c.NewIdempotenceKey()
	if k.RoutingNumber != 333 || k.LocallyGeneratedKey == "" {
		t.Errorf("bad key: %+v", k)
	}
}

func TestClient_SendNewTx_Yes(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderAPIKey) != "out-key" {
			t.Errorf("missing outbound key, got %q", r.Header.Get(HeaderAPIKey))
		}
		_ = json.NewEncoder(w).Encode(TransactionVote{Vote: VoteYes})
	}))
	vote, err := c.SendNewTx(context.Background(), 111, c.NewIdempotenceKey(), &Transaction{TransactionID: ForeignBankId{RoutingNumber: 333, ID: "t1"}})
	if err != nil || vote.Vote != VoteYes {
		t.Fatalf("vote=%+v err=%v", vote, err)
	}
}

func TestClient_SendNewTx_AcceptedThenYes(t *testing.T) {
	var calls int
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		_ = json.NewEncoder(w).Encode(TransactionVote{Vote: VoteYes})
	}))
	vote, err := c.SendNewTx(context.Background(), 111, c.NewIdempotenceKey(), &Transaction{})
	if err != nil || vote.Vote != VoteYes {
		t.Fatalf("vote=%+v err=%v calls=%d", vote, err, calls)
	}
	if calls < 2 {
		t.Errorf("expected retry, calls=%d", calls)
	}
}

func TestClient_SendNewTx_AcceptedTimeout(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	_, err := c.SendNewTx(context.Background(), 111, c.NewIdempotenceKey(), &Transaction{})
	if err != ErrAcceptedTimeout {
		t.Fatalf("err=%v, want ErrAcceptedTimeout", err)
	}
}

func TestClient_SendNewTx_RemoteError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	_, err := c.SendNewTx(context.Background(), 111, c.NewIdempotenceKey(), &Transaction{})
	if _, ok := err.(*RemoteError); !ok {
		t.Fatalf("err=%v, want *RemoteError", err)
	}
}

func TestClient_SendNewTx_EmptyBody(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if _, err := c.SendNewTx(context.Background(), 111, c.NewIdempotenceKey(), &Transaction{}); err == nil {
		t.Fatal("expected error on empty NEW_TX body")
	}
}

func TestClient_SendNewTx_NoPartner(t *testing.T) {
	reg := testRegistry(t, 333, 111, "http://x")
	c := NewClient(reg)
	if _, err := c.SendNewTx(context.Background(), 999, c.NewIdempotenceKey(), &Transaction{}); err == nil {
		t.Fatal("expected ErrNoSuchPartner")
	}
}

func TestClient_SendCommitAndRollback(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	id := ForeignBankId{RoutingNumber: 333, ID: "t1"}
	if err := c.SendCommitTx(context.Background(), 111, c.NewIdempotenceKey(), id); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := c.SendRollbackTx(context.Background(), 111, c.NewIdempotenceKey(), id); err != nil {
		t.Fatalf("rollback: %v", err)
	}
}

func TestClient_OTCMethods(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/public-stock":
			_ = json.NewEncoder(w).Encode(PublicStocksResponse{{Stock: StockDescription{Ticker: "AAPL"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/negotiations":
			_ = json.NewEncoder(w).Encode(ForeignBankId{RoutingNumber: 111, ID: "neg-1"})
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(OtcNegotiation{IsOngoing: true})
		case r.Method == http.MethodPut:
			_ = json.NewEncoder(w).Encode(OtcNegotiation{IsOngoing: false})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	ctx := context.Background()
	if ps, err := c.FetchPublicStock(ctx, 111); err != nil || len(ps) != 1 {
		t.Fatalf("FetchPublicStock: %+v %v", ps, err)
	}
	if id, err := c.CreateNegotiation(ctx, 111, OtcOffer{}); err != nil || id.ID != "neg-1" {
		t.Fatalf("CreateNegotiation: %+v %v", id, err)
	}
	if neg, err := c.GetNegotiation(ctx, 111, 111, "neg-1"); err != nil || !neg.IsOngoing {
		t.Fatalf("GetNegotiation: %+v %v", neg, err)
	}
	if neg, err := c.UpdateNegotiation(ctx, 111, 111, "neg-1", OtcOffer{}); err != nil || neg.IsOngoing {
		t.Fatalf("UpdateNegotiation: %+v %v", neg, err)
	}
	if err := c.CloseNegotiation(ctx, 111, 111, "neg-1"); err != nil {
		t.Fatalf("CloseNegotiation: %v", err)
	}
}

func TestClient_doJSON_NoPartnerAndRemoteError(t *testing.T) {
	reg := testRegistry(t, 333, 111, "http://x")
	c := NewClient(reg)
	if _, err := c.FetchPublicStock(context.Background(), 999); err == nil {
		t.Fatal("expected ErrNoSuchPartner")
	}

	c2, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	if _, err := c2.FetchPublicStock(context.Background(), 111); err == nil {
		t.Fatal("expected RemoteError from doJSON")
	}
}
