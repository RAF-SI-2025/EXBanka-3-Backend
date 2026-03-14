package util

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	EmployeeID  uint     `json:"employee_id"`
	Email       string   `json:"email"`
	Username    string   `json:"username"`
	Permissions []string `json:"permissions"`
	TokenType   string   `json:"token_type"`
	jwt.RegisteredClaims
}

func GenerateAccessToken(employeeID uint, email, username string, permissions []string, secret string, durationMinutes int) (string, error) {
	claims := Claims{
		EmployeeID:  employeeID,
		Email:       email,
		Username:    username,
		Permissions: permissions,
		TokenType:   "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(durationMinutes) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func GenerateRefreshToken(employeeID uint, email, username string, secret string, durationHours int) (string, error) {
	claims := Claims{
		EmployeeID: employeeID,
		Email:      email,
		Username:   username,
		TokenType:  "refresh",
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

func HasPermission(claims *Claims, perm string) bool {
	for _, p := range claims.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}
