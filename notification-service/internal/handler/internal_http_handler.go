package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/service"
)

// InternalHTTPHandler serves the service-to-service emit endpoint. It is NOT
// exposed through the public gateway and is guarded by a shared secret.
type InternalHTTPHandler struct {
	cfg      *config.Config
	repo     *repository.NotificationRepository
	emailSvc service.EmailSender
}

func NewInternalHTTPHandler(cfg *config.Config, repo *repository.NotificationRepository, emailSvc service.EmailSender) *InternalHTTPHandler {
	return &InternalHTTPHandler{cfg: cfg, repo: repo, emailSvc: emailSvc}
}

// EmitRequest is the payload other services POST to raise a notification.
type EmitRequest struct {
	UserID    uint   `json:"user_id"`
	UserType  string `json:"user_type"` // "client" | "employee"
	Type      string `json:"type"`      // machine code, e.g. "ORDER_CREATED"
	Title     string `json:"title"`
	Body      string `json:"body"`
	Link      string `json:"link"`
	SendEmail bool   `json:"send_email"`
	EmailTo   string `json:"email_to"`
}

// Emit handles POST /internal/v1/notifications
func (h *InternalHTTPHandler) Emit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if h.cfg.InternalAPIKey == "" || strings.TrimSpace(r.Header.Get("X-Internal-Key")) != h.cfg.InternalAPIKey {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
		return
	}

	var req EmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request body"})
		return
	}
	defer r.Body.Close()

	if req.UserID == 0 || (req.UserType != "client" && req.UserType != "employee") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "user_id and valid user_type required"})
		return
	}
	if strings.TrimSpace(req.Type) == "" || strings.TrimSpace(req.Title) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "type and title required"})
		return
	}

	n := &models.Notification{
		UserID:   req.UserID,
		UserType: req.UserType,
		Type:     req.Type,
		Title:    req.Title,
		Body:     req.Body,
		Link:     req.Link,
		IsRead:   false,
	}
	if err := h.repo.Create(n); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}

	// Optional email — best effort, fired async so a slow/down SMTP never
	// blocks or fails the in-app persist (which is the source of truth).
	if req.SendEmail && req.EmailTo != "" && h.emailSvc != nil {
		go func(to, subject, body string) {
			if err := h.emailSvc.Send(to, subject, body); err != nil {
				slog.Error("notification email failed", "to", to, "error", err)
			}
		}(req.EmailTo, req.Title, req.Body)
	}

	writeJSON(w, http.StatusCreated, n)
}
