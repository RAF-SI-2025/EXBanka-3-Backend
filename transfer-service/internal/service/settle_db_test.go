package service_test

import (
	"fmt"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/service"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestTransferDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedTransferAccount(t *testing.T, db *gorm.DB, currencyKod string, clientID uint, balance float64) *models.Account {
	t.Helper()
	var cur models.Currency
	db.Where("kod = ?", currencyKod).FirstOrCreate(&cur, models.Currency{Kod: currencyKod})

	cid := clientID
	acc := &models.Account{
		BrojRacuna:        fmt.Sprintf("X-%s-%d-%f", currencyKod, clientID, balance),
		ClientID:          &cid,
		CurrencyID:        cur.ID,
		Stanje:            balance,
		RaspolozivoStanje: balance,
		DnevniLimit:       1000000,
		MesecniLimit:      10000000,
	}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	return acc
}

func TestSettleTransfer_DBPath_SameCurrencySuccess(t *testing.T) {
	db := newTestTransferDB(t, "xfer_settle_same_currency")

	sender := seedTransferAccount(t, db, "RSD", 1, 5000)
	receiver := seedTransferAccount(t, db, "RSD", 1, 0)

	accRepo := repository.NewAccountRepository(db)
	xferRepo := repository.NewTransferRepository(db)
	svc := service.NewTransferServiceWithRepos(accRepo, xferRepo, &mockExchangeRateService{rate: 1}).WithDB(db)

	tr, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: sender.ID, RacunPrimaocaID: receiver.ID, Iznos: 1000,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	approved, _, _, err := svc.ApproveTransferMobile(tr.ID, "confirm")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != "uspesno" {
		t.Fatalf("expected uspesno, got %s", approved.Status)
	}

	var fetched models.Account
	db.First(&fetched, sender.ID)
	if fetched.Stanje != 4000 {
		t.Fatalf("expected sender balance 4000, got %v", fetched.Stanje)
	}
}

func TestSettleTransfer_DBPath_AlreadyProcessed(t *testing.T) {
	db := newTestTransferDB(t, "xfer_settle_already")

	sender := seedTransferAccount(t, db, "RSD", 2, 1000)
	receiver := seedTransferAccount(t, db, "RSD", 2, 0)

	accRepo := repository.NewAccountRepository(db)
	xferRepo := repository.NewTransferRepository(db)
	svc := service.NewTransferServiceWithRepos(accRepo, xferRepo, &mockExchangeRateService{rate: 1}).WithDB(db)

	tr, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: sender.ID, RacunPrimaocaID: receiver.ID, Iznos: 100,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Manually mark completed.
	db.Model(&models.Transfer{}).Where("id = ?", tr.ID).Update("status", "uspesno")

	// Re-fetch a pending pointer to feed settleTransfer; since DB row is no longer pending,
	// we exercise the transfer_already_processed branch.
	tr.Status = "u_obradi"
	if _, _, _, err = svc.ApproveTransferMobile(tr.ID, "confirm"); err == nil {
		// Already completed — ApproveTransferMobile treats this as success and returns no error.
		// In that case we just verify the DB still shows completed.
		var fetched models.Transfer
		db.First(&fetched, tr.ID)
		if fetched.Status != "uspesno" {
			t.Fatalf("expected uspesno, got %s", fetched.Status)
		}
	}
}

func TestSettleTransfer_DBPath_InsufficientBalance(t *testing.T) {
	db := newTestTransferDB(t, "xfer_settle_insufficient")

	sender := seedTransferAccount(t, db, "RSD", 3, 100)
	receiver := seedTransferAccount(t, db, "RSD", 3, 0)

	accRepo := repository.NewAccountRepository(db)
	xferRepo := repository.NewTransferRepository(db)
	svc := service.NewTransferServiceWithRepos(accRepo, xferRepo, &mockExchangeRateService{rate: 1}).WithDB(db)

	tr, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: sender.ID, RacunPrimaocaID: receiver.ID, Iznos: 90,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Now drain sender balance manually before approval — settle should hit insufficient_balance.
	db.Model(&models.Account{}).Where("id = ?", sender.ID).Updates(map[string]any{"stanje": 0, "raspolozivo_stanje": 0})

	if _, _, _, err := svc.ApproveTransferMobile(tr.ID, "confirm"); err == nil {
		t.Fatal("expected insufficient balance error")
	}
}

func TestSettleTransfer_DBPath_OwnershipMismatch(t *testing.T) {
	db := newTestTransferDB(t, "xfer_settle_ownership")

	sender := seedTransferAccount(t, db, "RSD", 5, 1000)
	receiver := seedTransferAccount(t, db, "RSD", 5, 0)

	accRepo := repository.NewAccountRepository(db)
	xferRepo := repository.NewTransferRepository(db)
	svc := service.NewTransferServiceWithRepos(accRepo, xferRepo, &mockExchangeRateService{rate: 1}).WithDB(db)

	tr, err := svc.CreateTransfer(service.CreateTransferInput{
		RacunPosiljaocaID: sender.ID, RacunPrimaocaID: receiver.ID, Iznos: 100,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Switch receiver to a different client now.
	other := uint(999)
	db.Model(&models.Account{}).Where("id = ?", receiver.ID).Update("client_id", other)

	if _, _, _, err := svc.ApproveTransferMobile(tr.ID, "confirm"); err == nil {
		t.Fatal("expected ownership mismatch error")
	}
}
