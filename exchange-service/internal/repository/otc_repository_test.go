package repository

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

func seedOtcOfferFixture(t *testing.T, name string) (*OtcRepository, *models.OtcOfferRecord) {
	t.Helper()
	_, _, portfolioRepo, _, assetID := seedExchangeAndAsset(t, name, "OTC"+name[:1])
	seedOtcAccountFixtures(t, portfolioRepo.db)

	if err := portfolioRepo.RecordBuyFill(20, "client", assetID, 1, 12, 100); err != nil {
		t.Fatalf("seed holding: %v", err)
	}
	holding, err := portfolioRepo.GetHoldingByUserAndAsset(20, "client", assetID)
	if err != nil {
		t.Fatalf("load holding: %v", err)
	}
	if err := portfolioRepo.SetHoldingPublicQuantity(holding.ID, 8); err != nil {
		t.Fatalf("public quantity: %v", err)
	}

	offer := &models.OtcOfferRecord{
		StockListingID:  assetID,
		SellerHoldingID: holding.ID,
		Amount:          3,
		PricePerStock:   105,
		SettlementDate:  time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC),
		Premium:         25,
		ModifiedByID:    10,
		ModifiedByType:  "client",
		BuyerID:         10,
		BuyerType:       "client",
		BuyerAccountID:  2,
		SellerID:        20,
		SellerType:      "client",
		SellerAccountID: holding.AccountID,
	}

	return NewOtcRepository(portfolioRepo.db), offer
}

func seedOtcAccountFixtures(t *testing.T, db *gorm.DB) {
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
		(1, 20, 1, 1000, 1000, 'aktivan'),
		(2, 10, 1, 1000, 1000, 'aktivan')`).Error; err != nil {
		t.Fatalf("seed accounts: %v", err)
	}
}

func TestOtcRepository_CreateAndGetOffer(t *testing.T) {
	repo, offer := seedOtcOfferFixture(t, "create_get")

	if err := repo.CreateOffer(offer); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if offer.ID == 0 {
		t.Fatal("expected offer ID")
	}
	if offer.Status != models.OtcOfferStatusPending {
		t.Fatalf("expected default pending status, got %q", offer.Status)
	}
	if offer.LastModified.IsZero() {
		t.Fatal("expected LastModified to be set")
	}

	got, err := repo.GetOfferByID(offer.ID)
	if err != nil {
		t.Fatalf("GetOfferByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected offer")
	}
	if got.StockListing.Ticker == "" || got.SellerHolding.ID == 0 {
		t.Fatalf("expected preloaded stock listing and seller holding, got %+v", got)
	}
}

func TestOtcRepository_GetOfferByID_NotFound(t *testing.T) {
	repo, _ := seedOtcOfferFixture(t, "not_found")

	got, err := repo.GetOfferByID(9999)
	if err != nil {
		t.Fatalf("GetOfferByID: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestOtcRepository_ListOffersForParticipant(t *testing.T) {
	repo, offer := seedOtcOfferFixture(t, "list")
	if err := repo.CreateOffer(offer); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	buyerOffers, err := repo.ListOffersForParticipant(10, "client", "")
	if err != nil {
		t.Fatalf("ListOffersForParticipant buyer: %v", err)
	}
	if len(buyerOffers) != 1 {
		t.Fatalf("expected buyer to see 1 offer, got %d", len(buyerOffers))
	}

	sellerOffers, err := repo.ListOffersForParticipant(20, "client", models.OtcOfferStatusPending)
	if err != nil {
		t.Fatalf("ListOffersForParticipant seller: %v", err)
	}
	if len(sellerOffers) != 1 {
		t.Fatalf("expected seller to see 1 pending offer, got %d", len(sellerOffers))
	}

	none, err := repo.ListOffersForParticipant(30, "client", "")
	if err != nil {
		t.Fatalf("ListOffersForParticipant none: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected unrelated participant to see no offers, got %d", len(none))
	}
}

func TestOtcRepository_UpdateTermsAndStatus(t *testing.T) {
	repo, offer := seedOtcOfferFixture(t, "update")
	if err := repo.CreateOffer(offer); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	newDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := repo.UpdateOfferTerms(offer.ID, 4, 108, newDate, 30, 20, "client"); err != nil {
		t.Fatalf("UpdateOfferTerms: %v", err)
	}
	got, err := repo.GetOfferByID(offer.ID)
	if err != nil {
		t.Fatalf("GetOfferByID: %v", err)
	}
	if got.Amount != 4 || got.PricePerStock != 108 || got.Premium != 30 || got.ModifiedByID != 20 {
		t.Fatalf("terms not updated: %+v", got)
	}

	if err := repo.UpdateOfferStatus(offer.ID, models.OtcOfferStatusAccepted, 20, "client"); err != nil {
		t.Fatalf("UpdateOfferStatus: %v", err)
	}
	got, err = repo.GetOfferByID(offer.ID)
	if err != nil {
		t.Fatalf("GetOfferByID after status: %v", err)
	}
	if got.Status != models.OtcOfferStatusAccepted {
		t.Fatalf("expected accepted status, got %q", got.Status)
	}
}

func TestOtcRepository_AcceptOfferAndCreateContract(t *testing.T) {
	repo, offer := seedOtcOfferFixture(t, "accept")
	if err := repo.CreateOffer(offer); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	contract, err := repo.AcceptOfferAndCreateContract(offer.ID, offer.SellerID, offer.SellerType)
	if err != nil {
		t.Fatalf("AcceptOfferAndCreateContract: %v", err)
	}
	if contract.ID == 0 || contract.Status != models.OtcContractStatusValid {
		t.Fatalf("expected valid contract, got %+v", contract)
	}
	if contract.Amount != offer.Amount || contract.StrikePrice != offer.PricePerStock || contract.Premium != offer.Premium {
		t.Fatalf("contract terms do not match offer: %+v", contract)
	}

	updatedOffer, err := repo.GetOfferByID(offer.ID)
	if err != nil {
		t.Fatalf("GetOfferByID: %v", err)
	}
	if updatedOffer.Status != models.OtcOfferStatusAccepted {
		t.Fatalf("expected accepted offer status, got %q", updatedOffer.Status)
	}

	var holding models.PortfolioHoldingRecord
	if err := repo.db.First(&holding, offer.SellerHoldingID).Error; err != nil {
		t.Fatal(err)
	}
	if holding.ReservedQuantity != offer.Amount {
		t.Fatalf("expected reserved quantity %.2f, got %.2f", offer.Amount, holding.ReservedQuantity)
	}

	var buyerBalance, sellerBalance float64
	if err := repo.db.Table("accounts").Select("raspolozivo_stanje").Where("id = ?", offer.BuyerAccountID).Scan(&buyerBalance).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Table("accounts").Select("raspolozivo_stanje").Where("id = ?", offer.SellerAccountID).Scan(&sellerBalance).Error; err != nil {
		t.Fatal(err)
	}
	if buyerBalance != 975 || sellerBalance != 1025 {
		t.Fatalf("expected premium transfer buyer=975 seller=1025, got buyer=%.2f seller=%.2f", buyerBalance, sellerBalance)
	}
}

func TestOtcRepository_AcceptOfferRejectsInsufficientPremiumFunds(t *testing.T) {
	repo, offer := seedOtcOfferFixture(t, "accept_no_funds")
	if err := repo.db.Table("accounts").Where("id = ?", offer.BuyerAccountID).Updates(map[string]interface{}{
		"stanje":             10,
		"raspolozivo_stanje": 10,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateOffer(offer); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	_, err := repo.AcceptOfferAndCreateContract(offer.ID, offer.SellerID, offer.SellerType)
	if err == nil {
		t.Fatal("expected insufficient premium funds error")
	}

	got, err := repo.GetOfferByID(offer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != models.OtcOfferStatusPending {
		t.Fatalf("expected offer to remain pending, got %q", got.Status)
	}
	var holding models.PortfolioHoldingRecord
	if err := repo.db.First(&holding, offer.SellerHoldingID).Error; err != nil {
		t.Fatal(err)
	}
	if holding.ReservedQuantity != 0 {
		t.Fatalf("expected no additional reservation after failed premium payment, got %.2f", holding.ReservedQuantity)
	}
}

func TestOtcRepository_AcceptOfferRejectsNonParticipant(t *testing.T) {
	repo, offer := seedOtcOfferFixture(t, "accept_non_participant")
	if err := repo.CreateOffer(offer); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	_, err := repo.AcceptOfferAndCreateContract(offer.ID, 999, offer.SellerType)
	if err == nil {
		t.Fatal("expected participant validation error")
	}
}

func seedOtcContractFixture(t *testing.T, name string) (*OtcRepository, *models.OtcOfferRecord, *models.OtcContractRecord) {
	t.Helper()
	repo, offer := seedOtcOfferFixture(t, name)
	if err := repo.CreateOffer(offer); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	contract := &models.OtcContractRecord{
		OfferID:         &offer.ID,
		StockListingID:  offer.StockListingID,
		SellerHoldingID: offer.SellerHoldingID,
		Amount:          offer.Amount,
		StrikePrice:     offer.PricePerStock,
		Premium:         offer.Premium,
		SettlementDate:  offer.SettlementDate,
		BuyerID:         offer.BuyerID,
		BuyerType:       offer.BuyerType,
		BuyerAccountID:  offer.BuyerAccountID,
		SellerID:        offer.SellerID,
		SellerType:      offer.SellerType,
		SellerAccountID: offer.SellerAccountID,
		BankID:          offer.BankID,
	}

	return repo, offer, contract
}

func TestOtcRepository_CreateAndGetContract(t *testing.T) {
	repo, offer, contract := seedOtcContractFixture(t, "contract_create")

	if err := repo.CreateContract(contract); err != nil {
		t.Fatalf("CreateContract: %v", err)
	}
	if contract.ID == 0 {
		t.Fatal("expected contract ID")
	}
	if contract.Status != models.OtcContractStatusValid {
		t.Fatalf("expected default valid status, got %q", contract.Status)
	}

	got, err := repo.GetContractByID(contract.ID)
	if err != nil {
		t.Fatalf("GetContractByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected contract")
	}
	if got.OfferID == nil || *got.OfferID != offer.ID {
		t.Fatalf("expected source offer ID %d, got %+v", offer.ID, got.OfferID)
	}
	if got.StockListing.Ticker == "" || got.SellerHolding.ID == 0 {
		t.Fatalf("expected preloaded stock listing and seller holding, got %+v", got)
	}
}

func TestOtcRepository_GetContractByID_NotFound(t *testing.T) {
	repo, _, _ := seedOtcContractFixture(t, "contract_nf")

	got, err := repo.GetContractByID(9999)
	if err != nil {
		t.Fatalf("GetContractByID: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestOtcRepository_ListContractsForParticipant(t *testing.T) {
	repo, _, contract := seedOtcContractFixture(t, "contract_list")
	if err := repo.CreateContract(contract); err != nil {
		t.Fatalf("CreateContract: %v", err)
	}

	buyerContracts, err := repo.ListContractsForParticipant(contract.BuyerID, contract.BuyerType, "")
	if err != nil {
		t.Fatalf("ListContractsForParticipant buyer: %v", err)
	}
	if len(buyerContracts) != 1 {
		t.Fatalf("expected buyer to see 1 contract, got %d", len(buyerContracts))
	}

	sellerContracts, err := repo.ListContractsForParticipant(contract.SellerID, contract.SellerType, models.OtcContractStatusValid)
	if err != nil {
		t.Fatalf("ListContractsForParticipant seller: %v", err)
	}
	if len(sellerContracts) != 1 {
		t.Fatalf("expected seller to see 1 valid contract, got %d", len(sellerContracts))
	}

	none, err := repo.ListContractsForParticipant(999, "client", "")
	if err != nil {
		t.Fatalf("ListContractsForParticipant unrelated: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected unrelated participant to see no contracts, got %d", len(none))
	}
}

func TestOtcRepository_ContractStatusAndExpiredQuery(t *testing.T) {
	repo, _, contract := seedOtcContractFixture(t, "contract_status")
	contract.SettlementDate = time.Now().UTC().Add(-24 * time.Hour)
	if err := repo.CreateContract(contract); err != nil {
		t.Fatalf("CreateContract: %v", err)
	}

	expired, err := repo.ListExpiredValidContracts(time.Now().UTC())
	if err != nil {
		t.Fatalf("ListExpiredValidContracts: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired valid contract, got %d", len(expired))
	}

	if err := repo.UpdateContractStatus(contract.ID, models.OtcContractStatusExpired); err != nil {
		t.Fatalf("UpdateContractStatus: %v", err)
	}
	got, err := repo.GetContractByID(contract.ID)
	if err != nil {
		t.Fatalf("GetContractByID: %v", err)
	}
	if got.Status != models.OtcContractStatusExpired {
		t.Fatalf("expected expired status, got %q", got.Status)
	}

	expired, err = repo.ListExpiredValidContracts(time.Now().UTC())
	if err != nil {
		t.Fatalf("ListExpiredValidContracts after status: %v", err)
	}
	if len(expired) != 0 {
		t.Fatalf("expected no valid expired contracts after status update, got %d", len(expired))
	}
}

func TestOtcRepository_ExpireValidContractsReleasesReservedQuantity(t *testing.T) {
	repo, offer := seedOtcOfferFixture(t, "contract_expire")
	if err := repo.CreateOffer(offer); err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	contract, err := repo.AcceptOfferAndCreateContract(offer.ID, offer.SellerID, offer.SellerType)
	if err != nil {
		t.Fatalf("AcceptOfferAndCreateContract: %v", err)
	}
	pastSettlement := time.Now().UTC().Add(-24 * time.Hour)
	if err := repo.db.Model(&models.OtcContractRecord{}).
		Where("id = ?", contract.ID).
		Update("settlement_date", pastSettlement).Error; err != nil {
		t.Fatalf("backdate contract: %v", err)
	}

	expired, err := repo.ExpireValidContracts(time.Now().UTC())
	if err != nil {
		t.Fatalf("ExpireValidContracts: %v", err)
	}
	if expired != 1 {
		t.Fatalf("expected one expired contract, got %d", expired)
	}

	got, err := repo.GetContractByID(contract.ID)
	if err != nil {
		t.Fatalf("GetContractByID: %v", err)
	}
	if got.Status != models.OtcContractStatusExpired {
		t.Fatalf("expected expired contract status, got %q", got.Status)
	}

	var holding models.PortfolioHoldingRecord
	if err := repo.db.First(&holding, offer.SellerHoldingID).Error; err != nil {
		t.Fatal(err)
	}
	if holding.ReservedQuantity != 0 {
		t.Fatalf("expected reserved quantity to be released, got %.2f", holding.ReservedQuantity)
	}

	expiredAgain, err := repo.ExpireValidContracts(time.Now().UTC())
	if err != nil {
		t.Fatalf("ExpireValidContracts second run: %v", err)
	}
	if expiredAgain != 0 {
		t.Fatalf("expected idempotent second run, got %d", expiredAgain)
	}
}
