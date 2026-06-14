package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/repository"
)

// NotificationHTTPHandler serves the caller-facing /api/v1/notifications routes.
type NotificationHTTPHandler struct {
	cfg  *config.Config
	repo *repository.NotificationRepository
}

func NewNotificationHTTPHandler(cfg *config.Config, repo *repository.NotificationRepository) *NotificationHTTPHandler {
	return &NotificationHTTPHandler{cfg: cfg, repo: repo}
}

// Collection handles GET /api/v1/notifications  (optional ?unread=true&limit=N)
func (h *NotificationHTTPHandler) Collection(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	uid, utype := notifCaller(claims)
	unreadOnly := r.URL.Query().Get("unread") == "true"
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.repo.ListByUser(uid, utype, unreadOnly, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// UnreadCount handles GET /api/v1/notifications/unread-count
func (h *NotificationHTTPHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	uid, utype := notifCaller(claims)
	count, err := h.repo.UnreadCount(uid, utype)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"unread": count})
}

// ReadAll handles POST /api/v1/notifications/read-all
func (h *NotificationHTTPHandler) ReadAll(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	uid, utype := notifCaller(claims)
	if err := h.repo.MarkAllRead(uid, utype); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Routes handles the per-id routes:
//
//	POST   /api/v1/notifications/{id}/read  — mark one read
//	DELETE /api/v1/notifications/{id}        — delete one
func (h *NotificationHTTPHandler) Routes(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireAuthenticatedHTTP(w, r, h.cfg)
	if !ok {
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/notifications/")
	markRead := false
	if strings.HasSuffix(rest, "/read") {
		markRead = true
		rest = strings.TrimSuffix(rest, "/read")
	}
	id, err := strconv.ParseUint(rest, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid notification id"})
		return
	}

	// Ownership check — a caller may only touch their own notifications.
	uid, utype := notifCaller(claims)
	n, err := h.repo.GetByID(uint(id))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
		return
	}
	if n == nil || n.UserID != uid || n.UserType != utype {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "notification not found"})
		return
	}

	switch {
	case markRead && r.Method == http.MethodPost:
		if err := h.repo.MarkRead(uint(id)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case !markRead && r.Method == http.MethodDelete:
		if err := h.repo.Delete(uint(id)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "internal error"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}
