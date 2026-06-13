package interbank

import (
	"context"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"gorm.io/gorm"
)

func futureSettlement() string { return time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339) }

// ---------------------------------------------------------------------------
// OTC option-acceptance processor (tx_processor.go)
// ---------------------------------------------------------------------------

func seedOtcNegotiation(t *testing.T, db *gorm.DB) *models.InterbankOtcNegotiation {
	t.Helper()
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber:  111,
		NegotiationID:             "neg-1",
		LocalRole:                 models.InterbankNegotiationRoleBuyer,
		CounterpartyRoutingNumber: 111,
		BuyerRoutingNumber:        333,
		BuyerID:                   "client-1",
		SellerRoutingNumber:       111,
		SellerID:                  "client-9",
		StockTicker:               "AAPL",
		Amount:                    4,
		PricePerUnitCurrency:      "USD",
		PricePerUnitAmount:        10,
		PremiumCurrency:           "RSD",
		PremiumAmount:             4000,
		SettlementDate:            futureSettlement(),
		LastModifiedByRoutingNumber: 111,
		LastModifiedByID:          "client-9",
		IsOngoing:                 true,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	}
	if err := repository.NewInterbankOtcRepository(db).Create(neg); err != nil {
		t.Fatalf("seed negotiation: %v", err)
	}
	return neg
}

func newOtcProcessor(db *gorm.DB) *OtcTxProcessor {
	reg, _ := NewRegistryFromJSON(333, `[{"code":111,"baseUrl":"http://p","outboundKey":"o","inboundKey":"i"}]`)
	return NewOtcTxProcessor(
		db, reg,
		repository.NewInterbankOtcRepository(db),
		repository.NewInterbankPendingTxRepository(db),
		repository.NewInterbankOptionContractRepository(db),
		repository.NewInterbankWalletRepository(db),
	)
}

func acceptanceTxFor(neg *models.InterbankOtcNegotiation) Transaction {
	tx := buildOptionAcceptanceTx(111, neg)
	tx.TransactionID = ForeignBankId{RoutingNumber: 111, ID: "otc-tx-1"}
	return tx
}

func TestOtcProcessor_FullLifecycle(t *testing.T) {
	db := openInterbankTestDB(t, "otc_full")
	neg := seedOtcNegotiation(t, db)
	rsd := seedCurrency(t, db, "RSD")
	seedClientAccount(t, db, 1, rsd, "333000111", 10000, 10000)

	p := newOtcProcessor(db)
	partner := &PartnerBank{Code: 111}
	tx := acceptanceTxFor(neg)

	vote, err := p.OnNewTx(context.Background(), partner, &tx)
	if err != nil || vote.Vote != VoteYes {
		t.Fatalf("OnNewTx = %+v, %v", vote, err)
	}
	// raspolozivo reserved.
	var rasp float64
	db.Raw(`SELECT raspolozivo_stanje FROM accounts WHERE client_id=1`).Scan(&rasp)
	if rasp != 6000 {
		t.Fatalf("raspolozivo = %v, want 6000", rasp)
	}
	// Replay → still YES, no double reserve.
	if vote, err := p.OnNewTx(context.Background(), partner, &tx); err != nil || vote.Vote != VoteYes {
		t.Fatalf("replay OnNewTx = %+v, %v", vote, err)
	}

	// Commit.
	if err := p.OnCommitTx(context.Background(), partner, tx.TransactionID); err != nil {
		t.Fatalf("OnCommitTx: %v", err)
	}
	var stanje float64
	db.Raw(`SELECT stanje FROM accounts WHERE client_id=1`).Scan(&stanje)
	if stanje != 6000 {
		t.Fatalf("stanje = %v, want 6000 (10000-4000 debit)", stanje)
	}
	// Option contract created, negotiation closed.
	if c, _ := repository.NewInterbankOptionContractRepository(db).Get(111, "neg-1"); c == nil {
		t.Fatal("option contract not created")
	}
	if got, _ := repository.NewInterbankOtcRepository(db).Get(111, "neg-1"); got.IsOngoing {
		t.Fatal("negotiation not closed after commit")
	}
	// Idempotent commit replay.
	if err := p.OnCommitTx(context.Background(), partner, tx.TransactionID); err != nil {
		t.Fatalf("commit replay: %v", err)
	}
}

func TestOtcProcessor_Rollback(t *testing.T) {
	db := openInterbankTestDB(t, "otc_rollback")
	neg := seedOtcNegotiation(t, db)
	rsd := seedCurrency(t, db, "RSD")
	seedClientAccount(t, db, 1, rsd, "333000111", 10000, 10000)

	p := newOtcProcessor(db)
	partner := &PartnerBank{Code: 111}
	tx := acceptanceTxFor(neg)

	if _, err := p.OnNewTx(context.Background(), partner, &tx); err != nil {
		t.Fatal(err)
	}
	if err := p.OnRollbackTx(context.Background(), partner, tx.TransactionID); err != nil {
		t.Fatalf("OnRollbackTx: %v", err)
	}
	var rasp float64
	db.Raw(`SELECT raspolozivo_stanje FROM accounts WHERE client_id=1`).Scan(&rasp)
	if rasp != 10000 {
		t.Fatalf("raspolozivo = %v, want 10000 (released)", rasp)
	}
	// Rollback of unknown tx is a no-op success.
	if err := p.OnRollbackTx(context.Background(), partner, ForeignBankId{RoutingNumber: 111, ID: "ghost"}); err != nil {
		t.Fatalf("rollback unknown: %v", err)
	}
	// Idempotent rollback replay.
	if err := p.OnRollbackTx(context.Background(), partner, tx.TransactionID); err != nil {
		t.Fatalf("rollback replay: %v", err)
	}
}

func TestOtcProcessor_VoteNoPaths(t *testing.T) {
	db := openInterbankTestDB(t, "otc_voteno")
	p := newOtcProcessor(db)
	partner := &PartnerBank{Code: 111}

	// Unbalanced / wrong shape.
	if v, _ := p.OnNewTx(context.Background(), partner, &Transaction{Postings: []Posting{}}); v.Vote != VoteNo {
		t.Error("empty tx should vote NO")
	}

	// Valid shape but no stored negotiation → OPTION_NEGOTIATION_NOT_FOUND.
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 111, NegotiationID: "absent",
		BuyerRoutingNumber: 333, BuyerID: "client-1",
		SellerRoutingNumber: 111, SellerID: "client-9",
		StockTicker: "AAPL", Amount: 4, PricePerUnitCurrency: "USD", PricePerUnitAmount: 10,
		PremiumCurrency: "RSD", PremiumAmount: 4000, SettlementDate: futureSettlement(),
	}
	tx := buildOptionAcceptanceTx(111, neg)
	tx.TransactionID = ForeignBankId{RoutingNumber: 111, ID: "x"}
	v, err := p.OnNewTx(context.Background(), partner, &tx)
	if err != nil || v.Vote != VoteNo {
		t.Fatalf("absent negotiation = %+v, %v", v, err)
	}

	// Insufficient buyer funds.
	seedOtcNegotiation(t, db)
	rsd := seedCurrency(t, db, "RSD")
	seedClientAccount(t, db, 1, rsd, "333000111", 100, 100) // not enough
	stored, _ := repository.NewInterbankOtcRepository(db).Get(111, "neg-1")
	tx2 := acceptanceTxFor(stored)
	v2, err := p.OnNewTx(context.Background(), partner, &tx2)
	if err != nil || v2.Vote != VoteNo {
		t.Fatalf("insufficient funds = %+v, %v", v2, err)
	}
}

// ---------------------------------------------------------------------------
// Payment processor (payment_tx_processor.go)
// ---------------------------------------------------------------------------

func newPaymentProcessor(db *gorm.DB) *PaymentTxProcessor {
	reg, _ := NewRegistryFromJSON(333, `[{"code":111,"baseUrl":"http://p","outboundKey":"o","inboundKey":"i"}]`)
	return NewPaymentTxProcessor(
		db, reg,
		repository.NewInterbankPaymentRepository(db),
		repository.NewInterbankPaymentWalletRepository(db),
	)
}

func TestPaymentProcessor_FullLifecycle(t *testing.T) {
	db := openInterbankTestDB(t, "pay_full")
	rsd := seedCurrency(t, db, "RSD")
	recvID := seedClientAccount(t, db, 7, rsd, "333000777", 500, 500)
	_ = recvID

	p := newPaymentProcessor(db)
	partner := &PartnerBank{Code: 111}
	txID := ForeignBankId{RoutingNumber: 111, ID: "pay-1"}
	tx := BuildPaymentTx(txID, "111000111", "333000777", "RSD", 250, "hi", "289", "purpose")

	vote, err := p.OnNewTx(context.Background(), partner, &tx)
	if err != nil || vote.Vote != VoteYes {
		t.Fatalf("OnNewTx = %+v, %v", vote, err)
	}
	// Replay YES.
	if vote, _ := p.OnNewTx(context.Background(), partner, &tx); vote.Vote != VoteYes {
		t.Fatal("replay should be YES")
	}
	// Commit credits recipient.
	if err := p.OnCommitTx(context.Background(), partner, txID); err != nil {
		t.Fatalf("commit: %v", err)
	}
	var stanje float64
	db.Raw(`SELECT stanje FROM accounts WHERE client_id=7`).Scan(&stanje)
	if stanje != 750 {
		t.Fatalf("stanje = %v, want 750", stanje)
	}
	if err := p.OnCommitTx(context.Background(), partner, txID); err != nil {
		t.Fatalf("commit replay: %v", err)
	}
}

func TestPaymentProcessor_Rollback(t *testing.T) {
	db := openInterbankTestDB(t, "pay_rollback")
	rsd := seedCurrency(t, db, "RSD")
	seedClientAccount(t, db, 7, rsd, "333000777", 500, 500)

	p := newPaymentProcessor(db)
	partner := &PartnerBank{Code: 111}
	txID := ForeignBankId{RoutingNumber: 111, ID: "pay-2"}
	tx := BuildPaymentTx(txID, "111000111", "333000777", "RSD", 250, "", "", "")
	if _, err := p.OnNewTx(context.Background(), partner, &tx); err != nil {
		t.Fatal(err)
	}
	if err := p.OnRollbackTx(context.Background(), partner, txID); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	// No wallet effect on rollback.
	var stanje float64
	db.Raw(`SELECT stanje FROM accounts WHERE client_id=7`).Scan(&stanje)
	if stanje != 500 {
		t.Fatalf("stanje = %v, want 500 (unchanged)", stanje)
	}
	// Unknown rollback → success.
	if err := p.OnRollbackTx(context.Background(), partner, ForeignBankId{RoutingNumber: 111, ID: "ghost"}); err != nil {
		t.Fatal(err)
	}
}

func TestPaymentProcessor_VoteNoPaths(t *testing.T) {
	db := openInterbankTestDB(t, "pay_voteno")
	p := newPaymentProcessor(db)
	partner := &PartnerBank{Code: 111}

	// Recipient routing isn't us (recipient account starts with 111 not 333).
	tx := BuildPaymentTx(ForeignBankId{RoutingNumber: 111, ID: "a"}, "111000111", "111000222", "RSD", 100, "", "", "")
	if v, _ := p.OnNewTx(context.Background(), partner, &tx); v.Vote != VoteNo {
		t.Error("non-local recipient should vote NO")
	}

	// Sender routing doesn't match partner.
	tx2 := BuildPaymentTx(ForeignBankId{RoutingNumber: 222, ID: "b"}, "222000111", "333000777", "RSD", 100, "", "", "")
	if v, _ := p.OnNewTx(context.Background(), partner, &tx2); v.Vote != VoteNo {
		t.Error("sender/partner mismatch should vote NO")
	}

	// Recipient account doesn't exist → NO_SUCH_ACCOUNT.
	tx3 := BuildPaymentTx(ForeignBankId{RoutingNumber: 111, ID: "c"}, "111000111", "333000999", "RSD", 100, "", "", "")
	if v, _ := p.OnNewTx(context.Background(), partner, &tx3); v.Vote != VoteNo {
		t.Error("missing recipient account should vote NO")
	}
}

// ---------------------------------------------------------------------------
// Exercise processor (exercise_tx_processor.go)
// ---------------------------------------------------------------------------

func newExerciseProcessor(db *gorm.DB) *ExerciseTxProcessor {
	reg, _ := NewRegistryFromJSON(333, `[{"code":111,"baseUrl":"http://p","outboundKey":"o","inboundKey":"i"}]`)
	return NewExerciseTxProcessor(
		db, reg,
		repository.NewInterbankOtcRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
		repository.NewInterbankWalletRepository(db),
	)
}

func exerciseTxFor() Transaction {
	neg := ForeignBankId{RoutingNumber: 333, ID: "neg-x"}
	buyer := ForeignBankId{RoutingNumber: 111, ID: "emp-1"}
	monas := func() Asset { return Asset{Type: AssetMonas, Monas: &MonetaryAsset{Currency: "USD"}} }
	stock := func() Asset { return Asset{Type: AssetStock, Stock: &StockDescription{Ticker: "AAPL"}} }
	return Transaction{
		Postings: []Posting{
			{Account: TxAccount{Type: TxAccountOption, ID: &neg}, Amount: 40, Asset: monas()},
			{Account: TxAccount{Type: TxAccountPerson, ID: &buyer}, Amount: -40, Asset: monas()},
			{Account: TxAccount{Type: TxAccountOption, ID: &neg}, Amount: -4, Asset: stock()},
			{Account: TxAccount{Type: TxAccountPerson, ID: &buyer}, Amount: 4, Asset: stock()},
		},
		TransactionID: ForeignBankId{RoutingNumber: 111, ID: "ex-tx-1"},
	}
}

func seedExerciseNegotiation(t *testing.T, db *gorm.DB) {
	t.Helper()
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber:  333,
		NegotiationID:             "neg-x",
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
		IsOngoing:                 false,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	}
	if err := repository.NewInterbankOtcRepository(db).Create(neg); err != nil {
		t.Fatalf("seed exercise neg: %v", err)
	}
}

func TestExerciseProcessor_FullLifecycle(t *testing.T) {
	db := openInterbankTestDB(t, "ex_full")
	seedExerciseNegotiation(t, db)
	assetID := seedListing(t, db, "AAPL")
	usd := seedCurrency(t, db, "USD")
	seedClientAccount(t, db, 5, usd, "333000555", 0, 0)
	// Seller holds the stock to deliver.
	holding := models.PortfolioHoldingRecord{
		UserID: 5, UserType: "client", AssetID: assetID,
		Quantity: 10, AvgBuyPrice: 8, AccountID: 1,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	p := newExerciseProcessor(db)
	partner := &PartnerBank{Code: 111}
	tx := exerciseTxFor()

	vote, err := p.OnNewTx(context.Background(), partner, &tx)
	if err != nil || vote.Vote != VoteYes {
		t.Fatalf("OnNewTx = %+v, %v", vote, err)
	}
	// Replay YES.
	if vote, _ := p.OnNewTx(context.Background(), partner, &tx); vote.Vote != VoteYes {
		t.Fatal("replay should be YES")
	}

	if err := p.OnCommitTx(context.Background(), partner, tx.TransactionID); err != nil {
		t.Fatalf("commit: %v", err)
	}
	// Seller credited cash (40) and stock reduced.
	var stanje, qty float64
	db.Raw(`SELECT stanje FROM accounts WHERE client_id=5`).Scan(&stanje)
	db.Raw(`SELECT quantity FROM portfolio_holdings WHERE user_id=5`).Scan(&qty)
	if stanje != 40 {
		t.Fatalf("seller stanje = %v, want 40", stanje)
	}
	if qty != 6 {
		t.Fatalf("seller qty = %v, want 6", qty)
	}
	if err := p.OnCommitTx(context.Background(), partner, tx.TransactionID); err != nil {
		t.Fatalf("commit replay: %v", err)
	}
}

func TestExerciseProcessor_Rollback(t *testing.T) {
	db := openInterbankTestDB(t, "ex_rollback")
	seedExerciseNegotiation(t, db)
	seedListing(t, db, "AAPL")

	p := newExerciseProcessor(db)
	partner := &PartnerBank{Code: 111}
	tx := exerciseTxFor()
	if _, err := p.OnNewTx(context.Background(), partner, &tx); err != nil {
		t.Fatal(err)
	}
	if err := p.OnRollbackTx(context.Background(), partner, tx.TransactionID); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	// Unknown rollback → no-op.
	if err := p.OnRollbackTx(context.Background(), partner, ForeignBankId{RoutingNumber: 111, ID: "ghost"}); err != nil {
		t.Fatal(err)
	}
}

func TestExerciseProcessor_VoteNoPaths(t *testing.T) {
	db := openInterbankTestDB(t, "ex_voteno")
	p := newExerciseProcessor(db)
	partner := &PartnerBank{Code: 111}

	// No negotiation stored.
	tx := exerciseTxFor()
	if v, err := p.OnNewTx(context.Background(), partner, &tx); err != nil || v.Vote != VoteNo {
		t.Fatalf("missing neg = %+v, %v", v, err)
	}

	// Option routing not ours.
	neg := ForeignBankId{RoutingNumber: 999, ID: "neg-x"}
	bad := exerciseTxFor()
	bad.Postings[0].Account.ID = &neg
	bad.Postings[2].Account.ID = &neg
	if v, _ := p.OnNewTx(context.Background(), partner, &bad); v.Vote != VoteNo {
		t.Error("foreign option routing should vote NO")
	}
}

// ---------------------------------------------------------------------------
// Dispatch processor (dispatch_tx_processor.go)
// ---------------------------------------------------------------------------

func TestDispatchProcessor_Routing(t *testing.T) {
	db := openInterbankTestDB(t, "dispatch")
	rsd := seedCurrency(t, db, "RSD")
	seedClientAccount(t, db, 7, rsd, "333000777", 500, 500)

	otc := newOtcProcessor(db)
	pay := newPaymentProcessor(db)
	ex := newExerciseProcessor(db)
	d := NewDispatchTxProcessor(
		otc, pay, ex,
		repository.NewInterbankPaymentRepository(db),
		repository.NewInterbankPendingTxRepository(db),
		repository.NewInterbankExerciseRepository(db),
	)
	partner := &PartnerBank{Code: 111}

	// Payment shape routes to payment processor and persists a row.
	txID := ForeignBankId{RoutingNumber: 111, ID: "disp-pay"}
	payTx := BuildPaymentTx(txID, "111000111", "333000777", "RSD", 100, "", "", "")
	if v, err := d.OnNewTx(context.Background(), partner, &payTx); err != nil || v.Vote != VoteYes {
		t.Fatalf("dispatch payment = %+v, %v", v, err)
	}
	// Commit routes by txID lookup.
	if err := d.OnCommitTx(context.Background(), partner, txID); err != nil {
		t.Fatalf("dispatch commit: %v", err)
	}

	// Unknown txID commit → error; rollback → nil.
	if err := d.OnCommitTx(context.Background(), partner, ForeignBankId{RoutingNumber: 111, ID: "ghost"}); err == nil {
		t.Error("unknown commit should error")
	}
	if err := d.OnRollbackTx(context.Background(), partner, ForeignBankId{RoutingNumber: 111, ID: "ghost"}); err != nil {
		t.Error("unknown rollback should be nil")
	}

	// Unmatched shape → vote NO.
	weird := Transaction{Postings: []Posting{{Account: TxAccount{Type: TxAccountPerson, ID: &ForeignBankId{}}, Asset: Asset{Type: AssetStock, Stock: &StockDescription{Ticker: "X"}}}}}
	if v, _ := d.OnNewTx(context.Background(), partner, &weird); v.Vote != VoteNo {
		t.Error("unmatched shape should vote NO")
	}
}

func TestDispatchProcessor_NilInners(t *testing.T) {
	db := openInterbankTestDB(t, "dispatch_nil")
	d := NewDispatchTxProcessor(nil, nil, nil,
		repository.NewInterbankPaymentRepository(db),
		repository.NewInterbankPendingTxRepository(db),
		repository.NewInterbankExerciseRepository(db),
	)
	partner := &PartnerBank{Code: 111}
	payTx := BuildPaymentTx(ForeignBankId{RoutingNumber: 111, ID: "x"}, "111000111", "333000777", "RSD", 100, "", "", "")
	if v, _ := d.OnNewTx(context.Background(), partner, &payTx); v.Vote != VoteNo {
		t.Error("nil payment processor should vote NO")
	}
	ex := exerciseTxFor()
	if v, _ := d.OnNewTx(context.Background(), partner, &ex); v.Vote != VoteNo {
		t.Error("nil exercise processor should vote NO")
	}
	otc := Transaction{Postings: make([]Posting, 4)}
	if v, _ := d.OnNewTx(context.Background(), partner, &otc); v.Vote != VoteNo {
		t.Error("nil otc processor should vote NO")
	}
}

// ---------------------------------------------------------------------------
// Parse / shape helpers
// ---------------------------------------------------------------------------

func TestParsePaymentTx_Errors(t *testing.T) {
	cases := []*Transaction{
		{Postings: []Posting{}},
		{Postings: []Posting{{Account: TxAccount{Type: TxAccountPerson}}, {Account: TxAccount{Type: TxAccountAccount, Num: "333"}}}},
	}
	for i, tx := range cases {
		if _, reason := parsePaymentTx(tx); reason == nil {
			t.Errorf("case %d: expected reason", i)
		}
	}
}

func TestIsPaymentAndExerciseShape(t *testing.T) {
	pay := BuildPaymentTx(ForeignBankId{}, "111", "333", "RSD", 1, "", "", "")
	if !IsPaymentShape(&pay) {
		t.Error("payment tx should match payment shape")
	}
	if IsExerciseShape(&pay) {
		t.Error("payment tx should not match exercise shape")
	}
	ex := exerciseTxFor()
	if !IsExerciseShape(&ex) {
		t.Error("exercise tx should match exercise shape")
	}
}

func TestParseSettlementAndSameDate(t *testing.T) {
	if _, ok := parseSettlement(""); ok {
		t.Error("empty should be not-ok")
	}
	if _, ok := parseSettlement("2025-04-16"); !ok {
		t.Error("date-only should parse")
	}
	if _, ok := parseSettlement("garbage"); ok {
		t.Error("garbage should be not-ok")
	}
	if !sameSettlementDate("2025-04-16T15:32:44+02:00", "2025-04-16T13:32:44Z") {
		t.Error("equivalent instants should match")
	}
	if sameSettlementDate("2025-04-16", "garbage") {
		t.Error("unparseable should not match")
	}
}
