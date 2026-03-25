package handler

import (
	"encoding/json"
	"net/http"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/repository"
	"gorm.io/gorm"
)

type FirmaHandler struct {
	firmaRepo *repository.FirmaRepository
	sifraRepo *repository.SifraDelatnostiRepository
	cfg       *config.Config
}

func NewFirmaHandler(db *gorm.DB, cfg *config.Config) *FirmaHandler {
	return &FirmaHandler{
		firmaRepo: repository.NewFirmaRepository(db),
		sifraRepo: repository.NewSifraDelatnostiRepository(db),
		cfg:       cfg,
	}
}

type createFirmaRequest struct {
	Naziv             string `json:"naziv"`
	MaticniBroj       string `json:"maticniBroj"`
	PIB               string `json:"pib"`
	SifraDelatnostiID uint   `json:"sifraDelatnostiId"`
	Adresa            string `json:"adresa"`
	VlasnikID         uint   `json:"vlasnikId"`
}

func (h *FirmaHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeBasic) {
		return
	}

	var req createFirmaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Naziv == "" || req.MaticniBroj == "" || req.PIB == "" {
		http.Error(w, `{"error":"naziv, maticniBroj i pib su obavezni"}`, http.StatusBadRequest)
		return
	}

	firma := &models.Firma{
		Naziv:       req.Naziv,
		MaticniBroj: req.MaticniBroj,
		PIB:         req.PIB,
		Adresa:      req.Adresa,
	}
	if req.SifraDelatnostiID != 0 {
		firma.SifraDelatnostiID = &req.SifraDelatnostiID
	}
	if req.VlasnikID != 0 {
		firma.VlasnikID = &req.VlasnikID
	}

	if err := h.firmaRepo.Create(firma); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "Firma sa tim matičnim brojem ili PIB-om već postoji"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"firma": firma})
}

func (h *FirmaHandler) ListSifreDelatnosti(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeBasic) {
		return
	}

	sifre, err := h.sifraRepo.FindAll()
	if err != nil {
		http.Error(w, `{"error":"failed to load sifre delatnosti"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"sifre": sifre})
}
