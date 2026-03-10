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

type ActivateHandler struct {
	CredentialService *services.CredentialService
}

func NewActivateHandler(db *sql.DB) *ActivateHandler {
	repo := repository.NewCredentialRepository(db)
	cfg := config.LoadConfig()
	jwtService := services.NewJWTService(
		cfg.JWTSecret,
		cfg.JWTAccessExpirationMinutes,
		cfg.JWTRefreshExpirationHours,
	)
	service := services.NewCredentialService(repo, jwtService)

	return &ActivateHandler{
		CredentialService: service,
	}
}
// Handle godoc
// @Summary Activate account
// @Description Activates account using activation token and sets password
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.ActivateRequest true "Activate request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {string} string
// @Router /auth/activate [post]
func (h *ActivateHandler) Handle(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.ActivateRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	err = h.CredentialService.ActivateAccount(
		req.ActivationToken,
		req.Password,
		req.ConfirmPassword,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := map[string]any{
		"message": "account activated successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}