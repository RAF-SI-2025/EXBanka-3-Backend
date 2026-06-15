package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
)

type PortfolioHTTPHandler struct {
	cfg          *config.Config
	portfolioSvc *service.PortfolioService
	dividendSvc  *service.DividendService
}

func NewPortfolioHTTPHandler(cfg *config.Config, portfolioSvc *service.PortfolioService) *PortfolioHTTPHandler {
	return &PortfolioHTTPHandler{cfg: cfg, portfolioSvc: portfolioSvc}
}

// WithDividendService wires the dividend history / manual-run endpoints.
func (h *PortfolioHTTPHandler) WithDividendService(d *service.DividendService) *PortfolioHTTPHandler {
	h.dividendSvc = d
	return h
}

// PortfolioCollection handles GET /api/v1/portfolio — summary view.
func (h *PortfolioHTTPHandler) PortfolioCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	userID, userType := callerIdentity(claims)
	holdings, err := h.portfolioSvc.ListHoldingsWithPnL(userID, userType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load portfolio"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"portfolio": buildPortfolioSummary(userID, userType, holdings),
	})
}

// PortfolioRoutes handles /api/v1/portfolio/* sub-paths.
func (h *PortfolioHTTPHandler) PortfolioRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/portfolio/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")

	// /api/v1/portfolio/dividends[/run]
	if parts[0] == "dividends" {
		h.dividendRoutes(w, r, parts)
		return
	}

	// /api/v1/portfolio/holdings
	if parts[0] != "holdings" {
		http.NotFound(w, r)
		return
	}

	if len(parts) == 1 {
		// GET /api/v1/portfolio/holdings
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.listHoldings(w, r)
		return
	}

	idStr := parts[1]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid holding id"})
		return
	}
	holdingID := uint(id)

	if len(parts) == 2 {
		// GET /api/v1/portfolio/holdings/{id}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.getHolding(w, r, holdingID)
		return
	}

	sub := parts[2]
	switch {
	case sub == "exercise" && r.Method == http.MethodPost:
		h.exerciseOption(w, r, holdingID)
	case sub == "public" && r.Method == http.MethodPut:
		h.setPublic(w, r, holdingID)
	default:
		http.NotFound(w, r)
	}
}

// --- handlers ---

func (h *PortfolioHTTPHandler) listHoldings(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	userID, userType := callerIdentity(claims)
	holdings, err := h.portfolioSvc.ListHoldingsWithPnL(userID, userType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load holdings"})
		return
	}

	items := make([]holdingResponse, 0, len(holdings))
	for _, h := range holdings {
		items = append(items, holdingToResponse(h))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"holdings": items,
		"count":    len(items),
	})
}

func (h *PortfolioHTTPHandler) getHolding(w http.ResponseWriter, r *http.Request, holdingID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}

	pnl, err := h.portfolioSvc.GetHoldingWithPnL(holdingID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "holding not found"})
		return
	}

	if !isSupervisor(claims) && !isHoldingOwner(claims, pnl.Holding) {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "access denied"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"holding": holdingToResponse(*pnl)})
}

func (h *PortfolioHTTPHandler) exerciseOption(w http.ResponseWriter, r *http.Request, holdingID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	// Actuary (agent or supervisor) only.
	if !util.HasPermission(claims, models.PermEmployeeAgent) || claims.TokenSource != "employee" {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "actuary access required"})
		return
	}

	if err := h.portfolioSvc.ExerciseOption(holdingID, claims.EmployeeID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "option exercised successfully"})
}

func (h *PortfolioHTTPHandler) setPublic(w http.ResponseWriter, r *http.Request, holdingID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}

	holding, err := h.portfolioSvc.GetHoldingByID(holdingID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "holding not found"})
		return
	}
	if !isHoldingOwner(claims, holding) {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "access denied"})
		return
	}

	var body struct {
		IsPublic       *bool    `json:"isPublic"`
		PublicQuantity *float64 `json:"publicQuantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}

	switch {
	case body.PublicQuantity != nil:
		err = h.portfolioSvc.SetPublicQuantity(holdingID, *body.PublicQuantity)
	case body.IsPublic != nil:
		err = h.portfolioSvc.SetPublic(holdingID, *body.IsPublic)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "publicQuantity or isPublic is required"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	updated, err := h.portfolioSvc.GetHoldingByID(holdingID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "holding not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"holdingId":        holdingID,
		"isPublic":         updated.EffectivePublicQuantity() > 0,
		"publicQuantity":   updated.EffectivePublicQuantity(),
		"reservedQuantity": updated.ReservedQuantity,
		"availableForOtc":  updated.AvailableForOTC(),
	})
}

// dividendRoutes handles /api/v1/portfolio/dividends and .../dividends/run.
func (h *PortfolioHTTPHandler) dividendRoutes(w http.ResponseWriter, r *http.Request, parts []string) {
	if h.dividendSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"message": "dividends unavailable"})
		return
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		h.listDividends(w, r)
	case len(parts) == 2 && parts[1] == "run" && r.Method == http.MethodPost:
		h.runDividends(w, r)
	default:
		http.NotFound(w, r)
	}
}

// listDividends returns the caller's own dividend payout history.
func (h *PortfolioHTTPHandler) listDividends(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	userID, userType := callerIdentity(claims)
	var assetID uint
	if raw := strings.TrimSpace(r.URL.Query().Get("assetId")); raw != "" {
		if v, err := strconv.ParseUint(raw, 10, 64); err == nil {
			assetID = uint(v)
		}
	}

	payouts, err := h.dividendSvc.ListPayoutsForUser(userID, userType, assetID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load dividends"})
		return
	}

	items := make([]dividendResponse, 0, len(payouts))
	for _, p := range payouts {
		items = append(items, dividendToResponse(p))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"dividends": items, "count": len(items)})
}

// runDividends triggers a dividend distribution on demand (supervisor only).
// Useful for demos/recovery; the cron normally runs it quarterly. Idempotent per
// quarter, so a manual run after the automatic one is a no-op.
func (h *PortfolioHTTPHandler) runDividends(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !isSupervisor(claims) {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "supervisor access required"})
		return
	}

	res, err := h.dividendSvc.DistributeForDate(time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"period":   res.Period,
		"eligible": res.Eligible,
		"paidOut":  res.PaidOut,
		"skipped":  res.Skipped,
		"failed":   res.Failed,
	})
}

type dividendResponse struct {
	ID               uint    `json:"id"`
	AssetID          uint    `json:"assetId"`
	Ticker           string  `json:"ticker"`
	AccountID        uint    `json:"accountId"`
	Quantity         float64 `json:"quantity"`
	PricePerShare    float64 `json:"pricePerShare"`
	DividendYield    float64 `json:"dividendYield"`
	Currency         string  `json:"currency"`
	GrossAmount      float64 `json:"grossAmount"`
	CreditedAmount   float64 `json:"creditedAmount"`
	CreditedCurrency string  `json:"creditedCurrency"`
	TaxRSD           float64 `json:"taxRSD"`
	Period           string  `json:"period"`
	PaidAt           string  `json:"paidAt"`
}

func dividendToResponse(p models.DividendPayoutRecord) dividendResponse {
	return dividendResponse{
		ID:               p.ID,
		AssetID:          p.AssetID,
		Ticker:           p.Ticker,
		AccountID:        p.AccountID,
		Quantity:         p.Quantity,
		PricePerShare:    p.PricePerShare,
		DividendYield:    p.DividendYield,
		Currency:         p.Currency,
		GrossAmount:      p.GrossAmount,
		CreditedAmount:   p.CreditedAmount,
		CreditedCurrency: p.CreditedCurrency,
		TaxRSD:           p.TaxRSD,
		Period:           p.Period,
		PaidAt:           p.PaidAt.UTC().Format(time.RFC3339),
	}
}

// --- helpers ---

func isHoldingOwner(claims *util.Claims, h *models.PortfolioHoldingRecord) bool {
	uid, utype := callerIdentity(claims)
	return h.UserType == utype && h.UserID == uid
}

// --- response types ---

type holdingResponse struct {
	ID               uint    `json:"id"`
	UserID           uint    `json:"userId"`
	UserType         string  `json:"userType"`
	AssetID          uint    `json:"assetId"`
	AssetTicker      string  `json:"assetTicker"`
	AssetName        string  `json:"assetName"`
	AssetType        string  `json:"assetType"`
	Exchange         string  `json:"exchange"`
	Currency         string  `json:"currency"`
	Quantity         float64 `json:"quantity"`
	AvgBuyPrice      float64 `json:"avgBuyPrice"`
	CurrentPrice     float64 `json:"currentPrice"`
	MarketValue      float64 `json:"marketValue"`
	UnrealizedPnL    float64 `json:"unrealizedPnL"`
	UnrealizedPnLPct float64 `json:"unrealizedPnLPct"`
	RealizedProfit   float64 `json:"realizedProfit"`
	IsPublic         bool    `json:"isPublic"`
	PublicQuantity   float64 `json:"publicQuantity"`
	ReservedQuantity float64 `json:"reservedQuantity"`
	AvailableForOTC  float64 `json:"availableForOtc"`
	AccountID        uint    `json:"accountId"`
	CreatedAt        string  `json:"createdAt"`
}

type portfolioSummaryResponse struct {
	OwnerID        string            `json:"ownerId"`
	OwnerType      string            `json:"ownerType"`
	GeneratedAt    string            `json:"generatedAt"`
	EstimatedValue float64           `json:"estimatedValue"`
	UnrealizedPnL  float64           `json:"unrealizedPnL"`
	RealizedProfit float64           `json:"realizedProfit"`
	PositionCount  int               `json:"positionCount"`
	Holdings       []holdingResponse `json:"holdings"`
}

func holdingToResponse(h service.HoldingWithPnL) holdingResponse {
	ticker, name, assetType, exchange, currency := "", "", "", "", ""
	if h.Holding.Asset.ID != 0 {
		ticker = h.Holding.Asset.Ticker
		name = h.Holding.Asset.Name
		assetType = h.Holding.Asset.Type
		exchange = h.Holding.Asset.Exchange.Acronym
		currency = h.Holding.Asset.Exchange.Currency
	}
	return holdingResponse{
		ID:               h.Holding.ID,
		UserID:           h.Holding.UserID,
		UserType:         h.Holding.UserType,
		AssetID:          h.Holding.AssetID,
		AssetTicker:      ticker,
		AssetName:        name,
		AssetType:        assetType,
		Exchange:         exchange,
		Currency:         currency,
		Quantity:         h.Holding.Quantity,
		AvgBuyPrice:      h.Holding.AvgBuyPrice,
		CurrentPrice:     h.CurrentPrice,
		MarketValue:      h.MarketValue,
		UnrealizedPnL:    h.UnrealizedPnL,
		UnrealizedPnLPct: h.UnrealizedPnLPct,
		RealizedProfit:   h.Holding.RealizedProfit,
		IsPublic:         h.Holding.EffectivePublicQuantity() > 0,
		PublicQuantity:   h.Holding.EffectivePublicQuantity(),
		ReservedQuantity: h.Holding.ReservedQuantity,
		AvailableForOTC:  h.Holding.AvailableForOTC(),
		AccountID:        h.Holding.AccountID,
		CreatedAt:        h.Holding.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func buildPortfolioSummary(userID uint, userType string, holdings []service.HoldingWithPnL) portfolioSummaryResponse {
	var totalValue, totalPnL, totalRealized float64
	items := make([]holdingResponse, 0, len(holdings))
	for _, h := range holdings {
		totalValue += h.MarketValue
		totalPnL += h.UnrealizedPnL
		totalRealized += h.Holding.RealizedProfit
		items = append(items, holdingToResponse(h))
	}
	return portfolioSummaryResponse{
		OwnerID:        strconv.FormatUint(uint64(userID), 10),
		OwnerType:      userType,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		EstimatedValue: round2(totalValue),
		UnrealizedPnL:  round2(totalPnL),
		RealizedProfit: round2(totalRealized),
		PositionCount:  len(items),
		Holdings:       items,
	}
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
