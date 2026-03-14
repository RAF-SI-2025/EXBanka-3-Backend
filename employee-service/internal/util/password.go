package util

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"unicode"

	"golang.org/x/crypto/pbkdf2"
)

const (
	saltLength = 32
	iterations = 100_000
	keyLength  = 32
)

func GenerateSalt() (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	return base64.StdEncoding.EncodeToString(salt), nil
}

func HashPassword(password, saltB64 string) (string, error) {
	saltBytes, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return "", fmt.Errorf("invalid salt encoding: %w", err)
	}

	hash := pbkdf2.Key([]byte(password), saltBytes, iterations, keyLength, sha256.New)
	return base64.StdEncoding.EncodeToString(hash), nil
}

func VerifyPassword(password, saltB64, hashedB64 string) (bool, error) {
	computed, err := HashPassword(password, saltB64)
	if err != nil {
		return false, err
	}

	return computed == hashedB64, nil
}

func ValidatePasswordPolicy(password string) error {
	if len(password) < 8 || len(password) > 32 {
		return errors.New("password must be between 8 and 32 characters")
	}

	var digits, upper, lower int
	for _, c := range password {
		switch {
		case unicode.IsDigit(c):
			digits++
		case unicode.IsUpper(c):
			upper++
		case unicode.IsLower(c):
			lower++
		}
	}

	if digits < 2 {
		return errors.New("password must contain at least 2 digits")
	}
	if upper < 1 {
		return errors.New("password must contain at least 1 uppercase letter")
	}
	if lower < 1 {
		return errors.New("password must contain at least 1 lowercase letter")
	}

	return nil
}
