package util

import (
	"testing"
)

const testSecret = "test-secret-key"

func TestGenerateAccessToken(t *testing.T) {
	tok, err := GenerateAccessToken(1, "user@bank.com", "user01", []string{"perm1"}, testSecret, 15)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if tok == "" {
		t.Error("expected non-empty token")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	tok, err := GenerateRefreshToken(1, "user@bank.com", "user01", testSecret, 24)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	if tok == "" {
		t.Error("expected non-empty token")
	}
}

func TestGenerateClientAccessToken(t *testing.T) {
	tok, err := GenerateClientAccessToken(42, "client@gmail.com", []string{"clientBasic"}, testSecret, 15)
	if err != nil {
		t.Fatalf("GenerateClientAccessToken: %v", err)
	}
	if tok == "" {
		t.Error("expected non-empty token")
	}
}

func TestGenerateClientRefreshToken(t *testing.T) {
	tok, err := GenerateClientRefreshToken(42, "client@gmail.com", testSecret, 24)
	if err != nil {
		t.Fatalf("GenerateClientRefreshToken: %v", err)
	}
	if tok == "" {
		t.Error("expected non-empty token")
	}
}

func TestGenerateClientSetupToken(t *testing.T) {
	tok, err := GenerateClientSetupToken(42, "client@gmail.com", testSecret, 24)
	if err != nil {
		t.Fatalf("GenerateClientSetupToken: %v", err)
	}
	if tok == "" {
		t.Error("expected non-empty token")
	}
}

func TestParseToken_AccessRoundTrip(t *testing.T) {
	tok, _ := GenerateAccessToken(1, "user@bank.com", "user01", []string{"a", "b"}, testSecret, 15)

	claims, err := ParseToken(tok, testSecret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.EmployeeID != 1 {
		t.Errorf("EmployeeID = %d, want 1", claims.EmployeeID)
	}
	if claims.Email != "user@bank.com" {
		t.Errorf("Email = %s, want user@bank.com", claims.Email)
	}
	if claims.TokenType != "access" {
		t.Errorf("TokenType = %s, want access", claims.TokenType)
	}
	if claims.ID == "" {
		t.Error("expected access token jti to be set")
	}
	if claims.TokenSource != "employee" {
		t.Errorf("TokenSource = %s, want employee", claims.TokenSource)
	}
	if len(claims.Permissions) != 2 {
		t.Errorf("Permissions len = %d, want 2", len(claims.Permissions))
	}
}

func TestParseToken_RefreshRoundTrip(t *testing.T) {
	tok, _ := GenerateRefreshToken(2, "user@bank.com", "user02", testSecret, 24)

	claims, err := ParseToken(tok, testSecret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.TokenType != "refresh" {
		t.Errorf("TokenType = %s, want refresh", claims.TokenType)
	}
	if claims.ID == "" {
		t.Error("expected refresh token jti to be set")
	}
}

func TestParseToken_ClientRoundTrip(t *testing.T) {
	tok, _ := GenerateClientAccessToken(99, "c@gmail.com", []string{"clientBasic"}, testSecret, 15)

	claims, err := ParseToken(tok, testSecret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.ClientID != 99 {
		t.Errorf("ClientID = %d, want 99", claims.ClientID)
	}
	if claims.TokenSource != "client" {
		t.Errorf("TokenSource = %s, want client", claims.TokenSource)
	}
	if claims.ID == "" {
		t.Error("expected client access token jti to be set")
	}
}

func TestParseToken_SetupRoundTrip(t *testing.T) {
	tok, _ := GenerateClientSetupToken(101, "c@gmail.com", testSecret, 24)

	claims, err := ParseToken(tok, testSecret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.TokenType != "setup" {
		t.Errorf("TokenType = %s, want setup", claims.TokenType)
	}
	if claims.TokenSource != "client_setup" {
		t.Errorf("TokenSource = %s, want client_setup", claims.TokenSource)
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	tok, _ := GenerateAccessToken(1, "user@bank.com", "user01", []string{}, testSecret, 15)
	if _, err := ParseToken(tok, "different-secret"); err == nil {
		t.Error("expected error for wrong secret")
	}
}

func TestParseToken_Garbage(t *testing.T) {
	if _, err := ParseToken("not-a-jwt", testSecret); err == nil {
		t.Error("expected error for garbage input")
	}
}

func TestParseToken_Expired(t *testing.T) {
	// Negative duration → already expired
	tok, _ := GenerateAccessToken(1, "u@b.c", "u", []string{}, testSecret, -1)
	if _, err := ParseToken(tok, testSecret); err == nil {
		t.Error("expected error for expired token")
	}
}
