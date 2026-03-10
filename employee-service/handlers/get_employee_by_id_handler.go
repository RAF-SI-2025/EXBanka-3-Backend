package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"employee-service/repository"
	"employee-service/services"
)

type GetEmployeeByIDHandler struct {
	Service *services.EmployeeService
}

func NewGetEmployeeByIDHandler(db *sql.DB) *GetEmployeeByIDHandler {
	repo := repository.NewEmployeeRepository(db)
	service := services.NewEmployeeService(repo, nil)

	return &GetEmployeeByIDHandler{
		Service: service,
	}
}
// Handle godoc
// @Summary Get employee by ID
// @Description Returns employee details by ID
// @Tags employees
// @Produce json
// @Param id path int true "Employee ID"
// @Success 200 {object} models.Employee
// @Failure 400 {string} string
// @Failure 404 {string} string
// @Router /employees/{id} [get]
func (h *GetEmployeeByIDHandler) Handle(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/employees/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid employee id", http.StatusBadRequest)
		return
	}

	employee, err := h.Service.GetEmployeeByID(id)
	if err != nil {
		http.Error(w, "employee not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(employee)
}