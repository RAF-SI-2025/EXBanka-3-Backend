package service

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"gopkg.in/gomail.v2"
)

type TransferNotificationSender interface {
	SendTransferVerificationCode(toEmail, toName string, transfer *models.Transfer) error
}

type NotificationService struct {
	cfg *config.Config
}

func NewNotificationService(cfg *config.Config) *NotificationService {
	return &NotificationService{cfg: cfg}
}

func (s *NotificationService) SendTransferVerificationCode(toEmail, toName string, transfer *models.Transfer) error {
	if toEmail == "" {
		return fmt.Errorf("missing recipient email")
	}

	expiresAt := transfer.CreatedAt.Add(5 * time.Minute)
	if transfer.VerificationExpiresAt != nil {
		expiresAt = *transfer.VerificationExpiresAt
	}

	body := fmt.Sprintf(`
<!DOCTYPE html><html><body>
<h2>Verifikacija transfera</h2>
<p>Poštovani %s,</p>
<p>Za potvrdu transfera unesite sledeći verifikacioni kod:</p>
<p style="font-size:28px;font-weight:bold;letter-spacing:6px;">%s</p>
<p>Iznos: <strong>%.2f %s</strong></p>
<p>Svrha: <strong>%s</strong></p>
<p>Kod važi do <strong>%s</strong> i može se uneti najviše tri puta.</p>
<p>Srdačan pozdrav,<br/>EXBanka</p>
</body></html>
`, toName, transfer.VerifikacioniKod, transfer.Iznos, transfer.ValutaIznosa, transfer.Svrha, expiresAt.Format("02.01.2006 15:04:05"))

	return s.sendEmail(toEmail, "Verifikacioni kod za transfer", body)
}

func (s *NotificationService) sendEmail(to, subject, htmlBody string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", s.cfg.SMTPFrom)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)

	d := gomail.NewDialer(s.cfg.SMTPHost, s.cfg.SMTPPort, "", "")
	if err := d.DialAndSend(m); err != nil {
		slog.Error("Transfer SMTP failed", "to", to, "subject", subject, "error", err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	slog.Info("Transfer verification email sent", "to", to, "subject", subject)
	return nil
}
