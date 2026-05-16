package handler

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

type OtcHTTPHandler struct {
	cfg      *config.Config
	svc      *service.OtcService
	sagaRepo SagaQuerier
}

// SagaQuerier is the read-side interface the OTC handler uses to surface the
// saga progress for an exercise call. Defined locally to avoid an import cycle
// with the repository package's wider surface.
type SagaQuerier interface {
	GetTransaction(id uint) (*models.SagaTransactionRecord, error)
}

func NewOtcHTTPHandler(cfg *config.Config, svc *service.OtcService) *OtcHTTPHandler {
	return &OtcHTTPHandler{cfg: cfg, svc: svc}
}

func (h *OtcHTTPHandler) WithSagaQuerier(q SagaQuerier) *OtcHTTPHandler {
	h.sagaRepo = q
	return h
}

func (h *OtcHTTPHandler) OtcRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/otc/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")

	switch {
	case len(parts) == 1 && parts[0] == "public-stocks":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.listPublicStocks(w, r)
	case len(parts) == 1 && parts[0] == "offers":
		switch r.Method {
		case http.MethodGet:
			h.listOffers(w, r)
		case http.MethodPost:
			h.createOffer(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case len(parts) == 1 && parts[0] == "contracts":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.listContracts(w, r)
	case len(parts) == 2 && parts[0] == "contracts":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		contractID, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid contract id"})
			return
		}
		h.getContract(w, r, uint(contractID))
	case len(parts) == 3 && parts[0] == "contracts" && parts[2] == "exercise":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		contractID, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid contract id"})
			return
		}
		h.exerciseContract(w, r, uint(contractID))
	case len(parts) == 2 && parts[0] == "saga":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		sagaID, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid saga id"})
			return
		}
		h.getSagaStatus(w, r, uint(sagaID))
	case len(parts) == 2 && parts[0] == "offers":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		offerID, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid offer id"})
			return
		}
		h.getOffer(w, r, uint(offerID))
	case len(parts) == 3 && parts[0] == "offers":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		offerID, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid offer id"})
			return
		}
		switch parts[2] {
		case "counter":
			h.counterOffer(w, r, uint(offerID))
		case "accept":
			h.acceptOffer(w, r, uint(offerID))
		case "decline":
			h.declineOffer(w, r, uint(offerID))
		case "cancel":
			h.cancelOffer(w, r, uint(offerID))
		default:
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

func (h *OtcHTTPHandler) listPublicStocks(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	requesterID, requesterType := callerIdentity(claims)
	stocks, err := h.svc.ListPublicStocks(requesterID, requesterType)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load public OTC stocks"})
		return
	}

	items := make([]publicOtcStockResponse, 0, len(stocks))
	for _, stock := range stocks {
		items = append(items, publicOtcStockToResponse(stock))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stocks": items,
		"count":  len(items),
	})
}

func (h *OtcHTTPHandler) createOffer(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	var body struct {
		SellerHoldingID uint    `json:"sellerHoldingId"`
		BuyerAccountID  uint    `json:"buyerAccountId"`
		Amount          float64 `json:"amount"`
		PricePerStock   float64 `json:"pricePerStock"`
		SettlementDate  string  `json:"settlementDate"`
		Premium         float64 `json:"premium"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}

	settlementDate, err := parseSettlementDate(body.SettlementDate)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	buyerID, buyerType := callerIdentity(claims)
	offer, err := h.svc.CreateOffer(service.CreateOtcOfferInput{
		BuyerID:         buyerID,
		BuyerType:       buyerType,
		BuyerAccountID:  body.BuyerAccountID,
		SellerHoldingID: body.SellerHoldingID,
		Amount:          body.Amount,
		PricePerStock:   body.PricePerStock,
		SettlementDate:  settlementDate,
		Premium:         body.Premium,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"offer": otcOfferToResponse(*offer)})
}

func (h *OtcHTTPHandler) listOffers(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	userID, userType := callerIdentity(claims)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	offers, err := h.svc.ListOffersForParticipant(userID, userType, status)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	items := make([]otcOfferResponse, 0, len(offers))
	for _, offer := range offers {
		items = append(items, otcOfferToResponse(offer))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"offers": items,
		"count":  len(items),
		"status": status,
	})
}

func (h *OtcHTTPHandler) listContracts(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	userID, userType := callerIdentity(claims)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	contracts, err := h.svc.ListContractsForParticipant(userID, userType, status)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	items := make([]otcContractResponse, 0, len(contracts))
	for _, contract := range contracts {
		items = append(items, otcContractToResponse(contract))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contracts": items,
		"count":     len(items),
		"status":    status,
	})
}

// getContract handles GET /api/v1/otc/contracts/{id}. Returns the contract
// only if the caller is buyer or seller. Added per OTC-2.
func (h *OtcHTTPHandler) getContract(w http.ResponseWriter, r *http.Request, contractID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	userID, userType := callerIdentity(claims)
	contract, err := h.svc.GetContractForParticipant(contractID, userID, userType)
	if err != nil {
		if errors.Is(err, service.ErrOtcContractNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "contract not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load contract"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"contract": otcContractToResponse(*contract)})
}

func (h *OtcHTTPHandler) getOffer(w http.ResponseWriter, r *http.Request, offerID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	userID, userType := callerIdentity(claims)
	offer, err := h.svc.GetOfferForParticipant(offerID, userID, userType)
	if err != nil {
		if errors.Is(err, service.ErrOtcOfferNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "offer not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load offer"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"offer": otcOfferToResponse(*offer)})
}

func (h *OtcHTTPHandler) acceptOffer(w http.ResponseWriter, r *http.Request, offerID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	userID, userType := callerIdentity(claims)
	contract, err := h.svc.AcceptOffer(offerID, userID, userType)
	if err != nil {
		if errors.Is(err, service.ErrOtcOfferNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "offer not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"contract": otcContractToResponse(*contract)})
}

func (h *OtcHTTPHandler) declineOffer(w http.ResponseWriter, r *http.Request, offerID uint) {
	h.updateOfferStatus(w, r, offerID, "decline")
}

func (h *OtcHTTPHandler) cancelOffer(w http.ResponseWriter, r *http.Request, offerID uint) {
	h.updateOfferStatus(w, r, offerID, "cancel")
}

func (h *OtcHTTPHandler) updateOfferStatus(w http.ResponseWriter, r *http.Request, offerID uint, action string) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	userID, userType := callerIdentity(claims)
	var (
		offer *models.OtcOfferRecord
		err   error
	)
	switch action {
	case "decline":
		offer, err = h.svc.DeclineOffer(offerID, userID, userType)
	case "cancel":
		offer, err = h.svc.CancelOffer(offerID, userID, userType)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		if errors.Is(err, service.ErrOtcOfferNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "offer not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"offer": otcOfferToResponse(*offer)})
}

func (h *OtcHTTPHandler) exerciseContract(w http.ResponseWriter, r *http.Request, contractID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	buyerID, buyerType := callerIdentity(claims)
	result, err := h.svc.ExerciseContract(contractID, buyerID, buyerType)
	if err != nil {
		if errors.Is(err, service.ErrOtcContractNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "contract not found"})
			return
		}
		// Even on saga failure we may have a saga id to surface.
		status := http.StatusBadRequest
		body := map[string]interface{}{"message": err.Error()}
		if result != nil && result.SagaID != 0 {
			body["sagaId"] = result.SagaID
		}
		writeJSON(w, status, body)
		return
	}

	body := map[string]interface{}{
		"sagaId": result.SagaID,
	}
	if result.Contract != nil {
		body["contract"] = otcContractToResponse(*result.Contract)
	}
	writeJSON(w, http.StatusOK, body)
}

func (h *OtcHTTPHandler) getSagaStatus(w http.ResponseWriter, r *http.Request, sagaID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}
	if h.sagaRepo == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "saga query unavailable"})
		return
	}

	saga, err := h.sagaRepo.GetTransaction(sagaID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load saga"})
		return
	}
	if saga == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "saga not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"saga": sagaTransactionToResponse(*saga)})
}

func (h *OtcHTTPHandler) counterOffer(w http.ResponseWriter, r *http.Request, offerID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	var body struct {
		Amount         float64 `json:"amount"`
		PricePerStock  float64 `json:"pricePerStock"`
		SettlementDate string  `json:"settlementDate"`
		Premium        float64 `json:"premium"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}

	settlementDate, err := parseSettlementDate(body.SettlementDate)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	modifiedByID, modifiedByType := callerIdentity(claims)
	offer, err := h.svc.CounterOffer(service.CounterOtcOfferInput{
		OfferID:        offerID,
		ModifiedByID:   modifiedByID,
		ModifiedByType: modifiedByType,
		Amount:         body.Amount,
		PricePerStock:  body.PricePerStock,
		SettlementDate: settlementDate,
		Premium:        body.Premium,
	})
	if err != nil {
		if errors.Is(err, service.ErrOtcOfferNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "offer not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"offer": otcOfferToResponse(*offer)})
}

type publicOtcStockResponse struct {
	HoldingID         uint    `json:"holdingId"`
	SellerID          uint    `json:"sellerId"`
	SellerType        string  `json:"sellerType"`
	AssetID           uint    `json:"assetId"`
	Ticker            string  `json:"ticker"`
	Name              string  `json:"name"`
	Exchange          string  `json:"exchange"`
	Currency          string  `json:"currency"`
	Price             float64 `json:"price"`
	Ask               float64 `json:"ask"`
	Bid               float64 `json:"bid"`
	PublicQuantity    float64 `json:"publicQuantity"`
	ReservedQuantity  float64 `json:"reservedQuantity"`
	AvailableQuantity float64 `json:"availableQuantity"`
	LastRefresh       string  `json:"lastRefresh"`
}

func publicOtcStockToResponse(stock service.PublicOtcStock) publicOtcStockResponse {
	return publicOtcStockResponse{
		HoldingID:         stock.HoldingID,
		SellerID:          stock.SellerID,
		SellerType:        stock.SellerType,
		AssetID:           stock.AssetID,
		Ticker:            stock.Ticker,
		Name:              stock.Name,
		Exchange:          stock.Exchange,
		Currency:          stock.Currency,
		Price:             stock.Price,
		Ask:               stock.Ask,
		Bid:               stock.Bid,
		PublicQuantity:    stock.PublicQuantity,
		ReservedQuantity:  stock.ReservedQuantity,
		AvailableQuantity: stock.AvailableQuantity,
		LastRefresh:       stock.LastRefresh.UTC().Format(time.RFC3339),
	}
}

type otcOfferResponse struct {
	ID              uint                    `json:"id"`
	StockListingID  uint                    `json:"stockListingId"`
	SellerHoldingID uint                    `json:"sellerHoldingId"`
	Ticker          string                  `json:"ticker"`
	Name            string                  `json:"name"`
	Exchange        exchangeSummaryResponse `json:"exchange"`
	Amount          float64                 `json:"amount"`
	PricePerStock   float64                 `json:"pricePerStock"`
	CurrentPrice    float64                 `json:"currentPrice"`
	DeviationPct    float64                 `json:"deviationPct"`
	SettlementDate  string                  `json:"settlementDate"`
	Premium         float64                 `json:"premium"`
	LastModified    string                  `json:"lastModified"`
	ModifiedByID    uint                    `json:"modifiedById"`
	ModifiedByType  string                  `json:"modifiedByType"`
	Status          string                  `json:"status"`
	BuyerID         uint                    `json:"buyerId"`
	BuyerType       string                  `json:"buyerType"`
	BuyerAccountID  uint                    `json:"buyerAccountId"`
	SellerID        uint                    `json:"sellerId"`
	SellerType      string                  `json:"sellerType"`
	SellerAccountID uint                    `json:"sellerAccountId"`
}

type otcContractResponse struct {
	ID              uint                    `json:"id"`
	OfferID         *uint                   `json:"offerId,omitempty"`
	StockListingID  uint                    `json:"stockListingId"`
	SellerHoldingID uint                    `json:"sellerHoldingId"`
	Ticker          string                  `json:"ticker"`
	Name            string                  `json:"name"`
	Exchange        exchangeSummaryResponse `json:"exchange"`
	Amount          float64                 `json:"amount"`
	StrikePrice     float64                 `json:"strikePrice"`
	CurrentPrice    float64                 `json:"currentPrice"`
	Premium         float64                 `json:"premium"`
	Profit          float64                 `json:"profit"`
	SettlementDate  string                  `json:"settlementDate"`
	BuyerID         uint                    `json:"buyerId"`
	BuyerType       string                  `json:"buyerType"`
	BuyerAccountID  uint                    `json:"buyerAccountId"`
	SellerID        uint                    `json:"sellerId"`
	SellerType      string                  `json:"sellerType"`
	SellerAccountID uint                    `json:"sellerAccountId"`
	Status          string                  `json:"status"`
	CreatedAt       string                  `json:"createdAt"`
}

func otcOfferToResponse(offer models.OtcOfferRecord) otcOfferResponse {
	currentPrice := offer.StockListing.Price
	deviationPct := 0.0
	if currentPrice > 0 {
		deviationPct = roundOtcDisplayValue(((offer.PricePerStock - currentPrice) / currentPrice) * 100)
	}

	return otcOfferResponse{
		ID:              offer.ID,
		StockListingID:  offer.StockListingID,
		SellerHoldingID: offer.SellerHoldingID,
		Ticker:          offer.StockListing.Ticker,
		Name:            offer.StockListing.Name,
		Exchange:        exchangeSummaryToResponse(offer.StockListing.Exchange.ToSummary()),
		Amount:          offer.Amount,
		PricePerStock:   offer.PricePerStock,
		CurrentPrice:    currentPrice,
		DeviationPct:    deviationPct,
		SettlementDate:  offer.SettlementDate.UTC().Format("2006-01-02"),
		Premium:         offer.Premium,
		LastModified:    offer.LastModified.UTC().Format(time.RFC3339),
		ModifiedByID:    offer.ModifiedByID,
		ModifiedByType:  offer.ModifiedByType,
		Status:          offer.Status,
		BuyerID:         offer.BuyerID,
		BuyerType:       offer.BuyerType,
		BuyerAccountID:  offer.BuyerAccountID,
		SellerID:        offer.SellerID,
		SellerType:      offer.SellerType,
		SellerAccountID: offer.SellerAccountID,
	}
}

func otcContractToResponse(contract models.OtcContractRecord) otcContractResponse {
	return otcContractResponse{
		ID:              contract.ID,
		OfferID:         contract.OfferID,
		StockListingID:  contract.StockListingID,
		SellerHoldingID: contract.SellerHoldingID,
		Ticker:          contract.StockListing.Ticker,
		Name:            contract.StockListing.Name,
		Exchange:        exchangeSummaryToResponse(contract.StockListing.Exchange.ToSummary()),
		Amount:          contract.Amount,
		StrikePrice:     contract.StrikePrice,
		CurrentPrice:    contract.StockListing.Price,
		Premium:         contract.Premium,
		Profit:          roundOtcDisplayValue((contract.StockListing.Price-contract.StrikePrice)*contract.Amount - contract.Premium),
		SettlementDate:  contract.SettlementDate.UTC().Format("2006-01-02"),
		BuyerID:         contract.BuyerID,
		BuyerType:       contract.BuyerType,
		BuyerAccountID:  contract.BuyerAccountID,
		SellerID:        contract.SellerID,
		SellerType:      contract.SellerType,
		SellerAccountID: contract.SellerAccountID,
		Status:          contract.Status,
		CreatedAt:       contract.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func roundOtcDisplayValue(v float64) float64 {
	return math.Round(v*100) / 100
}

func parseSettlementDate(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, errBadSettlementDate()
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC(), nil
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		return parsed.UTC(), nil
	}
	return time.Time{}, errBadSettlementDate()
}

func errBadSettlementDate() error {
	return &badRequestError{message: "settlementDate must be RFC3339 or YYYY-MM-DD"}
}

type badRequestError struct {
	message string
}

func (e *badRequestError) Error() string {
	return e.message
}

type sagaStepResponse struct {
	StepNumber   int    `json:"stepNumber"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	ExecutedAt   string `json:"executedAt,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

type sagaTransactionResponse struct {
	ID          uint               `json:"id"`
	Type        string             `json:"type"`
	Status      string             `json:"status"`
	CurrentStep int                `json:"currentStep"`
	RetryCount  int                `json:"retryCount"`
	Error       string             `json:"error,omitempty"`
	CreatedAt   string             `json:"createdAt"`
	UpdatedAt   string             `json:"updatedAt"`
	Steps       []sagaStepResponse `json:"steps"`
}

func sagaTransactionToResponse(saga models.SagaTransactionRecord) sagaTransactionResponse {
	steps := make([]sagaStepResponse, 0, len(saga.Steps))
	for _, step := range saga.Steps {
		executedAt := ""
		if step.ExecutedAt != nil {
			executedAt = step.ExecutedAt.UTC().Format(time.RFC3339)
		}
		steps = append(steps, sagaStepResponse{
			StepNumber:   step.StepNumber,
			Name:         step.StepName,
			Status:       step.Status,
			ExecutedAt:   executedAt,
			ErrorMessage: step.ErrorMessage,
		})
	}
	return sagaTransactionResponse{
		ID:          saga.ID,
		Type:        saga.Type,
		Status:      saga.Status,
		CurrentStep: saga.CurrentStep,
		RetryCount:  saga.RetryCount,
		Error:       saga.Error,
		CreatedAt:   saga.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   saga.UpdatedAt.UTC().Format(time.RFC3339),
		Steps:       steps,
	}
}
