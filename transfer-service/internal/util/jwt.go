package util

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	EmployeeID  uint     `json:"employee_id"`
	ClientID    uint     `json:"client_id"`
	Email       string   `json:"email"`
	Username    string   `json:"username"`
	Permissions []string `json:"permissions"`
	TokenType   string   `json:"token_type"`   // "access" | "refresh"
	TokenSource string   `json:"token_source"` // "employee" | "client"
	jwt.RegisteredClaims
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
