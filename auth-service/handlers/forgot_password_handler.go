package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"auth-service/config"
	"auth-service/models"
	"auth-service/repository"
	"auth-service/services"
)

type ForgotPasswordHandler struct {
	CredentialService *services.CredentialService
}

func NewForgotPasswordHandler(db *sql.DB) *ForgotPasswordHandler {
	repo := repository.NewCredentialRepository(db)
	cfg := config.LoadConfig()
	jwtService := services.NewJWTService(
		cfg.JWTSecret,
		cfg.JWTAccessExpirationMinutes,
		cfg.JWTRefreshExpirationHours,
	)
	service := services.NewCredentialService(repo, jwtService)

	return &ForgotPasswordHandler{
		CredentialService: service,
	}
}
// Handle godoc
// @Summary Forgot password
// @Description Generates password reset token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.ForgotPasswordRequest true "Forgot password request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {string} string
// @Router /auth/forgot-password [post]
func (h *ForgotPasswordHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.ForgotPasswordRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	resetToken, err := h.CredentialService.ForgotPassword(req.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := map[string]any{
		"message":     "password reset token generated",
		"reset_token": resetToken,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}