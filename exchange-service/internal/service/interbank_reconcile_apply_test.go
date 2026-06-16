package service

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func TestInterbankReconcile_ApplyAndBump(t *testing.T) {
	db := openSagaTestDB(t, "ib_apply")
	db.Exec(`INSERT INTO accounts (id, currency_id, status, stanje, raspolozivo_stanje) VALUES
		(1, 1, 'aktivan', 1000, 1000), (2, 1, 'aktivan', 1000, 1000), (3, 1, 'aktivan', 1000, 1000)`)

	r := NewInterbankReconcileRunner(
		db, nil, nil,
		repository.NewInterbankPaymentRepository(db),
		repository.NewInterbankPaymentWalletRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewInterbankWalletRepository(db),
		repository.NewInterbankOtcRepository(db),
	)

	seed := func(txid string, accountID uint, amount float64) *models.InterbankPayment {
		acct := accountID
		p := &models.InterbankPayment{
			TxRoutingNumber: 444, TxID: txid, Direction: "outbound", PartnerRoutingNumber: 444,
			SenderAccountNumber: "s", RecipientAccountNumber: "rr", Currency: "RSD", Amount: amount,
			LocalAccountID: &acct, Status: models.InterbankPaymentStatusPending,
		}
		if err := db.Create(p).Error; err != nil {
			t.Fatalf("seed payment: %v", err)
		}
		return p
	}

	// applyCommit: pending -> committed + debit.
	c := seed("commit-1", 1, 100)
	if err := r.applyCommit(c); err != nil {
		t.Fatalf("applyCommit: %v", err)
	}
	if c.Status != models.InterbankPaymentStatusCommitted {
		t.Errorf("applyCommit status=%s", c.Status)
	}
	// Idempotent re-apply (rows==0 path).
	if err := r.applyCommit(c); err != nil {
		t.Fatalf("applyCommit idempotent: %v", err)
	}

	// applyRejected: pending -> rejected + release + finalise.
	rej := seed("rej-1", 2, 50)
	if err := r.applyRejected(rej, "partner voted NO"); err != nil {
		t.Fatalf("applyRejected: %v", err)
	}
	if rej.Status != models.InterbankPaymentStatusRejected {
		t.Errorf("applyRejected status=%s", rej.Status)
	}

	// applyFailed: pending -> failed + release.
	f := seed("fail-1", 3, 30)
	if err := r.applyFailed(f, "transport error"); err != nil {
		t.Fatalf("applyFailed: %v", err)
	}

	// markPartnerFinalised + bumpUpdatedAt are DB-only nudges.
	r.markPartnerFinalised(c)
	r.bumpUpdatedAt(c)

	// Missing local_account_id -> each apply errors.
	bad := &models.InterbankPayment{TxRoutingNumber: 444, TxID: "bad", Direction: "outbound", Status: models.InterbankPaymentStatusPending}
	if err := db.Create(bad).Error; err != nil {
		t.Fatalf("seed bad: %v", err)
	}
	if err := r.applyCommit(bad); err == nil {
		t.Error("applyCommit should error without local_account_id")
	}
	if err := r.applyRejected(bad, "x"); err == nil {
		t.Error("applyRejected should error without local_account_id")
	}
	if err := r.applyFailed(bad, "x"); err == nil {
		t.Error("applyFailed should error without local_account_id")
	}
}
