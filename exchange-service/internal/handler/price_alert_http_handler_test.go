package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func alertTestCfg() *config.Config {
	return &config.Config{JWTSecret: testJWTSecret}
}

// clientTokenWithEmail returns a client JWT that includes an email address
// (required for the CreateAlert endpoint).
func clientTokenWithEmail(t *testing.T) string {
	t.Helper()
	return makeToken(t, util.Claims{
		ClientID:    100,
		TokenSource: "client",
		TokenType:   "access",
		Email:       "client@test.com",
		Permissions: []string{models.PermClientTrading, models.PermClientBasic},
	})
}

// client2TokenWithEmail is a second client (ID=200) used to test 403 cross-user access.
func client2TokenWithEmail(t *testing.T) string {
	t.Helper()
	return makeToken(t, util.Claims{
		ClientID:    200,
		TokenSource: "client",
		TokenType:   "access",
		Email:       "client2@test.com",
		Permissions: []string{models.PermClientTrading, models.PermClientBasic},
	})
}

func setupAlertHandler(t *testing.T, db *gorm.DB) *PriceAlertHTTPHandler {
	t.Helper()
	repo := repository.NewPriceAlertRepository(db)
	return NewPriceAlertHTTPHandler(alertTestCfg(), repo)
}

// doAlertRequest fires an HTTP request against the handler.
func doAlertRequest(t *testing.T, h *PriceAlertHTTPHandler, method, path, token string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	if path == "/api/v1/price-alerts" {
		h.Collection(rr, req)
	} else {
		h.Routes(rr, req)
	}
	return rr
}

// ---------------------------------------------------------------------------
// Mock email sender — records calls, never dials SMTP.
// ---------------------------------------------------------------------------

type mockEmail struct {
	calls []struct{ to, subject string }
	err   error
}

func (m *mockEmail) Send(to, subject, body string) error {
	m.calls = append(m.calls, struct{ to, subject string }{to, subject})
	return m.err
}

// ---------------------------------------------------------------------------
// HTTP handler tests
// ---------------------------------------------------------------------------

func TestPriceAlert_Unauthenticated(t *testing.T) {
	db := newTestDB(t, "pa_unauth")
	h := setupAlertHandler(t, db)

	rr := doAlertRequest(t, h, http.MethodGet, "/api/v1/price-alerts", "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestPriceAlert_CreateAndList(t *testing.T) {
	db := newTestDB(t, "pa_create_list")
	h := setupAlertHandler(t, db)
	tok := clientTokenWithEmail(t)

	// Create an ABOVE alert
	payload := map[string]interface{}{
		"ticker":    "AAPL",
		"condition": "ABOVE",
		"threshold": 200.0,
	}
	rr := doAlertRequest(t, h, http.MethodPost, "/api/v1/price-alerts", tok, payload)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d — body: %s", rr.Code, rr.Body.String())
	}
	var created models.PriceAlert
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Ticker != "AAPL" || created.Condition != "ABOVE" || created.Threshold != 200.0 {
		t.Errorf("wrong alert fields: %+v", created)
	}
	if !created.IsActive {
		t.Error("new alert should be active")
	}
	if created.NotificationEmail != "client@test.com" {
		t.Errorf("email: got %q, want client@test.com", created.NotificationEmail)
	}
	if created.UserID != 100 || created.UserType != "client" {
		t.Errorf("owner: got (%d,%s)", created.UserID, created.UserType)
	}

	// List — must contain the created alert
	rr = doAlertRequest(t, h, http.MethodGet, "/api/v1/price-alerts", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", rr.Code)
	}
	var alerts []models.PriceAlert
	_ = json.NewDecoder(rr.Body).Decode(&alerts)
	if len(alerts) != 1 || alerts[0].ID != created.ID {
		t.Fatalf("list: want 1 alert with id=%d, got %v", created.ID, alerts)
	}
}

func TestPriceAlert_Create_InvalidCondition(t *testing.T) {
	db := newTestDB(t, "pa_bad_cond")
	h := setupAlertHandler(t, db)
	tok := clientTokenWithEmail(t)

	rr := doAlertRequest(t, h, http.MethodPost, "/api/v1/price-alerts", tok,
		map[string]interface{}{"ticker": "AAPL", "condition": "SIDEWAYS", "threshold": 100.0})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad condition, got %d", rr.Code)
	}
}

func TestPriceAlert_DeleteOwnAlert(t *testing.T) {
	db := newTestDB(t, "pa_delete_own")
	h := setupAlertHandler(t, db)
	tok := clientTokenWithEmail(t)

	rr := doAlertRequest(t, h, http.MethodPost, "/api/v1/price-alerts", tok,
		map[string]interface{}{"ticker": "TSLA", "condition": "BELOW", "threshold": 50.0})
	var alert models.PriceAlert
	_ = json.NewDecoder(rr.Body).Decode(&alert)

	path := "/api/v1/price-alerts/" + uintToStr(alert.ID)
	rr = doAlertRequest(t, h, http.MethodDelete, path, tok, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete own: want 204, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// Deleted alert should no longer appear in list
	rr = doAlertRequest(t, h, http.MethodGet, "/api/v1/price-alerts", tok, nil)
	var remaining []models.PriceAlert
	_ = json.NewDecoder(rr.Body).Decode(&remaining)
	if len(remaining) != 0 {
		t.Errorf("after delete: want 0 alerts, got %d", len(remaining))
	}
}

// TestPriceAlert_DeleteForbiddenForOtherUser verifies scenario ownership: a
// different user cannot delete someone else's alert (must get 403).
func TestPriceAlert_DeleteForbiddenForOtherUser(t *testing.T) {
	db := newTestDB(t, "pa_forbidden")
	h := setupAlertHandler(t, db)

	// client1 creates an alert
	tok1 := clientTokenWithEmail(t) // ClientID=100
	rr := doAlertRequest(t, h, http.MethodPost, "/api/v1/price-alerts", tok1,
		map[string]interface{}{"ticker": "AAPL", "condition": "ABOVE", "threshold": 300.0})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d", rr.Code)
	}
	var alert models.PriceAlert
	_ = json.NewDecoder(rr.Body).Decode(&alert)

	// client2 tries to delete client1's alert → 403
	tok2 := client2TokenWithEmail(t) // ClientID=200
	path := "/api/v1/price-alerts/" + uintToStr(alert.ID)
	rr = doAlertRequest(t, h, http.MethodDelete, path, tok2, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 for cross-user delete, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// Alert must still be active
	var count int64
	db.Model(&models.PriceAlert{}).Where("id = ? AND is_active = true", alert.ID).Count(&count)
	if count != 1 {
		t.Errorf("alert should still be active after forbidden delete, count=%d", count)
	}
}

func TestPriceAlert_DeleteNotFound(t *testing.T) {
	db := newTestDB(t, "pa_del_notfound")
	h := setupAlertHandler(t, db)
	tok := clientTokenWithEmail(t)

	rr := doAlertRequest(t, h, http.MethodDelete, "/api/v1/price-alerts/9999", tok, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 for non-existent alert, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Cron / CheckPriceAlerts tests
// ---------------------------------------------------------------------------

// seedAlert inserts a PriceAlert directly into the DB and returns it.
func seedAlert(t *testing.T, db *gorm.DB, ticker, condition string, threshold float64, email string) models.PriceAlert {
	t.Helper()
	alert := models.PriceAlert{
		UserID:            100,
		UserType:          "client",
		Ticker:            ticker,
		Condition:         condition,
		Threshold:         threshold,
		NotificationEmail: email,
		IsActive:          true,
	}
	if err := db.Create(&alert).Error; err != nil {
		t.Fatalf("seedAlert: %v", err)
	}
	return alert
}

// TestCheckPriceAlerts_ABOVE_Fires verifies that an ABOVE alert fires when
// newPrice >= threshold, sends email, and is deactivated afterwards.
func TestCheckPriceAlerts_ABOVE_Fires(t *testing.T) {
	db := newTestDB(t, "pa_above_fires")
	alert := seedAlert(t, db, "AAPL", "ABOVE", 150.0, "user@test.com")

	mock := &mockEmail{}
	service.CheckPriceAlerts(db, mock, "AAPL", 155.0) // price 155 >= threshold 150 → fires

	if len(mock.calls) != 1 {
		t.Fatalf("email: want 1 call, got %d", len(mock.calls))
	}
	if mock.calls[0].to != "user@test.com" {
		t.Errorf("email to: got %q", mock.calls[0].to)
	}

	// Alert must be deactivated
	var refreshed models.PriceAlert
	db.First(&refreshed, alert.ID)
	if refreshed.IsActive {
		t.Error("alert should be deactivated after firing")
	}
}

// TestCheckPriceAlerts_BELOW_Fires verifies that a BELOW alert fires when
// newPrice <= threshold, sends email, and is deactivated.
func TestCheckPriceAlerts_BELOW_Fires(t *testing.T) {
	db := newTestDB(t, "pa_below_fires")
	alert := seedAlert(t, db, "TSLA", "BELOW", 100.0, "user@test.com")

	mock := &mockEmail{}
	service.CheckPriceAlerts(db, mock, "TSLA", 90.0) // price 90 <= threshold 100 → fires

	if len(mock.calls) != 1 {
		t.Fatalf("email: want 1 call, got %d", len(mock.calls))
	}

	var refreshed models.PriceAlert
	db.First(&refreshed, alert.ID)
	if refreshed.IsActive {
		t.Error("alert should be deactivated after firing")
	}
}

// TestCheckPriceAlerts_ABOVE_ExactThreshold verifies the >= boundary:
// firing at exactly the threshold price (scenario 27: ABOVE fires at >=).
func TestCheckPriceAlerts_ABOVE_ExactThreshold(t *testing.T) {
	db := newTestDB(t, "pa_above_exact")
	alert := seedAlert(t, db, "MSFT", "ABOVE", 200.0, "user@test.com")

	mock := &mockEmail{}
	service.CheckPriceAlerts(db, mock, "MSFT", 200.0) // exactly at threshold → fires

	if len(mock.calls) != 1 {
		t.Fatalf("exact threshold: want 1 email, got %d", len(mock.calls))
	}

	var refreshed models.PriceAlert
	db.First(&refreshed, alert.ID)
	if refreshed.IsActive {
		t.Error("alert should be deactivated")
	}
}

// TestCheckPriceAlerts_DoesNotFire verifies that when the condition is NOT
// met (price below ABOVE threshold) the alert stays active and no email is sent.
func TestCheckPriceAlerts_DoesNotFire(t *testing.T) {
	db := newTestDB(t, "pa_no_fire")
	alert := seedAlert(t, db, "AAPL", "ABOVE", 200.0, "user@test.com")

	mock := &mockEmail{}
	service.CheckPriceAlerts(db, mock, "AAPL", 150.0) // price 150 < threshold 200 → no fire

	if len(mock.calls) != 0 {
		t.Fatalf("no-fire: want 0 emails, got %d", len(mock.calls))
	}

	// Alert must remain active
	var refreshed models.PriceAlert
	db.First(&refreshed, alert.ID)
	if !refreshed.IsActive {
		t.Error("alert should still be active when condition not met")
	}
}

// TestCheckPriceAlerts_WrongTicker verifies that alerts for a different ticker
// are not affected when another ticker's price changes.
func TestCheckPriceAlerts_WrongTicker(t *testing.T) {
	db := newTestDB(t, "pa_wrong_ticker")
	alert := seedAlert(t, db, "AAPL", "ABOVE", 100.0, "user@test.com")

	mock := &mockEmail{}
	service.CheckPriceAlerts(db, mock, "TSLA", 999.0) // different ticker

	if len(mock.calls) != 0 {
		t.Fatalf("wrong ticker: want 0 emails, got %d", len(mock.calls))
	}

	var refreshed models.PriceAlert
	db.First(&refreshed, alert.ID)
	if !refreshed.IsActive {
		t.Error("AAPL alert should still be active after TSLA update")
	}
}

// TestCheckPriceAlerts_AlreadyInactive verifies that a deactivated alert is
// not re-triggered.
func TestCheckPriceAlerts_AlreadyInactive(t *testing.T) {
	db := newTestDB(t, "pa_already_inactive")
	alert := seedAlert(t, db, "AAPL", "ABOVE", 100.0, "user@test.com")
	db.Model(&models.PriceAlert{}).Where("id = ?", alert.ID).Update("is_active", false)

	mock := &mockEmail{}
	service.CheckPriceAlerts(db, mock, "AAPL", 999.0) // would fire if active

	if len(mock.calls) != 0 {
		t.Fatalf("inactive alert: want 0 emails, got %d", len(mock.calls))
	}
}
