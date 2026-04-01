package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

type MarketHTTPHandler struct {
	cfg  *config.Config
	svc  *service.MarketService
	repo *repository.MarketRepository
}

func NewMarketHTTPHandler(cfg *config.Config, svc *service.MarketService, repo *repository.MarketRepository) *MarketHTTPHandler {
	return &MarketHTTPHandler{cfg: cfg, svc: svc, repo: repo}
}

func (h *MarketHTTPHandler) ListExchanges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireMarketReadAccessHTTP(w, claims) {
		return
	}

	exchanges, err := h.svc.ListExchanges()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load exchanges"})
		return
	}
	items := make([]exchangeResponse, 0, len(exchanges))
	for _, exchange := range exchanges {
		items = append(items, exchangeToResponse(exchange))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"exchanges": items,
		"count":     len(items),
	})
}

func (h *MarketHTTPHandler) ListListings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireMarketReadAccessHTTP(w, claims) {
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	listingType := strings.TrimSpace(r.URL.Query().Get("type"))

	listings, err := h.svc.ListListings(query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load listings"})
		return
	}

	// Filter by type if specified
	if listingType != "" {
		filtered := make([]models.Listing, 0, len(listings))
		for _, l := range listings {
			if string(l.Type) == listingType {
				filtered = append(filtered, l)
			}
		}
		listings = filtered
	}

	// For clients, restrict to stocks and futures only
	if claims.TokenSource == "client" {
		filtered := make([]models.Listing, 0, len(listings))
		for _, l := range listings {
			if l.Type == models.ListingTypeStock || l.Type == models.ListingTypeFutures {
				filtered = append(filtered, l)
			}
		}
		listings = filtered
	}

	items := make([]listingResponse, 0, len(listings))
	for _, listing := range listings {
		items = append(items, listingToResponse(listing))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"listings": items,
		"count":    len(items),
		"query":    query,
		"type":     listingType,
	})
}

func (h *MarketHTTPHandler) ListingRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireMarketReadAccessHTTP(w, claims) {
		return
	}

	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/listings/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	ticker := parts[0]

	switch {
	case len(parts) == 1:
		record, err := h.repo.GetListingRecordByTicker(strings.ToUpper(ticker))
		if err != nil || record == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "listing not found"})
			return
		}
		response := listingToResponse(record.ToDomain())
		detail := map[string]interface{}{"listing": response}

		// Attach subtype-specific data
		switch record.Type {
		case "stock":
			if stock, err := h.repo.GetStockByListingID(record.ID); err == nil && stock != nil {
				detail["stock"] = map[string]interface{}{
					"outstandingShares": stock.OutstandingShares,
					"dividendYield":    stock.DividendYield,
				}
			}
		case "forex":
			if forex, err := h.repo.GetForexByListingID(record.ID); err == nil && forex != nil {
				detail["forex"] = map[string]interface{}{
					"baseCurrency":  forex.BaseCurrency,
					"quoteCurrency": forex.QuoteCurrency,
					"liquidity":     forex.Liquidity,
				}
			}
		case "futures":
			if futures, err := h.repo.GetFuturesByListingID(record.ID); err == nil && futures != nil {
				detail["futures"] = map[string]interface{}{
					"contractSize":   futures.ContractSize,
					"contractUnit":   futures.ContractUnit,
					"settlementDate": futures.SettlementDate.Format("2006-01-02"),
				}
			}
		}
		writeJSON(w, http.StatusOK, detail)
	case len(parts) == 2 && parts[1] == "options":
		// Get options chain for a stock
		record, err := h.repo.GetListingRecordByTicker(strings.ToUpper(ticker))
		if err != nil || record == nil || record.Type != "stock" {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "stock listing not found"})
			return
		}
		options, err := h.repo.GetOptionsByStockListingID(record.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load options"})
			return
		}
		optionItems := make([]map[string]interface{}, 0, len(options))
		for _, opt := range options {
			optionItems = append(optionItems, map[string]interface{}{
				"ticker":            opt.Listing.Ticker,
				"name":              opt.Listing.Name,
				"price":             opt.Listing.Price,
				"ask":               opt.Listing.Ask,
				"bid":               opt.Listing.Bid,
				"volume":            opt.Listing.Volume,
				"optionType":        opt.OptionType,
				"strikePrice":       opt.StrikePrice,
				"impliedVolatility": opt.ImpliedVolatility,
				"openInterest":      opt.OpenInterest,
				"settlementDate":    opt.SettlementDate.Format("2006-01-02"),
			})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ticker":     strings.ToUpper(ticker),
			"stockPrice": record.Price,
			"options":    optionItems,
			"count":      len(optionItems),
		})
	case len(parts) == 2 && parts[1] == "history":
		history, err := h.svc.GetListingHistory(ticker)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": err.Error()})
			return
		}
		items := make([]historyResponse, 0, len(history))
		for _, item := range history {
			items = append(items, historyToResponse(item))
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ticker":  strings.ToUpper(ticker),
			"history": items,
		})
	default:
		http.NotFound(w, r)
	}
}

func (h *MarketHTTPHandler) GetPortfolio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireMarketReadAccessHTTP(w, claims) {
		return
	}

	ownerID := claims.ClientID
	ownerType := models.PortfolioOwnerTypeClient
	if claims.TokenSource == "employee" {
		ownerID = claims.EmployeeID
		ownerType = models.PortfolioOwnerTypeEmployee
	}

	portfolio, err := h.svc.GetPortfolio(ownerID, ownerType)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"portfolio": portfolioToResponse(*portfolio),
	})
}

// ExchangeRoutes handles /api/v1/exchanges/{acronym}/... routes
func (h *MarketHTTPHandler) ExchangeRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/exchanges/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	acronym := strings.ToUpper(parts[0])

	switch {
	case len(parts) == 1 && parts[0] == "status":
		// This shouldn't match; status needs an acronym
		http.NotFound(w, r)
	case len(parts) == 2 && parts[1] == "status":
		h.getExchangeStatus(w, r, acronym)
	case len(parts) == 2 && parts[1] == "toggle":
		h.toggleExchangeTime(w, r, acronym)
	default:
		http.NotFound(w, r)
	}
}

func (h *MarketHTTPHandler) getExchangeStatus(w http.ResponseWriter, r *http.Request, acronym string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireMarketReadAccessHTTP(w, claims) {
		return
	}

	record, err := h.repo.GetExchangeByAcronym(acronym)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load exchange"})
		return
	}
	if record == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "exchange not found"})
		return
	}

	exchange := record.ToDomain()
	status := service.GetExchangeTimeStatus(exchange)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"exchange": acronym,
		"status":   status,
	})
}

func (h *MarketHTTPHandler) toggleExchangeTime(w http.ResponseWriter, r *http.Request, acronym string) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	// Only supervisors can toggle
	if !requireSupervisorHTTP(w, claims) {
		return
	}

	var body struct {
		UseManualTime  bool `json:"useManualTime"`
		ManualTimeOpen bool `json:"manualTimeOpen"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}

	if err := h.repo.ToggleExchangeManualTime(acronym, body.UseManualTime, body.ManualTimeOpen); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to update exchange"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"exchange":       acronym,
		"useManualTime":  body.UseManualTime,
		"manualTimeOpen": body.ManualTimeOpen,
		"message":        "exchange time toggle updated",
	})
}

type exchangeResponse struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Acronym        string `json:"acronym"`
	MICCode        string `json:"micCode"`
	Polity         string `json:"polity"`
	Currency       string `json:"currency"`
	Timezone       string `json:"timezone"`
	WorkingHours   string `json:"workingHours"`
	UseManualTime  bool   `json:"useManualTime"`
	ManualTimeOpen bool   `json:"manualTimeOpen"`
	Enabled        bool   `json:"enabled"`
}

type exchangeSummaryResponse struct {
	Name     string `json:"name"`
	Acronym  string `json:"acronym"`
	MICCode  string `json:"micCode"`
	Currency string `json:"currency"`
}

type listingResponse struct {
	Ticker      string                  `json:"ticker"`
	Name        string                  `json:"name"`
	Exchange    exchangeSummaryResponse `json:"exchange"`
	LastRefresh string                  `json:"lastRefresh"`
	Price       float64                 `json:"price"`
	Ask         float64                 `json:"ask"`
	Bid         float64                 `json:"bid"`
	Volume      int64                   `json:"volume"`
	Type        string                  `json:"type"`
}

type historyResponse struct {
	Date   string  `json:"date"`
	Price  float64 `json:"price"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Change float64 `json:"change"`
	Volume int64   `json:"volume"`
}

type portfolioItemResponse struct {
	Ticker       string  `json:"ticker"`
	Name         string  `json:"name"`
	Exchange     string  `json:"exchange"`
	Currency     string  `json:"currency"`
	Quantity     float64 `json:"quantity"`
	AveragePrice float64 `json:"averagePrice"`
	CurrentPrice float64 `json:"currentPrice"`
	MarketValue  float64 `json:"marketValue"`
	PnL          float64 `json:"pnl"`
	PnLPercent   float64 `json:"pnlPercent"`
}

type portfolioResponse struct {
	OwnerID           string                  `json:"ownerId"`
	OwnerType         string                  `json:"ownerType"`
	GeneratedAt       string                  `json:"generatedAt"`
	ValuationAsOf     string                  `json:"valuationAsOf"`
	ValuationCurrency string                  `json:"valuationCurrency"`
	EstimatedValue    float64                 `json:"estimatedValue"`
	UnrealizedPnL     float64                 `json:"unrealizedPnL"`
	PositionCount     int                     `json:"positionCount"`
	ReadOnly          bool                    `json:"readOnly"`
	ModelType         string                  `json:"modelType"`
	PositionSource    string                  `json:"positionSource"`
	PricingSource     string                  `json:"pricingSource"`
	Items             []portfolioItemResponse `json:"items"`
}

func exchangeToResponse(exchange models.Exchange) exchangeResponse {
	return exchangeResponse{
		ID:             exchange.ID,
		Name:           exchange.Name,
		Acronym:        exchange.Acronym,
		MICCode:        exchange.MICCode,
		Polity:         exchange.Polity,
		Currency:       exchange.Currency,
		Timezone:       exchange.Timezone,
		WorkingHours:   exchange.WorkingHours,
		UseManualTime:  exchange.UseManualTime,
		ManualTimeOpen: exchange.ManualTimeOpen,
		Enabled:        exchange.Enabled,
	}
}

func exchangeSummaryToResponse(exchange models.ExchangeSummary) exchangeSummaryResponse {
	return exchangeSummaryResponse{
		Name:     exchange.Name,
		Acronym:  exchange.Acronym,
		MICCode:  exchange.MICCode,
		Currency: exchange.Currency,
	}
}

func listingToResponse(listing models.Listing) listingResponse {
	return listingResponse{
		Ticker:      listing.Ticker,
		Name:        listing.Name,
		Exchange:    exchangeSummaryToResponse(listing.Exchange),
		LastRefresh: listing.LastRefresh.UTC().Format(time.RFC3339),
		Price:       listing.Price,
		Ask:         listing.Ask,
		Bid:         listing.Bid,
		Volume:      listing.Volume,
		Type:        string(listing.Type),
	}
}

func historyToResponse(item models.ListingDailyPriceInfo) historyResponse {
	return historyResponse{
		Date:   item.Date.Format("2006-01-02"),
		Price:  item.Price,
		High:   item.High,
		Low:    item.Low,
		Change: item.Change,
		Volume: item.Volume,
	}
}

func portfolioToResponse(portfolio models.Portfolio) portfolioResponse {
	items := make([]portfolioItemResponse, 0, len(portfolio.Items))
	for _, item := range portfolio.Items {
		items = append(items, portfolioItemResponse{
			Ticker:       item.Ticker,
			Name:         item.Name,
			Exchange:     item.Exchange,
			Currency:     item.Currency,
			Quantity:     item.Quantity,
			AveragePrice: item.AveragePrice,
			CurrentPrice: item.CurrentPrice,
			MarketValue:  item.MarketValue,
			PnL:          item.PnL,
			PnLPercent:   item.PnLPercent,
		})
	}

	return portfolioResponse{
		OwnerID:           strconv.FormatUint(uint64(portfolio.OwnerID), 10),
		OwnerType:         string(portfolio.OwnerType),
		GeneratedAt:       portfolio.GeneratedAt.UTC().Format(time.RFC3339),
		ValuationAsOf:     portfolio.ValuationAsOf.UTC().Format(time.RFC3339),
		ValuationCurrency: portfolio.ValuationCurrency,
		EstimatedValue:    portfolio.EstimatedValue,
		UnrealizedPnL:     portfolio.UnrealizedPnL,
		PositionCount:     portfolio.PositionCount,
		ReadOnly:          portfolio.ReadOnly,
		ModelType:         string(portfolio.ModelType),
		PositionSource:    portfolio.PositionSource,
		PricingSource:     portfolio.PricingSource,
		Items:             items,
	}
}
