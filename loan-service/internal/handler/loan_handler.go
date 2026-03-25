package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/service"
	"gorm.io/gorm"
)

// LoanServiceInterface allows handler tests to inject a mock service.
type LoanServiceInterface interface {
	RequestLoan(input service.CreateLoanInput) (*models.Loan, error)
	ApproveLoan(loanID, zaposleniID uint) (*models.Loan, error)
	RejectLoan(loanID, zaposleniID uint) (*models.Loan, error)
	ListByClient(clientID uint) ([]models.Loan, error)
	GetByID(loanID uint) (*models.Loan, error)
	ListInstallments(loanID uint) ([]models.LoanInstallment, error)
	ListRequests() ([]models.Loan, error)
	ListRequestsFiltered(vrsta, brojRacuna string) ([]models.Loan, error)
	ListAllFiltered(filter service.LoanFilter) ([]models.Loan, error)
}

type LoanHandler struct {
	svc LoanServiceInterface
	cfg *config.Config
	db  *gorm.DB
}

func NewLoanHandler(svc LoanServiceInterface) *LoanHandler {
	return &LoanHandler{svc: svc}
}

func NewLoanHandlerWithConfig(svc LoanServiceInterface, cfg *config.Config, db *gorm.DB) *LoanHandler {
	return &LoanHandler{svc: svc, cfg: cfg, db: db}
}

// ServeHTTP routes all /api/v1/loans/... requests.
func (h *LoanHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// strip prefix and split remaining path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/loans")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
		http.NotFound(w, r)
		return
	}

	switch {
	// POST /api/v1/loans/request
	case len(parts) == 1 && parts[0] == "request" && r.Method == http.MethodPost:
		h.handleRequest(w, r)

	// GET /api/v1/loans/requests[?vrsta=...&broj_racuna=...]
	case len(parts) == 1 && parts[0] == "requests" && r.Method == http.MethodGet:
		h.handleListRequests(w, r)

	// GET /api/v1/loans/all[?vrsta=...&status=...&broj_racuna=...]
	case len(parts) == 1 && parts[0] == "all" && r.Method == http.MethodGet:
		h.handleListAll(w, r)

	// GET /api/v1/loans/client/{id}
	case len(parts) == 2 && parts[0] == "client" && r.Method == http.MethodGet:
		h.handleListByClient(w, r, parts[1])

	// GET /api/v1/loans/{id}
	case len(parts) == 1 && r.Method == http.MethodGet:
		h.handleGetByID(w, r, parts[0])

	// GET /api/v1/loans/{id}/installments
	case len(parts) == 2 && parts[1] == "installments" && r.Method == http.MethodGet:
		h.handleListInstallments(w, r, parts[0])

	// POST /api/v1/loans/{id}/approve
	case len(parts) == 2 && parts[1] == "approve" && r.Method == http.MethodPost:
		h.handleApprove(w, r, parts[0])

	// POST /api/v1/loans/{id}/reject
	case len(parts) == 2 && parts[1] == "reject" && r.Method == http.MethodPost:
		h.handleReject(w, r, parts[0])

	default:
		http.NotFound(w, r)
	}
}

func (h *LoanHandler) handleRequest(w http.ResponseWriter, r *http.Request) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}

	var body struct {
		Vrsta      string  `json:"vrsta"`
		BrojRacuna string  `json:"broj_racuna"`
		Iznos      float64 `json:"iznos"`
		Period     int     `json:"period"`
		TipKamate  string  `json:"tip_kamate"`
		ClientID   uint    `json:"client_id"`
		CurrencyID uint    `json:"currency_id"`
		EURIBOR    float64 `json:"euribor_rate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if claims != nil {
		if body.ClientID != 0 && !requireClientPermissionHTTP(w, claims, body.ClientID) {
			return
		}
		if body.ClientID == 0 {
			if !requireClientPermissionHTTP(w, claims, claims.ClientID) {
				return
			}
		}
		body.ClientID = claims.ClientID
	}
	loan, err := h.svc.RequestLoan(service.CreateLoanInput{
		Vrsta:       body.Vrsta,
		BrojRacuna:  body.BrojRacuna,
		Iznos:       body.Iznos,
		Period:      body.Period,
		TipKamate:   body.TipKamate,
		ClientID:    body.ClientID,
		CurrencyID:  body.CurrencyID,
		EURIBORRate: body.EURIBOR,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		jsonError(w, "failed to create loan request", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(loan)
}

func (h *LoanHandler) handleListRequests(w http.ResponseWriter, r *http.Request) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, loanPermEmployeeBasic) {
		return
	}

	vrsta := r.URL.Query().Get("vrsta")
	brojRacuna := r.URL.Query().Get("broj_racuna")
	loans, err := h.svc.ListRequestsFiltered(vrsta, brojRacuna)
	if err != nil {
		jsonError(w, "failed to list requests", http.StatusInternalServerError)
		return
	}
	writeJSON(w, loans)
}

func (h *LoanHandler) handleListAll(w http.ResponseWriter, r *http.Request) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, loanPermEmployeeBasic) {
		return
	}

	q := r.URL.Query()
	filter := service.LoanFilter{
		Vrsta:      q.Get("vrsta"),
		BrojRacuna: q.Get("broj_racuna"),
		Status:     q.Get("status"),
	}
	loans, err := h.svc.ListAllFiltered(filter)
	if err != nil {
		jsonError(w, "failed to list loans", http.StatusInternalServerError)
		return
	}
	writeJSON(w, loans)
}

func (h *LoanHandler) handleListByClient(w http.ResponseWriter, r *http.Request, rawID string) {
	clientID, err := parseUint(rawID)
	if err != nil {
		jsonError(w, "invalid client id", http.StatusBadRequest)
		return
	}
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if claims != nil && (claims.ClientID != 0 || claims.TokenSource == "client") {
		if !requireClientPermissionHTTP(w, claims, clientID) {
			return
		}
	} else if !requireEmployeePermissionHTTP(w, claims, loanPermEmployeeBasic) {
		return
	}
	loans, err := h.svc.ListByClient(clientID)
	if err != nil {
		jsonError(w, "failed to list loans", http.StatusInternalServerError)
		return
	}
	writeJSON(w, loans)
}

func (h *LoanHandler) handleGetByID(w http.ResponseWriter, r *http.Request, rawID string) {
	id, err := parseUint(rawID)
	if err != nil {
		jsonError(w, "invalid loan id", http.StatusBadRequest)
		return
	}
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if claims != nil && (claims.ClientID != 0 || claims.TokenSource == "client") {
		if h.db == nil {
			jsonError(w, "loan ownership check unavailable", http.StatusInternalServerError)
			return
		}
		var loan models.Loan
		if err := h.db.First(&loan, id).Error; err != nil {
			jsonError(w, "loan not found", http.StatusNotFound)
			return
		}
		if !requireClientPermissionHTTP(w, claims, loan.ClientID) {
			return
		}
	} else if !requireEmployeePermissionHTTP(w, claims, loanPermEmployeeBasic) {
		return
	}
	loan, err := h.svc.GetByID(id)
	if err != nil {
		jsonError(w, "loan not found", http.StatusNotFound)
		return
	}
	writeJSON(w, loan)
}

func (h *LoanHandler) handleListInstallments(w http.ResponseWriter, r *http.Request, rawID string) {
	id, err := parseUint(rawID)
	if err != nil {
		jsonError(w, "invalid loan id", http.StatusBadRequest)
		return
	}
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if claims != nil && (claims.ClientID != 0 || claims.TokenSource == "client") {
		if h.db == nil {
			jsonError(w, "loan ownership check unavailable", http.StatusInternalServerError)
			return
		}
		var loan models.Loan
		if err := h.db.First(&loan, id).Error; err != nil {
			jsonError(w, "loan not found", http.StatusNotFound)
			return
		}
		if !requireClientPermissionHTTP(w, claims, loan.ClientID) {
			return
		}
	} else if !requireEmployeePermissionHTTP(w, claims, loanPermEmployeeBasic) {
		return
	}
	items, err := h.svc.ListInstallments(id)
	if err != nil {
		jsonError(w, "failed to list installments", http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

func (h *LoanHandler) handleApprove(w http.ResponseWriter, r *http.Request, rawID string) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, loanPermEmployeeBasic) {
		return
	}

	id, err := parseUint(rawID)
	if err != nil {
		jsonError(w, "invalid loan id", http.StatusBadRequest)
		return
	}
	var body struct {
		ZaposleniID uint `json:"zaposleni_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if claims != nil {
		if body.ZaposleniID != 0 && body.ZaposleniID != claims.EmployeeID {
			jsonError(w, "access denied", http.StatusForbidden)
			return
		}
		body.ZaposleniID = claims.EmployeeID
	}

	loan, err := h.svc.ApproveLoan(id, body.ZaposleniID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, loan)
}

func (h *LoanHandler) handleReject(w http.ResponseWriter, r *http.Request, rawID string) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, loanPermEmployeeBasic) {
		return
	}

	id, err := parseUint(rawID)
	if err != nil {
		jsonError(w, "invalid loan id", http.StatusBadRequest)
		return
	}
	var body struct {
		ZaposleniID uint `json:"zaposleni_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if claims != nil {
		if body.ZaposleniID != 0 && body.ZaposleniID != claims.EmployeeID {
			jsonError(w, "access denied", http.StatusForbidden)
			return
		}
		body.ZaposleniID = claims.EmployeeID
	}

	loan, err := h.svc.RejectLoan(id, body.ZaposleniID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, loan)
}

// --- helpers ---

func parseUint(s string) (uint, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	return uint(v), err
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
