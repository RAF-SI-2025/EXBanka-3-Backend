package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
)

type listClientAccountsRepo interface {
	ListByClientID(clientID uint) ([]models.Account, error)
}

type ListClientAccountsHTTPHandler struct {
	repo listClientAccountsRepo
}

func NewListClientAccountsHTTPHandler(repo listClientAccountsRepo) *ListClientAccountsHTTPHandler {
	return &ListClientAccountsHTTPHandler{repo: repo}
}

// clientIDFromPath extracts the last path segment from e.g. "/api/v1/accounts/client/42"
func clientIDFromPath(path string) (uint, error) {
	trimmed := strings.TrimRight(path, "/")
	parts := strings.Split(trimmed, "/")
	raw := parts[len(parts)-1]
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}

type clientAccountJSON struct {
	ID                uint       `json:"id"`
	BrojRacuna        string     `json:"brojRacuna"`
	ClientID          *uint      `json:"clientId"`
	FirmaID           *uint      `json:"firmaId"`
	CurrencyID        uint       `json:"currencyId"`
	CurrencyKod       string     `json:"currencyKod"`
	Tip               string     `json:"tip"`
	Vrsta             string     `json:"vrsta"`
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

func (h *ListClientAccountsHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clientID, err := clientIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, `{"error":"invalid client id"}`, http.StatusBadRequest)
		return
	}

	accounts, err := h.repo.ListByClientID(clientID)
	if err != nil {
		http.Error(w, `{"error":"failed to list accounts"}`, http.StatusInternalServerError)
		return
	}

	result := make([]clientAccountJSON, 0, len(accounts))
	for _, a := range accounts {
		item := clientAccountJSON{
			ID:                a.ID,
			BrojRacuna:        a.BrojRacuna,
			ClientID:          a.ClientID,
			FirmaID:           a.FirmaID,
			CurrencyID:        a.CurrencyID,
			CurrencyKod:       a.Currency.Kod,
			Tip:               a.Tip,
			Vrsta:             a.Vrsta,
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
		result = append(result, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
