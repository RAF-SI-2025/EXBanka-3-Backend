package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/service"
	"gorm.io/gorm"
)

type transferHTTPService interface {
	CreateTransfer(input service.CreateTransferInput) (*models.Transfer, error)
	PreviewTransfer(input service.CreateTransferInput) (*service.TransferPreview, error)
	ListTransfersByAccount(accountID uint, filter models.TransferFilter) ([]models.Transfer, int64, error)
	ListTransfersByClient(clientID uint, filter models.TransferFilter) ([]models.Transfer, int64, error)
}

type TransferHTTPHandler struct {
	svc transferHTTPService
	db  *gorm.DB
	cfg *config.Config
}

func NewTransferHTTPHandler(db *gorm.DB, exchangeServiceURL string, cfg *config.Config) *TransferHTTPHandler {
	accountRepo := repository.NewAccountRepository(db)
	transferRepo := repository.NewTransferRepository(db)
	exchangeSvc := service.NewHTTPExchangeRateService(exchangeServiceURL)
	notifier := service.NewNotificationService(cfg)
	svc := service.NewTransferServiceWithReposAndNotifier(accountRepo, transferRepo, exchangeSvc, notifier)
	return &TransferHTTPHandler{svc: svc, db: db, cfg: cfg}
}

type transferCreateHTTPRequest struct {
	RacunPosiljaocaIDSnake uint    `json:"racun_posiljaoca_id"`
	RacunPosiljaocaIDCamel uint    `json:"racunPosiljaocaId"`
	RacunPrimaocaIDSnake   uint    `json:"racun_primaoca_id"`
	RacunPrimaocaIDCamel   uint    `json:"racunPrimaocaId"`
	Iznos                  float64 `json:"iznos"`
	Svrha                  string  `json:"svrha"`
}

type transferHTTPJSON struct {
	ID                string  `json:"id"`
	RacunPosiljaocaID string  `json:"racunPosiljaocaId"`
	RacunPrimaocaID   string  `json:"racunPrimaocaId"`
	Iznos             float64 `json:"iznos"`
	ValutaIznosa      string  `json:"valutaIznosa"`
	KonvertovaniIznos float64 `json:"konvertovaniIznos"`
	Kurs              float64 `json:"kurs"`
	Provizija         float64 `json:"provizija"`
	ProvizijaProcent  float64 `json:"provizijaProcent"`
	Svrha             string  `json:"svrha"`
	Status            string  `json:"status"`
	VremeTransakcije  string  `json:"vremeTransakcije"`
}

type transferPreviewJSON struct {
	RacunPosiljaocaID string  `json:"racunPosiljaocaId"`
	RacunPrimaocaID   string  `json:"racunPrimaocaId"`
	Iznos             float64 `json:"iznos"`
	ValutaIznosa      string  `json:"valutaIznosa"`
	KonvertovaniIznos float64 `json:"konvertovaniIznos"`
	Kurs              float64 `json:"kurs"`
	Provizija         float64 `json:"provizija"`
	ProvizijaProcent  float64 `json:"provizijaProcent"`
	Svrha             string  `json:"svrha"`
}

func (h *TransferHTTPHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireClientPermissionHTTP(w, claims, models.PermClientBasic) {
		return
	}

	input, err := decodeTransferInput(r)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.RacunPosiljaocaID == 0 || input.RacunPrimaocaID == 0 {
		writeAuthError(w, http.StatusBadRequest, "sender and receiver account IDs are required")
		return
	}
	if ok := h.ensureAccountsOwnedByClient(w, claims.ClientID, input.RacunPosiljaocaID, input.RacunPrimaocaID); !ok {
		return
	}

	transfer, err := h.svc.CreateTransfer(input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"transfer": toTransferHTTPJSON(transfer),
		"message":  "Transfer created successfully. Verification code sent to email.",
	})
}

func (h *TransferHTTPHandler) Preview(w http.ResponseWriter, r *http.Request) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireClientPermissionHTTP(w, claims, models.PermClientBasic) {
		return
	}

	input, err := decodeTransferInput(r)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.RacunPosiljaocaID == 0 || input.RacunPrimaocaID == 0 {
		writeAuthError(w, http.StatusBadRequest, "sender and receiver account IDs are required")
		return
	}
	if ok := h.ensureAccountsOwnedByClient(w, claims.ClientID, input.RacunPosiljaocaID, input.RacunPrimaocaID); !ok {
		return
	}

	preview, err := h.svc.PreviewTransfer(input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"preview": transferPreviewJSON{
			RacunPosiljaocaID: uintToString(preview.RacunPosiljaocaID),
			RacunPrimaocaID:   uintToString(preview.RacunPrimaocaID),
			Iznos:             preview.Iznos,
			ValutaIznosa:      preview.ValutaIznosa,
			KonvertovaniIznos: preview.KonvertovaniIznos,
			Kurs:              preview.Kurs,
			Provizija:         preview.Provizija,
			ProvizijaProcent:  preview.ProvizijaProcent,
			Svrha:             preview.Svrha,
		},
	})
}

func (h *TransferHTTPHandler) ListByClient(w http.ResponseWriter, r *http.Request) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireClientPermissionHTTP(w, claims, models.PermClientBasic) {
		return
	}
	clientID, err := extractPathUint(r.URL.Path)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid client id")
		return
	}
	if !requireClientBasicHTTP(w, claims, clientID) {
		return
	}

	filter := parseHTTPTransferFilter(r)
	transfers, total, err := h.svc.ListTransfersByClient(clientID, filter)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to list transfers")
		return
	}

	items := make([]transferHTTPJSON, 0, len(transfers))
	for i := range transfers {
		items = append(items, toTransferHTTPJSON(&transfers[i]))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"transfers": items,
		"total":     total,
		"page":      filter.Page,
		"pageSize":  filter.PageSize,
	})
}

func (h *TransferHTTPHandler) ListByAccount(w http.ResponseWriter, r *http.Request) {
	claims, ok := parseHTTPClaims(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireClientPermissionHTTP(w, claims, models.PermClientBasic) {
		return
	}
	accountID, err := extractPathUint(r.URL.Path)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	owned, err := h.accountOwnedByClient(accountID, claims.ClientID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to verify account ownership")
		return
	}
	if !owned {
		writeAuthError(w, http.StatusForbidden, "access denied")
		return
	}

	filter := parseHTTPTransferFilter(r)
	transfers, total, err := h.svc.ListTransfersByAccount(accountID, filter)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to list transfers")
		return
	}

	items := make([]transferHTTPJSON, 0, len(transfers))
	for i := range transfers {
		items = append(items, toTransferHTTPJSON(&transfers[i]))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"transfers": items,
		"total":     total,
		"page":      filter.Page,
		"pageSize":  filter.PageSize,
	})
}

func decodeTransferInput(r *http.Request) (service.CreateTransferInput, error) {
	var req transferCreateHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return service.CreateTransferInput{}, err
	}

	input := service.CreateTransferInput{
		RacunPosiljaocaID: req.RacunPosiljaocaIDSnake,
		RacunPrimaocaID:   req.RacunPrimaocaIDSnake,
		Iznos:             req.Iznos,
		Svrha:             req.Svrha,
	}
	if input.RacunPosiljaocaID == 0 {
		input.RacunPosiljaocaID = req.RacunPosiljaocaIDCamel
	}
	if input.RacunPrimaocaID == 0 {
		input.RacunPrimaocaID = req.RacunPrimaocaIDCamel
	}

	return input, nil
}

func toTransferHTTPJSON(transfer *models.Transfer) transferHTTPJSON {
	return transferHTTPJSON{
		ID:                uintToString(transfer.ID),
		RacunPosiljaocaID: uintToString(transfer.RacunPosiljaocaID),
		RacunPrimaocaID:   uintToString(transfer.RacunPrimaocaID),
		Iznos:             transfer.Iznos,
		ValutaIznosa:      transfer.ValutaIznosa,
		KonvertovaniIznos: transfer.KonvertovaniIznos,
		Kurs:              transfer.Kurs,
		Provizija:         transfer.Provizija,
		ProvizijaProcent:  transfer.ProvizijaProcent,
		Svrha:             transfer.Svrha,
		Status:            transfer.Status,
		VremeTransakcije:  transfer.VremeTransakcije.UTC().Format(time.RFC3339),
	}
}

func parseHTTPTransferFilter(r *http.Request) models.TransferFilter {
	query := r.URL.Query()

	filter := models.TransferFilter{
		Status:   strings.TrimSpace(query.Get("status")),
		Page:     parsePositiveInt(query.Get("page"), 1),
		PageSize: parsePositiveInt(query.Get("page_size"), 20),
	}

	if minAmount := strings.TrimSpace(query.Get("min_amount")); minAmount != "" {
		if value, err := strconv.ParseFloat(minAmount, 64); err == nil {
			filter.MinAmount = &value
		}
	}
	if maxAmount := strings.TrimSpace(query.Get("max_amount")); maxAmount != "" {
		if value, err := strconv.ParseFloat(maxAmount, 64); err == nil {
			filter.MaxAmount = &value
		}
	}
	if dateFrom := strings.TrimSpace(query.Get("date_from")); dateFrom != "" {
		if value, err := time.Parse(time.RFC3339, dateFrom); err == nil {
			filter.DateFrom = &value
		}
	}
	if dateTo := strings.TrimSpace(query.Get("date_to")); dateTo != "" {
		if value, err := time.Parse(time.RFC3339, dateTo); err == nil {
			filter.DateTo = &value
		}
	}

	return filter
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func extractPathUint(path string) (uint, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 5 {
		return 0, strconv.ErrSyntax
	}
	id, err := strconv.ParseUint(parts[4], 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}

func (h *TransferHTTPHandler) ensureAccountsOwnedByClient(w http.ResponseWriter, clientID, senderID, receiverID uint) bool {
	senderOwned, err := h.accountOwnedByClient(senderID, clientID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to verify account ownership")
		return false
	}
	if !senderOwned {
		writeAuthError(w, http.StatusForbidden, "access denied")
		return false
	}

	receiverOwned, err := h.accountOwnedByClient(receiverID, clientID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to verify account ownership")
		return false
	}
	if !receiverOwned {
		writeAuthError(w, http.StatusForbidden, "access denied")
		return false
	}

	return true
}

func (h *TransferHTTPHandler) accountOwnedByClient(accountID, clientID uint) (bool, error) {
	if h.db == nil {
		return true, nil
	}

	var account models.Account
	if err := h.db.First(&account, accountID).Error; err != nil {
		return false, err
	}

	return account.ClientID != nil && *account.ClientID == clientID, nil
}

func uintToString(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
