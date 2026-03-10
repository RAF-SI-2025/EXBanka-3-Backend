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

type LoginHandler struct {
	CredentialService *services.CredentialService
}

func NewLoginHandler(db *sql.DB) *LoginHandler {
	repo := repository.NewCredentialRepository(db)
	cfg := config.LoadConfig()
	jwtService := services.NewJWTService(
		cfg.JWTSecret,
		cfg.JWTAccessExpirationMinutes,
		cfg.JWTRefreshExpirationHours,
	)
	service := services.NewCredentialService(repo, jwtService)

	return &LoginHandler{
		CredentialService: service,
	}
}
// Handle godoc
// @Summary Login user
// @Description Authenticates user and returns access and refresh token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body models.LoginRequest true "Login request"
// @Success 200 {object} models.LoginResponse
// @Failure 400 {string} string
// @Failure 401 {string} string
// @Router /auth/login [post]
func (h *LoginHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.LoginRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	response, err := h.CredentialService.Login(req.Email, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}