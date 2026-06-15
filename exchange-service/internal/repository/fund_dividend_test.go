package repository

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

func TestDividendRepository_FundEligibleHoldings_OnlyFunds(t *testing.T) {
	db := openRepoTestDB(t, "fund_div_eligible")
	r := NewDividendRepository(db)

	// fund-owned dividend stock (eligible), client-owned (excluded here), zero-yield fund (excluded)
	seedDividendStock(t, db, "FAAPL", "USD", 200, 0.04, 9, "fund", 5)
	seedDividendStock(t, db, "CAAPL", "USD", 200, 0.04, 1, "client", 5)
	seedDividendStock(t, db, "FZERO", "USD", 50, 0, 9, "fund", 5)

	rows, err := r.ListFundDividendEligibleHoldings()
	if err != nil {
		t.Fatalf("ListFundDividendEligibleHoldings: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 fund holding (client + zero-yield excluded), got %d", len(rows))
	}
	if rows[0].UserType != "fund" || rows[0].Ticker != "FAAPL" {
		t.Errorf("unexpected row: %+v", rows[0])
	}
}

func TestFundRepository_DividendHistoryAndPolicy(t *testing.T) {
	db := openRepoTestDB(t, "fund_div_hist")
	r := NewFundRepository(db)

	// A fund whose dividend policy we can flip.
	db.Exec(`INSERT INTO currencies (id, kod, aktivan) VALUES (1,'RSD',1)`)
	fund, err := r.CreateFundWithAccount("Div Fund", "x", 1000, 7)
	if err != nil {
		t.Fatalf("create fund: %v", err)
	}
	if fund.DividendPolicy != models.FundDividendPolicyReinvest {
		t.Errorf("expected default policy 'reinvest', got %q", fund.DividendPolicy)
	}

	if err := r.UpdateDividendPolicy(fund.ID, models.FundDividendPolicyPayout); err != nil {
		t.Fatalf("UpdateDividendPolicy: %v", err)
	}
	got, _ := r.GetFundByID(fund.ID)
	if got.DividendPolicy != models.FundDividendPolicyPayout {
		t.Errorf("expected policy 'payout', got %q", got.DividendPolicy)
	}

	// Idempotency + history.
	exists, _ := r.FundDividendExists(fund.ID, 1, "2026-Q2")
	if exists {
		t.Error("expected no dividend yet")
	}
	rec := &models.FundDividendRecord{
		FundID: fund.ID, AssetID: 1, Ticker: "AAPL", Period: "2026-Q2", Quantity: 10,
		GrossRSD: 500, Policy: models.FundDividendPolicyPayout, DistributedRSD: 480, PaidAt: time.Now().UTC(),
	}
	if err := r.CreateFundDividend(rec); err != nil {
		t.Fatalf("CreateFundDividend: %v", err)
	}
	exists, _ = r.FundDividendExists(fund.ID, 1, "2026-Q2")
	if !exists {
		t.Error("expected dividend to exist after create")
	}
	hist, _ := r.ListFundDividends(fund.ID)
	if len(hist) != 1 || hist[0].DistributedRSD != 480 {
		t.Errorf("unexpected history: %+v", hist)
	}
}

func TestFundRepository_FindParticipantRSDAccount(t *testing.T) {
	db := openRepoTestDB(t, "fund_div_acct")
	r := NewFundRepository(db)

	db.Exec(`INSERT INTO currencies (id, kod, aktivan) VALUES (1,'RSD',1),(2,'USD',1)`)
	db.Exec(`INSERT INTO firmas (id, naziv, is_state) VALUES (1,'EXBanka',0)`)
	// client RSD account, a fund (fondacija) RSD account that must be ignored for bank lookup,
	// and a bank firm RSD account.
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, podvrsta) VALUES (10,1,'aktivan',7,'tekuci')`)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, firma_id, podvrsta) VALUES (20,1,'aktivan',1,'fondacija')`)
	db.Exec(`INSERT INTO accounts (id, currency_id, status, firma_id, podvrsta) VALUES (21,1,'aktivan',1,'tekuci')`)

	if id, err := r.FindParticipantRSDAccount(7, "client"); err != nil || id != 10 {
		t.Errorf("expected client RSD account 10, got %d err=%v", id, err)
	}
	// Bank lookup must skip the fund (fondacija) account and pick the plain firm account.
	if id, err := r.FindParticipantRSDAccount(0, "bank"); err != nil || id != 21 {
		t.Errorf("expected bank firm RSD account 21 (not fondacija), got %d err=%v", id, err)
	}
}
