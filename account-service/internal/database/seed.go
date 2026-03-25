package database

import (
	"log/slog"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/util"
	"gorm.io/gorm"
)

// BankFirmaData returns the canonical Firma record for EXBanka itself.
// Exported so tests can verify the business data without a DB.
func BankFirmaData() models.Firma {
	return models.Firma{
		Naziv:       "EXBanka 3 DOO",
		MaticniBroj: "99999999",
		PIB:         "999999999",
		Adresa:      "Knez Mihailova 1, Beograd, Srbija",
	}
}

// BankCurrencyCodes returns the 8 currencies for which the bank holds internal accounts.
// Exported so tests can verify the list without a DB.
func BankCurrencyCodes() []string {
	return []string{"RSD", "EUR", "USD", "GBP", "CHF", "JPY", "CAD", "AUD"}
}

// SeedBankAccounts creates the bank's own Firma record and one tekuci/poslovni account
// per currency. Idempotent — safe to call on every restart.
func SeedBankAccounts(db *gorm.DB) error {
	// 1. Find the SifraDelatnosti "64.1" (Monetarno posredovanje)
	var sifra models.SifraDelatnosti
	if err := db.Where("sifra = ?", "64.1").First(&sifra).Error; err != nil {
		return err
	}

	// 2. Find or create the bank Firma
	firmaData := BankFirmaData()
	var firma models.Firma
	result := db.Where("maticni_broj = ?", firmaData.MaticniBroj).First(&firma)
	if result.Error == gorm.ErrRecordNotFound {
		firma = firmaData
		firma.SifraDelatnostiID = &sifra.ID
		if err := db.Create(&firma).Error; err != nil {
			return err
		}
		slog.Info("Seeded bank Firma", "naziv", firma.Naziv)
	}

	// 3. Create one account per currency (idempotent)
	for _, kod := range BankCurrencyCodes() {
		var currency models.Currency
		if err := db.Where("kod = ?", kod).First(&currency).Error; err != nil {
			slog.Warn("Currency not found, skipping bank account", "kod", kod)
			continue
		}

		var existing models.Account
		err := db.Where("firma_id = ? AND currency_id = ?", firma.ID, currency.ID).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			brojRacuna := util.GenerateAccountNumber("tekuci", "poslovni")
			acc := models.Account{
				BrojRacuna: brojRacuna,
				FirmaID:    &firma.ID,
				CurrencyID: currency.ID,
				Tip:        "tekuci",
				Vrsta:      "poslovni",
				Naziv:      "EXBanka — " + kod,
				Status:     "aktivan",
			}
			if err := db.Create(&acc).Error; err != nil {
				return err
			}
			slog.Info("Seeded bank account", "currency", kod, "broj", brojRacuna)
		}
	}

	slog.Info("Bank accounts seed complete")
	return nil
}

func SeedCurrencies(db *gorm.DB) error {
	currencies := []models.Currency{
		{Kod: "RSD", Naziv: "Srpski dinar", Simbol: "RSD", Drzava: "Srbija", Aktivan: true},
		{Kod: "EUR", Naziv: "Evro", Simbol: "€", Drzava: "Evropska unija", Aktivan: true},
		{Kod: "USD", Naziv: "Američki dolar", Simbol: "$", Drzava: "SAD", Aktivan: true},
		{Kod: "GBP", Naziv: "Britanska funta", Simbol: "£", Drzava: "Velika Britanija", Aktivan: true},
		{Kod: "CHF", Naziv: "Švajcarski franak", Simbol: "CHF", Drzava: "Švajcarska", Aktivan: true},
		{Kod: "JPY", Naziv: "Japanski jen", Simbol: "¥", Drzava: "Japan", Aktivan: true},
		{Kod: "CAD", Naziv: "Kanadski dolar", Simbol: "C$", Drzava: "Kanada", Aktivan: true},
		{Kod: "AUD", Naziv: "Australijski dolar", Simbol: "A$", Drzava: "Australija", Aktivan: true},
	}

	for _, c := range currencies {
		var existing models.Currency
		result := db.Where("kod = ?", c.Kod).First(&existing)
		if result.Error == gorm.ErrRecordNotFound {
			if err := db.Create(&c).Error; err != nil {
				return err
			}
			slog.Info("Seeded currency", "kod", c.Kod)
		}
	}

	slog.Info("Currency seed complete")
	return nil
}

func SeedSifreDelatnosti(db *gorm.DB) error {
	sifre := []models.SifraDelatnosti{
		{Sifra: "01.1", Naziv: "Gajenje jednogodišnjih useva"},
		{Sifra: "01.9", Naziv: "Pomoćne delatnosti u poljoprivredi"},
		{Sifra: "10.1", Naziv: "Proizvodnja hrane"},
		{Sifra: "10.7", Naziv: "Proizvodnja pekarskih proizvoda"},
		{Sifra: "25.1", Naziv: "Proizvodnja metalnih konstrukcija"},
		{Sifra: "41.2", Naziv: "Izgradnja stambenih i nestambenih zgrada"},
		{Sifra: "45.1", Naziv: "Trgovina motornim vozilima"},
		{Sifra: "46.1", Naziv: "Trgovina na veliko"},
		{Sifra: "47.1", Naziv: "Trgovina na malo u nespecijalizovanim prodavnicama"},
		{Sifra: "49.3", Naziv: "Ostali kopneni prevoz putnika"},
		{Sifra: "56.1", Naziv: "Delatnost restorana i pokretnih ugostiteljskih objekata"},
		{Sifra: "62.0", Naziv: "Računarsko programiranje i konsultantske delatnosti"},
		{Sifra: "64.1", Naziv: "Monetarno posredovanje"},
		{Sifra: "64.2", Naziv: "Delatnost holding kompanija"},
		{Sifra: "66.1", Naziv: "Pomoćne delatnosti u finansijskim uslugama"},
		{Sifra: "68.2", Naziv: "Iznajmljivanje i upravljanje nekretninama"},
		{Sifra: "69.1", Naziv: "Pravni poslovi"},
		{Sifra: "69.2", Naziv: "Računovodstveni i revizorski poslovi"},
		{Sifra: "70.2", Naziv: "Konsultantske aktivnosti u vezi sa upravljanjem"},
		{Sifra: "85.4", Naziv: "Visoko obrazovanje"},
	}

	for _, s := range sifre {
		var existing models.SifraDelatnosti
		result := db.Where("sifra = ?", s.Sifra).First(&existing)
		if result.Error == gorm.ErrRecordNotFound {
			if err := db.Create(&s).Error; err != nil {
				return err
			}
			slog.Info("Seeded sifra delatnosti", "sifra", s.Sifra)
		}
	}

	slog.Info("Sifre delatnosti seed complete")
	return nil
}
