package handler

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/repository"
)

// recordingEmail captures Send calls for assertion.
type recordingEmail struct {
	mu    sync.Mutex
	calls []struct{ to, subject, body string }
	err   error
}

func (m *recordingEmail) Send(to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, struct{ to, subject, body string }{to, subject, body})
	return m.err
}

func (m *recordingEmail) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// emit fires a request at the Emit endpoint with the given key header + body.
func emit(t *testing.T, h *InternalHTTPHandler, method, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/internal/v1/notifications", bytesReader(body))
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}
	rr := httptest.NewRecorder()
	h.Emit(rr, req)
	return rr
}

func TestEmit_PersistsAndEmails(t *testing.T) {
	db := newTestDB(t, "emit_ok")
	repo := repository.NewNotificationRepository(db)
	mail := &recordingEmail{}
	h := NewInternalHTTPHandler(testCfg(), repo, mail)

	body := `{"user_id":100,"user_type":"client","type":"ORDER_CREATED","title":"Order placed","body":"details","link":"/orders/1","send_email":true,"email_to":"u@x.com"}`
	rec := emit(t, h, http.MethodPost, "internal-key", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("emit: want 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	// The in-app row is the source of truth.
	rows, _ := repo.ListByUser(100, "client", false, 0)
	if len(rows) != 1 || rows[0].Title != "Order placed" {
		t.Fatalf("expected one persisted notification, got %+v", rows)
	}

	// Email is fired async — poll briefly for the goroutine to land.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && mail.count() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if mail.count() != 1 {
		t.Errorf("expected one email sent, got %d", mail.count())
	}
}

func TestEmit_NoEmailWhenNotRequested(t *testing.T) {
	db := newTestDB(t, "emit_no_email")
	mail := &recordingEmail{}
	h := NewInternalHTTPHandler(testCfg(), repository.NewNotificationRepository(db), mail)

	body := `{"user_id":5,"user_type":"employee","type":"X","title":"t"}`
	if rec := emit(t, h, http.MethodPost, "internal-key", body); rec.Code != http.StatusCreated {
		t.Fatalf("emit: want 201, got %d", rec.Code)
	}
	time.Sleep(50 * time.Millisecond)
	if mail.count() != 0 {
		t.Errorf("no email expected when send_email is false, got %d", mail.count())
	}
}

func TestEmit_Guards(t *testing.T) {
	db := newTestDB(t, "emit_guards")
	h := NewInternalHTTPHandler(testCfg(), repository.NewNotificationRepository(db), &recordingEmail{})

	// Wrong method -> 404.
	if rec := emit(t, h, http.MethodGet, "internal-key", ""); rec.Code != http.StatusNotFound {
		t.Errorf("wrong method: want 404, got %d", rec.Code)
	}
	// Missing / wrong key -> 401.
	if rec := emit(t, h, http.MethodPost, "", `{}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("no key: want 401, got %d", rec.Code)
	}
	if rec := emit(t, h, http.MethodPost, "wrong", `{}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong key: want 401, got %d", rec.Code)
	}
	// Malformed body -> 400.
	if rec := emit(t, h, http.MethodPost, "internal-key", `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad body: want 400, got %d", rec.Code)
	}
	// Invalid user_type / zero user_id -> 400.
	if rec := emit(t, h, http.MethodPost, "internal-key", `{"user_id":0,"user_type":"client","type":"T","title":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("zero user: want 400, got %d", rec.Code)
	}
	if rec := emit(t, h, http.MethodPost, "internal-key", `{"user_id":1,"user_type":"alien","type":"T","title":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad user_type: want 400, got %d", rec.Code)
	}
	// Missing type/title -> 400.
	if rec := emit(t, h, http.MethodPost, "internal-key", `{"user_id":1,"user_type":"client","type":"","title":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("missing type/title: want 400, got %d", rec.Code)
	}
}

// TestEmit_KeyDisabledRejects verifies that an empty configured key rejects all
// callers (no accidental open endpoint).
func TestEmit_KeyDisabledRejects(t *testing.T) {
	db := newTestDB(t, "emit_key_disabled")
	cfg := testCfg()
	cfg.InternalAPIKey = ""
	h := NewInternalHTTPHandler(cfg, repository.NewNotificationRepository(db), &recordingEmail{})
	if rec := emit(t, h, http.MethodPost, "", `{"user_id":1,"user_type":"client","type":"T","title":"x"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("disabled key: want 401, got %d", rec.Code)
	}
}
