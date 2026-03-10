package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"employee-service/models"
	"employee-service/repository"
	"employee-service/services"
)

type EmployeePermissionsHandler struct {
	Service *services.EmployeeService
}

func NewEmployeePermissionsHandler(db *sql.DB) *EmployeePermissionsHandler {
	repo := repository.NewEmployeeRepository(db)
	service := services.NewEmployeeService(repo, nil)

	return &EmployeePermissionsHandler{
		Service: service,
	}
}
// Handle godoc
// @Summary Get employee permissions
// @Description Returns employee permissions
// @Tags employees
// @Produce json
// @Param id path int true "Employee ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {string} string
// @Router /employees/{id}/permissions [get]
func (h *EmployeePermissionsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/employees/")
	path = strings.TrimSuffix(path, "/permissions")
	path = strings.Trim(path, "/")

	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "invalid employee id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		permissions, err := h.Service.GetEmployeePermissions(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		response := map[string]any{
			"employee_id":  id,
			"permissions": permissions,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)

	case http.MethodPut:
		var req models.UpdateEmployeePermissionsRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		permissions, err := h.Service.UpdateEmployeePermissions(id, req.Permissions)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		response := map[string]any{
			"employee_id":  id,
			"permissions": permissions,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}