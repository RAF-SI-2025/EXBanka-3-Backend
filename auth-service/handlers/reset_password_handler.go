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

type ResetPasswordHandler struct {
	CredentialService *services.CredentialService
}

func NewResetPasswordHandler(db *sql.DB) *ResetPasswordHandler {
	repo := repository.NewCredentialRepository(db)
	cfg := config.LoadConfig()
	jwtService := services.NewJWTService(
		cfg.JWTSecret,
		cfg.JWTAccessExpirationMinutes,
		cfg.JWTRefreshExpirationHours,
	)
	service := services.NewCredentialService(repo, jwtService)

	return &ResetPasswordHandler{
		CredentialService: service,
	}
}
// Handle godoc
// @Summary Reset password
// @Description Resets password using reset token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.ResetPasswordRequest true "Reset password request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {string} string
// @Router /auth/reset-password [post]
func (h *ResetPasswordHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.ResetPasswordRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	err = h.CredentialService.ResetPassword(
		req.ResetToken,
		req.Password,
		req.ConfirmPassword,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := map[string]any{
		"message": "password reset successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}