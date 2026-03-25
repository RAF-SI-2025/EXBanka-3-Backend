//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
)

var baseURL string

func init() {
	baseURL = os.Getenv("TEST_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost/api/v1"
	}
}

func postJSON(t *testing.T, path string, body interface{}) (*http.Response, map[string]interface{}) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	resp, err := http.Post(baseURL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s error: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp, result
}

func getWithToken(t *testing.T, path, token string) (*http.Response, map[string]interface{}) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest error: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s error: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp, result
}

func TestAuthFlow_LoginSuccess(t *testing.T) {
	resp, body := postJSON(t, "/auth/login", map[string]string{
		"email":    "admin@bank.com",
		"password": "Admin123!",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login expected 200, got %d: %v", resp.StatusCode, body)
	}
	if body["accessToken"] == nil {
		t.Error("login response missing accessToken")
	}
	if body["refreshToken"] == nil {
		t.Error("login response missing refreshToken")
	}
}

func TestAuthFlow_LoginWrongPassword(t *testing.T) {
	resp, _ := postJSON(t, "/auth/login", map[string]string{
		"email":    "admin@bank.com",
		"password": "WrongPassword99",
	})
	if resp.StatusCode == http.StatusOK {
		t.Error("login with wrong password expected non-200, got 200")
	}
}

func TestAuthFlow_LoginNonexistentEmail(t *testing.T) {
	resp, _ := postJSON(t, "/auth/login", map[string]string{
		"email":    "noone@bank.com",
		"password": "SomePass12",
	})
	if resp.StatusCode == http.StatusOK {
		t.Error("login with nonexistent email expected non-200, got 200")
	}
}

func TestAuthFlow_RefreshToken(t *testing.T) {
	_, loginBody := postJSON(t, "/auth/login", map[string]string{
		"email":    "admin@bank.com",
		"password": "Admin123!",
	})
	refreshToken, ok := loginBody["refreshToken"].(string)
	if !ok || refreshToken == "" {
		t.Skip("could not obtain refresh token; skipping refresh test")
	}

	resp, body := postJSON(t, "/auth/refresh", map[string]string{
		"refreshToken": refreshToken,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh expected 200, got %d: %v", resp.StatusCode, body)
	}
	if body["accessToken"] == nil {
		t.Error("refresh response missing accessToken")
	}
}

func TestAuthFlow_ActivationTokenValidation(t *testing.T) {
	// Attempt activation with an obviously invalid token
	resp, _ := postJSON(t, "/auth/activate", map[string]string{
		"token":           "invalid-token-value",
		"password":        "NewPass12",
		"passwordConfirm": "NewPass12",
	})
	if resp.StatusCode == http.StatusOK {
		t.Error("activation with invalid token expected non-200, got 200")
	}
}

func TestAuthFlow_ActivationPasswordMismatch(t *testing.T) {
	resp, _ := postJSON(t, "/auth/activate", map[string]string{
		"token":           "some-token",
		"password":        "NewPass12",
		"passwordConfirm": "DifferentPass12",
	})
	if resp.StatusCode == http.StatusOK {
		t.Error("activation with mismatched passwords expected non-200, got 200")
	}
}

func TestAuthFlow_ActivationPasswordPolicy(t *testing.T) {
	resp, _ := postJSON(t, "/auth/activate", map[string]string{
		"token":           "some-token",
		"password":        "weak",
		"passwordConfirm": "weak",
	})
	if resp.StatusCode == http.StatusOK {
		t.Error("activation with weak password expected non-200, got 200")
	}
}

func TestAuthFlow_RequestPasswordReset_DoesNotRevealEmail(t *testing.T) {
	// Both existing and non-existing emails should return 200 (no info leakage)
	resp1, _ := postJSON(t, "/auth/password-reset/request", map[string]string{
		"email": "admin@bank.com",
	})
	resp2, _ := postJSON(t, "/auth/password-reset/request", map[string]string{
		"email": fmt.Sprintf("nonexistent-%d@bank.com", 99999),
	})
	if resp1.StatusCode != resp2.StatusCode {
		t.Errorf("password reset leaks email existence: existing=%d, nonexistent=%d",
			resp1.StatusCode, resp2.StatusCode)
	}
}
