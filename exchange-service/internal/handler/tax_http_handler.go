package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/auditlog"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
	"gorm.io/gorm"
)

// TaxHTTPHandler exposes manual tax-collection trigger and record query endpoints.
type TaxHTTPHandler struct {
	cfg       *config.Config
	taxSvc    *service.TaxService
	collector *service.TaxCollector
	db        *gorm.DB
}

func NewTaxHTTPHandler(cfg *config.Config, taxSvc *service.TaxService, collector *service.TaxCollector, db *gorm.DB) *TaxHTTPHandler {
	return &TaxHTTPHandler{cfg: cfg, taxSvc: taxSvc, collector: collector, db: db}
}

// TaxRoutes handles paths under /api/v1/tax/.
//
//	POST /api/v1/tax/collect              — supervisor: trigger collection for a given period
//	GET  /api/v1/tax/records              — supervisor: list all records, filterable
//	GET  /api/v1/tax/summary/{userId}     — supervisor or the user themselves
func (h *TaxHTTPHandler) TaxRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/tax"), "/")

	switch {
	case path == "collect" && r.Method == http.MethodPost:
		h.triggerCollection(w, r)
	case path == "records" && r.Method == http.MethodGet:
		h.listRecords(w, r)
	case strings.HasPrefix(path, "summary/"):
		h.getUserSummary(w, r, strings.TrimPrefix(path, "summary/"))
	default:
		http.NotFound(w, r)
	}
}

// POST /api/v1/tax/collect
// Body (optional JSON): { "period": "YYYY-MM" }  — defaults to previous month.
func (h *TaxHTTPHandler) triggerCollection(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireSupervisorHTTP(w, claims) {
		return
	}

	period := service.PreviousMonthPeriod()
	var body struct {
		Period string `json:"period"`
	}
	// Ignore decode errors — body is optional.
	_ = json.NewDecoder(r.Body).Decode(&body)
	r.Body.Close()
	if body.Period != "" {
		period = body.Period
	}

	result := h.collector.CollectForPeriod(period)

	auditlog.Record(h.db, auditlog.AuditEntry{
		Action:   auditlog.ActionTaxCollectionTriggered,
		ActorID:  uint(claims.EmployeeID),
		OldValue: "",
		NewValue: fmt.Sprintf("period=%s users_processed=%d total_collected=%.4f",
			result.Period, result.UsersProcessed, result.TotalCollected),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"period":          result.Period,
		"users_processed": result.UsersProcessed,
		"total_collected": result.TotalCollected,
		"debts":           result.Debts,
	})
}

// GET /api/v1/tax/records
// Query params: userId (optional), userType (optional), period (optional)
func (h *TaxHTTPHandler) listRecords(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireSupervisorHTTP(w, claims) {
		return
	}

	q := r.URL.Query()
	period := q.Get("period")

	var recs []models.TaxRecord
	var err error

	if uidStr := q.Get("userId"); uidStr != "" {
		uid, parseErr := strconv.ParseUint(uidStr, 10, 64)
		if parseErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid userId"})
			return
		}
		recs, err = h.taxSvc.ListTaxRecords(uint(uid), q.Get("userType"), period)
	} else {
		recs, err = h.taxSvc.ListAllTaxRecords(period)
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to list records"})
		return
	}

	h.writeRecordsJSON(w, recs)
}

// GET /api/v1/tax/summary/{userId}
// Query params: userType (employee|client), period
func (h *TaxHTTPHandler) getUserSummary(w http.ResponseWriter, r *http.Request, userIDStr string) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}

	uid, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid userId"})
		return
	}
	targetUserID := uint(uid)
	userType := r.URL.Query().Get("userType")
	if userType == "" {
		userType = "client"
	}

	if !isSupervisor(claims) && !isCallerSelf(claims, targetUserID, userType) {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "access denied"})
		return
	}

	period := r.URL.Query().Get("period")
	year := r.URL.Query().Get("year")
	if year == "" {
		year = time.Now().UTC().Format("2006")
	}

	totalUnpaid, err := h.taxSvc.SumUnpaidTax(targetUserID, userType, period)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to compute tax summary"})
		return
	}

	paidThisYear, err := h.taxSvc.SumPaidTaxForYear(targetUserID, userType, year)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to compute paid tax"})
		return
	}

	records, err := h.taxSvc.ListTaxRecords(targetUserID, userType, period)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to list tax records"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":        targetUserID,
		"user_type":      userType,
		"period":         period,
		"total_unpaid":   totalUnpaid,
		"paid_this_year": paidThisYear,
		"record_count":   len(records),
	})
}

// --- helpers ---

func isCallerSelf(claims *util.Claims, targetUserID uint, targetUserType string) bool {
	uid, utype := callerIdentity(claims)
	return uid == targetUserID && utype == targetUserType
}

type taxRecordResponse struct {
	ID        uint      `json:"id"`
	UserID    uint      `json:"user_id"`
	UserType  string    `json:"user_type"`
	AssetID   uint      `json:"asset_id"`
	ProfitRSD float64   `json:"profit_rsd"`
	TaxRSD    float64   `json:"tax_rsd"`
	Period    string    `json:"period"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *TaxHTTPHandler) writeRecordsJSON(w http.ResponseWriter, recs []models.TaxRecord) {
	out := make([]taxRecordResponse, 0, len(recs))
	for _, rec := range recs {
		out = append(out, taxRecordResponse{
			ID:        rec.ID,
			UserID:    rec.UserID,
			UserType:  rec.UserType,
			AssetID:   rec.AssetID,
			ProfitRSD: rec.ProfitRSD,
			TaxRSD:    rec.TaxRSD,
			Period:    rec.Period,
			Status:    rec.Status,
			CreatedAt: rec.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
