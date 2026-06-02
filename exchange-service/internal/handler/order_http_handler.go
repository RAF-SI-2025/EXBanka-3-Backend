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

type OrderHTTPHandler struct {
	cfg     *config.Config
	svc     *service.OrderService
	fundSvc *service.FundService
	db      *gorm.DB
}

func NewOrderHTTPHandler(cfg *config.Config, svc *service.OrderService, db *gorm.DB) *OrderHTTPHandler {
	return &OrderHTTPHandler{cfg: cfg, svc: svc, db: db}
}

// WithFundService wires the optional fund service used to validate
// supervisor buy orders placed on behalf of an investment fund.
func (h *OrderHTTPHandler) WithFundService(fundSvc *service.FundService) *OrderHTTPHandler {
	h.fundSvc = fundSvc
	return h
}

// OrdersCollection handles /api/v1/orders (no trailing ID).
func (h *OrderHTTPHandler) OrdersCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listOrders(w, r)
	case http.MethodPost:
		h.createOrder(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// OrderRoutes handles /api/v1/orders/{id} and sub-paths.
func (h *OrderHTTPHandler) OrderRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/orders/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	idStr := parts[0]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid order id"})
		return
	}
	orderID := uint(id)

	if len(parts) == 1 {
		// GET /api/v1/orders/{id}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.getOrder(w, r, orderID)
		return
	}

	sub := parts[1]
	switch {
	case sub == "approve" && r.Method == http.MethodPost:
		h.approveOrder(w, r, orderID)
	case sub == "decline" && r.Method == http.MethodPost:
		h.declineOrder(w, r, orderID)
	case sub == "cancel" && r.Method == http.MethodPost:
		h.cancelOrder(w, r, orderID)
	case sub == "transactions" && r.Method == http.MethodGet:
		h.listTransactions(w, r, orderID)
	default:
		http.NotFound(w, r)
	}
}

// --- handlers ---

func (h *OrderHTTPHandler) createOrder(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	var body createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}

	userID, userType := callerIdentity(claims)

	// Supervisor placing a buy order for an investment fund: hijack the user
	// identity so the order is owned by the fund (not the bank). Validation
	// (supervisor manages the fund, account belongs to fund) happens here.
	if body.FundID != 0 {
		if h.fundSvc == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "fund service not available"})
			return
		}
		if claims.TokenSource != "employee" || !util.HasPermission(claims, models.PermEmployeeSupervisor) {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "samo supervisor moze trgovati za fond"})
			return
		}
		fund, err := h.fundSvc.ValidateFundBuyOrder(body.FundID, claims.EmployeeID, body.AccountID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		userID = fund.ID
		userType = models.PortfolioOwnerFund
	}

	// Auto-detect order type from provided values when the client sends "market".
	// Both values → stop_limit; limit only → limit; stop only → stop.
	orderType := body.OrderType
	if orderType == "market" {
		hasLimit := body.LimitValue != nil && *body.LimitValue > 0
		hasStop := body.StopValue != nil && *body.StopValue > 0
		if hasLimit && hasStop {
			orderType = "stop_limit"
		} else if hasLimit {
			orderType = "limit"
		} else if hasStop {
			orderType = "stop"
		}
	}

	input := service.CreateOrderInput{
		UserID:       userID,
		UserType:     userType,
		ActorID:      claims.EmployeeID,
		AssetTicker:  body.AssetTicker,
		OrderType:    orderType,
		Direction:    body.Direction,
		Quantity:     body.Quantity,
		ContractSize: body.ContractSize,
		LimitValue:   body.LimitValue,
		StopValue:    body.StopValue,
		IsAON:        body.IsAON,
		IsMargin:     body.IsMargin,
		AccountID:    body.AccountID,
		AfterHours:   body.AfterHours,
	}

	result, err := h.svc.CreateOrder(input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"order":      orderToResponse(result.Order),
		"commission": result.Commission,
		"totalPrice": result.TotalPrice,
	})
}

func (h *OrderHTTPHandler) listOrders(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireTradingAccessHTTP(w, claims) {
		return
	}

	statusFilter := r.URL.Query().Get("status")
	userID, userType := callerIdentity(claims)

	var orders []models.OrderRecord
	var err error

	if util.HasPermission(claims, models.PermEmployeeSupervisor) {
		// Supervisors see ALL orders by default.
		// Optionally narrow to a specific user via ?userId=N&userType=employee|client
		if uidStr := r.URL.Query().Get("userId"); uidStr != "" {
			uid, parseErr := strconv.ParseUint(uidStr, 10, 64)
			if parseErr == nil {
				userID = uint(uid)
				if ut := r.URL.Query().Get("userType"); ut != "" {
					userType = ut
				}
			}
			orders, err = h.svc.ListOrdersForUser(userID, userType, statusFilter)
		} else {
			orders, err = h.svc.ListAllOrders(statusFilter)
		}
	} else {
		orders, err = h.svc.ListOrdersForUser(userID, userType, statusFilter)
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load orders"})
		return
	}

	items := make([]orderResponse, 0, len(orders))
	for i := range orders {
		items = append(items, orderToResponse(&orders[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"orders": items,
		"count":  len(items),
	})
}

func (h *OrderHTTPHandler) getOrder(w http.ResponseWriter, r *http.Request, orderID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}

	order, err := h.svc.GetOrder(orderID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "order not found"})
		return
	}

	if !isSupervisor(claims) && !isOwner(claims, order) {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "access denied"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"order": orderToResponse(order)})
}

func (h *OrderHTTPHandler) approveOrder(w http.ResponseWriter, r *http.Request, orderID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireSupervisorHTTP(w, claims) {
		return
	}

	if err := h.svc.ApproveOrder(orderID, claims.EmployeeID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	resID := orderID
	auditlog.Record(h.db, auditlog.AuditEntry{
		Action:       auditlog.ActionOrderApproved,
		ActorID:      uint(claims.EmployeeID),
		ResourceID:   &resID,
		ResourceType: "order",
		OldValue:     "pending",
		NewValue:     fmt.Sprintf("approved_by=%d", claims.EmployeeID),
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "order approved"})
}

func (h *OrderHTTPHandler) declineOrder(w http.ResponseWriter, r *http.Request, orderID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireSupervisorHTTP(w, claims) {
		return
	}

	if err := h.svc.DeclineOrder(orderID, claims.EmployeeID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	resID := orderID
	auditlog.Record(h.db, auditlog.AuditEntry{
		Action:       auditlog.ActionOrderDeclined,
		ActorID:      uint(claims.EmployeeID),
		ResourceID:   &resID,
		ResourceType: "order",
		OldValue:     "pending",
		NewValue:     fmt.Sprintf("declined_by=%d", claims.EmployeeID),
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "order declined"})
}

func (h *OrderHTTPHandler) cancelOrder(w http.ResponseWriter, r *http.Request, orderID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}

	order, err := h.svc.GetOrder(orderID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "order not found"})
		return
	}

	if !isSupervisor(claims) && !isOwner(claims, order) {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "access denied"})
		return
	}

	var body struct {
		NewRemaining int64 `json:"newRemaining"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body.NewRemaining = 0 // default to full cancel
	}

	requesterID, _ := callerIdentity(claims)
	if err := h.svc.CancelOrder(orderID, requesterID, body.NewRemaining); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "order cancelled"})
}

func (h *OrderHTTPHandler) listTransactions(w http.ResponseWriter, r *http.Request, orderID uint) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}

	order, err := h.svc.GetOrder(orderID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "order not found"})
		return
	}

	if !isSupervisor(claims) && !isOwner(claims, order) {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "access denied"})
		return
	}

	txs, err := h.svc.ListTransactionsForOrder(orderID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load transactions"})
		return
	}

	items := make([]transactionResponse, 0, len(txs))
	for _, tx := range txs {
		items = append(items, transactionResponse{
			ID:           tx.ID,
			OrderID:      tx.OrderID,
			Quantity:     tx.Quantity,
			PricePerUnit: tx.PricePerUnit,
			ExecutedAt:   tx.ExecutedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"transactions": items,
		"count":        len(items),
	})
}

// --- helpers ---

// All employees act on behalf of the bank: orders, holdings, and taxes
// collapse onto a single shared bank identity rather than per-employee.
const (
	bankUserID   uint   = 0
	bankUserType string = "bank"
)

func callerIdentity(claims *util.Claims) (userID uint, userType string) {
	if claims.TokenSource == "client" {
		return claims.ClientID, "client"
	}
	return bankUserID, bankUserType
}

func isSupervisor(claims *util.Claims) bool {
	return claims.TokenSource == "employee" &&
		util.HasPermission(claims, models.PermEmployeeSupervisor)
}

func isOwner(claims *util.Claims, order *models.OrderRecord) bool {
	uid, utype := callerIdentity(claims)
	return order.UserType == utype && order.UserID == uid
}

// requireTradingAccessHTTP allows agents, supervisors, and clients with trading permission.
func requireTradingAccessHTTP(w http.ResponseWriter, claims *util.Claims) bool {
	if claims.TokenSource == "employee" {
		if !util.HasPermission(claims, models.PermEmployeeAgent) {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "actuary access required"})
			return false
		}
		return true
	}
	if claims.TokenSource == "client" {
		if !util.HasPermission(claims, models.PermClientTrading) {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "trading permission required"})
			return false
		}
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]string{"message": "access denied"})
	return false
}

// --- request / response types ---

type createOrderRequest struct {
	AssetTicker  string   `json:"assetTicker"`
	OrderType    string   `json:"orderType"`
	Direction    string   `json:"direction"`
	Quantity     int64    `json:"quantity"`
	ContractSize int64    `json:"contractSize"`
	LimitValue   *float64 `json:"limitValue"`
	StopValue    *float64 `json:"stopValue"`
	IsAON        bool     `json:"isAON"`
	IsMargin     bool     `json:"isMargin"`
	AccountID    uint     `json:"accountId"`
	AfterHours   bool     `json:"afterHours"`
	// FundID is set when a supervisor places a buy order on behalf of an
	// investment fund — the order is then owned by the fund (UserType="fund",
	// UserID=fund.ID) instead of the bank.
	FundID uint `json:"fundId"`
}

type orderResponse struct {
	ID                uint     `json:"id"`
	UserID            uint     `json:"userId"`
	UserType          string   `json:"userType"`
	AssetTicker       string   `json:"assetTicker"`
	AssetName         string   `json:"assetName"`
	OrderType         string   `json:"orderType"`
	Direction         string   `json:"direction"`
	Quantity          int64    `json:"quantity"`
	ContractSize      int64    `json:"contractSize"`
	PricePerUnit      float64  `json:"pricePerUnit"`
	LimitValue        *float64 `json:"limitValue"`
	StopValue         *float64 `json:"stopValue"`
	IsAON             bool     `json:"isAON"`
	IsMargin          bool     `json:"isMargin"`
	MarginLoan        float64  `json:"marginLoan"`
	StopTriggered     bool     `json:"stopTriggered"`
	Status            string   `json:"status"`
	IsDone            bool     `json:"isDone"`
	RemainingPortions int64    `json:"remainingPortions"`
	Commission        float64  `json:"commission"`
	TotalPaid         float64  `json:"totalPaid"`
	BalanceBefore     float64  `json:"balanceBefore"`
	BalanceAfter      float64  `json:"balanceAfter"`
	AfterHours        bool     `json:"afterHours"`
	AccountID         uint     `json:"accountId"`
	ApprovedBy        *uint    `json:"approvedBy"`
	LastModification  string   `json:"lastModification"`
	CreatedAt         string   `json:"createdAt"`
}

type transactionResponse struct {
	ID           uint    `json:"id"`
	OrderID      uint    `json:"orderId"`
	Quantity     int64   `json:"quantity"`
	PricePerUnit float64 `json:"pricePerUnit"`
	ExecutedAt   string  `json:"executedAt"`
}

func orderToResponse(o *models.OrderRecord) orderResponse {
	return orderResponse{
		ID:                o.ID,
		UserID:            o.UserID,
		UserType:          o.UserType,
		AssetTicker:       o.Asset.Ticker,
		AssetName:         o.Asset.Name,
		OrderType:         o.OrderType,
		Direction:         o.Direction,
		Quantity:          o.Quantity,
		ContractSize:      o.ContractSize,
		PricePerUnit:      o.PricePerUnit,
		LimitValue:        o.LimitValue,
		StopValue:         o.StopValue,
		IsAON:             o.IsAON,
		IsMargin:          o.IsMargin,
		MarginLoan:        o.MarginLoan,
		StopTriggered:     o.StopTriggered,
		Status:            o.Status,
		IsDone:            o.IsDone,
		RemainingPortions: o.RemainingPortions,
		Commission:        o.Commission,
		TotalPaid:         o.TotalPaid,
		BalanceBefore:     o.BalanceBefore,
		BalanceAfter:      o.BalanceAfter,
		AfterHours:        o.AfterHours,
		AccountID:         o.AccountID,
		ApprovedBy:        o.ApprovedBy,
		LastModification:  o.LastModification.UTC().Format(time.RFC3339),
		CreatedAt:         o.CreatedAt.UTC().Format(time.RFC3339),
	}
}
