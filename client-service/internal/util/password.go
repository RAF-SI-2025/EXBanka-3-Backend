package util

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

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

func GenerateRandomSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate random secret: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf), nil
}
