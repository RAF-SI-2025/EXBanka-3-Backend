package database

import (
	"fmt"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newInMemoryDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// =====================
// Migrate
// =====================

func TestMigrate_RunsWithoutError(t *testing.T) {
	db := newInMemoryDB(t, "test_migrate_acct")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
}

func TestMigrate_CreatesExpectedTables(t *testing.T) {
	db := newInMemoryDB(t, "test_migrate_tables_acct")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	tableModels := []interface{}{
		&models.Currency{},
		&models.SifraDelatnosti{},
		&models.Firma{},
		&models.Client{},
		&models.Account{},
		&models.Card{},
		&models.OvlascenoLice{},
		&models.CardRequest{},
	}
	for _, m := range tableModels {
		if !db.Migrator().HasTable(m) {
			t.Errorf("expected table for %T to exist after migration", m)
		}
	}
}

func TestMigrate_IsIdempotent(t *testing.T) {
	db := newInMemoryDB(t, "test_migrate_idem_acct")
	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate failed: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate failed: %v", err)
	}
}

// =====================
// SeedCurrencies
// =====================

func TestSeedCurrencies_RunsWithoutError(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_curr")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if err := SeedCurrencies(db); err != nil {
		t.Fatalf("SeedCurrencies failed: %v", err)
	}
}

func TestSeedCurrencies_InsertsAllEight(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_curr_count")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if err := SeedCurrencies(db); err != nil {
		t.Fatalf("SeedCurrencies failed: %v", err)
	}
	var count int64
	db.Model(&models.Currency{}).Count(&count)
	if count != 8 {
		t.Errorf("expected 8 currencies, got %d", count)
	}
}

func TestSeedCurrencies_IsIdempotent(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_curr_idem")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if err := SeedCurrencies(db); err != nil {
		t.Fatalf("first SeedCurrencies failed: %v", err)
	}
	if err := SeedCurrencies(db); err != nil {
		t.Fatalf("second SeedCurrencies failed: %v", err)
	}
	var count int64
	db.Model(&models.Currency{}).Count(&count)
	if count != 8 {
		t.Errorf("expected 8 currencies after re-seed, got %d", count)
	}
}

// =====================
// SeedSifreDelatnosti
// =====================

func TestSeedSifreDelatnosti_RunsWithoutError(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_sifre")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if err := SeedSifreDelatnosti(db); err != nil {
		t.Fatalf("SeedSifreDelatnosti failed: %v", err)
	}
}

func TestSeedSifreDelatnosti_InsertsRows(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_sifre_count")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if err := SeedSifreDelatnosti(db); err != nil {
		t.Fatalf("SeedSifreDelatnosti failed: %v", err)
	}
	var count int64
	db.Model(&models.SifraDelatnosti{}).Count(&count)
	if count < 15 {
		t.Errorf("expected at least 15 sifre, got %d", count)
	}
}

func TestSeedSifreDelatnosti_IsIdempotent(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_sifre_idem")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if err := SeedSifreDelatnosti(db); err != nil {
		t.Fatalf("first seed failed: %v", err)
	}
	var before int64
	db.Model(&models.SifraDelatnosti{}).Count(&before)
	if err := SeedSifreDelatnosti(db); err != nil {
		t.Fatalf("second seed failed: %v", err)
	}
	var after int64
	db.Model(&models.SifraDelatnosti{}).Count(&after)
	if before != after {
		t.Errorf("idempotency broken: before=%d after=%d", before, after)
	}
}

func TestSeedSifreDelatnosti_IncludesMonetarno(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_sifre_monetarno")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if err := SeedSifreDelatnosti(db); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	var s models.SifraDelatnosti
	if err := db.Where("sifra = ?", "64.1").First(&s).Error; err != nil {
		t.Errorf("expected sifra 64.1 to exist: %v", err)
	}
}

// =====================
// SeedBankAccounts
// =====================

func setupForBankSeed(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db := newInMemoryDB(t, name)
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if err := SeedSifreDelatnosti(db); err != nil {
		t.Fatalf("SeedSifreDelatnosti failed: %v", err)
	}
	if err := SeedCurrencies(db); err != nil {
		t.Fatalf("SeedCurrencies failed: %v", err)
	}
	return db
}

func TestSeedBankAccounts_RunsWithoutError(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_bank")
	if err := SeedBankAccounts(db); err != nil {
		t.Fatalf("SeedBankAccounts failed: %v", err)
	}
}

func TestSeedBankAccounts_CreatesFirma(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_bank_firma")
	if err := SeedBankAccounts(db); err != nil {
		t.Fatalf("SeedBankAccounts failed: %v", err)
	}
	var firma models.Firma
	if err := db.Where("maticni_broj = ?", "99999999").First(&firma).Error; err != nil {
		t.Errorf("expected bank Firma to exist: %v", err)
	}
}

func TestSeedBankAccounts_CreatesEightAccounts(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_bank_count")
	if err := SeedBankAccounts(db); err != nil {
		t.Fatalf("SeedBankAccounts failed: %v", err)
	}
	var count int64
	db.Model(&models.Account{}).Count(&count)
	if count != 8 {
		t.Errorf("expected 8 bank accounts, got %d", count)
	}
}

func TestSeedBankAccounts_IsIdempotent(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_bank_idem")
	if err := SeedBankAccounts(db); err != nil {
		t.Fatalf("first SeedBankAccounts failed: %v", err)
	}
	var before int64
	db.Model(&models.Account{}).Count(&before)
	if err := SeedBankAccounts(db); err != nil {
		t.Fatalf("second SeedBankAccounts failed: %v", err)
	}
	var after int64
	db.Model(&models.Account{}).Count(&after)
	if before != after {
		t.Errorf("idempotency broken: before=%d after=%d", before, after)
	}
}

func TestSeedBankAccounts_FailsWhenSifraMissing(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_bank_no_sifra")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if err := SeedBankAccounts(db); err == nil {
		t.Error("expected error when SifraDelatnosti 64.1 missing")
	}
}

// =====================
// SeedStateAccounts
// =====================

func TestSeedStateAccounts_RunsWithoutError(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_state")
	if err := SeedStateAccounts(db); err != nil {
		t.Fatalf("SeedStateAccounts failed: %v", err)
	}
}

func TestSeedStateAccounts_CreatesStateFirma(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_state_firma")
	if err := SeedStateAccounts(db); err != nil {
		t.Fatalf("SeedStateAccounts failed: %v", err)
	}
	var firma models.Firma
	if err := db.Where("maticni_broj = ?", "00000001").First(&firma).Error; err != nil {
		t.Errorf("expected state Firma to exist: %v", err)
	}
	if !firma.IsState {
		t.Error("expected IsState=true for state firma")
	}
}

func TestSeedStateAccounts_CreatesRSDAccount(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_state_acct")
	if err := SeedStateAccounts(db); err != nil {
		t.Fatalf("SeedStateAccounts failed: %v", err)
	}
	var count int64
	db.Model(&models.Account{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 state account, got %d", count)
	}
}

func TestSeedStateAccounts_IsIdempotent(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_state_idem")
	if err := SeedStateAccounts(db); err != nil {
		t.Fatalf("first failed: %v", err)
	}
	if err := SeedStateAccounts(db); err != nil {
		t.Fatalf("second failed: %v", err)
	}
	var firmaCount, acctCount int64
	db.Model(&models.Firma{}).Count(&firmaCount)
	db.Model(&models.Account{}).Count(&acctCount)
	if firmaCount != 1 {
		t.Errorf("expected 1 firma after re-seed, got %d", firmaCount)
	}
	if acctCount != 1 {
		t.Errorf("expected 1 account after re-seed, got %d", acctCount)
	}
}

func TestSeedStateAccounts_FailsWhenSifraMissing(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_state_no_sifra")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if err := SeedStateAccounts(db); err == nil {
		t.Error("expected error when SifraDelatnosti 64.1 missing")
	}
}

func TestSeedStateAccounts_SkipsWhenRSDMissing(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_state_no_rsd")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if err := SeedSifreDelatnosti(db); err != nil {
		t.Fatalf("seed sifre failed: %v", err)
	}
	// No currencies seeded — RSD missing path
	if err := SeedStateAccounts(db); err != nil {
		t.Errorf("expected nil error when RSD missing, got %v", err)
	}
	var acctCount int64
	db.Model(&models.Account{}).Count(&acctCount)
	if acctCount != 0 {
		t.Errorf("expected no accounts when RSD missing, got %d", acctCount)
	}
}

// =====================
// SeedClientAccounts
// =====================

func TestSeedClientAccounts_RunsWithoutError(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_client")
	if err := SeedClientAccounts(db); err != nil {
		t.Fatalf("SeedClientAccounts failed: %v", err)
	}
}

func TestSeedClientAccounts_SkipsWhenRSDMissing(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_client_no_rsd")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	// No currencies seeded
	if err := SeedClientAccounts(db); err != nil {
		t.Errorf("expected nil error when RSD missing, got %v", err)
	}
}

func TestSeedClientAccounts_SkipsWhenEURMissing(t *testing.T) {
	db := newInMemoryDB(t, "test_seed_client_no_eur")
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	// Seed only RSD
	if err := db.Create(&models.Currency{Kod: "RSD", Naziv: "Srpski dinar", Simbol: "RSD", Drzava: "Srbija", Aktivan: true}).Error; err != nil {
		t.Fatalf("seed RSD: %v", err)
	}
	if err := SeedClientAccounts(db); err != nil {
		t.Errorf("expected nil error when EUR missing, got %v", err)
	}
}

func TestSeedClientAccounts_CreatesAccountsForExistingClients(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_client_with_clients")

	clients := []models.Client{
		{Ime: "Petar", Prezime: "Petrović", Email: "klijent@bank.com"},
		{Ime: "Jelena", Prezime: "Nikolić", Email: "jelena.nikolic@bank.com"},
		{Ime: "Nikola", Prezime: "Đorđević", Email: "nikola.djordjevic@bank.com"},
	}
	for i := range clients {
		if err := db.Create(&clients[i]).Error; err != nil {
			t.Fatalf("create client: %v", err)
		}
	}

	if err := SeedClientAccounts(db); err != nil {
		t.Fatalf("SeedClientAccounts failed: %v", err)
	}

	var count int64
	db.Model(&models.Account{}).Count(&count)
	// 3 clients × 2 accounts (tekuci RSD + devizni EUR) = 6
	if count != 6 {
		t.Errorf("expected 6 client accounts, got %d", count)
	}
}

func TestSeedClientAccounts_IsIdempotent(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_client_idem")
	clients := []models.Client{
		{Ime: "Petar", Prezime: "Petrović", Email: "klijent@bank.com"},
		{Ime: "Jelena", Prezime: "Nikolić", Email: "jelena.nikolic@bank.com"},
		{Ime: "Nikola", Prezime: "Đorđević", Email: "nikola.djordjevic@bank.com"},
	}
	for i := range clients {
		if err := db.Create(&clients[i]).Error; err != nil {
			t.Fatalf("create client: %v", err)
		}
	}

	if err := SeedClientAccounts(db); err != nil {
		t.Fatalf("first failed: %v", err)
	}
	var before int64
	db.Model(&models.Account{}).Count(&before)
	if err := SeedClientAccounts(db); err != nil {
		t.Fatalf("second failed: %v", err)
	}
	var after int64
	db.Model(&models.Account{}).Count(&after)
	if before != after {
		t.Errorf("idempotency broken: before=%d after=%d", before, after)
	}
}

func TestSeedClientAccounts_SkipsMissingClients(t *testing.T) {
	db := setupForBankSeed(t, "test_seed_client_missing")
	// Seed currencies but no clients — function should run without error
	// and create no accounts (since all 3 expected emails are missing).
	if err := SeedClientAccounts(db); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	var count int64
	db.Model(&models.Account{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 accounts when clients missing, got %d", count)
	}
}
