package service

import (
	"fmt"
	"log/slog"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/config"
	"gopkg.in/gomail.v2"
)

type NotificationService struct {
	cfg *config.Config
}

func NewNotificationService(cfg *config.Config) *NotificationService {
	return &NotificationService{cfg: cfg}
}

func (s *NotificationService) SendActivationEmail(toEmail, toName, token string) error {
	link := fmt.Sprintf("%s/activate/%s", s.cfg.FrontendURL, token)

	body := fmt.Sprintf(`
<!DOCTYPE html><html><body>
<h2>Welcome to EXBanka, %s!</h2>
<p>Your account has been created. Please set your password by clicking the link below:</p>
<p><a href="%s" style="background:#007bff;color:#fff;padding:10px 20px;text-decoration:none;border-radius:4px;">Set Password</a></p>
<p>This link expires in <strong>24 hours</strong>.</p>
<p>If you did not expect this email, please ignore it.</p>
</body></html>
`, toName, link)

	return s.sendEmail(toEmail, "Activate Your EXBanka Account", body)
}

func (s *NotificationService) sendEmail(to, subject, htmlBody string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", s.cfg.SMTPFrom)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)

	d := gomail.NewDialer(s.cfg.SMTPHost, s.cfg.SMTPPort, s.cfg.SMTPUser, s.cfg.SMTPPassword)

	if err := d.DialAndSend(m); err != nil {
		slog.Error("SMTP failed to send email", "subject", subject, "to", to, "error", err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	slog.Info("SMTP email sent", "subject", subject, "to", to)
	return nil
}
