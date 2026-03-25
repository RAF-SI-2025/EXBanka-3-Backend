package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/util"
	"gorm.io/gorm"
)

type transferMobileVerificationService interface {
	ApproveTransferMobile(transferID uint, mode string) (*models.Transfer, string, *time.Time, error)
	RejectTransfer(transferID uint) (*models.Transfer, error)
}

type TransferMobileVerificationHandler struct {
	svc transferMobileVerificationService
	db  *gorm.DB
	cfg *config.Config
}

func NewTransferMobileVerificationHandler(db *gorm.DB, cfg *config.Config, exchangeServiceURL string) *TransferMobileVerificationHandler {
	accountRepo := repository.NewAccountRepository(db)
	transferRepo := repository.NewTransferRepository(db)
	exchangeSvc := service.NewHTTPExchangeRateService(exchangeServiceURL)
	notifier := service.NewNotificationService(cfg)
	svc := service.NewTransferServiceWithReposAndNotifier(accountRepo, transferRepo, exchangeSvc, notifier)
	return &TransferMobileVerificationHandler{svc: svc, db: db, cfg: cfg}
}

type transferApprovalRequest struct {
	Mode string `json:"mode"`
}

func (h *TransferMobileVerificationHandler) Approve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	transferID, ok := h.authorize(w, r)
	if !ok {
		return
	}

	var req transferApprovalRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeTransferJSON(w, http.StatusBadRequest, map[string]interface{}{"message": "invalid request body"})
			return
		}
	}

	transfer, verificationCode, expiresAt, err := h.svc.ApproveTransferMobile(transferID, req.Mode)
	if err != nil {
		h.writeVerificationError(w, err)
		return
	}

	response := map[string]interface{}{
		"id":     transfer.ID,
		"status": transfer.Status,
		"mode":   strings.ToLower(strings.TrimSpace(defaultTransferMode(req.Mode))),
	}
	if verificationCode != "" {
		response["verificationCode"] = verificationCode
		if expiresAt != nil {
			response["expiresAt"] = expiresAt.UTC().Format(time.RFC3339)
		}
		response["message"] = "Verification code ready"
	} else {
		response["message"] = "Transfer confirmed successfully"
	}

	writeTransferJSON(w, http.StatusOK, response)
}

func (h *TransferMobileVerificationHandler) Reject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	transferID, ok := h.authorize(w, r)
	if !ok {
		return
	}

	transfer, err := h.svc.RejectTransfer(transferID)
	if err != nil {
		h.writeVerificationError(w, err)
		return
	}

	writeTransferJSON(w, http.StatusOK, map[string]interface{}{
		"id":      transfer.ID,
		"status":  transfer.Status,
		"message": "Transfer cancelled successfully",
	})
}

func (h *TransferMobileVerificationHandler) authorize(w http.ResponseWriter, r *http.Request) (uint, bool) {
	if h.cfg != nil {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if authHeader == "" || !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			writeTransferJSON(w, http.StatusUnauthorized, map[string]interface{}{"message": "missing authorization header"})
			return 0, false
		}
		tokenStr := strings.TrimSpace(authHeader[len("Bearer "):])
		claims, err := util.ParseToken(tokenStr, h.cfg.JWTSecret)
		if err != nil || claims.TokenType != "access" || claims.ClientID == 0 || claims.TokenSource != "client" {
			writeTransferJSON(w, http.StatusUnauthorized, map[string]interface{}{"message": "invalid or expired token"})
			return 0, false
		}
		transferID, err := extractTransferID(r.URL.Path)
		if err != nil {
			writeTransferJSON(w, http.StatusBadRequest, map[string]interface{}{"message": err.Error()})
			return 0, false
		}
		if h.db != nil {
			var transfer models.Transfer
			if err := h.db.First(&transfer, transferID).Error; err != nil {
				writeTransferJSON(w, http.StatusNotFound, map[string]interface{}{"message": "transfer not found"})
				return 0, false
			}
			var account models.Account
			if err := h.db.First(&account, transfer.RacunPosiljaocaID).Error; err != nil {
				writeTransferJSON(w, http.StatusInternalServerError, map[string]interface{}{"message": "failed to verify transfer ownership"})
				return 0, false
			}
			if account.ClientID == nil || *account.ClientID != claims.ClientID {
				writeTransferJSON(w, http.StatusForbidden, map[string]interface{}{"message": "access denied"})
				return 0, false
			}
		}
		return transferID, true
	}

	transferID, err := extractTransferID(r.URL.Path)
	if err != nil {
		writeTransferJSON(w, http.StatusBadRequest, map[string]interface{}{"message": err.Error()})
		return 0, false
	}
	return transferID, true
}

func extractTransferID(path string) (uint, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 5 {
		return 0, errors.New("invalid path")
	}
	id, err := strconv.ParseUint(parts[3], 10, 64)
	if err != nil {
		return 0, errors.New("invalid transfer id")
	}
	return uint(id), nil
}

func (h *TransferMobileVerificationHandler) writeVerificationError(w http.ResponseWriter, err error) {
	statusCode := http.StatusBadRequest
	response := map[string]interface{}{
		"message": err.Error(),
	}

	var verificationErr *service.TransferVerificationError
	if errors.As(err, &verificationErr) {
		response["code"] = verificationErr.Code
		response["status"] = verificationErr.Status
		if verificationErr.AttemptsRemaining > 0 {
			response["attemptsRemaining"] = verificationErr.AttemptsRemaining
		}
		switch verificationErr.Code {
		case "transfer_not_pending":
			statusCode = http.StatusConflict
		case "insufficient_balance", "daily_limit_exceeded", "monthly_limit_exceeded":
			statusCode = http.StatusConflict
		}
	}

	if err.Error() == "unsupported approval mode" {
		response["code"] = "unsupported_approval_mode"
	}

	writeTransferJSON(w, statusCode, response)
}

func defaultTransferMode(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return "code"
	}
	return mode
}

func writeTransferJSON(w http.ResponseWriter, statusCode int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
