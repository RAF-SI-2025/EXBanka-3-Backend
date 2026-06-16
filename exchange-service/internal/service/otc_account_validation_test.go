package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestOtcService_CreateOffer_AccountValidation(t *testing.T) {
	db := openTestDB(t, "otc_acct_val")
	seedOtcAccountTables(t, db)
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (2, 'EUR')`)
	// 3: inactive, 4: wrong currency (EUR), 5: owned by someone else.
	db.Exec(`INSERT INTO accounts (id, client_id, currency_id, stanje, raspolozivo_stanje, status) VALUES
		(3, 100, 1, 1000, 1000, 'zatvoren'),
		(4, 100, 2, 1000, 1000, 'aktivan'),
		(5, 999, 1, 1000, 1000, 'aktivan')`)

	assetID := seedAsset(t, db, "OAV", 100, "USD")
	holding := seedOtcHolding(t, db, assetID, 200, "client", 10, 6, 1)
	svc := service.NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db))

	base := func() service.CreateOtcOfferInput {
		return service.CreateOtcOfferInput{
			BuyerID: 100, BuyerType: "client", SellerHoldingID: holding.ID,
			Amount: 3, PricePerStock: 105, SettlementDate: time.Now().UTC().AddDate(0, 1, 0), Premium: 25,
		}
	}
	for name, acct := range map[string]uint{"inactive": 3, "wrong-currency": 4, "not-owned": 5} {
		in := base()
		in.BuyerAccountID = acct
		if _, err := svc.CreateOffer(in); err == nil {
			t.Errorf("%s buyer account should be rejected", name)
		}
	}
	// Non-existent account too.
	in := base()
	in.BuyerAccountID = 9999
	if _, err := svc.CreateOffer(in); err == nil {
		t.Error("non-existent buyer account should be rejected")
	}
}
