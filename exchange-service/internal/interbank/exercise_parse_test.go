package interbank

import "testing"

// TestParseExerciseTx_AcceptsAccountTypedCashLeg pins the §2.6 rule that
// monetary values are held by ACCOUNTs (a currency account number) while
// stocks are held by PERSONs. An option-exercise NEW_TX whose buyer CASH
// leg is an ACCOUNT — with the buyer STOCK leg as PERSON — must parse, not
// be rejected as UNACCEPTABLE_ASSET. This is exactly the shape partner bank
// 111 sent exercising an AAPL option against us, which we previously
// NO-voted.
func TestParseExerciseTx_AcceptsAccountTypedCashLeg(t *testing.T) {
	neg := ForeignBankId{RoutingNumber: 333, ID: "neg-x"}
	buyer := ForeignBankId{RoutingNumber: 111, ID: "employee-1"}

	tx := &Transaction{
		Postings: []Posting{
			{ // buyer cash leg as ACCOUNT (currency account number)
				Account: TxAccount{Type: TxAccountAccount, Num: "111000196619164711"},
				Amount:  -4000,
				Asset:   Asset{Type: AssetMonas, Monas: &MonetaryAsset{Currency: "RSD"}},
			},
			{ // option pseudo cash leg
				Account: TxAccount{Type: TxAccountOption, ID: &neg},
				Amount:  4000,
				Asset:   Asset{Type: AssetMonas, Monas: &MonetaryAsset{Currency: "RSD"}},
			},
			{ // option pseudo stock leg
				Account: TxAccount{Type: TxAccountOption, ID: &neg},
				Amount:  -4,
				Asset:   Asset{Type: AssetStock, Stock: &StockDescription{Ticker: "AAPL"}},
			},
			{ // buyer stock leg as PERSON — carries the buyer identity
				Account: TxAccount{Type: TxAccountPerson, ID: &buyer},
				Amount:  4,
				Asset:   Asset{Type: AssetStock, Stock: &StockDescription{Ticker: "AAPL"}},
			},
		},
		TransactionID: ForeignBankId{RoutingNumber: 111, ID: "tx-x"},
	}

	parsed, reason := parseExerciseTx(tx)
	if reason != nil {
		t.Fatalf("parseExerciseTx rejected a spec-valid ACCOUNT cash leg: %+v", *reason)
	}
	if parsed.buyerRouting != 111 || parsed.buyerID != "employee-1" {
		t.Fatalf("buyer identity = %d/%q, want 111/employee-1 (must come from the PERSON stock leg)",
			parsed.buyerRouting, parsed.buyerID)
	}
	if parsed.optionNegID != "neg-x" || parsed.stockTicker != "AAPL" ||
		parsed.stockAmount != 4 || parsed.cashAmount != 4000 {
		t.Fatalf("parsed exercise fields wrong: negID=%q ticker=%q stock=%v cash=%v",
			parsed.optionNegID, parsed.stockTicker, parsed.stockAmount, parsed.cashAmount)
	}
}

// TestParseExerciseTx_RejectsAccountTypedStockLeg ensures stocks must still
// be held by a PERSON: an ACCOUNT-typed stock leg is rejected (only the
// MONAS cash leg may be ACCOUNT-typed).
func TestParseExerciseTx_RejectsAccountTypedStockLeg(t *testing.T) {
	neg := ForeignBankId{RoutingNumber: 333, ID: "neg-x"}
	tx := &Transaction{
		Postings: []Posting{
			{Account: TxAccount{Type: TxAccountAccount, Num: "111000000000000000"}, Amount: -4000, Asset: Asset{Type: AssetMonas, Monas: &MonetaryAsset{Currency: "RSD"}}},
			{Account: TxAccount{Type: TxAccountOption, ID: &neg}, Amount: 4000, Asset: Asset{Type: AssetMonas, Monas: &MonetaryAsset{Currency: "RSD"}}},
			{Account: TxAccount{Type: TxAccountOption, ID: &neg}, Amount: -4, Asset: Asset{Type: AssetStock, Stock: &StockDescription{Ticker: "AAPL"}}},
			{Account: TxAccount{Type: TxAccountAccount, Num: "111000000000000000"}, Amount: 4, Asset: Asset{Type: AssetStock, Stock: &StockDescription{Ticker: "AAPL"}}}, // stock as ACCOUNT — invalid
		},
		TransactionID: ForeignBankId{RoutingNumber: 111, ID: "tx-x"},
	}
	if _, reason := parseExerciseTx(tx); reason == nil {
		t.Fatal("parseExerciseTx accepted an ACCOUNT-typed stock leg; stocks must be held by a PERSON")
	}
}
