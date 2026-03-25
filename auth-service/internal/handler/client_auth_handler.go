package handler

import (
	"encoding/json"
	"net/http"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/config"
	authsvc "github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/service"
	"gorm.io/gorm"
)

type ClientAuthHandler struct {
	svc *authsvc.AuthService
}

func NewClientAuthHandler(cfg *config.Config, db *gorm.DB, notifSvc *authsvc.NotificationService) *ClientAuthHandler {
	return &ClientAuthHandler{
		svc: authsvc.NewAuthService(cfg, db, notifSvc),
	}
}

type clientLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type clientActivateRequest struct {
	Token           string `json:"token"`
	Password        string `json:"password"`
	PasswordConfirm string `json:"passwordConfirm"`
}

type clientLoginResponse struct {
	AccessToken  string     `json:"accessToken"`
	RefreshToken string     `json:"refreshToken"`
	Client       clientInfo `json:"client"`
}

type clientInfo struct {
	ID          uint     `json:"id"`
	Ime         string   `json:"ime"`
	Prezime     string   `json:"prezime"`
	Email       string   `json:"email"`
	Permissions []string `json:"permissions"`
}

func (h *ClientAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req clientLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	accessToken, refreshToken, client, err := h.svc.ClientLogin(req.Email, req.Password)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	resp := clientLoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Client: clientInfo{
			ID:          client.ID,
			Ime:         client.Ime,
			Prezime:     client.Prezime,
			Email:       client.Email,
			Permissions: client.PermissionNames(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *ClientAuthHandler) Activate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req clientActivateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if err := h.svc.ActivateClientAccount(req.Token, req.Password, req.PasswordConfirm); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Client account activated successfully",
	})
}
