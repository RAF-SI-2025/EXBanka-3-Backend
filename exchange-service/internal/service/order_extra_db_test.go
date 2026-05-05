package service_test

import (
	"strings"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"gorm.io/gorm"
)

// --- helpers ---

func floatPtr(v float64) *float64 { return &v }

func newOrderSvc(db *gorm.DB) *service.OrderService {
	return service.NewOrderService(
		repository.NewOrderRepository(db),
		repository.NewMarketRepository(db),
		&mockRateProv{},
	)
}

func seedOrder(t *testing.T, db *gorm.DB, o *models.OrderRecord) *models.OrderRecord {
	t.Helper()
	if err := db.Create(o).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	return o
}

// --- ApproveOrder ---

func TestApproveOrder_NotFound(t *testing.T) {
	db := openTestDB(t, "ao_nf")
	svc := newOrderSvc(db)
	if err := svc.ApproveOrder(9999, 1); err == nil {
		t.Fatal("expected error for missing order")
	}
}

func TestApproveOrder_NotPending(t *testing.T) {
	db := openTestDB(t, "ao_not_pending")
	assetID := seedAsset(t, db, "APR", 50, "USD")
	o := seedOrder(t, db, &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "limit",
		Direction: "sell", Quantity: 1, ContractSize: 1, PricePerUnit: 50,
		Status: "approved", RemainingPortions: 1, AccountID: 1,
	})
	svc := newOrderSvc(db)
	err := svc.ApproveOrder(o.ID, 5)
	if err == nil || !strings.Contains(err.Error(), "not pending") {
		t.Fatalf("expected not-pending error, got %v", err)
	}
}

func TestApproveOrder_Success_ClientSell(t *testing.T) {
	db := openTestDB(t, "ao_ok_sell")
	assetID := seedAsset(t, db, "APROK", 50, "USD")
	o := seedOrder(t, db, &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "limit",
		Direction: "sell", Quantity: 1, ContractSize: 1, PricePerUnit: 50,
		Status: "pending", RemainingPortions: 1, AccountID: 1,
	})
	svc := newOrderSvc(db)
	if err := svc.ApproveOrder(o.ID, 5); err != nil {
		t.Fatalf("ApproveOrder: %v", err)
	}
	got, _ := svc.GetOrder(o.ID)
	if got.Status != "approved" {
		t.Errorf("status=%s, want approved", got.Status)
	}
}

func TestApproveOrder_AutoDeclineOnExpiredSettlement(t *testing.T) {
	db := openTestDB(t, "ao_expired")
	assetID := seedAsset(t, db, "FUT", 50, "USD")
	// Mark this listing as a futures contract with past settlement.
	past := time.Now().UTC().Add(-24 * time.Hour)
	if err := db.Create(&models.FuturesContractRecord{
		ListingID: assetID, ContractSize: 1, ContractUnit: "barrel",
		SettlementDate: past,
	}).Error; err != nil {
		t.Fatal(err)
	}
	o := seedOrder(t, db, &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "limit",
		Direction: "sell", Quantity: 1, ContractSize: 1, PricePerUnit: 50,
		Status: "pending", RemainingPortions: 1, AccountID: 1,
	})
	svc := newOrderSvc(db)
	if err := svc.ApproveOrder(o.ID, 5); err != nil {
		t.Fatalf("ApproveOrder: %v", err)
	}
	got, _ := svc.GetOrder(o.ID)
	if got.Status != "declined" {
		t.Errorf("expected auto-decline due to expired settlement, status=%s", got.Status)
	}
}

// --- DeclineOrder ---

func TestDeclineOrder_NotFound(t *testing.T) {
	db := openTestDB(t, "do_nf")
	svc := newOrderSvc(db)
	if err := svc.DeclineOrder(9999, 1); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestDeclineOrder_NotPending(t *testing.T) {
	db := openTestDB(t, "do_not_pending")
	assetID := seedAsset(t, db, "DEC", 50, "USD")
	o := seedOrder(t, db, &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "limit",
		Direction: "sell", Quantity: 1, ContractSize: 1, PricePerUnit: 50,
		Status: "approved", RemainingPortions: 1, AccountID: 1,
	})
	svc := newOrderSvc(db)
	err := svc.DeclineOrder(o.ID, 5)
	if err == nil || !strings.Contains(err.Error(), "not pending") {
		t.Fatalf("expected not-pending error, got %v", err)
	}
}

func TestDeclineOrder_Success_SellNoRefund(t *testing.T) {
	db := openTestDB(t, "do_ok")
	assetID := seedAsset(t, db, "DECOK", 50, "USD")
	o := seedOrder(t, db, &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "limit",
		Direction: "sell", Quantity: 1, ContractSize: 1, PricePerUnit: 50,
		Status: "pending", RemainingPortions: 1, AccountID: 1, Commission: 1,
	})
	svc := newOrderSvc(db)
	if err := svc.DeclineOrder(o.ID, 5); err != nil {
		t.Fatalf("DeclineOrder: %v", err)
	}
	got, _ := svc.GetOrder(o.ID)
	if got.Status != "declined" {
		t.Errorf("status=%s, want declined", got.Status)
	}
}

// --- CancelOrder ---

func TestCancelOrder_NotFound(t *testing.T) {
	db := openTestDB(t, "co_nf")
	svc := newOrderSvc(db)
	if err := svc.CancelOrder(9999, 1, 0); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestCancelOrder_AlreadyDone(t *testing.T) {
	db := openTestDB(t, "co_done")
	assetID := seedAsset(t, db, "CXLD", 50, "USD")
	o := seedOrder(t, db, &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "limit",
		Direction: "sell", Quantity: 1, ContractSize: 1, PricePerUnit: 50,
		Status: "done", IsDone: true, RemainingPortions: 0, AccountID: 1,
	})
	svc := newOrderSvc(db)
	err := svc.CancelOrder(o.ID, 1, 0)
	if err == nil || !strings.Contains(err.Error(), "already done") {
		t.Fatalf("expected already-done error, got %v", err)
	}
}

func TestCancelOrder_InvalidNewRemaining(t *testing.T) {
	db := openTestDB(t, "co_bad_rem")
	assetID := seedAsset(t, db, "CXLB", 50, "USD")
	o := seedOrder(t, db, &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "limit",
		Direction: "sell", Quantity: 5, ContractSize: 1, PricePerUnit: 50,
		Status: "approved", RemainingPortions: 5, AccountID: 1,
	})
	svc := newOrderSvc(db)
	// newRemaining must be < current remainingPortions; 5 == remaining → invalid
	err := svc.CancelOrder(o.ID, 1, 5)
	if err == nil || !strings.Contains(err.Error(), "newRemaining") {
		t.Fatalf("expected invalid newRemaining error, got %v", err)
	}
	// Negative is also invalid.
	if err := svc.CancelOrder(o.ID, 1, -1); err == nil {
		t.Fatal("expected error for negative newRemaining")
	}
}

// --- ExerciseOption (PortfolioService) ---

func TestExerciseOption_HoldingNotFound(t *testing.T) {
	db := openTestDB(t, "eo_nf")
	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	if err := psvc.ExerciseOption(9999, 1); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestExerciseOption_NotAnOption(t *testing.T) {
	db := openTestDB(t, "eo_not_opt")
	assetID := seedAsset(t, db, "STK", 100, "USD") // type=stock
	holding := models.PortfolioHoldingRecord{
		UserID: 0, UserType: "bank", AssetID: assetID, Quantity: 5, AvgBuyPrice: 100, AccountID: 1,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	err := psvc.ExerciseOption(holding.ID, 1)
	if err == nil || !strings.Contains(err.Error(), "not an option") {
		t.Fatalf("expected not-an-option error, got %v", err)
	}
}

func TestExerciseOption_ZeroQuantity(t *testing.T) {
	db := openTestDB(t, "eo_zero")
	exch := models.MarketExchangeRecord{
		Acronym: "OPT", Name: "OPT", MICCode: "O1", Polity: "X", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatal(err)
	}
	listing := models.MarketListingRecord{
		Ticker: "OPTC", Name: "Option C", Type: "option",
		ExchangeID: exch.ID, Price: 5, Ask: 5.1, Bid: 4.9, Volume: 100,
	}
	if err := db.Create(&listing).Error; err != nil {
		t.Fatal(err)
	}
	holding := models.PortfolioHoldingRecord{
		UserID: 0, UserType: "bank", AssetID: listing.ID, Quantity: 0, AvgBuyPrice: 5, AccountID: 1,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	err := psvc.ExerciseOption(holding.ID, 1)
	if err == nil || !strings.Contains(err.Error(), "no remaining quantity") {
		t.Fatalf("expected no-remaining-quantity error, got %v", err)
	}
}

func TestExerciseOption_NoOptionContractData(t *testing.T) {
	db := openTestDB(t, "eo_no_optdata")
	exch := models.MarketExchangeRecord{
		Acronym: "OPT2", Name: "OPT2", MICCode: "O2", Polity: "X", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatal(err)
	}
	listing := models.MarketListingRecord{
		Ticker: "OPND", Name: "Option ND", Type: "option",
		ExchangeID: exch.ID, Price: 5, Ask: 5.1, Bid: 4.9, Volume: 100,
	}
	if err := db.Create(&listing).Error; err != nil {
		t.Fatal(err)
	}
	holding := models.PortfolioHoldingRecord{
		UserID: 0, UserType: "bank", AssetID: listing.ID, Quantity: 1, AvgBuyPrice: 5, AccountID: 1,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	err := psvc.ExerciseOption(holding.ID, 1)
	if err == nil || !strings.Contains(err.Error(), "option contract data not found") {
		t.Fatalf("expected missing-option-data error, got %v", err)
	}
}

func TestExerciseOption_Expired(t *testing.T) {
	db := openTestDB(t, "eo_expired")
	exch := models.MarketExchangeRecord{
		Acronym: "OPTX", Name: "OPTX", MICCode: "OX", Polity: "X", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatal(err)
	}
	stockListing := models.MarketListingRecord{
		Ticker: "UND", Name: "Underlying", Type: "stock",
		ExchangeID: exch.ID, Price: 110, Ask: 111, Bid: 109, Volume: 100,
	}
	if err := db.Create(&stockListing).Error; err != nil {
		t.Fatal(err)
	}
	optListing := models.MarketListingRecord{
		Ticker: "OEXP", Name: "Opt Exp", Type: "option",
		ExchangeID: exch.ID, Price: 5, Ask: 5.1, Bid: 4.9, Volume: 100,
	}
	if err := db.Create(&optListing).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.OptionRecord{
		ListingID: optListing.ID, StockListingID: stockListing.ID,
		StrikePrice: 100, OptionType: "call",
		SettlementDate: time.Now().UTC().Add(-24 * time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	holding := models.PortfolioHoldingRecord{
		UserID: 0, UserType: "bank", AssetID: optListing.ID, Quantity: 1, AvgBuyPrice: 5, AccountID: 1,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	err := psvc.ExerciseOption(holding.ID, 1)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestExerciseOption_NotInTheMoney_Call(t *testing.T) {
	db := openTestDB(t, "eo_oom_call")
	exch := models.MarketExchangeRecord{
		Acronym: "OPTC", Name: "OPTC", MICCode: "OC", Polity: "X", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatal(err)
	}
	stockListing := models.MarketListingRecord{
		Ticker: "UNDC", Name: "Underlying C", Type: "stock",
		ExchangeID: exch.ID, Price: 90, Ask: 91, Bid: 89, Volume: 100,
	}
	if err := db.Create(&stockListing).Error; err != nil {
		t.Fatal(err)
	}
	optListing := models.MarketListingRecord{
		Ticker: "OOMC", Name: "Opt OOM C", Type: "option",
		ExchangeID: exch.ID, Price: 5, Ask: 5.1, Bid: 4.9, Volume: 100,
	}
	if err := db.Create(&optListing).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.OptionRecord{
		ListingID: optListing.ID, StockListingID: stockListing.ID,
		StrikePrice: 100, OptionType: "call",
		SettlementDate: time.Now().UTC().Add(7 * 24 * time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	holding := models.PortfolioHoldingRecord{
		UserID: 0, UserType: "bank", AssetID: optListing.ID, Quantity: 1, AvgBuyPrice: 5, AccountID: 1,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	err := psvc.ExerciseOption(holding.ID, 1)
	if err == nil || !strings.Contains(err.Error(), "not in-the-money") {
		t.Fatalf("expected OOM error, got %v", err)
	}
}

func TestExerciseOption_NotInTheMoney_Put(t *testing.T) {
	db := openTestDB(t, "eo_oom_put")
	exch := models.MarketExchangeRecord{
		Acronym: "OPTP", Name: "OPTP", MICCode: "OP", Polity: "X", Currency: "USD",
		Timezone: "UTC", WorkingHours: "09:00-17:00",
	}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatal(err)
	}
	stockListing := models.MarketListingRecord{
		Ticker: "UNDP", Name: "Underlying P", Type: "stock",
		ExchangeID: exch.ID, Price: 110, Ask: 111, Bid: 109, Volume: 100,
	}
	if err := db.Create(&stockListing).Error; err != nil {
		t.Fatal(err)
	}
	optListing := models.MarketListingRecord{
		Ticker: "OOMP", Name: "Opt OOM P", Type: "option",
		ExchangeID: exch.ID, Price: 5, Ask: 5.1, Bid: 4.9, Volume: 100,
	}
	if err := db.Create(&optListing).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.OptionRecord{
		ListingID: optListing.ID, StockListingID: stockListing.ID,
		StrikePrice: 100, OptionType: "put",
		SettlementDate: time.Now().UTC().Add(7 * 24 * time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	holding := models.PortfolioHoldingRecord{
		UserID: 0, UserType: "bank", AssetID: optListing.ID, Quantity: 1, AvgBuyPrice: 5, AccountID: 1,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatal(err)
	}

	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	err := psvc.ExerciseOption(holding.ID, 1)
	if err == nil || !strings.Contains(err.Error(), "not in-the-money") {
		t.Fatalf("expected OOM error, got %v", err)
	}
}

// --- ReserveOTCQuantity / ReleaseOTCReservedQuantity ---

func TestReserveOTCQuantity_HoldingNotFound(t *testing.T) {
	db := openTestDB(t, "ro_nf")
	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	if err := psvc.ReserveOTCQuantity(9999, 1); err == nil {
		t.Fatal("expected error for missing holding")
	}
}

func TestReserveOTCQuantity_NegativeQty(t *testing.T) {
	db := openTestDB(t, "ro_neg")
	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	if err := psvc.ReserveOTCQuantity(1, -1); err == nil {
		t.Fatal("expected error for negative quantity")
	}
}

func TestReleaseOTCReservedQuantity_NegativeQty(t *testing.T) {
	db := openTestDB(t, "rel_neg")
	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	if err := psvc.ReleaseOTCReservedQuantity(1, -1); err == nil {
		t.Fatal("expected error for negative quantity")
	}
}

// --- OrderExecutor.Run on empty DB (no active orders) ---

func TestOrderExecutor_Run_EmptyDB(t *testing.T) {
	db := openTestDB(t, "exec_empty")
	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	exec := service.NewOrderExecutor(
		repository.NewOrderRepository(db), repository.NewMarketRepository(db),
		psvc, &mockRateProv{},
	)
	exec.Run() // should be a no-op without panicking
}

// --- CreateOrder additional paths (toRSD, resolveStatus) ---

func TestCreateOrder_ClientSell_USD_Success(t *testing.T) {
	db := openTestDB(t, "co_sell_usd")
	seedAsset(t, db, "USD1", 50, "USD")
	svc := service.NewOrderService(
		repository.NewOrderRepository(db),
		repository.NewMarketRepository(db),
		&mockRateProv{rates: map[string]float64{"USD:RSD": 110}},
	)

	res, err := svc.CreateOrder(service.CreateOrderInput{
		UserID: 1, UserType: "client", AssetTicker: "USD1",
		OrderType: "limit", Direction: "sell", Quantity: 1,
		LimitValue: floatPtr(50), AccountID: 1,
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if res == nil || res.Order == nil {
		t.Fatal("expected order result")
	}
	if res.Order.Status != "approved" {
		t.Errorf("status=%s, want approved", res.Order.Status)
	}
}

// Note: Bank-side CreateOrder paths (resolveStatus) cannot be exercised here —
// the repository's actuary_profiles SELECT uses `trading_limit AS limit`, which
// is a reserved word in SQLite. Those branches are covered in production via Postgres.

func TestOrderExecutor_Run_SkipsPendingNonApproved(t *testing.T) {
	db := openTestDB(t, "exec_pending")
	assetID := seedAsset(t, db, "PEND", 50, "USD")
	seedOrder(t, db, &models.OrderRecord{
		UserID: 1, UserType: "client", AssetID: assetID, OrderType: "limit",
		Direction: "sell", Quantity: 1, ContractSize: 1, PricePerUnit: 50,
		Status: "pending", RemainingPortions: 1, AccountID: 1,
	})

	taxSvc := service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{})
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db), taxSvc,
		repository.NewMarketRepository(db), repository.NewOrderRepository(db),
	)
	exec := service.NewOrderExecutor(
		repository.NewOrderRepository(db), repository.NewMarketRepository(db),
		psvc, &mockRateProv{},
	)
	exec.Run() // pending orders should be skipped without errors
}
