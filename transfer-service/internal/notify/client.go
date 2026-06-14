// Package notify is a thin, best-effort client for notification-service. Every
// call is fire-and-forget: a notification must never block or fail the business
// action that raised it (settling a transfer, ...).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Event is the payload posted to notification-service's internal emit endpoint.
type Event struct {
	UserID    uint   `json:"user_id"`
	UserType  string `json:"user_type"` // "client" | "employee"
	Type      string `json:"type"`      // machine code, e.g. "TRANSFER_EXECUTED"
	Title     string `json:"title"`
	Body      string `json:"body"`
	Link      string `json:"link"`
	SendEmail bool   `json:"send_email"`
	EmailTo   string `json:"email_to"`
}

// Client posts events to notification-service. Construct once and share.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Emit sends the event asynchronously and returns immediately. A nil client,
// empty base URL, or transport error is logged and swallowed.
func (c *Client) Emit(ev Event) {
	if c == nil || c.baseURL == "" {
		return
	}
	go func() {
		body, err := json.Marshal(ev)
		if err != nil {
			slog.Error("notify: marshal failed", "type", ev.Type, "error", err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/v1/notifications", bytes.NewReader(body))
		if err != nil {
			slog.Error("notify: build request failed", "type", ev.Type, "error", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Key", c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			slog.Error("notify: emit failed", "type", ev.Type, "error", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			slog.Error("notify: emit rejected", "type", ev.Type, "status", resp.StatusCode)
			return
		}
		slog.Info("notify: emitted", "type", ev.Type, "user_id", ev.UserID, "user_type", ev.UserType)
	}()
}
