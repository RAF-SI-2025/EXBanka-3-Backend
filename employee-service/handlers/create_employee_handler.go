package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"employee-service/config"
	"employee-service/models"
	"employee-service/repository"
	"employee-service/services"
)

type CreateEmployeeHandler struct {
	Service *services.EmployeeService
}

func NewCreateEmployeeHandler(db *sql.DB) *CreateEmployeeHandler {
	repo := repository.NewEmployeeRepository(db)
	cfg := config.LoadConfig()
	authClient := services.NewAuthClient(cfg.AuthServiceURL)
	service := services.NewEmployeeService(repo, authClient)

	return &CreateEmployeeHandler{
		Service: service,
	}
}
// Handle godoc
// @Summary Create employee
// @Description Creates employee and triggers credential creation in auth-service
// @Tags employees
// @Accept json
// @Produce json
// @Param request body models.CreateEmployeeRequest true "Create employee request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {string} string
// @Failure 401 {string} string
// @Failure 403 {string} string
// @Router /employees [post]
func (h *CreateEmployeeHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.CreateEmployeeRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	employee, credentialResponse, err := h.Service.CreateEmployee(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := map[string]any{
		"message": "employee created successfully",
		"employee": employee,
		"credential": credentialResponse.Credential,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}