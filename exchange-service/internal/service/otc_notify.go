package service

import (
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
)

// otcOtherParty returns the counterparty of the given actor on an OTC offer —
// the recipient who should be notified about the actor's action.
func otcOtherParty(offer *models.OtcOfferRecord, actorID uint, actorType string) (uint, string) {
	if offer.BuyerID == actorID && offer.BuyerType == actorType {
		return offer.SellerID, offer.SellerType
	}
	return offer.BuyerID, offer.BuyerType
}

// emitOtcNotification sends an in-app OTC notification to a client/employee
// recipient. Best-effort and nil-safe. OTC parties' emails aren't stored
// locally, so this is in-app only (email enrichment is a later follow-up).
func emitOtcNotification(n *notify.Client, recipientID uint, recipientType, typ, title, body string) {
	if n == nil {
		return
	}
	if recipientID == 0 || (recipientType != "client" && recipientType != "employee") {
		return
	}
	n.Emit(notify.Event{
		UserID:   recipientID,
		UserType: recipientType,
		Type:     typ,
		Title:    title,
		Body:     body,
		Link:     "/otc/offers",
	})
}
