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