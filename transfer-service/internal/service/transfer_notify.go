package service

import (
	"fmt"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/notify"
)

// NewAppNotifier builds the in-app notification client from config. Returns a
// client whose Emit is a no-op when NOTIFICATION_SERVICE_URL is unset, so it is
// always safe to attach.
func NewAppNotifier(cfg *config.Config) *notify.Client {
	return notify.NewClient(cfg.NotificationServiceURL, cfg.InternalAPIKey)
}

// WithAppNotifier attaches the in-app notification client to a TransferService.
func (s *TransferService) WithAppNotifier(n *notify.Client) *TransferService {
	s.appNotifier = n
	return s
}

// emitTransferSettled fires an in-app notification after a transfer settles.
// Transfers move money between accounts owned by the same client, so there is a
// single recipient. clientID is nil only in malformed cases and is skipped.
func emitTransferSettled(n *notify.Client, transfer *models.Transfer, clientID *uint) {
	if n == nil || clientID == nil {
		return
	}
	n.Emit(notify.Event{
		UserID:   *clientID,
		UserType: "client",
		Type:     "TRANSFER_EXECUTED",
		Title:    "Prenos sredstava izvršen",
		Body:     fmt.Sprintf("Vaš prenos od %.2f %s je uspešno izvršen.", transfer.Iznos, transfer.ValutaIznosa),
		Link:     "/transfers",
	})
}
