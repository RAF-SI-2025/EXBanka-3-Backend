package service

import (
	"fmt"
	"log/slog"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
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
<h2>Welcome to the Bank, %s!</h2>
<p>Your account has been created. Please activate it by clicking the link below:</p>
<p><a href="%s" style="background:#007bff;color:#fff;padding:10px 20px;text-decoration:none;border-radius:4px;">Activate Account</a></p>
<p>This link expires in <strong>24 hours</strong>.</p>
<p>If you did not expect this email, please ignore it.</p>
</body></html>
`, toName, link)

	return s.sendEmail(toEmail, "Activate Your Bank Account", body)
}

func (s *NotificationService) SendResetPasswordEmail(toEmail, toName, token string) error {
	link := fmt.Sprintf("%s/reset-password/%s", s.cfg.FrontendURL, token)

	body := fmt.Sprintf(`
<!DOCTYPE html><html><body>
<h2>Password Reset Request</h2>
<p>Hello, %s!</p>
<p>We received a request to reset your password. Click the link below to proceed:</p>
<p><a href="%s" style="background:#dc3545;color:#fff;padding:10px 20px;text-decoration:none;border-radius:4px;">Reset Password</a></p>
<p>This link expires in <strong>1 hour</strong>.</p>
<p>If you did not request a password reset, please ignore this email.</p>
</body></html>
`, toName, link)

	return s.sendEmail(toEmail, "Reset Your Password", body)
}

func (s *NotificationService) SendConfirmationEmail(toEmail, toName string) error {
	body := fmt.Sprintf(`
<!DOCTYPE html><html><body>
<h2>Account Activated!</h2>
<p>Hello, %s!</p>
<p>Your account has been successfully activated. You can now log in to the bank portal.</p>
</body></html>
`, toName)

	return s.sendEmail(toEmail, "Your Account Is Now Active", body)
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
