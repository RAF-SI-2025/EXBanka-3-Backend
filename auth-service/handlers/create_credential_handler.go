package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"auth-service/models"
	"auth-service/repository"
	"auth-service/services"
	"auth-service/config"
)

type CreateCredentialHandler struct {
	CredentialService *services.CredentialService
}

func NewCreateCredentialHandler(db *sql.DB) *CreateCredentialHandler {
	credentialRepo := repository.NewCredentialRepository(db)
	cfg := config.LoadConfig()
	jwtService := services.NewJWTService(
		cfg.JWTSecret,
		cfg.JWTAccessExpirationMinutes,
		cfg.JWTRefreshExpirationHours,
	)
	credentialService := services.NewCredentialService(credentialRepo, jwtService)

	return &CreateCredentialHandler{
		CredentialService: credentialService,
	}
}
// Handle godoc
// @Summary Create credential
// @Description Internal endpoint for creating employee credentials
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.CreateCredentialRequest true "Create credential request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {string} string
// @Router /auth/internal/create-credential [post]
func (h *CreateCredentialHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.CreateCredentialRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	credential, err := h.CredentialService.CreateCredential(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := map[string]any{
		"message": "credential created successfully",
		"credential": map[string]any{
			"id":               credential.ID,
			"employee_id":      credential.EmployeeID,
			"email":            credential.Email,
			"is_active":        credential.IsActive,
			"activation_token": credential.ActivationToken,
			"created_at":       credential.CreatedAt,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}