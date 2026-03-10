package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"employee-service/models"
	"employee-service/repository"
	"employee-service/services"
)

type GetEmployeesHandler struct {
	Service *services.EmployeeService
}

func NewGetEmployeesHandler(db *sql.DB) *GetEmployeesHandler {
	repo := repository.NewEmployeeRepository(db)
	service := services.NewEmployeeService(repo, nil)

	return &GetEmployeesHandler{
		Service: service,
	}
}
// Handle godoc
// @Summary List employees
// @Description Returns all employees with optional filters
// @Tags employees
// @Produce json
// @Param email query string false "Filter by email"
// @Param first_name query string false "Filter by first name"
// @Param last_name query string false "Filter by last name"
// @Param position query string false "Filter by position"
// @Success 200 {array} models.Employee
// @Failure 401 {string} string
// @Failure 403 {string} string
// @Router /employees [get]
func (h *GetEmployeesHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := &models.EmployeeFilter{
		Email:     strings.TrimSpace(r.URL.Query().Get("email")),
		FirstName: strings.TrimSpace(r.URL.Query().Get("first_name")),
		LastName:  strings.TrimSpace(r.URL.Query().Get("last_name")),
		Position:  strings.TrimSpace(r.URL.Query().Get("position")),
	}

	employees, err := h.Service.GetAllEmployees(filter)
	if err != nil {
		http.Error(w, "failed to fetch employees", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(employees)
}