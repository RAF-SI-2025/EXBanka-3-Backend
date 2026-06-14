package service

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// EmailSender is the minimal interface used by the emit endpoint and by tests
// (via a mock implementation). Mirrors the exchange-service contract.
type EmailSender interface {
	Send(to, subject, body string) error
}

// SMTPEmailService sends plain-text emails via a Mailhog-compatible SMTP server
// (no authentication required), matching the pattern used across the platform.
type SMTPEmailService struct {
	host string
	port int
	from string
}

func NewSMTPEmailService(host string, port int, from string) *SMTPEmailService {
	return &SMTPEmailService{host: host, port: port, from: from}
}

func (s *SMTPEmailService) Send(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	var buf strings.Builder
	buf.WriteString("From: " + s.from + "\r\n")
	buf.WriteString("To: " + to + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(body)

	if err := smtp.SendMail(addr, nil, s.from, []string{to}, []byte(buf.String())); err != nil {
		slog.Error("SMTP send failed", "to", to, "subject", subject, "error", err)
		return fmt.Errorf("smtp send: %w", err)
	}
	slog.Info("Email sent", "to", to, "subject", subject)
	return nil
}
