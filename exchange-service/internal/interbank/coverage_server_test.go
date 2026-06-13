package interbank

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// fakeProcessor is a configurable TxProcessor for server tests.
type fakeProcessor struct {
	vote       *TransactionVote
	newErr     error
	commitErr  error
	rollbackErr error
}

func (f fakeProcessor) OnNewTx(context.Context, *PartnerBank, *Transaction) (*TransactionVote, error) {
	if f.newErr != nil {
		return nil, f.newErr
	}
	if f.vote != nil {
		return f.vote, nil
	}
	return &TransactionVote{Vote: VoteYes}, nil
}
func (f fakeProcessor) OnCommitTx(context.Context, *PartnerBank, ForeignBankId) error {
	return f.commitErr
}
func (f fakeProcessor) OnRollbackTx(context.Context, *PartnerBank, ForeignBankId) error {
	return f.rollbackErr
}

func postEnvelope(t *testing.T, srv *Server, apiKey string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/interbank", bytes.NewReader(body))
	if apiKey != "" {
		req.Header.Set(HeaderAPIKey, apiKey)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func envelopeBytes(t *testing.T, routing RoutingNumber, key string, mt MessageType, body any) []byte {
	t.Helper()
	raw, _ := json.Marshal(body)
	env := Message{
		IdempotenceKey: IdempotenceKey{RoutingNumber: routing, LocallyGeneratedKey: key},
		MessageType:    mt,
		Body:           raw,
	}
	b, _ := json.Marshal(env)
	return b
}

func newTestServer(t *testing.T, proc TxProcessor) *Server {
	t.Helper()
	db := openInterbankTestDB(t, "ib_server_"+t.Name())
	inbound := repository.NewInterbankInboundRepository(db)
	reg := testRegistry(t, 333, 111, "http://partner")
	return NewServer(reg, inbound, proc)
}

func TestServer_NewTx_Yes(t *testing.T) {
	srv := newTestServer(t, fakeProcessor{vote: &TransactionVote{Vote: VoteYes}})
	body := envelopeBytes(t, 111, "k1", MessageTypeNewTx, Transaction{TransactionID: ForeignBankId{RoutingNumber: 111, ID: "t1"}})
	rec := postEnvelope(t, srv, "in-key", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var vote TransactionVote
	_ = json.Unmarshal(rec.Body.Bytes(), &vote)
	if vote.Vote != VoteYes {
		t.Errorf("vote = %q", vote.Vote)
	}

	// Replay same idempotence key → cached identical response.
	rec2 := postEnvelope(t, srv, "in-key", body)
	if rec2.Code != http.StatusOK || rec2.Body.String() != rec.Body.String() {
		t.Errorf("replay mismatch: %d %s", rec2.Code, rec2.Body.String())
	}
}

func TestServer_CommitAndRollback(t *testing.T) {
	srv := newTestServer(t, fakeProcessor{})
	commit := envelopeBytes(t, 111, "c1", MessageTypeCommitTx, CommitTransaction{TransactionID: ForeignBankId{RoutingNumber: 111, ID: "t1"}})
	if rec := postEnvelope(t, srv, "in-key", commit); rec.Code != http.StatusNoContent {
		t.Errorf("commit status = %d", rec.Code)
	}
	rb := envelopeBytes(t, 111, "r1", MessageTypeRollbackTx, RollbackTransaction{TransactionID: ForeignBankId{RoutingNumber: 111, ID: "t1"}})
	if rec := postEnvelope(t, srv, "in-key", rb); rec.Code != http.StatusNoContent {
		t.Errorf("rollback status = %d", rec.Code)
	}
}

func TestServer_AuthAndValidation(t *testing.T) {
	srv := newTestServer(t, fakeProcessor{})

	// Wrong method.
	req := httptest.NewRequest(http.MethodGet, "/interbank", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d", rec.Code)
	}

	// Bad API key.
	if rec := postEnvelope(t, srv, "wrong", []byte("{}")); rec.Code != http.StatusUnauthorized {
		t.Errorf("bad key status = %d", rec.Code)
	}

	// Malformed envelope.
	if rec := postEnvelope(t, srv, "in-key", []byte("not json")); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed status = %d", rec.Code)
	}

	// Routing mismatch (envelope says 222 but key is partner 111).
	body := envelopeBytes(t, 222, "k", MessageTypeNewTx, Transaction{})
	if rec := postEnvelope(t, srv, "in-key", body); rec.Code != http.StatusForbidden {
		t.Errorf("routing mismatch status = %d", rec.Code)
	}

	// Missing locally generated key.
	body = envelopeBytes(t, 111, "", MessageTypeNewTx, Transaction{})
	if rec := postEnvelope(t, srv, "in-key", body); rec.Code != http.StatusBadRequest {
		t.Errorf("missing key status = %d", rec.Code)
	}

	// Over-long key.
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	body = envelopeBytes(t, 111, string(long), MessageTypeNewTx, Transaction{})
	if rec := postEnvelope(t, srv, "in-key", body); rec.Code != http.StatusBadRequest {
		t.Errorf("long key status = %d", rec.Code)
	}
}

func TestServer_ProcessorError(t *testing.T) {
	srv := newTestServer(t, fakeProcessor{newErr: context.DeadlineExceeded})
	body := envelopeBytes(t, 111, "e1", MessageTypeNewTx, Transaction{TransactionID: ForeignBankId{RoutingNumber: 111, ID: "t1"}})
	if rec := postEnvelope(t, srv, "in-key", body); rec.Code != http.StatusInternalServerError {
		t.Errorf("processor error status = %d", rec.Code)
	}
}

func TestServer_UnknownMessageType(t *testing.T) {
	srv := newTestServer(t, fakeProcessor{})
	body := envelopeBytes(t, 111, "u1", "WEIRD_TX", Transaction{})
	if rec := postEnvelope(t, srv, "in-key", body); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown type status = %d", rec.Code)
	}
}

func TestServer_SetProcessorAndNoop(t *testing.T) {
	srv := newTestServer(t, NoopProcessor{})
	body := envelopeBytes(t, 111, "n1", MessageTypeNewTx, Transaction{TransactionID: ForeignBankId{RoutingNumber: 111, ID: "t1"}})
	rec := postEnvelope(t, srv, "in-key", body)
	var vote TransactionVote
	_ = json.Unmarshal(rec.Body.Bytes(), &vote)
	if vote.Vote != VoteNo {
		t.Errorf("Noop vote = %q, want NO", vote.Vote)
	}
	// Noop commit/rollback are no-ops.
	np := NoopProcessor{}
	if err := np.OnCommitTx(context.Background(), nil, ForeignBankId{}); err != nil {
		t.Error(err)
	}
	if err := np.OnRollbackTx(context.Background(), nil, ForeignBankId{}); err != nil {
		t.Error(err)
	}
	srv.SetProcessor(fakeProcessor{})
}

func TestAuthMiddleware(t *testing.T) {
	reg := testRegistry(t, 333, 111, "http://partner")
	var gotPartner *PartnerBank
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPartner = PartnerFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	h := AuthMiddleware(reg, next)

	// Authorized.
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderAPIKey, "in-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || gotPartner == nil || gotPartner.Code != 111 {
		t.Errorf("authorized: code=%d partner=%+v", rec.Code, gotPartner)
	}

	// Unauthorized.
	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("unauthorized: code=%d", rec2.Code)
	}

	if PartnerFromContext(context.Background()) != nil {
		t.Error("empty context should yield nil partner")
	}
}
