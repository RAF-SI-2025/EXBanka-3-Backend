package service

import (
	"fmt"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/notify"
)

// NewAppNotifier builds the in-app notification client from config. Returns a
// client whose Emit is a no-op when NOTIFICATION_SERVICE_URL is unset, so it is
// always safe to attach.
func NewAppNotifier(cfg *config.Config) *notify.Client {
	return notify.NewClient(cfg.NotificationServiceURL, cfg.InternalAPIKey)
}

// WithAppNotifier attaches the in-app notification client to a PaymentService.
func (s *PaymentService) WithAppNotifier(n *notify.Client) *PaymentService {
	s.appNotifier = n
	return s
}

// WithAppNotifier attaches the in-app notification client to a PrenosService.
func (s *PrenosService) WithAppNotifier(n *notify.Client) *PrenosService {
	s.appNotifier = n
	return s
}

// emitPaymentSettled fires in-app notifications after a payment/prenos settles.
// kind is "payment" or "prenos" and only changes the wording/link. The existing
// email flow (verification code) is untouched, so SendEmail stays false here to
// avoid duplicate mail. clientID args are nil for non-client (bank/merchant)
// accounts and are simply skipped.
func emitPaymentSettled(n *notify.Client, payment *models.Payment, senderClientID, receiverClientID *uint, kind string) {
	if n == nil {
		return
	}
	amount := fmt.Sprintf("%.2f RSD", payment.Iznos)

	link := "/payments"
	execTitle := "Plaćanje izvršeno"
	execBody := fmt.Sprintf("Vaše plaćanje od %s ka računu %s je uspešno izvršeno.", amount, payment.RacunPrimaocaBroj)
	if kind == "prenos" {
		link = "/prenos"
		execTitle = "Prenos izvršen"
		execBody = fmt.Sprintf("Vaš prenos od %s ka računu %s je uspešno izvršen.", amount, payment.RacunPrimaocaBroj)
	}

	if senderClientID != nil {
		n.Emit(notify.Event{
			UserID:   *senderClientID,
			UserType: "client",
			Type:     "PAYMENT_EXECUTED",
			Title:    execTitle,
			Body:     execBody,
			Link:     link,
		})
	}
	if receiverClientID != nil {
		n.Emit(notify.Event{
			UserID:   *receiverClientID,
			UserType: "client",
			Type:     "PAYMENT_RECEIVED",
			Title:    "Primili ste uplatu",
			Body:     fmt.Sprintf("Primili ste %s na račun %s.", amount, payment.RacunPrimaocaBroj),
			Link:     link,
		})
	}
}
