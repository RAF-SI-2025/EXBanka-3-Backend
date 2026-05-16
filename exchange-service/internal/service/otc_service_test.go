package service_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"gorm.io/gorm"
)

func setupOtcServiceFixture(t *testing.T, name string) (*service.OtcService, *models.PortfolioHoldingRecord, *repository.OtcRepository) {
	t.Helper()
	db := openTestDB(t, name)
	seedOtcAccountTables(t, db)
	assetID := seedAsset(t, db, "OTC"+name[:1], 100, "USD")
	holding := seedOtcHolding(t, db, assetID, 200, "client", 10, 6, 1)
	portfolioRepo := repository.NewPortfolioRepository(db)
	otcRepo := repository.NewOtcRepository(db)
	return service.NewOtcService(portfolioRepo, otcRepo), holding, otcRepo
}

func seedOtcAccountTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS currencies (id integer primary key, kod text not null unique)`).Error; err != nil {
		t.Fatalf("create currencies table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS accounts (
		id integer primary key,
		client_id integer,
		firma_id integer,
		zaposleni_id integer,
		currency_id integer not null,
		stanje real not null default 0,
		raspolozivo_stanje real not null default 0,
		dnevna_potrosnja real not null default 0,
		mesecna_potrosnja real not null default 0,
		status text not null default 'aktivan'
	)`).Error; err != nil {
		t.Fatalf("create accounts table: %v", err)
	}
	if err := db.Exec(`INSERT OR IGNORE INTO currencies (id, kod) VALUES (1, 'USD')`).Error; err != nil {
		t.Fatalf("seed currency: %v", err)
	}
	if err := db.Exec(`INSERT OR IGNORE INTO accounts (id, client_id, currency_id, stanje, raspolozivo_stanje, status) VALUES
		(1, 200, 1, 1000, 1000, 'aktivan'),
		(2, 100, 1, 1000, 1000, 'aktivan')`).Error; err != nil {
		t.Fatalf("seed accounts: %v", err)
	}
}

func seedOtcHolding(t *testing.T, db *gorm.DB, assetID uint, userID uint, userType string, quantity, publicQuantity, reservedQuantity float64) *models.PortfolioHoldingRecord {
	t.Helper()
	holding := &models.PortfolioHoldingRecord{
		UserID: userID, UserType: userType, AssetID: assetID, Quantity: quantity,
		PublicQuantity: publicQuantity, ReservedQuantity: reservedQuantity,
		AvgBuyPrice: 90, AccountID: 1, CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(holding).Error; err != nil {
		t.Fatal(err)
	}
	return holding
}

func TestOtcService_CreateOffer(t *testing.T) {
	svc, holding, otcRepo := setupOtcServiceFixture(t, "otc_create_offer")

	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if offer.ID == 0 || offer.Status != models.OtcOfferStatusPending {
		t.Fatalf("expected persisted pending offer, got %+v", offer)
	}
	if offer.BuyerID != 100 || offer.SellerID != 200 || offer.StockListingID != holding.AssetID {
		t.Fatalf("unexpected offer ownership/listing fields: %+v", offer)
	}

	offers, err := otcRepo.ListOffersForParticipant(100, "client", models.OtcOfferStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 1 {
		t.Fatalf("expected buyer to see created offer, got %d", len(offers))
	}
}

func TestOtcService_CreateOfferRejectsOwnHolding(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_create_self")

	_, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         200,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          1,
		PricePerStock:   100,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         10,
	})
	if err == nil || !strings.Contains(err.Error(), "own holding") {
		t.Fatalf("expected own holding error, got %v", err)
	}
}

func TestOtcService_CreateOfferRejectsUnavailableQuantity(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_create_excess")

	_, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          6,
		PricePerStock:   100,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         10,
	})
	if err == nil || !strings.Contains(err.Error(), "available OTC quantity") {
		t.Fatalf("expected available quantity error, got %v", err)
	}
}

func TestOtcService_ListAndGetOffersForParticipant(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_list_get")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	buyerOffers, err := svc.ListOffersForParticipant(100, "client", models.OtcOfferStatusPending)
	if err != nil {
		t.Fatalf("ListOffersForParticipant buyer: %v", err)
	}
	if len(buyerOffers) != 1 || buyerOffers[0].ID != offer.ID {
		t.Fatalf("expected buyer to see created offer, got %+v", buyerOffers)
	}

	sellerOffer, err := svc.GetOfferForParticipant(offer.ID, 200, "client")
	if err != nil {
		t.Fatalf("GetOfferForParticipant seller: %v", err)
	}
	if sellerOffer.ID != offer.ID {
		t.Fatalf("expected offer %d, got %d", offer.ID, sellerOffer.ID)
	}
}

func TestOtcService_GetOfferForParticipantRejectsUnrelatedUser(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_get_unrelated")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	_, err = svc.GetOfferForParticipant(offer.ID, 999, "client")
	if !errors.Is(err, service.ErrOtcOfferNotFound) {
		t.Fatalf("expected ErrOtcOfferNotFound for unrelated participant, got %v", err)
	}
}

func TestOtcService_ListOffersRejectsInvalidStatus(t *testing.T) {
	svc, _, _ := setupOtcServiceFixture(t, "otc_list_bad_status")

	_, err := svc.ListOffersForParticipant(100, "client", "mystery")
	if err == nil || !strings.Contains(err.Error(), "invalid offer status") {
		t.Fatalf("expected invalid status error, got %v", err)
	}
}

func TestOtcService_CounterOffer(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_counter")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	newDate := time.Now().UTC().AddDate(0, 2, 0)
	updated, err := svc.CounterOffer(service.CounterOtcOfferInput{
		OfferID:        offer.ID,
		ModifiedByID:   200,
		ModifiedByType: "client",
		Amount:         4,
		PricePerStock:  108,
		SettlementDate: newDate,
		Premium:        30,
	})
	if err != nil {
		t.Fatalf("CounterOffer: %v", err)
	}
	if updated.Amount != 4 || updated.PricePerStock != 108 || updated.Premium != 30 {
		t.Fatalf("terms were not updated: %+v", updated)
	}
	if updated.ModifiedByID != 200 || updated.ModifiedByType != "client" {
		t.Fatalf("modifier not updated: %+v", updated)
	}
}

func TestOtcService_CounterOfferRejectsUnrelatedParticipant(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_counter_unrelated")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	_, err = svc.CounterOffer(service.CounterOtcOfferInput{
		OfferID:        offer.ID,
		ModifiedByID:   999,
		ModifiedByType: "client",
		Amount:         4,
		PricePerStock:  108,
		SettlementDate: time.Now().UTC().AddDate(0, 2, 0),
		Premium:        30,
	})
	if !errors.Is(err, service.ErrOtcOfferNotFound) {
		t.Fatalf("expected ErrOtcOfferNotFound, got %v", err)
	}
}

func TestOtcService_CounterOfferRejectsNonPendingOffer(t *testing.T) {
	svc, holding, otcRepo := setupOtcServiceFixture(t, "otc_counter_status")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if err := otcRepo.UpdateOfferStatus(offer.ID, models.OtcOfferStatusDeclined, 200, "client"); err != nil {
		t.Fatal(err)
	}

	_, err = svc.CounterOffer(service.CounterOtcOfferInput{
		OfferID:        offer.ID,
		ModifiedByID:   100,
		ModifiedByType: "client",
		Amount:         4,
		PricePerStock:  108,
		SettlementDate: time.Now().UTC().AddDate(0, 2, 0),
		Premium:        30,
	})
	if err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("expected pending status error, got %v", err)
	}
}

func TestOtcService_DeclineOffer(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_decline")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	updated, err := svc.DeclineOffer(offer.ID, 200, "client")
	if err != nil {
		t.Fatalf("DeclineOffer: %v", err)
	}
	if updated.Status != models.OtcOfferStatusDeclined || updated.ModifiedByID != 200 {
		t.Fatalf("expected seller decline status, got %+v", updated)
	}
}

func TestOtcService_CancelOffer(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_cancel")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	updated, err := svc.CancelOffer(offer.ID, 100, "client")
	if err != nil {
		t.Fatalf("CancelOffer: %v", err)
	}
	if updated.Status != models.OtcOfferStatusCancelled || updated.ModifiedByID != 100 {
		t.Fatalf("expected buyer cancel status, got %+v", updated)
	}
}

func TestOtcService_DeclineOfferRejectsBuyer(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_decline_buyer")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	_, err = svc.DeclineOffer(offer.ID, 100, "client")
	if err == nil || !strings.Contains(err.Error(), "only seller") {
		t.Fatalf("expected seller-only decline error, got %v", err)
	}
}

func TestOtcService_CancelOfferRejectsSeller(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_cancel_seller")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	_, err = svc.CancelOffer(offer.ID, 200, "client")
	if err == nil || !strings.Contains(err.Error(), "only buyer") {
		t.Fatalf("expected buyer-only cancel error, got %v", err)
	}
}

func TestOtcService_AcceptOfferCreatesContractAndReservesQuantity(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_accept")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	contract, err := svc.AcceptOffer(offer.ID, 200, "client")
	if err != nil {
		t.Fatalf("AcceptOffer: %v", err)
	}
	if contract.ID == 0 || contract.Status != models.OtcContractStatusValid {
		t.Fatalf("expected valid contract, got %+v", contract)
	}
	if contract.Amount != 3 || contract.StrikePrice != 105 || contract.Premium != 25 {
		t.Fatalf("unexpected contract terms: %+v", contract)
	}

	acceptedOffer, err := svc.GetOfferForParticipant(offer.ID, 100, "client")
	if err != nil {
		t.Fatalf("GetOfferForParticipant: %v", err)
	}
	if acceptedOffer.Status != models.OtcOfferStatusAccepted {
		t.Fatalf("expected accepted offer, got %q", acceptedOffer.Status)
	}

	publicStocks, err := svc.ListPublicStocks(100, "client")
	if err != nil {
		t.Fatal(err)
	}
	if len(publicStocks) != 1 || publicStocks[0].ReservedQuantity != 4 || publicStocks[0].AvailableQuantity != 2 {
		t.Fatalf("expected reserved quantity to include accepted amount, got %+v", publicStocks)
	}
}

func TestOtcService_ListContractsForParticipant(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_contracts")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	contract, err := svc.AcceptOffer(offer.ID, 200, "client")
	if err != nil {
		t.Fatalf("AcceptOffer: %v", err)
	}

	buyerContracts, err := svc.ListContractsForParticipant(100, "client", models.OtcContractStatusValid)
	if err != nil {
		t.Fatalf("ListContractsForParticipant buyer: %v", err)
	}
	if len(buyerContracts) != 1 || buyerContracts[0].ID != contract.ID {
		t.Fatalf("expected buyer contract %d, got %+v", contract.ID, buyerContracts)
	}

	sellerContracts, err := svc.ListContractsForParticipant(200, "client", "")
	if err != nil {
		t.Fatalf("ListContractsForParticipant seller: %v", err)
	}
	if len(sellerContracts) != 1 || sellerContracts[0].ID != contract.ID {
		t.Fatalf("expected seller contract %d, got %+v", contract.ID, sellerContracts)
	}

	none, err := svc.ListContractsForParticipant(999, "client", "")
	if err != nil {
		t.Fatalf("ListContractsForParticipant unrelated: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected unrelated participant to see no contracts, got %+v", none)
	}
}

func TestOtcService_ListContractsRejectsInvalidStatus(t *testing.T) {
	svc, _, _ := setupOtcServiceFixture(t, "otc_contracts_bad_status")

	_, err := svc.ListContractsForParticipant(100, "client", "pending")
	if err == nil || !strings.Contains(err.Error(), "invalid contract status") {
		t.Fatalf("expected invalid contract status error, got %v", err)
	}
}

func TestOtcService_ExpireDueContracts(t *testing.T) {
	db := openTestDB(t, "otc_expire_due")
	seedOtcAccountTables(t, db)
	assetID := seedAsset(t, db, "OTCE", 100, "USD")
	holding := seedOtcHolding(t, db, assetID, 200, "client", 10, 6, 1)
	portfolioRepo := repository.NewPortfolioRepository(db)
	otcRepo := repository.NewOtcRepository(db)
	svc := service.NewOtcService(portfolioRepo, otcRepo)

	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	contract, err := svc.AcceptOffer(offer.ID, 200, "client")
	if err != nil {
		t.Fatalf("AcceptOffer: %v", err)
	}
	if err := db.Model(&models.OtcContractRecord{}).
		Where("id = ?", contract.ID).
		Update("settlement_date", time.Now().UTC().Add(-24*time.Hour)).Error; err != nil {
		t.Fatalf("backdate contract: %v", err)
	}

	expired, err := svc.ExpireDueContracts(time.Now().UTC())
	if err != nil {
		t.Fatalf("ExpireDueContracts: %v", err)
	}
	if expired != 1 {
		t.Fatalf("expected one expired contract, got %d", expired)
	}

	contracts, err := svc.ListContractsForParticipant(100, "client", models.OtcContractStatusExpired)
	if err != nil {
		t.Fatalf("ListContractsForParticipant expired: %v", err)
	}
	if len(contracts) != 1 || contracts[0].ID != contract.ID {
		t.Fatalf("expected expired contract %d, got %+v", contract.ID, contracts)
	}
}

func TestOtcService_AcceptOfferRejectsOwnCounter(t *testing.T) {
	// Buyer creates the offer (so buyer is the last modifier). They cannot
	// accept their own pending offer — only the counterparty can.
	svc, holding, _ := setupOtcServiceFixture(t, "otc_accept_own_counter")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	_, err = svc.AcceptOffer(offer.ID, 100, "client")
	if err == nil || !strings.Contains(err.Error(), "own counter") {
		t.Fatalf("expected own-counter accept error, got %v", err)
	}
}

func TestOtcService_AcceptOfferAllowsBuyerAfterSellerCounter(t *testing.T) {
	// After the seller counters, the seller is the last modifier; the buyer
	// (recipient of the counter) must be able to accept.
	svc, holding, _ := setupOtcServiceFixture(t, "otc_accept_after_counter")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	if _, err := svc.CounterOffer(service.CounterOtcOfferInput{
		OfferID:        offer.ID,
		ModifiedByID:   200,
		ModifiedByType: "client",
		Amount:         3,
		PricePerStock:  110,
		SettlementDate: time.Now().UTC().AddDate(0, 1, 0),
		Premium:        30,
	}); err != nil {
		t.Fatalf("CounterOffer: %v", err)
	}

	contract, err := svc.AcceptOffer(offer.ID, 100, "client")
	if err != nil {
		t.Fatalf("AcceptOffer by buyer after seller counter: %v", err)
	}
	if contract.StrikePrice != 110 || contract.Premium != 30 {
		t.Fatalf("expected counter terms to apply, got %+v", contract)
	}
}

func TestOtcService_AcceptOfferRejectsNonPending(t *testing.T) {
	svc, holding, _ := setupOtcServiceFixture(t, "otc_accept_non_pending")
	offer, err := svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         100,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Now().UTC().AddDate(0, 1, 0),
		Premium:         25,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if _, err := svc.CancelOffer(offer.ID, 100, "client"); err != nil {
		t.Fatalf("CancelOffer: %v", err)
	}

	_, err = svc.AcceptOffer(offer.ID, 200, "client")
	if err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("expected pending status error, got %v", err)
	}
}
