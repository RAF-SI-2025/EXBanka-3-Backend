package util

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	EmployeeID  uint     `json:"employee_id"`
	ClientID    uint     `json:"client_id"` // 0 for employee tokens
	Email       string   `json:"email"`
	Username    string   `json:"username"` // empty for client tokens
	Permissions []string `json:"permissions"`
	TokenType   string   `json:"token_type"`   // "access" | "refresh" | "setup"
	TokenSource string   `json:"token_source"` // "employee" | "client" | "client_setup"
	jwt.RegisteredClaims
}

func GenerateAccessToken(employeeID uint, email, username string, permissions []string, secret string, durationMinutes int) (string, error) {
	claims := Claims{
		EmployeeID:  employeeID,
		Email:       email,
		Username:    username,
		Permissions: permissions,
		TokenType:   "access",
		TokenSource: "employee",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(durationMinutes) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func GenerateRefreshToken(employeeID uint, email, username string, secret string, durationHours int) (string, error) {
	claims := Claims{
		EmployeeID:  employeeID,
		Email:       email,
		Username:    username,
		TokenType:   "refresh",
		TokenSource: "employee",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(durationHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func GenerateClientAccessToken(clientID uint, email string, permissions []string, secret string, durationMinutes int) (string, error) {
	claims := Claims{
		ClientID:    clientID,
		Email:       email,
		Permissions: permissions,
		TokenType:   "access",
		TokenSource: "client",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(durationMinutes) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func GenerateClientRefreshToken(clientID uint, email string, secret string, durationHours int) (string, error) {
	claims := Claims{
		ClientID:    clientID,
		Email:       email,
		TokenType:   "refresh",
		TokenSource: "client",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(durationHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func GenerateClientSetupToken(clientID uint, email string, secret string, durationHours int) (string, error) {
	claims := Claims{
		ClientID:    clientID,
		Email:       email,
		TokenType:   "setup",
		TokenSource: "client_setup",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(durationHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func ParseToken(tokenString, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
