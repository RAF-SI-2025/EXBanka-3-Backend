package handler

import (
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
)

type firmaJSON struct {
	ID                uint   `json:"id"`
	Naziv             string `json:"naziv"`
	MaticniBroj       string `json:"maticniBroj"`
	PIB               string `json:"pib"`
	Adresa            string `json:"adresa"`
	SifraDelatnostiID uint   `json:"sifraDelatnostiId"`
}

type accountHTTPJSON struct {
	ID                uint       `json:"id"`
	BrojRacuna        string     `json:"brojRacuna"`
	ClientID          *uint      `json:"clientId"`
	FirmaID           *uint      `json:"firmaId"`
	Firma             *firmaJSON `json:"firma,omitempty"`
	CurrencyID        uint       `json:"currencyId"`
	CurrencyKod       string     `json:"currencyKod"`
	Tip               string     `json:"tip"`
	Vrsta             string     `json:"vrsta"`
	Podvrsta          string     `json:"podvrsta"`
	Stanje            float64    `json:"stanje"`
	RaspolozivoStanje float64    `json:"raspolozivoStanje"`
	DnevniLimit       float64    `json:"dnevniLimit"`
	MesecniLimit      float64    `json:"mesecniLimit"`
	DnevnaPotrosnja   float64    `json:"dnevnaPotrosnja"`
	MesecnaPotrosnja  float64    `json:"mesecnaPotrosnja"`
	DatumIsteka       *time.Time `json:"datumIsteka"`
	OdrzavanjeRacuna  float64    `json:"odrzavanjeRacuna"`
	Naziv             string     `json:"naziv"`
	Status            string     `json:"status"`
}

func toFirmaJSON(f *models.Firma) *firmaJSON {
	if f == nil {
		return nil
	}

	sifraDelatnostiID := uint(0)
	if f.SifraDelatnostiID != nil {
		sifraDelatnostiID = *f.SifraDelatnostiID
	}

	return &firmaJSON{
		ID:                f.ID,
		Naziv:             f.Naziv,
		MaticniBroj:       f.MaticniBroj,
		PIB:               f.PIB,
		Adresa:            f.Adresa,
		SifraDelatnostiID: sifraDelatnostiID,
	}
}

func toAccountHTTPJSON(a *models.Account) accountHTTPJSON {
	currencyKod := a.Currency.Kod
	return accountHTTPJSON{
		ID:                a.ID,
		BrojRacuna:        a.BrojRacuna,
		ClientID:          a.ClientID,
		FirmaID:           a.FirmaID,
		Firma:             toFirmaJSON(a.Firma),
		CurrencyID:        a.CurrencyID,
		CurrencyKod:       currencyKod,
		Tip:               a.Tip,
		Vrsta:             a.Vrsta,
		Podvrsta:          a.Podvrsta,
		Stanje:            a.Stanje,
		RaspolozivoStanje: a.RaspolozivoStanje,
		DnevniLimit:       a.DnevniLimit,
		MesecniLimit:      a.MesecniLimit,
		DnevnaPotrosnja:   a.DnevnaPotrosnja,
		MesecnaPotrosnja:  a.MesecnaPotrosnja,
		DatumIsteka:       a.DatumIsteka,
		OdrzavanjeRacuna:  a.OdrzavanjeRacuna,
		Naziv:             a.Naziv,
		Status:            a.Status,
	}
}
