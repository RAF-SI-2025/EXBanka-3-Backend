package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/service"
	"gorm.io/gorm"
)

type prenosService interface {
	CreatePrenos(input service.CreatePrenosInput) (*models.Payment, error)
	VerifyPrenos(paymentID uint, verificationCode string) (*models.Payment, error)
}

type PrenosHTTPHandler struct {
	svc prenosService
	db  *gorm.DB
	cfg *config.Config
}

func NewPrenosHTTPHandler(db *gorm.DB, cfg *config.Config) *PrenosHTTPHandler {
	accountRepo := repository.NewAccountRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	notifSvc := service.NewNotificationService(cfg)
	svc := service.NewPrenosServiceWithRepos(accountRepo, paymentRepo, notifSvc)
	return &PrenosHTTPHandler{svc: svc, db: db, cfg: cfg}
}

type createPrenosHTTPRequest struct {
	RacunPosiljaocaIDSnake uint    `json:"racun_posiljaoca_id"`
	RacunPosiljaocaIDCamel uint    `json:"racunPosiljaocaId"`
	RacunPrimaocaBrojSnake string  `json:"racun_primaoca_broj"`
	RacunPrimaocaBrojCamel string  `json:"racunPrimaocaBroj"`
	Iznos                  float64 `json:"iznos"`
	Svrha                  string  `json:"svrha"`
}

func (h *PrenosHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/v1/prenos" && r.Method == http.MethodPost:
		h.create(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/verify"):
		h.verify(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *PrenosHTTPHandler) create(w http.ResponseWriter, r *http.Request) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireClientPermissionHTTP(w, claims, models.PermClientBasic) {
		return
	}

	var req createPrenosHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	racunPosiljaocaID := req.RacunPosiljaocaIDSnake
	if racunPosiljaocaID == 0 {
		racunPosiljaocaID = req.RacunPosiljaocaIDCamel
	}
	racunPrimaocaBroj := strings.TrimSpace(req.RacunPrimaocaBrojSnake)
	if racunPrimaocaBroj == "" {
		racunPrimaocaBroj = strings.TrimSpace(req.RacunPrimaocaBrojCamel)
	}

	owned, err := h.accountOwnedByClient(racunPosiljaocaID, claims.ClientID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to verify account ownership")
		return
	}
	if !owned {
		writeAuthError(w, http.StatusForbidden, "access denied")
		return
	}

	input := service.CreatePrenosInput{
		RacunPosiljaocaID: racunPosiljaocaID,
		RacunPrimaocaBroj: racunPrimaocaBroj,
		Iznos:             req.Iznos,
		Svrha:             req.Svrha,
	}

	if h.db != nil {
		var client models.Client
		if err := h.db.First(&client, claims.ClientID).Error; err == nil {
			input.ClientEmail = client.Email
			input.ClientName = strings.TrimSpace(client.Ime + " " + client.Prezime)
		}
	}

	prenos, err := h.svc.CreatePrenos(input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"prenos":  toPaymentHTTPJSON(prenos),
		"message": "Prenos created, verification code sent to email",
	})
}

func (h *PrenosHTTPHandler) verify(w http.ResponseWriter, r *http.Request) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireClientPermissionHTTP(w, claims, models.PermClientBasic) {
		return
	}

	prenosID, err := extractPrenosID(r.URL.Path)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return
	}
	owned, err := h.paymentOwnedByClient(prenosID, claims.ClientID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to verify prenos ownership")
		return
	}
	if !owned {
		writeAuthError(w, http.StatusForbidden, "access denied")
		return
	}

	var body struct {
		VerificationCode string `json:"verification_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.VerificationCode) == "" {
		writeAuthError(w, http.StatusBadRequest, "verification_code required")
		return
	}

	prenos, err := h.svc.VerifyPrenos(prenosID, body.VerificationCode)
	if err != nil {
		statusCode := http.StatusBadRequest
		payload := map[string]interface{}{
			"message": err.Error(),
		}

		var verificationErr *service.PaymentVerificationError
		if errors.As(err, &verificationErr) {
			payload["code"] = verificationErr.Code
			payload["status"] = verificationErr.Status
			if verificationErr.AttemptsRemaining > 0 {
				payload["attemptsRemaining"] = verificationErr.AttemptsRemaining
			}
			switch verificationErr.Code {
			case "payment_not_pending", "insufficient_balance", "daily_limit_exceeded", "monthly_limit_exceeded", "unsupported_prenos_accounts":
				statusCode = http.StatusConflict
			}
		}

		writeJSON(w, statusCode, payload)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"prenos":  toPaymentHTTPJSON(prenos),
		"message": "Prenos verified successfully",
	})
}

func extractPrenosID(path string) (uint, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 5 {
		return 0, errors.New("invalid path")
	}
	id, err := strconv.ParseUint(parts[3], 10, 64)
	if err != nil {
		return 0, errors.New("invalid prenos id")
	}
	return uint(id), nil
}

func (h *PrenosHTTPHandler) accountOwnedByClient(accountID, clientID uint) (bool, error) {
	if h.db == nil {
		return true, nil
	}

	var account models.Account
	if err := h.db.First(&account, accountID).Error; err != nil {
		return false, err
	}

	return account.ClientID != nil && *account.ClientID == clientID, nil
}

func (h *PrenosHTTPHandler) paymentOwnedByClient(paymentID, clientID uint) (bool, error) {
	if h.db == nil {
		return true, nil
	}

	var payment models.Payment
	if err := h.db.First(&payment, paymentID).Error; err != nil {
		return false, err
	}
	if payment.SifraPlacanja != "254" {
		return false, nil
	}

	var account models.Account
	if err := h.db.First(&account, payment.RacunPosiljaocaID).Error; err != nil {
		return false, err
	}

	return account.ClientID != nil && *account.ClientID == clientID, nil
}
