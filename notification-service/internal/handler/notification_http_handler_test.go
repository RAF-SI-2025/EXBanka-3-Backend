package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/util"
)

func TestNotif_Collection_Unauthenticated(t *testing.T) {
	db := newTestDB(t, "notif_coll_unauth")
	h := NewNotificationHTTPHandler(testCfg(), repository.NewNotificationRepository(db))
	if rec := do(t, h.Collection, http.MethodGet, "/api/v1/notifications", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
	// A refresh token (wrong token_type) is also rejected.
	refresh := makeToken(t, util.Claims{ClientID: 100, TokenSource: "client", TokenType: "refresh"})
	if rec := do(t, h.Collection, http.MethodGet, "/api/v1/notifications", refresh, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("refresh token: want 401, got %d", rec.Code)
	}
}

func TestNotif_Collection_ListAndUnreadFilter(t *testing.T) {
	db := newTestDB(t, "notif_coll_list")
	h := NewNotificationHTTPHandler(testCfg(), repository.NewNotificationRepository(db))
	repo := repository.NewNotificationRepository(db)

	n1 := seedNotif(t, db, 100, "client")
	seedNotif(t, db, 100, "client")
	_ = repo.MarkRead(n1.ID)
	seedNotif(t, db, 200, "client") // a different user's notif must not appear

	tok := clientToken(t)

	// Full list -> 2 for client 100.
	rec := do(t, h.Collection, http.MethodGet, "/api/v1/notifications", tok, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var all []models.Notification
	_ = json.Unmarshal(rec.Body.Bytes(), &all)
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}

	// Unread filter -> 1.
	rec = do(t, h.Collection, http.MethodGet, "/api/v1/notifications?unread=true&limit=10", tok, "")
	var unread []models.Notification
	_ = json.Unmarshal(rec.Body.Bytes(), &unread)
	if len(unread) != 1 {
		t.Errorf("unread filter: want 1, got %d", len(unread))
	}

	// Wrong method -> 404.
	if rec := do(t, h.Collection, http.MethodPost, "/api/v1/notifications", tok, ""); rec.Code != http.StatusNotFound {
		t.Errorf("wrong method: want 404, got %d", rec.Code)
	}
}

func TestNotif_UnreadCount(t *testing.T) {
	db := newTestDB(t, "notif_unread_count")
	h := NewNotificationHTTPHandler(testCfg(), repository.NewNotificationRepository(db))
	seedNotif(t, db, 5, "employee")
	seedNotif(t, db, 5, "employee")

	rec := do(t, h.UnreadCount, http.MethodGet, "/api/v1/notifications/unread-count", employeeToken(t), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]int64
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["unread"] != 2 {
		t.Errorf("unread: want 2, got %d", resp["unread"])
	}
	// Unauthenticated + wrong method.
	if rec := do(t, h.UnreadCount, http.MethodGet, "/api/v1/notifications/unread-count", "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauth: want 401, got %d", rec.Code)
	}
	if rec := do(t, h.UnreadCount, http.MethodPost, "/api/v1/notifications/unread-count", employeeToken(t), ""); rec.Code != http.StatusNotFound {
		t.Errorf("wrong method: want 404, got %d", rec.Code)
	}
}

func TestNotif_ReadAll(t *testing.T) {
	db := newTestDB(t, "notif_read_all")
	repo := repository.NewNotificationRepository(db)
	h := NewNotificationHTTPHandler(testCfg(), repo)
	seedNotif(t, db, 100, "client")
	seedNotif(t, db, 100, "client")

	if rec := do(t, h.ReadAll, http.MethodPost, "/api/v1/notifications/read-all", clientToken(t), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("read-all: want 204, got %d", rec.Code)
	}
	if c, _ := repo.UnreadCount(100, "client"); c != 0 {
		t.Errorf("after read-all unread: want 0, got %d", c)
	}
	// Wrong method -> 404.
	if rec := do(t, h.ReadAll, http.MethodGet, "/api/v1/notifications/read-all", clientToken(t), ""); rec.Code != http.StatusNotFound {
		t.Errorf("wrong method: want 404, got %d", rec.Code)
	}
}

func TestNotif_Routes_MarkReadAndDelete(t *testing.T) {
	db := newTestDB(t, "notif_routes")
	repo := repository.NewNotificationRepository(db)
	h := NewNotificationHTTPHandler(testCfg(), repo)
	n := seedNotif(t, db, 100, "client")
	tok := clientToken(t)

	// Mark one read.
	if rec := do(t, h.Routes, http.MethodPost, fmt.Sprintf("/api/v1/notifications/%d/read", n.ID), tok, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("mark read: want 204, got %d", rec.Code)
	}
	got, _ := repo.GetByID(n.ID)
	if got == nil || !got.IsRead {
		t.Error("notification should be marked read")
	}

	// Delete it.
	if rec := do(t, h.Routes, http.MethodDelete, fmt.Sprintf("/api/v1/notifications/%d", n.ID), tok, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d", rec.Code)
	}
	if after, _ := repo.GetByID(n.ID); after != nil {
		t.Error("notification should be deleted")
	}
}

func TestNotif_Routes_Guards(t *testing.T) {
	db := newTestDB(t, "notif_routes_guards")
	h := NewNotificationHTTPHandler(testCfg(), repository.NewNotificationRepository(db))
	n := seedNotif(t, db, 100, "client")
	tok := clientToken(t)

	// Unauthenticated.
	if rec := do(t, h.Routes, http.MethodPost, fmt.Sprintf("/api/v1/notifications/%d/read", n.ID), "", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauth: want 401, got %d", rec.Code)
	}
	// Bad id.
	if rec := do(t, h.Routes, http.MethodDelete, "/api/v1/notifications/not-a-number", tok, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: want 400, got %d", rec.Code)
	}
	// Missing notification -> 404.
	if rec := do(t, h.Routes, http.MethodDelete, "/api/v1/notifications/99999", tok, ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing: want 404, got %d", rec.Code)
	}
	// Another user's notification -> 404 (ownership hidden as not-found).
	if rec := do(t, h.Routes, http.MethodDelete, fmt.Sprintf("/api/v1/notifications/%d", n.ID), client2Token(t), ""); rec.Code != http.StatusNotFound {
		t.Errorf("non-owner: want 404, got %d", rec.Code)
	}
	// Valid id + owner but unsupported method/path combo -> 404.
	if rec := do(t, h.Routes, http.MethodPut, fmt.Sprintf("/api/v1/notifications/%d", n.ID), tok, ""); rec.Code != http.StatusNotFound {
		t.Errorf("unsupported method: want 404, got %d", rec.Code)
	}
}
