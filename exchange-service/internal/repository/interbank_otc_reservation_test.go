package repository

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

// seedReservedNegotiation creates a seller-role negotiation that holds a
// live stock reservation against holdingID, with the given settlement
// date and amount.
func seedReservedNegotiation(t *testing.T, repo *InterbankOtcRepository, negID string, holdingID uint, amount float64, settlement string) {
	t.Helper()
	hid := holdingID
	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber:    333,
		NegotiationID:               negID,
		LocalRole:                   models.InterbankNegotiationRoleSeller,
		CounterpartyRoutingNumber:   444,
		BuyerRoutingNumber:          444,
		BuyerID:                     "client-1",
		SellerRoutingNumber:         333,
		SellerID:                    "client-5",
		StockTicker:                 "RES",
		Amount:                      amount,
		PricePerUnitCurrency:        "USD",
		PricePerUnitAmount:          10,
		PremiumCurrency:             "USD",
		PremiumAmount:               5,
		SettlementDate:              settlement,
		LastModifiedByRoutingNumber: 444,
		LastModifiedByID:            "client-1",
		IsOngoing:                   false,
		SellerReservedHoldingID:     &hid,
	}
	if err := repo.Create(neg); err != nil {
		t.Fatalf("create negotiation %s: %v", negID, err)
	}
}

// TestExpireDueSellerReservations verifies the §2.7.2 settlement sweep:
// past-settlement reservations are released and their marker cleared,
// while future-settlement reservations are left intact.
func TestExpireDueSellerReservations(t *testing.T) {
	db := openRepoTestDB(t, "ib_otc_reservation_expiry")
	repo := NewInterbankOtcRepository(db)

	assetID := seedRepositoryListing(t, db, "RES", string(models.ListingTypeStock))
	// 10 owned, 10 public, 7 reserved (3 for the expired neg + 4 for the live neg).
	holding := models.PortfolioHoldingRecord{
		UserID: 5, UserType: "client", AssetID: assetID,
		Quantity: 10, PublicQuantity: 10, ReservedQuantity: 7, AvgBuyPrice: 50, AccountID: 1,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatalf("create holding: %v", err)
	}

	now := time.Now().UTC()
	past := now.Add(-48 * time.Hour).Format(time.RFC3339)   // settlement+24h already elapsed
	future := now.Add(48 * time.Hour).Format(time.RFC3339)  // still well within window

	seedReservedNegotiation(t, repo, "neg-expired", holding.ID, 3, past)
	seedReservedNegotiation(t, repo, "neg-live", holding.ID, 4, future)

	released, err := repo.ExpireDueSellerReservations(now)
	if err != nil {
		t.Fatalf("ExpireDueSellerReservations: %v", err)
	}
	if released != 1 {
		t.Fatalf("released = %d, want 1 (only the past-settlement negotiation)", released)
	}

	// Holding reserved quantity drops by the expired negotiation's 3.
	var got models.PortfolioHoldingRecord
	if err := db.First(&got, holding.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ReservedQuantity != 4 {
		t.Fatalf("reserved_quantity = %v, want 4 (7 - 3 released)", got.ReservedQuantity)
	}

	// Expired negotiation's marker is cleared; live one's is intact.
	expired, _ := repo.Get(333, "neg-expired")
	if expired.SellerReservedHoldingID != nil {
		t.Fatalf("expired negotiation still marks a reserved holding: %v", *expired.SellerReservedHoldingID)
	}
	live, _ := repo.Get(333, "neg-live")
	if live.SellerReservedHoldingID == nil {
		t.Fatal("live negotiation lost its reservation marker")
	}

	// Sweep is idempotent — a second run releases nothing more.
	released2, err := repo.ExpireDueSellerReservations(now)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if released2 != 0 {
		t.Fatalf("second sweep released = %d, want 0", released2)
	}
}

// TestReleaseSellerReservationTxClamps verifies that releasing more than
// is reserved clamps to the reserved amount rather than going negative.
func TestReleaseSellerReservationTxClamps(t *testing.T) {
	db := openRepoTestDB(t, "ib_otc_reservation_clamp")
	repo := NewInterbankOtcRepository(db)

	assetID := seedRepositoryListing(t, db, "RES", string(models.ListingTypeStock))
	holding := models.PortfolioHoldingRecord{
		UserID: 5, UserType: "client", AssetID: assetID,
		Quantity: 10, PublicQuantity: 10, ReservedQuantity: 2, AvgBuyPrice: 50, AccountID: 1,
	}
	if err := db.Create(&holding).Error; err != nil {
		t.Fatalf("create holding: %v", err)
	}
	seedReservedNegotiation(t, repo, "neg-clamp", holding.ID, 5, time.Now().UTC().Format(time.RFC3339))

	// Release 5 against a holding that only has 2 reserved — must clamp.
	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.ReleaseSellerReservationTx(tx, 333, "neg-clamp", holding.ID, 5)
	})
	if err != nil {
		t.Fatalf("ReleaseSellerReservationTx: %v", err)
	}

	var got models.PortfolioHoldingRecord
	if err := db.First(&got, holding.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ReservedQuantity != 0 {
		t.Fatalf("reserved_quantity = %v, want 0 (clamped, not negative)", got.ReservedQuantity)
	}
}
