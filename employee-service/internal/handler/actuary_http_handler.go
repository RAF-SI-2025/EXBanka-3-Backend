package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/models"
	svc "github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/service"
)

type ActuaryHTTPHandler struct {
	cfg *config.Config
	svc *svc.EmployeeService
}

func NewActuaryHTTPHandler(cfg *config.Config, svc *svc.EmployeeService) *ActuaryHTTPHandler {
	return &ActuaryHTTPHandler{cfg: cfg, svc: svc}
}

func (h *ActuaryHTTPHandler) ListActuaries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	claims, ok := requireAuthenticatedEmployeeHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeSupervisor) {
		return
	}

	items, err := h.svc.ListActuaryStates()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load actuaries"})
		return
	}

	response := make([]actuaryManagementResponse, 0, len(items))
	for _, item := range items {
		response = append(response, actuaryManagementToResponse(item))
	}

	// Apply search filter
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if q != "" {
		filtered := make([]actuaryManagementResponse, 0, len(response))
		for _, item := range response {
			if strings.Contains(strings.ToLower(item.Email), q) ||
				strings.Contains(strings.ToLower(item.Ime), q) ||
				strings.Contains(strings.ToLower(item.Prezime), q) {
				filtered = append(filtered, item)
			}
		}
		response = filtered
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"actuaries": response,
		"count":     len(response),
	})
}

// ActuaryRoutes handles /api/v1/actuaries/{id}/... routes
func (h *ActuaryHTTPHandler) ActuaryRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/actuaries/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	employeeID, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid employee ID"})
		return
	}

	claims, ok := requireAuthenticatedEmployeeHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if !requireEmployeePermissionHTTP(w, claims, models.PermEmployeeSupervisor) {
		return
	}

	switch {
	case len(parts) == 2 && parts[1] == "limit" && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		h.updateAgentLimit(w, r, uint(employeeID))
	case len(parts) == 2 && parts[1] == "reset-used-limit" && r.Method == http.MethodPost:
		h.resetAgentUsedLimit(w, uint(employeeID))
	case len(parts) == 2 && parts[1] == "need-approval" && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		h.setNeedApproval(w, r, uint(employeeID))
	default:
		http.NotFound(w, r)
	}
}

func (h *ActuaryHTTPHandler) updateAgentLimit(w http.ResponseWriter, r *http.Request, employeeID uint) {
	var body struct {
		Limit *float64 `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}

	if err := h.svc.UpdateAgentLimit(employeeID, body.Limit); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "limit updated",
		"employeeId": employeeID,
		"limit":      body.Limit,
	})
}

func (h *ActuaryHTTPHandler) resetAgentUsedLimit(w http.ResponseWriter, employeeID uint) {
	if err := h.svc.ResetAgentUsedLimit(employeeID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "used limit reset to 0",
		"employeeId": employeeID,
	})
}

func (h *ActuaryHTTPHandler) setNeedApproval(w http.ResponseWriter, r *http.Request, employeeID uint) {
	var body struct {
		NeedApproval bool `json:"needApproval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}

	if err := h.svc.SetNeedApproval(employeeID, body.NeedApproval); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":      "need approval updated",
		"employeeId":   employeeID,
		"needApproval": body.NeedApproval,
	})
}

type actuaryManagementResponse struct {
	EmployeeID      string   `json:"employeeId"`
	Ime             string   `json:"ime"`
	Prezime         string   `json:"prezime"`
	Email           string   `json:"email"`
	Username        string   `json:"username"`
	Pozicija        string   `json:"pozicija"`
	Departman       string   `json:"departman"`
	Aktivan         bool     `json:"aktivan"`
	PermissionNames []string `json:"permissionNames"`
	IsActuary       bool     `json:"isActuary"`
	IsSupervisor    bool     `json:"isSupervisor"`
	Limit           *float64 `json:"limit,omitempty"`
	UsedLimit       float64  `json:"usedLimit"`
	NeedApproval    bool     `json:"needApproval"`
}

func actuaryManagementToResponse(item models.ActuaryManagementItem) actuaryManagementResponse {
	return actuaryManagementResponse{
		EmployeeID:      strconv.FormatUint(uint64(item.EmployeeID), 10),
		Ime:             item.Ime,
		Prezime:         item.Prezime,
		Email:           item.Email,
		Username:        item.Username,
		Pozicija:        item.Pozicija,
		Departman:       item.Departman,
		Aktivan:         item.Aktivan,
		PermissionNames: item.PermissionNames,
		IsActuary:       item.IsActuary,
		IsSupervisor:    item.IsSupervisor,
		Limit:           item.Limit,
		UsedLimit:       item.UsedLimit,
		NeedApproval:    item.NeedApproval,
	}
}
