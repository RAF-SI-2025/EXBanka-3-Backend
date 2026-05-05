package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
)

func makeClient() *models.Client {
	return &models.Client{
		ID:      42,
		Ime:     "Marko",
		Prezime: "Petrović",
		Email:   "marko@example.com",
		Aktivan: true,
		Permissions: []models.Permission{
			{Name: "client.read"},
		},
	}
}

func doRequest(h func(http.ResponseWriter, *http.Request), method, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, "/", nil)
	} else {
		r = httptest.NewRequest(method, "/", bytes.NewBufferString(body))
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

// --- ClientLogin ---

func TestClientLogin_Success(t *testing.T) {
	svc := &mockAuthSvc{access: "a", refresh: "r", client: makeClient()}
	h := handler.NewClientAuthHandlerWithService(svc)

	body := `{"email":"marko@example.com","password":"x"}`
	w := doRequest(h.Login, http.MethodPost, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		Client       struct {
			ID          uint     `json:"id"`
			Email       string   `json:"email"`
			Permissions []string `json:"permissions"`
		} `json:"client"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AccessToken != "a" || resp.RefreshToken != "r" {
		t.Errorf("unexpected tokens: %+v", resp)
	}
	if resp.Client.ID != 42 || resp.Client.Email != "marko@example.com" {
		t.Errorf("unexpected client: %+v", resp.Client)
	}
	if len(resp.Client.Permissions) != 1 || resp.Client.Permissions[0] != "client.read" {
		t.Errorf("unexpected permissions: %+v", resp.Client.Permissions)
	}
}

func TestClientLogin_WrongMethod_Returns405(t *testing.T) {
	svc := &mockAuthSvc{}
	h := handler.NewClientAuthHandlerWithService(svc)

	w := doRequest(h.Login, http.MethodGet, "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestClientLogin_BadJSON_Returns400(t *testing.T) {
	svc := &mockAuthSvc{}
	h := handler.NewClientAuthHandlerWithService(svc)

	w := doRequest(h.Login, http.MethodPost, "{not-json")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestClientLogin_ServiceError_Returns401(t *testing.T) {
	svc := &mockAuthSvc{clientLoginErr: errors.New("invalid credentials")}
	h := handler.NewClientAuthHandlerWithService(svc)

	w := doRequest(h.Login, http.MethodPost, `{"email":"x","password":"y"}`)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid credentials") {
		t.Errorf("expected error message in body, got %s", w.Body.String())
	}
}

// --- ClientActivate ---

func TestClientActivate_Success(t *testing.T) {
	svc := &mockAuthSvc{}
	h := handler.NewClientAuthHandlerWithService(svc)

	body := `{"token":"t","password":"p","passwordConfirm":"p"}`
	w := doRequest(h.Activate, http.MethodPost, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.NewDecoder(io.NopCloser(bytes.NewReader(w.Body.Bytes()))).Decode(&resp)
	if resp["message"] == "" {
		t.Error("expected message in response")
	}
}

func TestClientActivate_WrongMethod_Returns405(t *testing.T) {
	svc := &mockAuthSvc{}
	h := handler.NewClientAuthHandlerWithService(svc)

	w := doRequest(h.Activate, http.MethodGet, "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestClientActivate_BadJSON_Returns400(t *testing.T) {
	svc := &mockAuthSvc{}
	h := handler.NewClientAuthHandlerWithService(svc)

	w := doRequest(h.Activate, http.MethodPost, "not-json")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestClientActivate_ServiceError_Returns400(t *testing.T) {
	svc := &mockAuthSvc{clientActErr: errors.New("invalid activation token")}
	h := handler.NewClientAuthHandlerWithService(svc)

	body := `{"token":"bad","password":"p","passwordConfirm":"p"}`
	w := doRequest(h.Activate, http.MethodPost, body)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid activation token") {
		t.Errorf("expected error message in body, got %s", w.Body.String())
	}
}
