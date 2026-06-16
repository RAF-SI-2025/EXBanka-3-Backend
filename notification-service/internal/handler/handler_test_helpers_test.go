package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/util"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

const testJWTSecret = "notif-test-secret"

func newTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Notification{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testCfg() *config.Config {
	return &config.Config{JWTSecret: testJWTSecret, InternalAPIKey: "internal-key"}
}

func makeToken(t *testing.T, claims util.Claims) string {
	t.Helper()
	if claims.TokenType == "" {
		claims.TokenType = "access"
	}
	claims.RegisteredClaims = jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	signed, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

// clientToken is client 100; client2Token is client 200 (for ownership tests);
// employeeToken is employee 5.
func clientToken(t *testing.T) string {
	return makeToken(t, util.Claims{ClientID: 100, TokenSource: "client", TokenType: "access"})
}
func client2Token(t *testing.T) string {
	return makeToken(t, util.Claims{ClientID: 200, TokenSource: "client", TokenType: "access"})
}
func employeeToken(t *testing.T) string {
	return makeToken(t, util.Claims{EmployeeID: 5, TokenSource: "employee", TokenType: "access"})
}

// do fires an HTTP request at a handler func and returns the recorder.
func do(t *testing.T, h http.HandlerFunc, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Buffer
	if body != "" {
		rdr = bytes.NewBufferString(body)
	} else {
		rdr = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func bytesReader(body string) *bytes.Buffer {
	if body == "" {
		return bytes.NewBuffer(nil)
	}
	return bytes.NewBufferString(body)
}

func seedNotif(t *testing.T, db *gorm.DB, userID uint, userType string) *models.Notification {
	t.Helper()
	n := &models.Notification{UserID: userID, UserType: userType, Type: "ORDER_CREATED", Title: "hi", Body: "b"}
	if err := db.Create(n).Error; err != nil {
		t.Fatalf("seed notif: %v", err)
	}
	return n
}
