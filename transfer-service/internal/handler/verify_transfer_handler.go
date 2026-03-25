package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/util"
	"gorm.io/gorm"
)

// TransferVerifierInterface is the minimal interface needed for verification.
type TransferVerifierInterface interface {
	VerifyTransfer(transferID uint, verificationCode string) (*models.Transfer, error)
}

// VerifyTransferHTTPHandler handles POST /api/v1/transfers/{id}/verify
type VerifyTransferHTTPHandler struct {
	svc TransferVerifierInterface
	db  *gorm.DB
	cfg *config.Config
}

func NewVerifyTransferHTTPHandler(svc TransferVerifierInterface) *VerifyTransferHTTPHandler {
	return &VerifyTransferHTTPHandler{svc: svc}
}

func NewVerifyTransferHTTPHandlerWithConfig(svc TransferVerifierInterface, db *gorm.DB, cfg *config.Config) *VerifyTransferHTTPHandler {
	return &VerifyTransferHTTPHandler{svc: svc, db: db, cfg: cfg}
}

func (h *VerifyTransferHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cfg != nil {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if authHeader == "" || !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimSpace(authHeader[len("Bearer "):])
		claims, err := util.ParseToken(tokenStr, h.cfg.JWTSecret)
		if err != nil || claims.TokenType != "access" || claims.ClientID == 0 || claims.TokenSource != "client" {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}
		if h.db != nil {
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if len(parts) >= 5 {
				if transferID, err := strconv.ParseUint(parts[3], 10, 64); err == nil {
					var transfer models.Transfer
					if err := h.db.First(&transfer, uint(transferID)).Error; err == nil {
						var account models.Account
						if err := h.db.First(&account, transfer.RacunPosiljaocaID).Error; err == nil {
							if account.ClientID == nil || *account.ClientID != claims.ClientID {
								http.Error(w, "access denied", http.StatusForbidden)
								return
							}
						}
					}
				}
			}
		}
	}

	// Extract {id} from path: /api/v1/transfers/{id}/verify
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// Expected: ["api", "v1", "transfers", "{id}", "verify"]
	if len(parts) < 5 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	idStr := parts[3]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid transfer id", http.StatusBadRequest)
		return
	}

	var body struct {
		VerificationCode string `json:"verification_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.VerificationCode == "" {
		http.Error(w, "verification_code required", http.StatusBadRequest)
		return
	}

	transfer, err := h.svc.VerifyTransfer(uint(id), body.VerificationCode)
	if err != nil {
		statusCode := http.StatusBadRequest
		response := map[string]interface{}{
			"message": err.Error(),
		}

		var verificationErr *service.TransferVerificationError
		if errors.As(err, &verificationErr) {
			response["code"] = verificationErr.Code
			response["status"] = verificationErr.Status
			if verificationErr.AttemptsRemaining > 0 || verificationErr.Code == "invalid_verification_code" {
				response["attempts_remaining"] = verificationErr.AttemptsRemaining
			}
			switch verificationErr.Code {
			case "transfer_not_pending":
				statusCode = http.StatusConflict
			case "insufficient_balance", "daily_limit_exceeded", "monthly_limit_exceeded":
				statusCode = http.StatusConflict
			}
		}

		writeJSON(w, statusCode, response)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":     transfer.ID,
		"status": transfer.Status,
	})
}

// CombinedTransferHandler routes /transfers/{id}/verify to the verify handler,
// and all other /transfers requests to the gRPC gateway.
type CombinedTransferHandler struct {
	http     *TransferHTTPHandler
	verify   *VerifyTransferHTTPHandler
	mobile   *TransferMobileVerificationHandler
	fallback http.Handler
}

func NewCombinedTransferHandler(httpHandler *TransferHTTPHandler, verify *VerifyTransferHTTPHandler, mobile *TransferMobileVerificationHandler, fallback http.Handler) *CombinedTransferHandler {
	return &CombinedTransferHandler{http: httpHandler, verify: verify, mobile: mobile, fallback: fallback}
}

func (h *CombinedTransferHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/transfers" && r.Method == http.MethodPost {
		h.http.Create(w, r)
		return
	}
	if r.URL.Path == "/api/v1/transfers/preview" && r.Method == http.MethodPost {
		h.http.Preview(w, r)
		return
	}
	if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/api/v1/transfers/client/") {
		h.http.ListByClient(w, r)
		return
	}
	if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/api/v1/transfers/account/") {
		h.http.ListByAccount(w, r)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/verify") {
		h.verify.ServeHTTP(w, r)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/approve") {
		h.mobile.Approve(w, r)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reject") {
		h.mobile.Reject(w, r)
		return
	}
	h.fallback.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
