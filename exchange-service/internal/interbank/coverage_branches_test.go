package interbank

import (
	"context"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// validParsedAcceptance builds a parsed acceptance tx + the matching
// negotiation model so term-mismatch branches can be exercised one at a time.
func validParsedAcceptance(t *testing.T) (*otcAcceptanceTx, models.InterbankOtcNegotiation) {
	t.Helper()
	neg := models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 111, NegotiationID: "neg-1",
		BuyerRoutingNumber: 333, BuyerID: "client-1",
		SellerRoutingNumber: 111, SellerID: "client-9",
		StockTicker: "AAPL", Amount: 4,
		PricePerUnitCurrency: "USD", PricePerUnitAmount: 10,
		PremiumCurrency: "RSD", PremiumAmount: 4000,
		SettlementDate: futureSettlement(),
	}
	tx := buildOptionAcceptanceTx(111, &neg)
	parsed, reason := parseOtcAcceptance(&tx)
	if reason != nil {
		t.Fatalf("setup parse failed: %+v", reason)
	}
	return parsed, neg
}

func TestMatchAcceptanceTerms_Mismatches(t *testing.T) {
	base := func() (*otcAcceptanceTx, models.InterbankOtcNegotiation) { return validParsedAcceptance(t) }

	// Baseline matches.
	if p, neg := base(); matchAcceptanceTerms(p, &neg) != nil {
		t.Fatal("baseline should match")
	}

	mutators := map[string]func(*models.InterbankOtcNegotiation){
		"buyer":      func(n *models.InterbankOtcNegotiation) { n.BuyerID = "other" },
		"seller":     func(n *models.InterbankOtcNegotiation) { n.SellerID = "other" },
		"premiumCur": func(n *models.InterbankOtcNegotiation) { n.PremiumCurrency = "EUR" },
		"premiumAmt": func(n *models.InterbankOtcNegotiation) { n.PremiumAmount = 1 },
		"ticker":     func(n *models.InterbankOtcNegotiation) { n.StockTicker = "MSFT" },
		"priceCur":   func(n *models.InterbankOtcNegotiation) { n.PricePerUnitCurrency = "EUR" },
		"priceAmt":   func(n *models.InterbankOtcNegotiation) { n.PricePerUnitAmount = 99 },
		"settlement": func(n *models.InterbankOtcNegotiation) { n.SettlementDate = "2099-01-01T00:00:00Z" },
		"amount":     func(n *models.InterbankOtcNegotiation) { n.Amount = 99 },
		"negRouting": func(n *models.InterbankOtcNegotiation) { n.NegotiationRoutingNumber = 222 },
	}
	for name, mut := range mutators {
		p, neg := base()
		mut(&neg)
		if matchAcceptanceTerms(p, &neg) == nil {
			t.Errorf("%s: expected a NoVoteReason", name)
		}
	}
}

func TestParseOtcAcceptance_Errors(t *testing.T) {
	person := func(id string) TxAccount {
		return TxAccount{Type: TxAccountPerson, ID: &ForeignBankId{RoutingNumber: 1, ID: id}}
	}
	monas := Asset{Type: AssetMonas, Monas: &MonetaryAsset{Currency: "RSD"}}

	// Wrong posting count.
	if _, r := parseOtcAcceptance(&Transaction{Postings: make([]Posting, 3)}); r == nil {
		t.Error("3 postings should fail")
	}
	// ACCOUNT-typed posting (not PERSON).
	tx := &Transaction{Postings: []Posting{
		{Account: TxAccount{Type: TxAccountAccount, Num: "1"}, Amount: -1, Asset: monas},
		{Account: person("b"), Amount: 1, Asset: monas},
		{Account: person("c"), Amount: 1, Asset: monas},
		{Account: person("d"), Amount: 1, Asset: monas},
	}}
	if _, r := parseOtcAcceptance(tx); r == nil {
		t.Error("ACCOUNT-typed posting should fail")
	}
	// Unknown asset type.
	tx2 := &Transaction{Postings: []Posting{
		{Account: person("a"), Amount: -1, Asset: Asset{Type: "WEIRD"}},
		{Account: person("b"), Amount: 1, Asset: monas},
		{Account: person("c"), Amount: 1, Asset: monas},
		{Account: person("d"), Amount: 1, Asset: monas},
	}}
	if _, r := parseOtcAcceptance(tx2); r == nil {
		t.Error("unknown asset should fail")
	}
}

func TestCheckBalancedAndAssetGroupKey(t *testing.T) {
	unbal := &Transaction{Postings: []Posting{
		{Amount: 5, Asset: Asset{Type: AssetMonas, Monas: &MonetaryAsset{Currency: "RSD"}}},
	}}
	if checkBalanced(unbal) == nil {
		t.Error("unbalanced should return reason")
	}
	if assetGroupKey(&Asset{Type: AssetMonas}) != "monas:?" {
		t.Error("nil monas key")
	}
	if assetGroupKey(&Asset{Type: AssetStock}) != "stock:?" {
		t.Error("nil stock key")
	}
	if assetGroupKey(&Asset{Type: AssetOption}) != "option:?" {
		t.Error("nil option key")
	}
	if assetGroupKey(&Asset{Type: "X"}) != "?" {
		t.Error("unknown key")
	}
	if sameOption(nil, nil) {
		t.Error("nil options should not be same")
	}
}

func TestParseExerciseTx_MoreErrors(t *testing.T) {
	monas := Asset{Type: AssetMonas, Monas: &MonetaryAsset{Currency: "USD"}}
	stock := Asset{Type: AssetStock, Stock: &StockDescription{Ticker: "AAPL"}}
	opt := func(id string) TxAccount { return TxAccount{Type: TxAccountOption, ID: &ForeignBankId{RoutingNumber: 333, ID: id}} }
	per := func(id string) TxAccount { return TxAccount{Type: TxAccountPerson, ID: &ForeignBankId{RoutingNumber: 111, ID: id}} }

	// OPTION account with nil ID.
	bad := &Transaction{Postings: []Posting{
		{Account: TxAccount{Type: TxAccountOption}, Amount: 40, Asset: monas},
		{Account: per("b"), Amount: -40, Asset: monas},
		{Account: opt("n"), Amount: -4, Asset: stock},
		{Account: per("b"), Amount: 4, Asset: stock},
	}}
	if _, r := parseExerciseTx(bad); r == nil {
		t.Error("nil option id should fail")
	}

	// Mismatched option negotiation between the two option legs.
	bad2 := &Transaction{Postings: []Posting{
		{Account: opt("n1"), Amount: 40, Asset: monas},
		{Account: per("b"), Amount: -40, Asset: monas},
		{Account: opt("n2"), Amount: -4, Asset: stock},
		{Account: per("b"), Amount: 4, Asset: stock},
	}}
	if _, r := parseExerciseTx(bad2); r == nil {
		t.Error("mismatched option negs should fail")
	}

	// Wrong cash signs (option cash negative).
	bad3 := &Transaction{Postings: []Posting{
		{Account: opt("n"), Amount: -40, Asset: monas},
		{Account: per("b"), Amount: 40, Asset: monas},
		{Account: opt("n"), Amount: -4, Asset: stock},
		{Account: per("b"), Amount: 4, Asset: stock},
	}}
	if _, r := parseExerciseTx(bad3); r == nil {
		t.Error("wrong cash signs should fail")
	}
}

func TestExerciseOnCommit_ErrorStates(t *testing.T) {
	db := openInterbankTestDB(t, "ex_commit_errs")
	p := newExerciseProcessor(db)
	partner := &PartnerBank{Code: 111}

	// Unknown commit.
	if err := p.OnCommitTx(context.Background(), partner, ForeignBankId{RoutingNumber: 111, ID: "nope"}); err == nil {
		t.Error("unknown commit should error")
	}
	// Unknown rollback → nil.
	if err := p.OnRollbackTx(context.Background(), partner, ForeignBankId{RoutingNumber: 111, ID: "nope"}); err != nil {
		t.Error("unknown rollback should be nil")
	}
}

func TestPaymentOnCommit_ErrorStates(t *testing.T) {
	db := openInterbankTestDB(t, "pay_commit_errs")
	p := newPaymentProcessor(db)
	partner := &PartnerBank{Code: 111}
	if err := p.OnCommitTx(context.Background(), partner, ForeignBankId{RoutingNumber: 111, ID: "nope"}); err == nil {
		t.Error("unknown payment commit should error")
	}
}

func TestPaymentOnNewTx_InactiveAndCurrencyMismatch(t *testing.T) {
	db := openInterbankTestDB(t, "pay_inactive")
	// Inactive account.
	cur := seedCurrency(t, db, "RSD")
	db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, status, stanje, raspolozivo_stanje, client_id, created_at, updated_at)
		VALUES ('333000777', ?, 'neaktivan', 500, 500, 7, datetime('now'), datetime('now'))`, cur)

	p := newPaymentProcessor(db)
	partner := &PartnerBank{Code: 111}
	tx := BuildPaymentTx(ForeignBankId{RoutingNumber: 111, ID: "p"}, "111000111", "333000777", "RSD", 100, "", "", "")
	if v, _ := p.OnNewTx(context.Background(), partner, &tx); v.Vote != VoteNo {
		t.Error("inactive account should vote NO")
	}

	// Currency mismatch.
	eur := seedCurrency(t, db, "EUR")
	db.Exec(`INSERT INTO accounts (broj_racuna, currency_id, status, stanje, raspolozivo_stanje, client_id, created_at, updated_at)
		VALUES ('333000888', ?, 'aktivan', 500, 500, 8, datetime('now'), datetime('now'))`, eur)
	tx2 := BuildPaymentTx(ForeignBankId{RoutingNumber: 111, ID: "q"}, "111000111", "333000888", "RSD", 100, "", "", "")
	if v, _ := p.OnNewTx(context.Background(), partner, &tx2); v.Vote != VoteNo {
		t.Error("currency mismatch should vote NO")
	}
}

func TestOtcOnNewTx_WrongPartnerAndNotOngoing(t *testing.T) {
	db := openInterbankTestDB(t, "otc_wrongpartner")
	neg := seedOtcNegotiation(t, db) // seller routing 111, buyer routing 333 (us)
	p := newOtcProcessor(db)
	tx := acceptanceTxFor(neg)

	// Partner that isn't the seller's bank.
	if v, _ := p.OnNewTx(context.Background(), &PartnerBank{Code: 222}, &tx); v.Vote != VoteNo {
		t.Error("wrong partner should vote NO")
	}

	// Not ongoing.
	_ = repository.NewInterbankOtcRepository(db).MarkClosed(111, neg.NegotiationID)
	rsd := seedCurrency(t, db, "RSD")
	seedClientAccount(t, db, 1, rsd, "333000111", 10000, 10000)
	if v, _ := p.OnNewTx(context.Background(), &PartnerBank{Code: 111}, &tx); v.Vote != VoteNo {
		t.Error("closed negotiation should vote NO")
	}
}
