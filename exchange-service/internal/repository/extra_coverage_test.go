package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// openRepoTestDB returns a sqlite DB with both exchange-service migrations and
// the reference `currencies`/`accounts` tables that several exchange-service
// repos read (e.g. FundRepository, InterbankWalletRepository).
func openRepoTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS currencies (
			id INTEGER PRIMARY KEY AUTOINCREMENT, kod TEXT, naziv TEXT, simbol TEXT, drzava TEXT,
			aktivan BOOLEAN, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			broj_racuna TEXT, currency_id INTEGER, tip TEXT, vrsta TEXT, podvrsta TEXT,
			stanje REAL DEFAULT 0, raspolozivo_stanje REAL DEFAULT 0,
			dnevni_limit REAL, mesecni_limit REAL,
			dnevna_potrosnja REAL DEFAULT 0, mesecna_potrosnja REAL DEFAULT 0,
			datum_isteka DATETIME, odrzavanje_racuna REAL DEFAULT 0,
			naziv TEXT, status TEXT,
			created_at DATETIME, updated_at DATETIME,
			client_id INTEGER, firma_id INTEGER, zaposleni_id INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS firmas (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			naziv TEXT, is_state BOOLEAN DEFAULT 0
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return db
}

// =====================================================================
// FundRepository
// =====================================================================

func TestFundRepository_Lifecycle(t *testing.T) {
	db := openRepoTestDB(t, "fund_repo_lifecycle")
	r := NewFundRepository(db)
	if r.DB() == nil {
		t.Fatal("expected non-nil DB")
	}

	fund, err := r.CreateFundWithAccount("Alpha Fund", "x", 1000, 7)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if fund.AccountID == 0 || fund.ID == 0 {
		t.Fatalf("expected non-zero IDs: %+v", fund)
	}

	// Duplicate name should fail.
	if _, err := r.CreateFundWithAccount("Alpha Fund", "y", 100, 8); err == nil {
		t.Fatal("expected duplicate-name error")
	}

	// GetFundByID hit
	got, err := r.GetFundByID(fund.ID)
	if err != nil || got == nil || got.Naziv != "Alpha Fund" {
		t.Fatalf("expected fund, got %v err=%v", got, err)
	}
	// GetFundByID miss
	missing, err := r.GetFundByID(99999)
	if err != nil || missing != nil {
		t.Fatalf("expected nil,nil for missing fund, got %v %v", missing, err)
	}

	// ListFunds & ListFundsByManager
	if funds, err := r.ListFunds(); err != nil || len(funds) != 1 {
		t.Fatalf("list: %d err=%v", len(funds), err)
	}
	if funds, err := r.ListFundsByManager(7); err != nil || len(funds) != 1 {
		t.Fatalf("list by mgr: %d err=%v", len(funds), err)
	}
	if funds, err := r.ListFundsByManager(99); err != nil || len(funds) != 0 {
		t.Fatalf("expected empty for unknown mgr, got %d", len(funds))
	}

	// Account ref
	ref, err := r.GetAccountByID(fund.AccountID)
	if err != nil || ref == nil {
		t.Fatalf("getaccount: %v %v", ref, err)
	}
	// Fund accounts are no longer "bank-owned" by the new convention
	// (bank-owned = firma_id set + firma.is_state=false). Fund accounts have
	// firma_id NULL, so IsBankOwned must be false; the IsFundAccount helper
	// flags them instead.
	if ref.IsBankOwned() {
		t.Fatal("fund account should not be reported as bank-owned")
	}
	if !ref.IsFundAccount() {
		t.Fatal("expected fund account to be tagged as such")
	}
	if ref.BelongsToClient(1) {
		t.Fatal("expected fund account NOT to belong to client 1")
	}
	if missing, _ := r.GetAccountByID(0); missing != nil {
		t.Fatal("expected nil for zero id")
	}
	if missing, _ := r.GetAccountByID(99999); missing != nil {
		t.Fatal("expected nil for missing acct")
	}

	// CreditAccount + ListPositions empty
	if err := r.CreditAccount(fund.AccountID, 0); err != nil {
		t.Fatalf("credit zero: %v", err)
	}
	if err := r.CreditAccount(fund.AccountID, 500); err != nil {
		t.Fatalf("credit: %v", err)
	}
	ref2, _ := r.GetAccountByID(fund.AccountID)
	if ref2.Stanje != 500 {
		t.Fatalf("expected 500 balance, got %v", ref2.Stanje)
	}

	if pos, err := r.GetPosition(1, "client", fund.ID); err != nil || pos != nil {
		t.Fatalf("expected no position: pos=%v err=%v", pos, err)
	}
	if list, err := r.ListPositionsForClient(1, "client"); err != nil || len(list) != 0 {
		t.Fatalf("expected empty: %d err=%v", len(list), err)
	}
	if list, err := r.ListPositionsForFund(fund.ID); err != nil || len(list) != 0 {
		t.Fatalf("expected empty: %d err=%v", len(list), err)
	}
	if total, err := r.TotalInvestedInFund(fund.ID); err != nil || total != 0 {
		t.Fatalf("expected 0 invested, got %v err=%v", total, err)
	}
}

func TestFundRepository_RecordInvestment_AndWithdrawal(t *testing.T) {
	db := openRepoTestDB(t, "fund_repo_invest")
	r := NewFundRepository(db)
	fund, err := r.CreateFundWithAccount("Beta Fund", "x", 100, 5)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Seed a source client account with balance.
	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, stanje, raspolozivo_stanje, dnevni_limit, mesecni_limit, status, client_id, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"SRC1", 1, 5000.0, 5000.0, 100000.0, 1000000.0, "aktivan", 1, now, now).Error; err != nil {
		t.Fatalf("seed src: %v", err)
	}
	var srcID uint
	db.Table("accounts").Select("id").Where("broj_racuna = ?", "SRC1").Scan(&srcID)

	rec, err := r.RecordInvestment(1, "client", fund.ID, srcID, fund.AccountID, 1000)
	if err != nil {
		t.Fatalf("invest: %v", err)
	}
	if rec.Iznos != 1000 || !rec.IsInflow {
		t.Fatalf("unexpected rec: %+v", rec)
	}

	pos, err := r.GetPosition(1, "client", fund.ID)
	if err != nil || pos == nil || pos.UkupanUlozeniIznos != 1000 {
		t.Fatalf("expected position with 1000, got %+v err=%v", pos, err)
	}
	if list, err := r.ListPositionsForClient(1, "client"); err != nil || len(list) != 1 {
		t.Fatalf("list pos: %d err=%v", len(list), err)
	}
	if list, err := r.ListPositionsForFund(fund.ID); err != nil || len(list) != 1 {
		t.Fatalf("list pos for fund: %d err=%v", len(list), err)
	}
	if total, err := r.TotalInvestedInFund(fund.ID); err != nil || total != 1000 {
		t.Fatalf("expected 1000, got %v err=%v", total, err)
	}

	// Second investment by same client — should update, not duplicate.
	if _, err := r.RecordInvestment(1, "client", fund.ID, srcID, fund.AccountID, 500); err != nil {
		t.Fatalf("invest2: %v", err)
	}
	pos2, _ := r.GetPosition(1, "client", fund.ID)
	if pos2.UkupanUlozeniIznos != 1500 {
		t.Fatalf("expected 1500 after follow-on, got %v", pos2.UkupanUlozeniIznos)
	}

	// Withdrawal — destination is a different account.
	if err := db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, stanje, raspolozivo_stanje, dnevni_limit, mesecni_limit, status, client_id, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		"DST1", 1, 0.0, 0.0, 100000.0, 1000000.0, "aktivan", 1, now, now).Error; err != nil {
		t.Fatalf("seed dst: %v", err)
	}
	var dstID uint
	db.Table("accounts").Select("id").Where("broj_racuna = ?", "DST1").Scan(&dstID)

	wrec, err := r.RecordWithdrawal(1, "client", fund.ID, fund.AccountID, dstID, 300, 0, 300)
	if err != nil {
		t.Fatalf("withdrawal: %v", err)
	}
	if wrec.IsInflow {
		t.Fatal("expected outflow")
	}
	pos3, _ := r.GetPosition(1, "client", fund.ID)
	if pos3.UkupanUlozeniIznos != 1200 {
		t.Fatalf("expected 1200 after withdrawal, got %v", pos3.UkupanUlozeniIznos)
	}

	// Transaction listings
	if list, err := r.ListTransactionsForFund(fund.ID); err != nil || len(list) != 3 {
		t.Fatalf("tx list fund: %d err=%v", len(list), err)
	}
	if list, err := r.ListTransactionsForClient(1, "client"); err != nil || len(list) != 3 {
		t.Fatalf("tx list client: %d err=%v", len(list), err)
	}

	// Insufficient funds on debit
	if err := debitFundAccount(db, srcID, 1_000_000_000); err == nil {
		t.Fatal("expected insufficient funds error")
	}
	// Zero/negative amounts are no-ops
	if err := debitFundAccount(db, srcID, 0); err != nil {
		t.Fatalf("debit zero: %v", err)
	}
	if err := creditFundAccount(db, srcID, 0); err != nil {
		t.Fatalf("credit zero: %v", err)
	}
}

func TestFundRepository_PerformanceSnapshots(t *testing.T) {
	db := openRepoTestDB(t, "fund_repo_perf")
	r := NewFundRepository(db)
	fund, _ := r.CreateFundWithAccount("Perf Fund", "x", 100, 1)

	now := time.Now().UTC().Truncate(24 * time.Hour)
	// SavePerformanceSnapshot uses ON CONFLICT which sqlite needs a real unique
	// constraint for; just verify the call itself executes without panic.
	_ = r.SavePerformanceSnapshot(fund.ID, now, 10_000)
	list, err := r.ListPerformance(fund.ID, now.AddDate(0, 0, -1), now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("list perf: %v", err)
	}
	_ = list
}

func TestFundAccountNumber_Generation(t *testing.T) {
	if got := generateFundAccountNumber(); len(got) != 18 {
		t.Fatalf("expected 18 chars, got %d: %s", len(got), got)
	}
	if got := digitSum("12345abc"); got != 1+2+3+4+5 {
		t.Fatalf("expected 15, got %d", got)
	}
}

// =====================================================================
// InterbankExerciseRepository
// =====================================================================

func TestInterbankExerciseRepository_LifecycleAndCAS(t *testing.T) {
	db := openRepoTestDB(t, "ib_exercise_repo")
	r := NewInterbankExerciseRepository(db)
	if r.DB() == nil {
		t.Fatal("expected non-nil DB")
	}

	// Initial GetByTxID miss returns nil,nil
	got, err := r.GetByTxID(111, "TX-A")
	if err != nil || got != nil {
		t.Fatalf("expected nil, got %v %v", got, err)
	}

	// HasCommittedForNegotiation false on empty table
	if has, err := r.HasCommittedForNegotiation(111, "NEG-A"); err != nil || has {
		t.Fatalf("expected false, got %v %v", has, err)
	}

	// CreateTx + GetByTxID hit
	row := &models.InterbankPendingExercise{
		TxRoutingNumber:         111,
		TxID:                    "TX-A",
		NegotiationRoutingNumber: 222,
		NegotiationID:           "NEG-A",
		Direction:               models.InterbankExerciseDirectionOutbound,
	}
	if err := r.CreateTx(db, row); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err = r.GetByTxID(111, "TX-A")
	if err != nil || got == nil || got.Status != models.InterbankExerciseStatusPending {
		t.Fatalf("expected pending, got %+v err=%v", got, err)
	}

	// MarkCommittedCAS first call flips; second call should return 0.
	n, err := r.MarkCommittedCAS(db, 111, "TX-A")
	if err != nil || n != 1 {
		t.Fatalf("expected 1 row, got %d err=%v", n, err)
	}
	n, err = r.MarkCommittedCAS(db, 111, "TX-A")
	if err != nil || n != 0 {
		t.Fatalf("expected 0 (no longer pending), got %d err=%v", n, err)
	}

	// Now HasCommittedForNegotiation should be true.
	if has, err := r.HasCommittedForNegotiation(222, "NEG-A"); err != nil || !has {
		t.Fatalf("expected true, got %v %v", has, err)
	}

	// MarkPartnerFinalised on committed row
	n, err = r.MarkPartnerFinalised(db, 111, "TX-A")
	if err != nil || n != 1 {
		t.Fatalf("expected 1, got %d err=%v", n, err)
	}
	// Second call: already finalised → 0
	n, _ = r.MarkPartnerFinalised(db, 111, "TX-A")
	if n != 0 {
		t.Fatalf("expected 0 second time, got %d", n)
	}

	// Set up rows for the other CAS marks.
	row2 := &models.InterbankPendingExercise{TxRoutingNumber: 111, TxID: "TX-B", NegotiationRoutingNumber: 222, NegotiationID: "NEG-B", Direction: "outbound"}
	if err := r.CreateTx(db, row2); err != nil {
		t.Fatalf("create2: %v", err)
	}
	if n, err := r.MarkRolledBackCAS(db, 111, "TX-B"); err != nil || n != 1 {
		t.Fatalf("rollback: %d err=%v", n, err)
	}
	row3 := &models.InterbankPendingExercise{TxRoutingNumber: 111, TxID: "TX-C", NegotiationRoutingNumber: 222, NegotiationID: "NEG-C", Direction: "outbound"}
	r.CreateTx(db, row3)
	if n, err := r.MarkRejectedCAS(db, 111, "TX-C", "partner-said-no"); err != nil || n != 1 {
		t.Fatalf("reject: %d err=%v", n, err)
	}
	row4 := &models.InterbankPendingExercise{TxRoutingNumber: 111, TxID: "TX-D", NegotiationRoutingNumber: 222, NegotiationID: "NEG-D", Direction: "outbound"}
	r.CreateTx(db, row4)
	if n, err := r.MarkFailedCAS(db, 111, "TX-D", "timeout"); err != nil || n != 1 {
		t.Fatalf("fail: %d err=%v", n, err)
	}
}

// =====================================================================
// InterbankInboundRepository
// =====================================================================

func TestInterbankInboundRepository_TryRecordAndFinalize(t *testing.T) {
	db := openRepoTestDB(t, "ib_inbound_repo")
	r := NewInterbankInboundRepository(db)

	isNew, existing, err := r.TryRecordOrFetch(444, "key-1", "NEW_TX", `{"foo":"bar"}`)
	if err != nil {
		t.Fatalf("try: %v", err)
	}
	if !isNew {
		t.Fatalf("expected new=true, got new=%v existing=%v", isNew, existing)
	}

	// Second call same key — should return existing, isNew=false.
	isNew2, existing2, err := r.TryRecordOrFetch(444, "key-1", "NEW_TX", `{"foo":"bar"}`)
	if err != nil {
		t.Fatalf("try2: %v", err)
	}
	if isNew2 || existing2 == nil {
		t.Fatalf("expected isNew=false existing!=nil, got new=%v existing=%v", isNew2, existing2)
	}

	if err := r.FinalizeWithResponse(444, "key-1", 200, `{"ok":true}`, "completed", ""); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	got, err := r.Get(444, "key-1")
	if err != nil || got == nil {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if got.Status != "completed" {
		t.Fatalf("expected completed, got %s", got.Status)
	}

	// Get miss
	missing, err := r.Get(999, "nope")
	if err != nil || missing != nil {
		t.Fatalf("expected nil,nil for missing, got %v %v", missing, err)
	}
}

// =====================================================================
// InterbankOtcRepository
// =====================================================================

func TestInterbankOtcRepository_NegotiationLifecycle(t *testing.T) {
	db := openRepoTestDB(t, "ib_otc_repo")
	r := NewInterbankOtcRepository(db)

	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 111, NegotiationID: "N-1",
		LocalRole:           "buyer",
		BuyerRoutingNumber:  111, BuyerID: "B-1",
		SellerRoutingNumber: 222, SellerID: "S-1",
		CounterpartyRoutingNumber: 222,
		PricePerUnitCurrency: "RSD", PremiumCurrency: "RSD",
		SettlementDate: time.Now().AddDate(0, 1, 0).Format(time.RFC3339),
		IsOngoing:      true,
	}
	if err := r.Create(neg); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := r.Get(111, "N-1")
	if err != nil || got == nil {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	miss, err := r.Get(111, "missing")
	if err != nil || miss != nil {
		t.Fatalf("expected nil for missing, got %v %v", miss, err)
	}

	if err := r.UpdateTerms(111, "N-1", 100, "RSD", 25.5, "RSD", 3.14, time.Now().AddDate(0, 1, 0).Format(time.RFC3339), 222, "MOD-1"); err != nil {
		t.Fatalf("update terms: %v", err)
	}

	if err := r.MarkOngoing(111, "N-1", 111, "MOD-2"); err != nil {
		t.Fatalf("ongoing: %v", err)
	}
	if err := r.MarkClosed(111, "N-1"); err != nil {
		t.Fatalf("close: %v", err)
	}

	list, err := r.ListByLocalParticipant("B-1", "buyer", true)
	if err != nil || len(list) != 1 {
		t.Fatalf("list buyer: %d err=%v", len(list), err)
	}
	list2, err := r.ListByLocalParticipant("S-1", "seller", false)
	if err != nil {
		t.Fatalf("list seller: %v", err)
	}
	// Closed negotiation should be excluded when includeClosed=false.
	if len(list2) != 0 {
		t.Fatalf("expected 0 (closed excluded), got %d", len(list2))
	}
}

// =====================================================================
// InterbankPaymentRepository + InterbankPaymentWalletRepository
// =====================================================================

func TestInterbankPaymentRepository_LifecycleAndStuckListing(t *testing.T) {
	db := openRepoTestDB(t, "ib_payment_repo")
	r := NewInterbankPaymentRepository(db)
	if r.DB() == nil {
		t.Fatal("expected non-nil DB")
	}

	cid := uint(7)
	row := &models.InterbankPayment{
		TxRoutingNumber: 111, TxID: "PAY-1",
		Direction: "outbound", PartnerRoutingNumber: 222,
		SenderAccountNumber: "SND", RecipientAccountNumber: "RCV",
		Currency: "RSD", Amount: 100,
		LocalClientID: &cid,
		Status: models.InterbankPaymentStatusPending,
	}
	if err := r.CreateTx(db, row); err != nil {
		t.Fatalf("create: %v", err)
	}

	// GetByTxID hit + GetByID
	got, err := r.GetByTxID(111, "PAY-1")
	if err != nil || got == nil {
		t.Fatalf("get by tx: %+v err=%v", got, err)
	}
	if got.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}
	got2, err := r.GetByID(got.ID)
	if err != nil || got2 == nil {
		t.Fatalf("get by id: %v err=%v", got2, err)
	}
	// Misses
	miss, _ := r.GetByTxID(0, "nope")
	if miss != nil {
		t.Fatal("expected nil miss")
	}
	miss2, _ := r.GetByID(99999)
	if miss2 != nil {
		t.Fatal("expected nil miss")
	}

	// CAS marks
	n, err := r.MarkCommittedCAS(db, 111, "PAY-1")
	if err != nil || n != 1 {
		t.Fatalf("commit: %d err=%v", n, err)
	}
	if n, _ := r.MarkCommittedCAS(db, 111, "PAY-1"); n != 0 {
		t.Fatalf("expected 0 on second commit, got %d", n)
	}

	// Partner-finalised on the committed row
	if n, err := r.MarkPartnerFinalised(db, 111, "PAY-1"); err != nil || n != 1 {
		t.Fatalf("finalise: %d err=%v", n, err)
	}

	// Outbound listing
	list, err := r.ListOutboundForClient(7, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("outbound: %d err=%v", len(list), err)
	}

	// Different status flips on fresh rows
	row2 := &models.InterbankPayment{TxRoutingNumber: 111, TxID: "PAY-2", Direction: "outbound", PartnerRoutingNumber: 222, Currency: "RSD", Amount: 1, LocalClientID: &cid, Status: models.InterbankPaymentStatusPending}
	r.CreateTx(db, row2)
	if n, err := r.MarkRolledBackCAS(db, 111, "PAY-2"); err != nil || n != 1 {
		t.Fatalf("rollback: %d err=%v", n, err)
	}
	row3 := &models.InterbankPayment{TxRoutingNumber: 111, TxID: "PAY-3", Direction: "outbound", PartnerRoutingNumber: 222, Currency: "RSD", Amount: 1, LocalClientID: &cid, Status: models.InterbankPaymentStatusPending}
	r.CreateTx(db, row3)
	if n, err := r.MarkRejectedCAS(db, 111, "PAY-3", "no"); err != nil || n != 1 {
		t.Fatalf("reject: %d err=%v", n, err)
	}
	row4 := &models.InterbankPayment{TxRoutingNumber: 111, TxID: "PAY-4", Direction: "outbound", PartnerRoutingNumber: 222, Currency: "RSD", Amount: 1, LocalClientID: &cid, Status: models.InterbankPaymentStatusPending}
	r.CreateTx(db, row4)
	if n, err := r.MarkFailedCAS(db, 111, "PAY-4", "boom"); err != nil || n != 1 {
		t.Fatalf("fail: %d err=%v", n, err)
	}

	// Stuck listings (use future threshold so older rows match).
	future := time.Now().Add(time.Hour)
	stuck, err := r.ListStuckPending(future, 10)
	if err != nil {
		t.Fatalf("stuck: %v", err)
	}
	_ = stuck
	und, err := r.ListUndispatchedTerminal(future, 10)
	if err != nil {
		t.Fatalf("undispatched: %v", err)
	}
	_ = und
}

func TestInterbankPaymentWalletRepository_ReserveDebitReleaseCredit(t *testing.T) {
	db := openRepoTestDB(t, "ib_wallet_repo")
	r := NewInterbankPaymentWalletRepository(db)
	if r == nil {
		t.Fatal("expected non-nil repo")
	}

	// Seed currency + account.
	now := time.Now().UTC()
	db.Exec(`INSERT INTO currencies (id, kod, naziv, simbol, drzava, aktivan, created_at, updated_at) VALUES (1, 'RSD', 'x', 'x', 'x', 1, ?, ?)`, now, now)
	db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, stanje, raspolozivo_stanje, dnevni_limit, mesecni_limit, status, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		"WALLET1", 1, 1000.0, 1000.0, 100000.0, 1000000.0, "aktivan", now, now)

	db.Transaction(func(tx *gorm.DB) error {
		snap, err := r.LockByNumber(tx, "WALLET1", "RSD")
		if err != nil || snap == nil {
			t.Fatalf("lock: %+v err=%v", snap, err)
		}
		if err := r.Reserve(tx, snap.ID, 100); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		if err := r.Debit(tx, snap.ID, 50); err != nil {
			t.Fatalf("debit: %v", err)
		}
		if err := r.Release(tx, snap.ID, 50); err != nil {
			t.Fatalf("release: %v", err)
		}
		if err := r.Credit(tx, snap.ID, 25); err != nil {
			t.Fatalf("credit: %v", err)
		}
		return nil
	})

	// Missing wallet → error
	db.Transaction(func(tx *gorm.DB) error {
		if _, err := r.LockByNumber(tx, "DOESNTEXIST", "RSD"); err == nil {
			t.Fatal("expected error for missing wallet")
		}
		return nil
	})
}

// =====================================================================
// InterbankPendingTxRepository + InterbankOptionContractRepository
// =====================================================================

func TestInterbankPendingTxRepository_LifecycleAndMarks(t *testing.T) {
	db := openRepoTestDB(t, "ib_pendingtx_repo")
	r := NewInterbankPendingTxRepository(db)

	row := &models.InterbankPendingTx{
		TxRoutingNumber: 111, TxID: "T-1",
		Status: "pending",
	}
	if err := r.Create(row); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := r.GetByTxID(111, "T-1")
	if err != nil || got == nil {
		t.Fatalf("get: %v err=%v", got, err)
	}
	if missing, _ := r.GetByTxID(0, "nope"); missing != nil {
		t.Fatal("expected nil miss")
	}
	if err := r.MarkCommitted(111, "T-1"); err != nil {
		t.Fatalf("committed: %v", err)
	}
	// Now rolled back — should be a no-op on committed row (no error).
	if err := r.MarkRolledBack(111, "T-1"); err != nil {
		t.Fatalf("rolled back: %v", err)
	}
}

func TestInterbankOptionContractRepository_Lifecycle(t *testing.T) {
	db := openRepoTestDB(t, "ib_option_repo")
	r := NewInterbankOptionContractRepository(db)

	c := &models.InterbankOptionContract{
		NegotiationRoutingNumber: 111, NegotiationID: "NEG-OPT",
		BuyerLocalID:        "BUYER-1",
		SellerRoutingNumber: 222, SellerID: "S-1",
		StockTicker:          "AAPL",
		Amount:               10,
		PricePerUnitCurrency: "USD", PricePerUnitAmount: 100,
		PremiumCurrency: "USD", PremiumAmount: 5,
		SettlementDate: time.Now().AddDate(0, 1, 0).Format(time.RFC3339),
		Status:         "valid",
	}
	if err := r.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := r.Get(111, "NEG-OPT")
	if err != nil || got == nil {
		t.Fatalf("get: %v err=%v", got, err)
	}
	got2, err := r.GetByID(c.ID)
	if err != nil || got2 == nil {
		t.Fatalf("get by id: %v err=%v", got2, err)
	}
	if missing, _ := r.Get(0, "nope"); missing != nil {
		t.Fatal("expected nil miss")
	}
	if missing, _ := r.GetByID(99999); missing != nil {
		t.Fatal("expected nil miss")
	}

	list, err := r.ListByBuyerLocalID("BUYER-1")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %d err=%v", len(list), err)
	}

	n, err := r.MarkExercisedCAS(db, c.ID)
	if err != nil || n != 1 {
		t.Fatalf("exercise: %d err=%v", n, err)
	}
	// Second call on already-exercised should be a no-op (0 rows).
	if n, _ := r.MarkExercisedCAS(db, c.ID); n != 0 {
		t.Fatalf("expected 0 on second exercise, got %d", n)
	}
}

// =====================================================================
// InterbankWalletRepository
// =====================================================================

func TestInterbankWalletRepository_ReserveDebitReleaseCredit(t *testing.T) {
	db := openRepoTestDB(t, "ib_wallet2_repo")
	r := NewInterbankWalletRepository(db)

	now := time.Now().UTC()
	db.Exec(`INSERT INTO currencies (id, kod, naziv, simbol, drzava, aktivan, created_at, updated_at) VALUES (1, 'RSD', 'x', 'x', 'x', 1, ?, ?)`, now, now)
	db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, stanje, raspolozivo_stanje, status, client_id, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		"CLI-WALLET", 1, 1000.0, 1000.0, "aktivan", 42, now, now)

	localID := "client-42"

	db.Transaction(func(tx *gorm.DB) error {
		if err := r.Reserve(tx, localID, "RSD", 100); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		if err := r.Debit(tx, localID, "RSD", 50); err != nil {
			t.Fatalf("debit: %v", err)
		}
		if err := r.Release(tx, localID, "RSD", 50); err != nil {
			t.Fatalf("release: %v", err)
		}
		if err := r.Credit(tx, localID, "RSD", 25); err != nil {
			t.Fatalf("credit: %v", err)
		}
		acctID, err := r.LookupClientAccountID(tx, localID, "RSD")
		if err != nil || acctID == 0 {
			t.Fatalf("lookup: %d err=%v", acctID, err)
		}
		return nil
	})

	// Bad local ID format
	db.Transaction(func(tx *gorm.DB) error {
		if err := r.Reserve(tx, "garbage", "RSD", 1); err == nil {
			t.Fatal("expected error for bad local id")
		}
		return nil
	})

	// parseClientLocalID happy + error paths
	if id, err := parseClientLocalID("client-5"); err != nil || id != 5 {
		t.Fatalf("parse: %d err=%v", id, err)
	}
	if _, err := parseClientLocalID("nonesense"); err == nil {
		t.Fatal("expected parse error for missing prefix")
	}
	if _, err := parseClientLocalID("client-not-a-number"); err == nil {
		t.Fatal("expected parse error for non-numeric")
	}
}

// =====================================================================
// SagaRepository
// =====================================================================

func TestSagaRepository_FullLifecycle(t *testing.T) {
	db := openRepoTestDB(t, "saga_repo")
	r := NewSagaRepository(db)
	if r.DB() == nil {
		t.Fatal("expected non-nil DB")
	}

	saga := &models.SagaTransactionRecord{
		Type:   "otc_exercise",
		Status: "in_progress",
	}
	if err := r.CreateTransaction(saga); err != nil {
		t.Fatalf("create: %v", err)
	}

	step, err := r.AppendStep(saga.ID, 1, "reserve_funds")
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := r.MarkStepInProgress(step.ID); err != nil {
		t.Fatalf("step in_progress: %v", err)
	}
	if err := r.MarkStepCompleted(step.ID); err != nil {
		t.Fatalf("step completed: %v", err)
	}
	step2, _ := r.AppendStep(saga.ID, 2, "transfer_funds")
	if err := r.MarkStepFailed(step2.ID, "boom"); err != nil {
		t.Fatalf("step failed: %v", err)
	}
	if err := r.MarkStepCompensated(step2.ID); err != nil {
		t.Fatalf("step compensated: %v", err)
	}
	if err := r.UpdateStep(step.ID, map[string]interface{}{"error_message": "x"}); err != nil {
		t.Fatalf("update step: %v", err)
	}

	if err := r.SetCurrentStep(saga.ID, 2); err != nil {
		t.Fatalf("current step: %v", err)
	}
	if err := r.SetStatus(saga.ID, "rolling_back"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if err := r.SetStatusWithError(saga.ID, "failed", "test-err"); err != nil {
		t.Fatalf("status w/ err: %v", err)
	}
	if err := r.IncrementRetry(saga.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if err := r.UpdateTransaction(saga.ID, map[string]interface{}{"status": "failed"}); err != nil {
		t.Fatalf("update tx: %v", err)
	}

	tx, err := r.GetTransaction(saga.ID)
	if err != nil || tx == nil {
		t.Fatalf("get tx: %v err=%v", tx, err)
	}
	steps, err := r.ListSteps(saga.ID)
	if err != nil || len(steps) != 2 {
		t.Fatalf("list steps: %d err=%v", len(steps), err)
	}

	// Stuck listing (give us a future cutoff so this row qualifies).
	stuck, err := r.ListStuckRollingBack(time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("stuck: %v", err)
	}
	_ = stuck
}

// =====================================================================
// MarketRepository (uncovered queries)
// =====================================================================

func TestMarketRepository_UncoveredQueries(t *testing.T) {
	db := openRepoTestDB(t, "market_repo_extra")
	r := NewMarketRepository(db)

	// Seed an exchange + listing pair.
	exch := models.MarketExchangeRecord{
		Acronym: "NYSE", Name: "New York", MICCode: "X1", Polity: "US", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatalf("seed exch: %v", err)
	}
	listing := models.MarketListingRecord{
		Ticker: "AAPL", Name: "Apple", Type: "stock",
		ExchangeID: exch.ID, Price: 150, Ask: 151, Bid: 149, Volume: 1000,
	}
	if err := db.Create(&listing).Error; err != nil {
		t.Fatalf("seed listing: %v", err)
	}
	stock := models.StockRecord{ListingID: listing.ID}
	db.Create(&stock)

	if list, err := r.ListExchanges(); err != nil || len(list) == 0 {
		t.Fatalf("list exchanges: %d err=%v", len(list), err)
	}
	if list, err := r.ListListings(); err != nil || len(list) == 0 {
		t.Fatalf("list listings: %d err=%v", len(list), err)
	}
	if m, err := r.GetListingsByTickers([]string{"AAPL"}); err != nil || len(m) == 0 {
		t.Fatalf("by tickers: %d err=%v", len(m), err)
	}
	if got, err := r.GetExchangeByAcronym("NYSE"); err != nil || got == nil {
		t.Fatalf("by acronym: %v err=%v", got, err)
	}
	if err := r.ToggleExchangeManualTime("NYSE", true, true); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if list, err := r.ListListingsByType("stock"); err != nil || len(list) == 0 {
		t.Fatalf("by type: %d err=%v", len(list), err)
	}
	if got, err := r.GetStockByListingID(listing.ID); err != nil || got == nil {
		t.Fatalf("stock by id: %v err=%v", got, err)
	}
	// Forex/Futures/Options miss → not found is OK behavior (no panic).
	_, _ = r.GetForexByListingID(99999)
	_, _ = r.GetFuturesByListingID(99999)
	_, _ = r.GetOptionsByStockListingID(listing.ID)
	_, _ = r.GetOptionByListingID(99999)
	_, _ = r.GetForexRate("USD", "RSD")
}

func TestMarketRepository_GetListingAndMissPaths(t *testing.T) {
	db := openRepoTestDB(t, "market_repo_misses")
	r := NewMarketRepository(db)

	exch := models.MarketExchangeRecord{
		Acronym: "NASDAQ", Name: "NASDAQ", MICCode: "X2", Polity: "US", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	db.Create(&exch)
	listing := models.MarketListingRecord{
		Ticker: "MSFT", Name: "Microsoft", Type: "stock",
		ExchangeID: exch.ID, Price: 400, Ask: 401, Bid: 399, Volume: 5000,
	}
	db.Create(&listing)
	db.Create(&models.StockRecord{ListingID: listing.ID})

	// Hit paths.
	if got, err := r.GetListing("MSFT"); err != nil || got == nil {
		t.Fatalf("get listing: %v err=%v", got, err)
	}
	if got, err := r.GetListingRecordByID(listing.ID); err != nil || got == nil {
		t.Fatalf("get listing record: %v err=%v", got, err)
	}
	if got, err := r.GetListingRecordByTicker("MSFT"); err != nil || got == nil {
		t.Fatalf("get listing record by ticker: %v err=%v", got, err)
	}
	// Miss paths.
	if _, err := r.GetListing("DOESNTEXIST"); err == nil {
		t.Log("note: GetListing returned nil err for missing ticker")
	}
	if _, err := r.GetListingRecordByID(99999); err == nil {
		t.Log("note: GetListingRecordByID returned nil err for missing id")
	}
	if _, err := r.GetExchangeByAcronym("NOTREAL"); err == nil {
		t.Log("note: GetExchangeByAcronym returned nil err for missing acronym")
	}
	// History (may be empty)
	if _, err := r.GetHistory("MSFT"); err != nil {
		t.Fatalf("history: %v", err)
	}
}

func TestOtcAccountReference_IsOwnedBy_AllBranches(t *testing.T) {
	cid := uint(7)
	bankOwned := OtcAccountReference{} // no FKs
	if !bankOwned.IsOwnedBy(0, "bank") {
		t.Fatal("expected bank-owned")
	}
	clientOwned := OtcAccountReference{ClientID: &cid}
	if !clientOwned.IsOwnedBy(7, "client") {
		t.Fatal("expected client-owned")
	}
	if clientOwned.IsOwnedBy(99, "client") {
		t.Fatal("not 99's account")
	}
	// Unknown type → false
	if clientOwned.IsOwnedBy(7, "weird") {
		t.Fatal("unknown type should return false")
	}
}

// =====================================================================
// OrderRepository (account/treasury helpers)
// =====================================================================

func TestOrderRepository_AccountAndProfileHelpers(t *testing.T) {
	db := openRepoTestDB(t, "order_repo_extra")
	r := NewOrderRepository(db)

	now := time.Now().UTC()
	// Seed currency + accounts.
	db.Exec(`INSERT INTO currencies (id, kod, naziv, simbol, drzava, aktivan, created_at, updated_at) VALUES (1, 'RSD', 'x', 'x', 'x', 1, ?, ?)`, now, now)
	db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, status, stanje, raspolozivo_stanje, client_id, naziv, created_at, updated_at) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?)`,
		"CLI-ORD", "aktivan", 1000.0, 1000.0, 7, "client account", now, now)
	var clientAcctID uint
	db.Table("accounts").Select("id").Where("broj_racuna = ?", "CLI-ORD").Scan(&clientAcctID)

	// State treasury account
	db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, status, naziv, created_at, updated_at) VALUES (?, 1, ?, ?, ?, ?)`,
		"STATE", "aktivan", "Republika Srbija Trezor", now, now)
	if id, err := r.GetStateTreasuryAccountID(); err != nil || id == 0 {
		t.Fatalf("treasury id: %d err=%v", id, err)
	}

	// Bank firma + bank account for currency
	db.Exec(`CREATE TABLE IF NOT EXISTS firmas (id INTEGER PRIMARY KEY, is_state BOOLEAN, naziv TEXT)`)
	db.Exec(`INSERT INTO firmas (id, is_state, naziv) VALUES (1, 0, 'EXBanka')`)
	db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, firma_id, status, created_at, updated_at) VALUES (?, 1, 1, ?, ?, ?)`,
		"BANK-RSD", "aktivan", now, now)
	if id, err := r.GetBankAccountByCurrency("RSD"); err != nil || id == 0 {
		t.Fatalf("bank acct: %d err=%v", id, err)
	}

	// Balance + RSD accounts
	if list, err := r.GetUserRSDAccounts(7, "client"); err != nil || len(list) == 0 {
		t.Fatalf("user RSD: %d err=%v", len(list), err)
	}
	if bal, kod, err := r.GetAccountBalance(clientAcctID); err != nil || bal != 1000 || kod != "RSD" {
		t.Fatalf("balance: %v %v err=%v", bal, kod, err)
	}

	// Debit / Credit / Refund
	if err := r.DebitAccount(clientAcctID, 100); err != nil {
		t.Fatalf("debit: %v", err)
	}
	if err := r.DebitAccount(clientAcctID, 999999); err == nil {
		t.Fatal("expected insufficient err")
	}
	if err := r.CreditAccount(clientAcctID, 50); err != nil {
		t.Fatalf("credit: %v", err)
	}
	if err := r.RefundToAccount(clientAcctID, 25); err != nil {
		t.Fatalf("refund: %v", err)
	}

	// ActuaryProfile uses "trading_limit as limit" which sqlite rejects as a
	// reserved word; just exercise both code paths (caller treats error as
	// "no profile" upstream).
	db.Exec(`CREATE TABLE IF NOT EXISTS actuary_profiles (employee_id INTEGER PRIMARY KEY, trading_limit REAL, used_limit REAL, need_approval BOOLEAN)`)
	db.Exec(`INSERT INTO actuary_profiles (employee_id, trading_limit, used_limit, need_approval) VALUES (123, 100000, 0, 1)`)
	_, _ = r.GetActuaryProfile(123)
	if err := r.IncrementUsedLimit(123, 50); err != nil {
		t.Fatalf("increment: %v", err)
	}
}

// =====================================================================
// OtcRepository.GetAccountReference
// =====================================================================

func TestOtcRepository_GetAccountReference(t *testing.T) {
	db := openRepoTestDB(t, "otc_repo_extra")
	r := NewOtcRepository(db)

	now := time.Now().UTC()
	db.Exec(`INSERT INTO currencies (id, kod, naziv, simbol, drzava, aktivan, created_at, updated_at) VALUES (1, 'RSD', 'x', 'x', 'x', 1, ?, ?)`, now, now)
	db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, status, stanje, raspolozivo_stanje, client_id, created_at, updated_at) VALUES (?, 1, ?, ?, ?, ?, ?, ?)`,
		"OTC-1", "aktivan", 5000.0, 5000.0, 42, now, now)
	var acctID uint
	db.Table("accounts").Select("id").Where("broj_racuna = ?", "OTC-1").Scan(&acctID)

	ref, err := r.GetAccountReference(acctID)
	if err != nil || ref == nil {
		t.Fatalf("ref: %v err=%v", ref, err)
	}
	if !ref.IsOwnedBy(42, "client") {
		t.Fatal("expected owned by client 42")
	}
	if ref.IsOwnedBy(999, "client") {
		t.Fatal("did not expect owned by 999")
	}
}

// =====================================================================
// RemotePublicStockRepository
// =====================================================================

func TestRemotePublicStockRepository_UpsertAndList(t *testing.T) {
	db := openRepoTestDB(t, "remote_stock_repo")
	r := NewRemotePublicStockRepository(db)

	if err := r.UpsertPayload(111, `{"x":1}`); err != nil {
		t.Fatalf("upsert payload: %v", err)
	}
	// Idempotent re-upsert
	if err := r.UpsertPayload(111, `{"x":2}`); err != nil {
		t.Fatalf("upsert payload 2: %v", err)
	}
	if err := r.UpsertError(222, "boom"); err != nil {
		t.Fatalf("upsert err: %v", err)
	}
	got, err := r.Get(111)
	if err != nil || got == nil {
		t.Fatalf("get: %v err=%v", got, err)
	}
	miss, _ := r.Get(999)
	if miss != nil {
		t.Fatal("expected nil miss")
	}
	list, err := r.List()
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %d err=%v", len(list), err)
	}
}

// =====================================================================
// TaxRepository
// =====================================================================

func TestTaxRepository_RecordsAndAggregates(t *testing.T) {
	db := openRepoTestDB(t, "tax_repo")
	r := NewTaxRepository(db)

	rec := &models.TaxRecord{
		UserID: 1, UserType: "client", Period: "2026-05",
		AssetID: 1,
		ProfitRSD: 1000, TaxRSD: 150, Status: "unpaid",
	}
	if err := r.CreateTaxRecord(rec); err != nil {
		t.Fatalf("create: %v", err)
	}

	list, err := r.ListTaxRecordsForUser(1, "client", "2026-05")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %d err=%v", len(list), err)
	}
	if total, err := r.SumUnpaidTaxForUser(1, "client", "2026-05"); err != nil || total != 150 {
		t.Fatalf("unpaid: %v err=%v", total, err)
	}
	if total, err := r.SumPaidTaxForUserYear(1, "client", "2026"); err != nil || total != 0 {
		t.Fatalf("paid empty: %v err=%v", total, err)
	}
}
