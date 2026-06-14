package service

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
)

// emitOrderNotification sends an order-lifecycle notification (in-app + email)
// to the human who placed the order. Best-effort and nil-safe: a missing
// notifier or recipient is silently skipped, never blocking order processing.
func emitOrderNotification(n *notify.Client, order *models.OrderRecord, typ, title, body string) {
	if n == nil || order == nil {
		return
	}
	// Only client/employee recipients have an in-app bell; fund-only orders
	// without a captured human are skipped.
	if order.NotifyUserID == 0 || (order.NotifyUserType != "client" && order.NotifyUserType != "employee") {
		return
	}
	n.Emit(notify.Event{
		UserID:    order.NotifyUserID,
		UserType:  order.NotifyUserType,
		Type:      typ,
		Title:     title,
		Body:      body,
		Link:      "/orders",
		SendEmail: order.NotifyEmail != "",
		EmailTo:   order.NotifyEmail,
	})
}
