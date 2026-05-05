package repository_test

import (
	"fmt"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// =====================
// Helpers
// =====================

func newTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Currency{},
		&models.SifraDelatnosti{},
		&models.Firma{},
		&models.Client{},
		&models.Account{},
		&models.Card{},
		&models.OvlascenoLice{},
		&models.CardRequest{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func seedRSD(t *testing.T, db *gorm.DB) models.Currency {
	t.Helper()
	c := models.Currency{Kod: "RSD", Naziv: "Srpski dinar", Simbol: "RSD", Drzava: "Srbija", Aktivan: true}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("seed RSD: %v", err)
	}
	return c
}

func seedClient(t *testing.T, db *gorm.DB, email string) models.Client {
	t.Helper()
	c := models.Client{Ime: "Test", Prezime: "User", Email: email}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
	return c
}

// =====================
// Constructor compile checks
// =====================

func TestNewFirmaRepository_ReturnsNonNil(t *testing.T) {
	if repository.NewFirmaRepository(nil) == nil {
		t.Error("expected non-nil FirmaRepository")
	}
}

func TestNewSifraDelatnostiRepository_ReturnsNonNil(t *testing.T) {
	if repository.NewSifraDelatnostiRepository(nil) == nil {
		t.Error("expected non-nil SifraDelatnostiRepository")
	}
}

func TestNewAccountRepository_ReturnsNonNil(t *testing.T) {
	if repository.NewAccountRepository(nil) == nil {
		t.Error("expected non-nil AccountRepository")
	}
}

func TestNewCurrencyRepository_ReturnsNonNil(t *testing.T) {
	if repository.NewCurrencyRepository(nil) == nil {
		t.Error("expected non-nil CurrencyRepository")
	}
}

func TestNewCardRepository_ReturnsNonNil(t *testing.T) {
	if repository.NewCardRepository(nil) == nil {
		t.Error("expected non-nil CardRepository")
	}
}

// =====================
// CurrencyRepository
// =====================

func TestCurrencyRepository_FindByID(t *testing.T) {
	db := newTestDB(t, "repo_curr_byid")
	rsd := seedRSD(t, db)
	repo := repository.NewCurrencyRepository(db)

	got, err := repo.FindByID(rsd.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Kod != "RSD" {
		t.Errorf("expected Kod=RSD, got %s", got.Kod)
	}
}

func TestCurrencyRepository_FindByID_NotFound(t *testing.T) {
	db := newTestDB(t, "repo_curr_byid_nf")
	repo := repository.NewCurrencyRepository(db)

	if _, err := repo.FindByID(9999); err == nil {
		t.Error("expected error for non-existent currency")
	}
}

func TestCurrencyRepository_FindByKod(t *testing.T) {
	db := newTestDB(t, "repo_curr_bykod")
	seedRSD(t, db)
	repo := repository.NewCurrencyRepository(db)

	got, err := repo.FindByKod("RSD")
	if err != nil {
		t.Fatalf("FindByKod: %v", err)
	}
	if got.Naziv != "Srpski dinar" {
		t.Errorf("expected Naziv 'Srpski dinar', got %s", got.Naziv)
	}
}

func TestCurrencyRepository_FindByKod_NotFound(t *testing.T) {
	db := newTestDB(t, "repo_curr_bykod_nf")
	repo := repository.NewCurrencyRepository(db)

	if _, err := repo.FindByKod("XYZ"); err == nil {
		t.Error("expected error for missing kod")
	}
}

func TestCurrencyRepository_FindAll(t *testing.T) {
	db := newTestDB(t, "repo_curr_all")
	seedRSD(t, db)
	if err := db.Create(&models.Currency{Kod: "EUR", Naziv: "Evro", Simbol: "€", Drzava: "EU", Aktivan: true}).Error; err != nil {
		t.Fatalf("seed EUR: %v", err)
	}
	repo := repository.NewCurrencyRepository(db)

	all, err := repo.FindAll()
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 currencies, got %d", len(all))
	}
}

// =====================
// SifraDelatnostiRepository
// =====================

func TestSifraDelatnostiRepository_FindAll(t *testing.T) {
	db := newTestDB(t, "repo_sifra_all")
	if err := db.Create(&models.SifraDelatnosti{Sifra: "64.1", Naziv: "Monetarno"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := repository.NewSifraDelatnostiRepository(db)

	all, err := repo.FindAll()
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1, got %d", len(all))
	}
}

func TestSifraDelatnostiRepository_FindByID(t *testing.T) {
	db := newTestDB(t, "repo_sifra_byid")
	s := models.SifraDelatnosti{Sifra: "64.1", Naziv: "Monetarno"}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo := repository.NewSifraDelatnostiRepository(db)

	got, err := repo.FindByID(s.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Sifra != "64.1" {
		t.Errorf("expected Sifra=64.1, got %s", got.Sifra)
	}
}

func TestSifraDelatnostiRepository_FindByID_NotFound(t *testing.T) {
	db := newTestDB(t, "repo_sifra_byid_nf")
	repo := repository.NewSifraDelatnostiRepository(db)
	if _, err := repo.FindByID(9999); err == nil {
		t.Error("expected error")
	}
}

// =====================
// FirmaRepository
// =====================

func TestFirmaRepository_Create(t *testing.T) {
	db := newTestDB(t, "repo_firma_create")
	repo := repository.NewFirmaRepository(db)
	f := &models.Firma{Naziv: "ACME", MaticniBroj: "12345678", PIB: "123456789"}
	if err := repo.Create(f); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.ID == 0 {
		t.Error("expected ID to be set")
	}
}

func TestFirmaRepository_FindByID(t *testing.T) {
	db := newTestDB(t, "repo_firma_byid")
	repo := repository.NewFirmaRepository(db)
	f := &models.Firma{Naziv: "ACME", MaticniBroj: "12345678", PIB: "123456789"}
	if err := repo.Create(f); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.FindByID(f.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Naziv != "ACME" {
		t.Errorf("expected Naziv=ACME, got %s", got.Naziv)
	}
}

func TestFirmaRepository_FindByID_NotFound(t *testing.T) {
	db := newTestDB(t, "repo_firma_byid_nf")
	repo := repository.NewFirmaRepository(db)
	if _, err := repo.FindByID(9999); err == nil {
		t.Error("expected error")
	}
}

func TestFirmaRepository_FindByMaticniBroj(t *testing.T) {
	db := newTestDB(t, "repo_firma_bymat")
	repo := repository.NewFirmaRepository(db)
	if err := repo.Create(&models.Firma{Naziv: "ACME", MaticniBroj: "12345678", PIB: "123456789"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.FindByMaticniBroj("12345678")
	if err != nil {
		t.Fatalf("FindByMaticniBroj: %v", err)
	}
	if got.PIB != "123456789" {
		t.Errorf("expected PIB=123456789, got %s", got.PIB)
	}
}

func TestFirmaRepository_FindByMaticniBroj_NotFound(t *testing.T) {
	db := newTestDB(t, "repo_firma_bymat_nf")
	repo := repository.NewFirmaRepository(db)
	if _, err := repo.FindByMaticniBroj("nope"); err == nil {
		t.Error("expected error")
	}
}

func TestFirmaRepository_FindAll(t *testing.T) {
	db := newTestDB(t, "repo_firma_all")
	repo := repository.NewFirmaRepository(db)
	for i := 0; i < 3; i++ {
		f := &models.Firma{
			Naziv:       fmt.Sprintf("Firma %d", i),
			MaticniBroj: fmt.Sprintf("1234567%d", i),
			PIB:         fmt.Sprintf("12345678%d", i),
		}
		if err := repo.Create(f); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	all, err := repo.FindAll()
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3, got %d", len(all))
	}
}

// =====================
// AccountRepository
// =====================

func makeAccount(brojRacuna string, clientID, currencyID uint, tip, vrsta, naziv string) *models.Account {
	return &models.Account{
		BrojRacuna: brojRacuna,
		ClientID:   &clientID,
		CurrencyID: currencyID,
		Tip:        tip,
		Vrsta:      vrsta,
		Naziv:      naziv,
		Status:     "aktivan",
	}
}

func TestAccountRepository_Create(t *testing.T) {
	db := newTestDB(t, "repo_acct_create")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	a := makeAccount("333000100000000111", cl.ID, rsd.ID, "tekuci", "licni", "Test")
	if err := repo.Create(a); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == 0 {
		t.Error("expected ID to be set")
	}
}

func TestAccountRepository_FindByID(t *testing.T) {
	db := newTestDB(t, "repo_acct_byid")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	a := makeAccount("333000100000000211", cl.ID, rsd.ID, "tekuci", "licni", "T")
	if err := repo.Create(a); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.FindByID(a.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.BrojRacuna != "333000100000000211" {
		t.Errorf("BrojRacuna mismatch: %s", got.BrojRacuna)
	}
}

func TestAccountRepository_FindByID_NotFound(t *testing.T) {
	db := newTestDB(t, "repo_acct_byid_nf")
	repo := repository.NewAccountRepository(db)
	if _, err := repo.FindByID(9999); err == nil {
		t.Error("expected error")
	}
}

func TestAccountRepository_FindByBrojRacuna(t *testing.T) {
	db := newTestDB(t, "repo_acct_byb")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	a := makeAccount("333000100000000311", cl.ID, rsd.ID, "tekuci", "licni", "T")
	if err := repo.Create(a); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.FindByBrojRacuna("333000100000000311")
	if err != nil {
		t.Fatalf("FindByBrojRacuna: %v", err)
	}
	if got.ID != a.ID {
		t.Errorf("ID mismatch: %d vs %d", got.ID, a.ID)
	}
}

func TestAccountRepository_FindByBrojRacuna_NotFound(t *testing.T) {
	db := newTestDB(t, "repo_acct_byb_nf")
	repo := repository.NewAccountRepository(db)
	if _, err := repo.FindByBrojRacuna("nope"); err == nil {
		t.Error("expected error")
	}
}

func TestAccountRepository_ListByClientID(t *testing.T) {
	db := newTestDB(t, "repo_acct_bycli")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	other := seedClient(t, db, "x@y.z")
	repo := repository.NewAccountRepository(db)

	if err := repo.Create(makeAccount("333000100000000411", cl.ID, rsd.ID, "tekuci", "licni", "A1")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeAccount("333000100000000511", cl.ID, rsd.ID, "tekuci", "licni", "A2")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeAccount("333000100000000611", other.ID, rsd.ID, "tekuci", "licni", "B1")); err != nil {
		t.Fatalf("create: %v", err)
	}

	list, err := repo.ListByClientID(cl.ID)
	if err != nil {
		t.Fatalf("ListByClientID: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestAccountRepository_ListAll_Empty(t *testing.T) {
	db := newTestDB(t, "repo_acct_listall_empty")
	repo := repository.NewAccountRepository(db)
	list, total, err := repo.ListAll(models.AccountFilter{})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if total != 0 || len(list) != 0 {
		t.Errorf("expected empty, got total=%d len=%d", total, len(list))
	}
}

func TestAccountRepository_ListAll_Pagination(t *testing.T) {
	db := newTestDB(t, "repo_acct_listall_page")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	for i := 0; i < 5; i++ {
		broj := fmt.Sprintf("33300010000000%04d", i)
		if err := repo.Create(makeAccount(broj, cl.ID, rsd.ID, "tekuci", "licni", "T")); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	list, total, err := repo.ListAll(models.AccountFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total=5, got %d", total)
	}
	if len(list) != 2 {
		t.Errorf("expected page size=2, got %d", len(list))
	}
}

func TestAccountRepository_ListAll_DefaultPagination(t *testing.T) {
	db := newTestDB(t, "repo_acct_listall_default")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	for i := 0; i < 3; i++ {
		broj := fmt.Sprintf("33300010000001%04d", i)
		if err := repo.Create(makeAccount(broj, cl.ID, rsd.ID, "tekuci", "licni", "T")); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	// Page=0, PageSize=0 → defaults to 1, 10
	list, total, err := repo.ListAll(models.AccountFilter{})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if total != 3 || len(list) != 3 {
		t.Errorf("expected 3 items, got total=%d len=%d", total, len(list))
	}
}

func TestAccountRepository_ListAll_FilterByTip(t *testing.T) {
	db := newTestDB(t, "repo_acct_listall_tip")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	if err := repo.Create(makeAccount("333000100000010011", cl.ID, rsd.ID, "tekuci", "licni", "T")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeAccount("333000100000010121", cl.ID, rsd.ID, "devizni", "licni", "D")); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, total, err := repo.ListAll(models.AccountFilter{Tip: "tekuci"})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
}

func TestAccountRepository_ListAll_FilterByVrsta(t *testing.T) {
	db := newTestDB(t, "repo_acct_listall_vrsta")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	if err := repo.Create(makeAccount("333000100000020011", cl.ID, rsd.ID, "tekuci", "licni", "L")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeAccount("333000100000020112", cl.ID, rsd.ID, "tekuci", "poslovni", "P")); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, total, err := repo.ListAll(models.AccountFilter{Vrsta: "poslovni"})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
}

func TestAccountRepository_ListAll_FilterByStatus(t *testing.T) {
	db := newTestDB(t, "repo_acct_listall_status")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	a := makeAccount("333000100000030011", cl.ID, rsd.ID, "tekuci", "licni", "A")
	a.Status = "blokiran"
	if err := repo.Create(a); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeAccount("333000100000030111", cl.ID, rsd.ID, "tekuci", "licni", "B")); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, total, err := repo.ListAll(models.AccountFilter{Status: "blokiran"})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
}

func TestAccountRepository_ListAll_FilterByCurrencyID(t *testing.T) {
	db := newTestDB(t, "repo_acct_listall_curr")
	rsd := seedRSD(t, db)
	eur := models.Currency{Kod: "EUR", Naziv: "Evro", Simbol: "€", Drzava: "EU", Aktivan: true}
	if err := db.Create(&eur).Error; err != nil {
		t.Fatalf("seed EUR: %v", err)
	}
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	if err := repo.Create(makeAccount("333000100000040011", cl.ID, rsd.ID, "tekuci", "licni", "R")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeAccount("333000100000040121", cl.ID, eur.ID, "devizni", "licni", "E")); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, total, err := repo.ListAll(models.AccountFilter{CurrencyID: &rsd.ID})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
}

func TestAccountRepository_UpdateFields(t *testing.T) {
	db := newTestDB(t, "repo_acct_update")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	a := makeAccount("333000100000050011", cl.ID, rsd.ID, "tekuci", "licni", "X")
	if err := repo.Create(a); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.UpdateFields(a.ID, map[string]interface{}{"naziv": "NewName", "stanje": 5000.0}); err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}

	updated, err := repo.FindByID(a.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if updated.Naziv != "NewName" {
		t.Errorf("expected Naziv=NewName, got %s", updated.Naziv)
	}
	if updated.Stanje != 5000.0 {
		t.Errorf("expected Stanje=5000, got %f", updated.Stanje)
	}
}

func TestAccountRepository_ExistsByNameForClient_True(t *testing.T) {
	db := newTestDB(t, "repo_acct_exists_t")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	a := makeAccount("333000100000060011", cl.ID, rsd.ID, "tekuci", "licni", "Naziv1")
	if err := repo.Create(a); err != nil {
		t.Fatalf("create: %v", err)
	}

	exists, err := repo.ExistsByNameForClient(cl.ID, "Naziv1", 0)
	if err != nil {
		t.Fatalf("ExistsByNameForClient: %v", err)
	}
	if !exists {
		t.Error("expected exists=true")
	}
}

func TestAccountRepository_ExistsByNameForClient_False(t *testing.T) {
	db := newTestDB(t, "repo_acct_exists_f")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	a := makeAccount("333000100000060111", cl.ID, rsd.ID, "tekuci", "licni", "Naziv1")
	if err := repo.Create(a); err != nil {
		t.Fatalf("create: %v", err)
	}

	exists, err := repo.ExistsByNameForClient(cl.ID, "Other", 0)
	if err != nil {
		t.Fatalf("ExistsByNameForClient: %v", err)
	}
	if exists {
		t.Error("expected exists=false")
	}
}

func TestAccountRepository_ExistsByNameForClient_ExcludesID(t *testing.T) {
	db := newTestDB(t, "repo_acct_exists_excl")
	rsd := seedRSD(t, db)
	cl := seedClient(t, db, "a@b.c")
	repo := repository.NewAccountRepository(db)

	a := makeAccount("333000100000060211", cl.ID, rsd.ID, "tekuci", "licni", "Naziv1")
	if err := repo.Create(a); err != nil {
		t.Fatalf("create: %v", err)
	}

	exists, err := repo.ExistsByNameForClient(cl.ID, "Naziv1", a.ID)
	if err != nil {
		t.Fatalf("ExistsByNameForClient: %v", err)
	}
	if exists {
		t.Error("expected exists=false when excluding the only matching account")
	}
}

// =====================
// CardRepository
// =====================

func makeCard(broj string, accountID, clientID uint) *models.Card {
	return &models.Card{
		BrojKartice:  broj,
		CVV:          "123",
		VrstaKartice: "visa",
		NazivKartice: "Test",
		AccountID:    accountID,
		ClientID:     clientID,
		Status:       "aktivna",
	}
}

func TestCardRepository_Create(t *testing.T) {
	db := newTestDB(t, "repo_card_create")
	repo := repository.NewCardRepository(db)
	c := makeCard("4111111111111111", 1, 1)
	if err := repo.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.ID == 0 {
		t.Error("expected ID to be set")
	}
}

func TestCardRepository_FindByID(t *testing.T) {
	db := newTestDB(t, "repo_card_byid")
	repo := repository.NewCardRepository(db)
	c := makeCard("4111111111111112", 1, 1)
	if err := repo.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.FindByID(c.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.BrojKartice != c.BrojKartice {
		t.Errorf("BrojKartice mismatch")
	}
}

func TestCardRepository_FindByID_NotFound(t *testing.T) {
	db := newTestDB(t, "repo_card_byid_nf")
	repo := repository.NewCardRepository(db)
	if _, err := repo.FindByID(9999); err == nil {
		t.Error("expected error")
	}
}

func TestCardRepository_CountByAccountID(t *testing.T) {
	db := newTestDB(t, "repo_card_byacct")
	repo := repository.NewCardRepository(db)
	if err := repo.Create(makeCard("4111111111111113", 5, 1)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeCard("4111111111111114", 5, 1)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeCard("4111111111111115", 9, 1)); err != nil {
		t.Fatalf("create: %v", err)
	}
	n, err := repo.CountByAccountID(5)
	if err != nil {
		t.Fatalf("CountByAccountID: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
}

func TestCardRepository_CountByClientAndAccount(t *testing.T) {
	db := newTestDB(t, "repo_card_clientacct")
	repo := repository.NewCardRepository(db)
	if err := repo.Create(makeCard("4111111111111116", 1, 7)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeCard("4111111111111117", 1, 7)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeCard("4111111111111118", 2, 7)); err != nil {
		t.Fatalf("create: %v", err)
	}
	n, err := repo.CountByClientAndAccount(7, 1)
	if err != nil {
		t.Fatalf("CountByClientAndAccount: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
}

func TestCardRepository_CountByOvlascenoLice(t *testing.T) {
	db := newTestDB(t, "repo_card_byovl")
	repo := repository.NewCardRepository(db)
	ovlID := uint(42)
	c := makeCard("4111111111111119", 1, 1)
	c.OvlascenoLiceID = &ovlID
	if err := repo.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	n, err := repo.CountByOvlascenoLice(42)
	if err != nil {
		t.Fatalf("CountByOvlascenoLice: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
}

func TestCardRepository_ListByAccountID(t *testing.T) {
	db := newTestDB(t, "repo_card_listacct")
	repo := repository.NewCardRepository(db)
	if err := repo.Create(makeCard("4111111111111120", 8, 1)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeCard("4111111111111121", 8, 2)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeCard("4111111111111122", 9, 1)); err != nil {
		t.Fatalf("create: %v", err)
	}
	list, err := repo.ListByAccountID(8)
	if err != nil {
		t.Fatalf("ListByAccountID: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestCardRepository_ListByClientID(t *testing.T) {
	db := newTestDB(t, "repo_card_listcli")
	repo := repository.NewCardRepository(db)
	if err := repo.Create(makeCard("4111111111111123", 1, 11)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeCard("4111111111111124", 2, 11)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Create(makeCard("4111111111111125", 1, 99)); err != nil {
		t.Fatalf("create: %v", err)
	}
	list, err := repo.ListByClientID(11)
	if err != nil {
		t.Fatalf("ListByClientID: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestCardRepository_Save(t *testing.T) {
	db := newTestDB(t, "repo_card_save")
	repo := repository.NewCardRepository(db)
	c := makeCard("4111111111111126", 1, 1)
	if err := repo.Create(c); err != nil {
		t.Fatalf("create: %v", err)
	}
	c.Status = "blokirana"
	if err := repo.Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(c.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status != "blokirana" {
		t.Errorf("expected Status=blokirana, got %s", got.Status)
	}
}
