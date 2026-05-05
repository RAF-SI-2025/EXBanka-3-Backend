package util

import (
	"strings"
	"testing"
)

func TestGenerateSalt_NotEmpty(t *testing.T) {
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt: %v", err)
	}
	if salt == "" {
		t.Error("expected non-empty salt")
	}
}

func TestGenerateSalt_Unique(t *testing.T) {
	a, _ := GenerateSalt()
	b, _ := GenerateSalt()
	if a == b {
		t.Error("expected unique salts")
	}
}

func TestHashPassword_Deterministic(t *testing.T) {
	salt, _ := GenerateSalt()
	h1, err := HashPassword("Password12", salt)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	h2, _ := HashPassword("Password12", salt)
	if h1 != h2 {
		t.Error("same password+salt should produce same hash")
	}
}

func TestHashPassword_DifferentSaltDifferentHash(t *testing.T) {
	saltA, _ := GenerateSalt()
	saltB, _ := GenerateSalt()
	hA, _ := HashPassword("Password12", saltA)
	hB, _ := HashPassword("Password12", saltB)
	if hA == hB {
		t.Error("different salts should yield different hashes")
	}
}

func TestHashPassword_InvalidSalt(t *testing.T) {
	_, err := HashPassword("Password12", "not-base64!!!")
	if err == nil {
		t.Error("expected error for invalid salt encoding")
	}
}

func TestVerifyPassword_True(t *testing.T) {
	salt, _ := GenerateSalt()
	hash, _ := HashPassword("Password12", salt)
	ok, err := VerifyPassword("Password12", salt, hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Error("expected verification true")
	}
}

func TestVerifyPassword_False(t *testing.T) {
	salt, _ := GenerateSalt()
	hash, _ := HashPassword("Password12", salt)
	ok, _ := VerifyPassword("WrongPass99", salt, hash)
	if ok {
		t.Error("expected verification false")
	}
}

func TestVerifyPassword_InvalidSalt(t *testing.T) {
	_, err := VerifyPassword("Password12", "bad!!", "hash")
	if err == nil {
		t.Error("expected error for invalid salt")
	}
}

func TestValidatePasswordPolicy_TooShort(t *testing.T) {
	if err := ValidatePasswordPolicy("Aa1"); err == nil {
		t.Error("expected error for password < 8 chars")
	}
}

func TestValidatePasswordPolicy_TooLong(t *testing.T) {
	long := strings.Repeat("Aa1", 12) // 36 chars
	if err := ValidatePasswordPolicy(long); err == nil {
		t.Error("expected error for password > 32 chars")
	}
}

func TestValidatePasswordPolicy_NoDigits(t *testing.T) {
	if err := ValidatePasswordPolicy("Password"); err == nil {
		t.Error("expected error for password without digits")
	}
}

func TestValidatePasswordPolicy_OneDigit(t *testing.T) {
	if err := ValidatePasswordPolicy("Password1"); err == nil {
		t.Error("expected error for password with only 1 digit")
	}
}

func TestValidatePasswordPolicy_NoUppercase(t *testing.T) {
	if err := ValidatePasswordPolicy("password12"); err == nil {
		t.Error("expected error for password without uppercase")
	}
}

func TestValidatePasswordPolicy_NoLowercase(t *testing.T) {
	if err := ValidatePasswordPolicy("PASSWORD12"); err == nil {
		t.Error("expected error for password without lowercase")
	}
}

func TestValidatePasswordPolicy_Valid(t *testing.T) {
	if err := ValidatePasswordPolicy("Password12"); err != nil {
		t.Errorf("expected nil for valid password, got %v", err)
	}
}
