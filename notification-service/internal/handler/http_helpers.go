package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/util"
)

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			slog.Error("writeJSON encode failed", "error", err)
		}
	}
}

func parseHTTPClaims(r *http.Request, cfg *config.Config) (*util.Claims, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return nil, http.ErrNoCookie
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return util.ParseToken(token, cfg.JWTSecret)
}

func requireAuthenticatedHTTP(w http.ResponseWriter, r *http.Request, cfg *config.Config) (*util.Claims, bool) {
	claims, err := parseHTTPClaims(r, cfg)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "authentication required"})
		return nil, false
	}
	if claims.TokenType != "access" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "authentication required"})
		return nil, false
	}
	return claims, true
}

// notifCaller returns per-user identity for notification ownership — same
// pattern as alertCaller/watchlistCaller in exchange-service.
func notifCaller(claims *util.Claims) (userID uint, userType string) {
	if claims.TokenSource == "employee" {
		return claims.EmployeeID, "employee"
	}
	return claims.ClientID, "client"
}
