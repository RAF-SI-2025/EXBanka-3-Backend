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

type UpdateEmployeeHandler struct {
	Service *services.EmployeeService
}

func NewUpdateEmployeeHandler(db *sql.DB) *UpdateEmployeeHandler {
	repo := repository.NewEmployeeRepository(db)
	service := services.NewEmployeeService(repo, nil)

	return &UpdateEmployeeHandler{
		Service: service,
	}
}
// Handle godoc
// @Summary Update employee
// @Description Updates employee information
// @Tags employees
// @Accept json
// @Produce json
// @Param id path int true "Employee ID"
// @Param request body models.UpdateEmployeeRequest true "Update employee request"
// @Success 200 {object} models.Employee
// @Failure 400 {string} string
// @Router /employees/{id} [put]
func (h *UpdateEmployeeHandler) Handle(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/employees/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid employee id", http.StatusBadRequest)
		return
	}

	var req models.UpdateEmployeeRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	employee, err := h.Service.UpdateEmployee(id, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(employee)
}