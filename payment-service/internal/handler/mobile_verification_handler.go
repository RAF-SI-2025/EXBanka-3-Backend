package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/util"
	"gorm.io/gorm"
)

type paymentMobileVerificationService interface {
	ApprovePaymentMobile(paymentID uint, mode string) (*models.Payment, string, *time.Time, error)
	RejectPayment(paymentID uint) (*models.Payment, error)
}

type PaymentMobileVerificationHandler struct {
	svc paymentMobileVerificationService
	db  *gorm.DB
	cfg *config.Config
}

func NewPaymentMobileVerificationHandler(db *gorm.DB, cfg *config.Config) *PaymentMobileVerificationHandler {
	accountRepo := repository.NewAccountRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	recipientRepo := repository.NewPaymentRecipientRepository(db)
	notifSvc := service.NewNotificationService(cfg)
	svc := service.NewPaymentServiceWithRepos(accountRepo, paymentRepo, recipientRepo, notifSvc)
	return &PaymentMobileVerificationHandler{svc: svc, db: db, cfg: cfg}
}

type paymentApprovalRequest struct {
	Mode string `json:"mode"`
}

func (h *PaymentMobileVerificationHandler) Approve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	paymentID, claims, ok := h.authorize(w, r)
	if !ok {
		return
	}
	if !requireClientBasicHTTP(w, claims, claims.ClientID) {
		return
	}

	var req paymentApprovalRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAuthError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	payment, verificationCode, expiresAt, err := h.svc.ApprovePaymentMobile(paymentID, req.Mode)
	if err != nil {
		h.writeVerificationError(w, err)
		return
	}

	response := map[string]interface{}{
		"payment": toPaymentHTTPJSON(payment),
		"mode":    strings.ToLower(strings.TrimSpace(defaultString(req.Mode, "code"))),
	}
	if verificationCode != "" {
		response["verificationCode"] = verificationCode
		if expiresAt != nil {
			response["expiresAt"] = expiresAt.UTC().Format(time.RFC3339)
		}
		response["message"] = "Verification code ready"
	} else {
		response["message"] = "Payment confirmed successfully"
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *PaymentMobileVerificationHandler) Reject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	paymentID, claims, ok := h.authorize(w, r)
	if !ok {
		return
	}
	if !requireClientBasicHTTP(w, claims, claims.ClientID) {
		return
	}

	payment, err := h.svc.RejectPayment(paymentID)
	if err != nil {
		h.writeVerificationError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"payment": toPaymentHTTPJSON(payment),
		"message": "Payment cancelled successfully",
	})
}

func (h *PaymentMobileVerificationHandler) authorize(w http.ResponseWriter, r *http.Request) (uint, *util.Claims, bool) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return 0, nil, false
	}
	if !requireClientPermissionHTTP(w, claims, models.PermClientBasic) {
		return 0, nil, false
	}

	paymentID, err := extractPaymentID(r.URL.Path)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, err.Error())
		return 0, nil, false
	}
	owned, err := h.paymentOwnedByClient(paymentID, claims.ClientID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to verify payment ownership")
		return 0, nil, false
	}
	if !owned {
		writeAuthError(w, http.StatusForbidden, "access denied")
		return 0, nil, false
	}

	return paymentID, claims, true
}

func extractPaymentID(path string) (uint, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 5 {
		return 0, errors.New("invalid path")
	}
	id, err := strconv.ParseUint(parts[3], 10, 64)
	if err != nil {
		return 0, errors.New("invalid payment id")
	}
	return uint(id), nil
}

func (h *PaymentMobileVerificationHandler) paymentOwnedByClient(paymentID, clientID uint) (bool, error) {
	if h.db == nil {
		return true, nil
	}

	var payment models.Payment
	if err := h.db.First(&payment, paymentID).Error; err != nil {
		return false, err
	}

	var account models.Account
	if err := h.db.First(&account, payment.RacunPosiljaocaID).Error; err != nil {
		return false, err
	}

	return account.ClientID != nil && *account.ClientID == clientID, nil
}

func (h *PaymentMobileVerificationHandler) writeVerificationError(w http.ResponseWriter, err error) {
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
		case "payment_not_pending":
			statusCode = http.StatusConflict
		case "insufficient_balance", "daily_limit_exceeded", "monthly_limit_exceeded", "unsupported_payment_currency":
			statusCode = http.StatusConflict
		}
	}

	if err.Error() == "unsupported approval mode" {
		statusCode = http.StatusBadRequest
		payload["code"] = "unsupported_approval_mode"
	}

	writeJSON(w, statusCode, payload)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

type CombinedPaymentHandler struct {
	create   *CreatePaymentHTTPHandler
	mobile   *PaymentMobileVerificationHandler
	fallback http.Handler
}

func NewCombinedPaymentHandler(create *CreatePaymentHTTPHandler, mobile *PaymentMobileVerificationHandler, fallback http.Handler) *CombinedPaymentHandler {
	return &CombinedPaymentHandler{create: create, mobile: mobile, fallback: fallback}
}

func (h *CombinedPaymentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/v1/payments" && r.Method == http.MethodPost:
		h.create.ServeHTTP(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/approve"):
		h.mobile.Approve(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reject"):
		h.mobile.Reject(w, r)
	default:
		h.fallback.ServeHTTP(w, r)
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
