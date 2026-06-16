package handler

import (
	"errors"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func TestInterbankPaymentHandler_FinaliseHelpers(t *testing.T) {
	db := newFundTestDB(t, "ib_pay_finalise")
	db.Exec(`INSERT INTO accounts (id, currency_id, status, stanje, raspolozivo_stanje) VALUES
		(1, 1, 'aktivan', 1000, 1000), (2, 1, 'aktivan', 1000, 1000), (3, 1, 'aktivan', 1000, 1000)`)

	cfg := &config.Config{JWTSecret: testJWTSecret}
	reg, _ := interbank.NewRegistryFromJSON(333,
		`[{"code":444,"baseUrl":"http://127.0.0.1:1","outboundKey":"o","inboundKey":"i","displayName":"P"}]`)
	client := interbank.NewClient(reg)
	h := NewInterbankPaymentHTTPHandler(cfg, reg, client,
		repository.NewInterbankPaymentRepository(db), repository.NewInterbankPaymentWalletRepository(db), db)

	seed := func(txid string, acctID uint) *models.InterbankPayment {
		acct := acctID
		p := &models.InterbankPayment{
			TxRoutingNumber: 444, TxID: txid, Direction: "outbound", PartnerRoutingNumber: 444,
			SenderAccountNumber: "s", RecipientAccountNumber: "rr", Currency: "RSD", Amount: 100,
			LocalAccountID: &acct, Status: models.InterbankPaymentStatusPending,
		}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("seed %s: %v", txid, err)
		}
		return p
	}

	// finaliseCommit: pending -> committed + debit.
	c := seed("c1", 1)
	if err := h.finaliseCommit(c); err != nil {
		t.Fatalf("finaliseCommit: %v", err)
	}
	if c.Status != models.InterbankPaymentStatusCommitted {
		t.Errorf("finaliseCommit status=%s", c.Status)
	}
	h.markCommitDispatched(c) // stamp partner_finalised_at

	// finaliseRejected: pending -> rejected + release + finalise.
	rej := seed("r1", 2)
	if err := h.finaliseRejected(rej, "partner NO"); err != nil {
		t.Fatalf("finaliseRejected: %v", err)
	}

	// finaliseTransportFailure: pending -> failed + release (best-effort ROLLBACK to dead URL).
	f := seed("f1", 3)
	h.finaliseTransportFailure(f, 444, client.NewIdempotenceKey(), errors.New("boom"))

	// Missing local_account_id -> errors / early returns.
	bad := &models.InterbankPayment{TxRoutingNumber: 444, TxID: "bad", Direction: "outbound", Status: models.InterbankPaymentStatusPending}
	if err := db.Create(bad).Error; err != nil {
		t.Fatalf("seed bad: %v", err)
	}
	if err := h.finaliseCommit(bad); err == nil {
		t.Error("finaliseCommit should error without local_account_id")
	}
	if err := h.finaliseRejected(bad, "x"); err == nil {
		t.Error("finaliseRejected should error without local_account_id")
	}
	h.finaliseTransportFailure(bad, 444, client.NewIdempotenceKey(), errors.New("x")) // logs + returns
}
