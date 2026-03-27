package service

import (
	"crypto/tls"
	"fmt"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/config"
	gomail "gopkg.in/gomail.v2"
)

type NotificationService struct {
	cfg *config.Config
}

func NewNotificationService(cfg *config.Config) *NotificationService {
	return &NotificationService{cfg: cfg}
}

func (s *NotificationService) SendLoanApprovedEmail(toEmail, toName string, iznos float64, vrsta string, period int, iznosRate float64, kamatnaStopa float64, brojKredita string) error {
	subject := "Vaš kredit je odobren — EXBanka"
	body := fmt.Sprintf(`<html><body style="font-family:sans-serif;color:#1e293b">
<h2 style="color:#166534">Kredit je odobren!</h2>
<p>Poštovani/a <strong>%s</strong>,</p>
<p>Obaveštavamo Vas da je Vaš zahtev za kredit uspešno odobren. U nastavku su detalji:</p>
<table style="border-collapse:collapse;margin:16px 0">
  <tr><td style="padding:6px 16px 6px 0;color:#64748b">Broj kredita</td><td style="padding:6px 0"><strong>%s</strong></td></tr>
  <tr><td style="padding:6px 16px 6px 0;color:#64748b">Vrsta kredita</td><td style="padding:6px 0">%s</td></tr>
  <tr><td style="padding:6px 16px 6px 0;color:#64748b">Iznos</td><td style="padding:6px 0"><strong>%.2f RSD</strong></td></tr>
  <tr><td style="padding:6px 16px 6px 0;color:#64748b">Period otplate</td><td style="padding:6px 0">%d meseci</td></tr>
  <tr><td style="padding:6px 16px 6px 0;color:#64748b">Mesečna rata</td><td style="padding:6px 0"><strong>%.2f RSD</strong></td></tr>
  <tr><td style="padding:6px 16px 6px 0;color:#64748b">Kamatna stopa</td><td style="padding:6px 0">%.2f%% godišnje</td></tr>
</table>
<p>Sredstva su preneta na Vaš račun. Prva rata dospeva narednog meseca.</p>
<p>Srdačan pozdrav,<br><strong>EXBanka tim</strong></p>
</body></html>`,
		toName, brojKredita, vrsta, iznos, period, iznosRate, kamatnaStopa)

	return s.sendEmail(toEmail, subject, body)
}

func (s *NotificationService) sendEmail(to, subject, body string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", s.cfg.SMTPFrom)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	d := gomail.NewDialer(s.cfg.SMTPHost, s.cfg.SMTPPort, "", "")
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	return d.DialAndSend(m)
}
