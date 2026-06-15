package models

import "time"

// OTC negotiation entry actions.
const (
	OtcNegotiationActionCreated   = "created"
	OtcNegotiationActionCountered = "countered"
	OtcNegotiationActionAccepted  = "accepted"
	OtcNegotiationActionDeclined  = "declined"
	OtcNegotiationActionCancelled = "cancelled"
)

// OtcNegotiationEntryRecord is one immutable step in an OTC offer's negotiation
// history (Celina 4): the initial offer, every counter-offer (with the terms it
// replaced), and the terminal accept/decline/cancel. The OtcOfferRecord itself
// only retains the latest terms, so this append-only log is the source of truth
// for "ko je, kada i sa kojom izmenom" — who changed what, when, old vs new.
//
// Prev* fields are nil on the initial "created" entry and on status-only
// transitions; they carry the pre-change terms on a "countered" entry.
type OtcNegotiationEntryRecord struct {
	ID        uint   `gorm:"primaryKey"`
	OfferID   uint   `gorm:"column:offer_id;not null;index"`
	Action    string `gorm:"not null"`
	ActorID   uint   `gorm:"column:actor_id;not null"`
	ActorType string `gorm:"column:actor_type;not null"`

	Amount         float64   `gorm:"not null"`
	PricePerStock  float64   `gorm:"column:price_per_stock;not null"`
	Premium        float64   `gorm:"not null"`
	SettlementDate time.Time `gorm:"column:settlement_date;type:date;not null"`

	PrevAmount         *float64   `gorm:"column:prev_amount"`
	PrevPricePerStock  *float64   `gorm:"column:prev_price_per_stock"`
	PrevPremium        *float64   `gorm:"column:prev_premium"`
	PrevSettlementDate *time.Time `gorm:"column:prev_settlement_date;type:date"`

	CreatedAt time.Time `gorm:"not null;index"`
}

func (OtcNegotiationEntryRecord) TableName() string { return "otc_negotiation_entries" }
