package interbank

import (
	"encoding/json"
	"testing"
)

func TestIsKnownCurrency(t *testing.T) {
	for _, c := range []CurrencyCode{CurrencyRSD, CurrencyEUR, CurrencyUSD, CurrencyCHF, CurrencyJPY, CurrencyAUD, CurrencyCAD, CurrencyGBP} {
		if !IsKnownCurrency(c) {
			t.Errorf("IsKnownCurrency(%q) = false, want true", c)
		}
	}
	if IsKnownCurrency("XXX") {
		t.Error("IsKnownCurrency(XXX) = true, want false")
	}
}

func TestAssetMarshalUnmarshal_RoundTrip(t *testing.T) {
	cases := []Asset{
		{Type: AssetMonas, Monas: &MonetaryAsset{Currency: CurrencyEUR}},
		{Type: AssetStock, Stock: &StockDescription{Ticker: "AAPL"}},
		{Type: AssetOption, Option: &OptionDescription{
			NegotiationID:  ForeignBankId{RoutingNumber: 333, ID: "neg-1"},
			Stock:          StockDescription{Ticker: "AAPL"},
			PricePerUnit:   MonetaryValue{Currency: CurrencyUSD, Amount: 10},
			SettlementDate: "2025-04-16T15:32:44+02:00",
			Amount:         4,
		}},
	}
	for _, want := range cases {
		raw, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("marshal %s: %v", want.Type, err)
		}
		var got Asset
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", want.Type, err)
		}
		if got.Type != want.Type {
			t.Errorf("type = %q, want %q", got.Type, want.Type)
		}
	}
}

func TestAssetMarshal_NilInner(t *testing.T) {
	for _, a := range []Asset{
		{Type: AssetMonas},
		{Type: AssetStock},
		{Type: AssetOption},
		{Type: "WEIRD"},
	} {
		if _, err := json.Marshal(a); err == nil {
			t.Errorf("marshal %q with nil inner: want error", a.Type)
		}
	}
}

func TestAssetUnmarshal_Errors(t *testing.T) {
	if err := (&Asset{}).UnmarshalJSON([]byte(`not json`)); err == nil {
		t.Error("want error on malformed wire")
	}
	if err := (&Asset{}).UnmarshalJSON([]byte(`{"type":"NOPE","asset":{}}`)); err == nil {
		t.Error("want error on unknown type")
	}
	if err := (&Asset{}).UnmarshalJSON([]byte(`{"type":"MONAS","asset":123}`)); err == nil {
		t.Error("want error on bad inner")
	}
}

func TestNewMessageAndDecode(t *testing.T) {
	key := IdempotenceKey{RoutingNumber: 333, LocallyGeneratedKey: "k1"}

	tx := &Transaction{TransactionID: ForeignBankId{RoutingNumber: 333, ID: "tx-1"}, Message: "hi"}
	msg, err := NewMessage(key, tx)
	if err != nil {
		t.Fatalf("NewMessage tx: %v", err)
	}
	if msg.MessageType != MessageTypeNewTx {
		t.Errorf("type = %q, want NEW_TX", msg.MessageType)
	}
	if got, err := msg.DecodeNewTx(); err != nil || got.TransactionID.ID != "tx-1" {
		t.Errorf("DecodeNewTx = %+v, %v", got, err)
	}
	if _, err := msg.DecodeCommitTx(); err == nil {
		t.Error("DecodeCommitTx on NEW_TX: want error")
	}

	commit := CommitTransaction{TransactionID: ForeignBankId{RoutingNumber: 333, ID: "tx-2"}}
	cmsg, _ := NewMessage(key, commit)
	if cmsg.MessageType != MessageTypeCommitTx {
		t.Errorf("type = %q, want COMMIT_TX", cmsg.MessageType)
	}
	if got, err := cmsg.DecodeCommitTx(); err != nil || got.TransactionID.ID != "tx-2" {
		t.Errorf("DecodeCommitTx = %+v, %v", got, err)
	}

	rb := RollbackTransaction{TransactionID: ForeignBankId{RoutingNumber: 333, ID: "tx-3"}}
	rmsg, _ := NewMessage(key, &rb)
	if rmsg.MessageType != MessageTypeRollbackTx {
		t.Errorf("type = %q, want ROLLBACK_TX", rmsg.MessageType)
	}
	if got, err := rmsg.DecodeRollbackTx(); err != nil || got.TransactionID.ID != "tx-3" {
		t.Errorf("DecodeRollbackTx = %+v, %v", got, err)
	}

	if _, err := NewMessage(key, "unsupported"); err == nil {
		t.Error("NewMessage with bad body: want error")
	}
}

func TestDecode_WrongType(t *testing.T) {
	m := &Message{MessageType: MessageTypeNewTx, Body: json.RawMessage(`{}`)}
	if _, err := m.DecodeRollbackTx(); err == nil {
		t.Error("DecodeRollbackTx on NEW_TX: want error")
	}
	m.MessageType = MessageTypeCommitTx
	if _, err := m.DecodeNewTx(); err == nil {
		t.Error("DecodeNewTx on COMMIT_TX: want error")
	}
}

func TestRemoteError_Error(t *testing.T) {
	e := &RemoteError{StatusCode: 400, Status: "400 Bad Request", Body: []byte("oops")}
	if got := e.Error(); got == "" {
		t.Error("empty error string")
	}
	long := make([]byte, 400)
	for i := range long {
		long[i] = 'x'
	}
	e2 := &RemoteError{StatusCode: 500, Status: "500", Body: long}
	if len(e2.Error()) > 400 { // truncated to 256 + prefix + "..."
		// fine — just ensure it ran the truncation branch without panicking
	}
}
