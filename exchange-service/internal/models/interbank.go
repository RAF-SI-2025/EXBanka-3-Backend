package models

import "time"

const (
	InterbankInboundStatusReceived  = "received"
	InterbankInboundStatusProcessed = "processed"
	InterbankInboundStatusFailed    = "failed"
)

const (
	InterbankNegotiationRoleBuyer  = "buyer"
	InterbankNegotiationRoleSeller = "seller"
)

const (
	InterbankPendingTxStatusPending     = "pending"
	InterbankPendingTxStatusCommitted   = "committed"
	InterbankPendingTxStatusRolledBack  = "rolled_back"
	InterbankPendingTxStatusRejected    = "rejected"
)

const (
	InterbankPaymentDirectionOutbound = "outbound"
	InterbankPaymentDirectionInbound  = "inbound"
)

const (
	InterbankPaymentStatusPending    = "pending"
	InterbankPaymentStatusCommitted  = "committed"
	InterbankPaymentStatusRolledBack = "rolled_back"
	InterbankPaymentStatusRejected   = "rejected"
	InterbankPaymentStatusFailed     = "failed"
)

const (
	InterbankOptionContractStatusValid     = "valid"
	InterbankOptionContractStatusExercised = "exercised"
	InterbankOptionContractStatusExpired   = "expired"
)

// InterbankInboundMessage is the audit + idempotence log for every
// /interbank request we accept from a partner bank. The composite
// (routing_number, locally_generated_key) is the protocol-defined
// idempotence key; the receiver MUST return an identical response on
// replay, so we persist the rendered response body alongside.
type InterbankInboundMessage struct {
	ID                  uint   `gorm:"primaryKey"`
	RoutingNumber       int    `gorm:"column:routing_number;not null;uniqueIndex:idx_interbank_idem_key,priority:1"`
	LocallyGeneratedKey string `gorm:"column:locally_generated_key;type:varchar(64);not null;uniqueIndex:idx_interbank_idem_key,priority:2"`

	MessageType string `gorm:"column:message_type;not null;index"`
	RequestBody string `gorm:"column:request_body;type:text;not null"`

	Status       string `gorm:"not null;default:'received';index"`
	HTTPStatus   int    `gorm:"column:http_status;not null;default:0"`
	ResponseBody string `gorm:"column:response_body;type:text"`
	Error        string `gorm:"type:text"`

	CreatedAt   time.Time  `gorm:"not null"`
	ProcessedAt *time.Time `gorm:"column:processed_at"`
}

func (InterbankInboundMessage) TableName() string { return "interbank_inbound_messages" }

// InterbankOtcNegotiation is our local copy of a cross-bank OTC option
// negotiation. The negotiation's globally-unique key is
// (NegotiationRoutingNumber, NegotiationID), which is the seller bank's
// routing number + the seller bank's locally-generated id (per spec
// §3.2 — the seller's bank mints the id when POST /negotiations
// arrives). Both banks store an identical copy and update it on each
// counter-offer.
type InterbankOtcNegotiation struct {
	ID uint `gorm:"primaryKey"`

	NegotiationRoutingNumber int    `gorm:"column:negotiation_routing_number;not null;uniqueIndex:idx_interbank_negotiation_key,priority:1"`
	NegotiationID            string `gorm:"column:negotiation_id;type:varchar(64);not null;uniqueIndex:idx_interbank_negotiation_key,priority:2"`

	// LocalRole is which side of the negotiation we play —
	// "buyer" if our client initiated, "seller" if a partner's
	// client posted to us.
	LocalRole string `gorm:"column:local_role;not null;index"`

	// CounterpartyRoutingNumber is the OTHER bank in this negotiation.
	// We always have exactly one counterparty (the buyer's bank or
	// the seller's bank — whichever isn't us).
	CounterpartyRoutingNumber int `gorm:"column:counterparty_routing_number;not null;index"`

	// Buyer / seller identities. The local-side identity is encoded
	// via interbank.EncodeLocalParticipantID; the remote-side is the
	// partner's opaque string.
	BuyerRoutingNumber  int    `gorm:"column:buyer_routing_number;not null"`
	BuyerID             string `gorm:"column:buyer_id;type:varchar(64);not null"`
	SellerRoutingNumber int    `gorm:"column:seller_routing_number;not null"`
	SellerID            string `gorm:"column:seller_id;type:varchar(64);not null"`

	StockTicker string `gorm:"column:stock_ticker;not null;index"`
	Amount      float64

	PricePerUnitCurrency string  `gorm:"column:price_per_unit_currency;not null"`
	PricePerUnitAmount   float64 `gorm:"column:price_per_unit_amount;not null"`
	PremiumCurrency      string  `gorm:"column:premium_currency;not null"`
	PremiumAmount        float64 `gorm:"column:premium_amount;not null"`

	// SettlementDate stores the ISO8601 timestamp as a string at the
	// DB boundary so we don't lose the partner's original timezone.
	SettlementDate string `gorm:"column:settlement_date;not null"`

	LastModifiedByRoutingNumber int    `gorm:"column:last_modified_by_routing_number;not null"`
	LastModifiedByID            string `gorm:"column:last_modified_by_id;type:varchar(64);not null"`

	IsOngoing bool `gorm:"column:is_ongoing;not null;default:true;index"`

	// SellerReservedHoldingID is the local portfolio holding whose shares
	// we reserved when WE (the seller's bank) accepted this negotiation,
	// backing the option contract per spec §2.7.2. Non-nil only on
	// seller-role rows between acceptance and exercise/expiry; set back to
	// nil once the reservation is released (on exercise, on a reopened
	// accept, or by the settlement-expiry sweep). nil for buyer-role rows
	// and bank-side sellers (for which we hold no portfolio reservation).
	SellerReservedHoldingID *uint `gorm:"column:seller_reserved_holding_id;index"`

	// AcceptTxID / AcceptTxRoutingNumber record the transactionId of the
	// option-acceptance NEW_TX we (the seller's bank) dispatched to the
	// buyer's bank. They are set only after the buyer voted YES, so their
	// presence means "the buyer reserved the premium and we owe a
	// COMMIT_TX + local seller credit". AcceptCommitFinalisedAt is stamped
	// once both land. The reconcile cron resends COMMIT_TX for rows where
	// AcceptTxID is set and AcceptCommitFinalisedAt is NULL — closing the
	// window where a crash after the YES vote would otherwise strand the
	// buyer's reserved premium indefinitely.
	AcceptTxRoutingNumber   int        `gorm:"column:accept_tx_routing_number;not null;default:0"`
	AcceptTxID              string     `gorm:"column:accept_tx_id;type:varchar(64);not null;default:''"`
	AcceptCommitFinalisedAt *time.Time `gorm:"column:accept_commit_finalised_at"`

	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (InterbankOtcNegotiation) TableName() string { return "interbank_otc_negotiations" }

// InterbankPendingTx is the per-TransactionID state machine row for an
// inbound NEW_TX that we voted YES on but haven't yet seen a
// COMMIT_TX/ROLLBACK_TX for. The protocol allows arbitrary delay between
// the vote and the resolution, so we persist what we reserved + what
// posting effect should fire on commit. Idempotence is enforced at the
// /interbank handler layer by InterbankInboundMessage; this table is
// the BUSINESS-layer record of "we promised to do X at commit time".
//
// The composite (TxRoutingNumber, TxID) is the protocol's
// transactionId.routingNumber + transactionId.id — i.e. the
// initiating bank's coordinates, which the protocol guarantees are
// globally unique.
type InterbankPendingTx struct {
	ID uint `gorm:"primaryKey"`

	TxRoutingNumber int    `gorm:"column:tx_routing_number;not null;uniqueIndex:idx_interbank_pending_tx_key,priority:1"`
	TxID            string `gorm:"column:tx_id;type:varchar(64);not null;uniqueIndex:idx_interbank_pending_tx_key,priority:2"`

	// PartnerRoutingNumber is the partner that POSTed the NEW_TX to us;
	// it's almost always equal to TxRoutingNumber (the initiator is
	// also the sender) but we record it separately to catch a
	// hypothetical relay-through-third-party case during audit.
	PartnerRoutingNumber int `gorm:"column:partner_routing_number;not null;index"`

	// NegotiationRoutingNumber + NegotiationID let us find back the
	// InterbankOtcNegotiation row that this NEW_TX is settling, so
	// COMMIT_TX can flip it closed + write the option contract.
	NegotiationRoutingNumber int    `gorm:"column:negotiation_routing_number;not null;index"`
	NegotiationID            string `gorm:"column:negotiation_id;type:varchar(64);not null"`

	// Resource-reservation snapshot. We persist what was reserved on
	// NEW_TX so we can release it on ROLLBACK_TX even if the original
	// envelope can't be replayed.
	ReservedFromLocalID  string  `gorm:"column:reserved_from_local_id;type:varchar(64);not null"`
	ReservedCurrency     string  `gorm:"column:reserved_currency;not null"`
	ReservedAmount       float64 `gorm:"column:reserved_amount;not null"`

	// Snapshot of the option contract terms — applied on COMMIT_TX to
	// create the InterbankOptionContract row.
	StockTicker          string  `gorm:"column:stock_ticker;not null"`
	OptionAmount         float64 `gorm:"column:option_amount;not null"`
	PricePerUnitCurrency string  `gorm:"column:price_per_unit_currency;not null"`
	PricePerUnitAmount   float64 `gorm:"column:price_per_unit_amount;not null"`
	SettlementDate       string  `gorm:"column:settlement_date;not null"`

	// Identities of the two sides; the local side is our customer.
	BuyerRoutingNumber  int    `gorm:"column:buyer_routing_number;not null"`
	BuyerID             string `gorm:"column:buyer_id;type:varchar(64);not null"`
	SellerRoutingNumber int    `gorm:"column:seller_routing_number;not null"`
	SellerID            string `gorm:"column:seller_id;type:varchar(64);not null"`

	Status string `gorm:"not null;default:'pending';index"`

	CreatedAt   time.Time  `gorm:"not null"`
	ResolvedAt  *time.Time `gorm:"column:resolved_at"`
}

func (InterbankPendingTx) TableName() string { return "interbank_pending_txs" }

// InterbankOptionContract is our local record of an option contract
// formed with a partner bank. We always sit on the BUYER side of these
// rows — when we're the seller, the contract is just the negotiation
// we've already closed plus a corresponding reservation in our local
// portfolio.
//
// The contract's global identity is the negotiation's identity
// (NegotiationRoutingNumber, NegotiationID) per spec §3.6.1 ("Set the
// OTC option contract negotiationId to the ID of the negotiation that
// lead to its creation. This ensures that the option pseudo-account is
// always in the bank of the seller").
type InterbankOptionContract struct {
	ID uint `gorm:"primaryKey"`

	NegotiationRoutingNumber int    `gorm:"column:negotiation_routing_number;not null;uniqueIndex:idx_interbank_option_contract_key,priority:1"`
	NegotiationID            string `gorm:"column:negotiation_id;type:varchar(64);not null;uniqueIndex:idx_interbank_option_contract_key,priority:2"`

	// The local user holding the option (always our side; this table
	// is only written when WE are the buyer's bank). For the
	// symmetric "we are the seller's bank" case the local-side
	// effect lives in the existing portfolio holding reservation, not
	// in this table.
	BuyerLocalID string `gorm:"column:buyer_local_id;type:varchar(64);not null;index"`

	SellerRoutingNumber int    `gorm:"column:seller_routing_number;not null"`
	SellerID            string `gorm:"column:seller_id;type:varchar(64);not null"`

	StockTicker          string  `gorm:"column:stock_ticker;not null;index"`
	Amount               float64 `gorm:"not null"`
	PricePerUnitCurrency string  `gorm:"column:price_per_unit_currency;not null"`
	PricePerUnitAmount   float64 `gorm:"column:price_per_unit_amount;not null"`
	PremiumCurrency      string  `gorm:"column:premium_currency;not null"`
	PremiumAmount        float64 `gorm:"column:premium_amount;not null"`
	SettlementDate       string  `gorm:"column:settlement_date;not null"`

	Status string `gorm:"not null;default:'valid';index"`

	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (InterbankOptionContract) TableName() string { return "interbank_option_contracts" }

// InterbankPayment is the per-side state record for a cross-bank direct
// payment (the 2-posting MONAS shape of NEW_TX, where both postings are
// TxAccount type ACCOUNT — see protocol §2.6). One row per side: the
// initiator persists Direction=outbound, the recipient persists
// Direction=inbound. The (TxRoutingNumber, TxID) composite is the
// protocol's transactionId — globally unique because the initiator owns
// its routing number.
//
// State machine:
//
//	pending → committed     (NEW_TX YES → COMMIT_TX applied)
//	pending → rolled_back   (we voted YES, then ROLLBACK_TX arrived)
//	pending → rejected      (partner voted NO; sender-side only)
//	pending → failed        (transport error after NEW_TX; sender-side only)
type InterbankPayment struct {
	ID uint `gorm:"primaryKey"`

	TxRoutingNumber int    `gorm:"column:tx_routing_number;not null;uniqueIndex:idx_interbank_payment_key,priority:1"`
	TxID            string `gorm:"column:tx_id;type:varchar(64);not null;uniqueIndex:idx_interbank_payment_key,priority:2"`

	// Direction: outbound = we initiated this payment; inbound = a
	// partner POSTed NEW_TX to us as the recipient bank.
	Direction string `gorm:"not null;index"`

	// PartnerRoutingNumber is the OTHER bank in this payment.
	PartnerRoutingNumber int `gorm:"column:partner_routing_number;not null;index"`

	// Wire posting snapshot — needed so commit/rollback can apply local
	// effects without re-parsing the original NEW_TX envelope.
	SenderAccountNumber    string  `gorm:"column:sender_account_number;type:varchar(32);not null"`
	RecipientAccountNumber string  `gorm:"column:recipient_account_number;type:varchar(32);not null"`
	Currency               string  `gorm:"not null"`
	Amount                 float64 `gorm:"not null"`

	// LocalAccountID is the account we touched (sender's for outbound,
	// recipient's for inbound). Used for status lookups and audit.
	LocalAccountID *uint `gorm:"column:local_account_id;index"`

	// LocalClientID identifies the local owner (outbound only — we
	// persist nothing about the partner's customer for inbound).
	LocalClientID *uint `gorm:"column:local_client_id;index"`

	// Tx metadata copied verbatim from the envelope.
	Message        string `gorm:"type:text"`
	PaymentCode    string `gorm:"column:payment_code"`
	PaymentPurpose string `gorm:"column:payment_purpose;type:text"`

	Status    string `gorm:"not null;default:'pending';index"`
	LastError string `gorm:"column:last_error;type:text"`

	// PartnerFinalisedAt is set when the terminal partner message
	// (COMMIT_TX for committed; ROLLBACK_TX for failed) has been
	// acknowledged. The reconciliation cron picks up resolved
	// outbound rows where this is still null and replays the terminal
	// message until the partner ACKs. Always nil for inbound rows —
	// the receiver doesn't drive retransmission, the sender does.
	PartnerFinalisedAt *time.Time `gorm:"column:partner_finalised_at"`

	CreatedAt  time.Time  `gorm:"not null"`
	UpdatedAt  time.Time  `gorm:"not null"`
	ResolvedAt *time.Time `gorm:"column:resolved_at"`
}

func (InterbankPayment) TableName() string { return "interbank_payments" }

// RemotePublicStockSnapshot caches one partner bank's /public-stock
// response so the local frontend's "browse cross-bank OTC offers" page
// doesn't pay a fan-out cost on every request. The reconciliation cron
// refreshes one row per partner @every 5m. Stale-but-good rows are
// returned with a `stale=true` flag rather than dropped, so a transient
// partner outage doesn't blank the catalogue.
//
// PartnerRoutingNumber is the primary key — one row per partner.
// PayloadJSON holds the raw interbank.PublicStocksResponse JSON to
// keep the cache schema-agnostic; the handler unmarshals on read.
type RemotePublicStockSnapshot struct {
	PartnerRoutingNumber int    `gorm:"column:partner_routing_number;primaryKey"`
	PayloadJSON          string `gorm:"column:payload_json;type:text"`
	LastError            string `gorm:"column:last_error;type:text"`

	FetchedAt time.Time `gorm:"column:fetched_at;not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (RemotePublicStockSnapshot) TableName() string { return "remote_public_stock_snapshots" }

const (
	InterbankExerciseDirectionOutbound = "outbound" // we are the buyer's bank initiating
	InterbankExerciseDirectionInbound  = "inbound"  // we are the seller's bank receiving
)

const (
	InterbankExerciseStatusPending    = "pending"
	InterbankExerciseStatusCommitted  = "committed"
	InterbankExerciseStatusRolledBack = "rolled_back"
	InterbankExerciseStatusRejected   = "rejected"
	InterbankExerciseStatusFailed     = "failed"
)

// InterbankPendingExercise tracks the per-side state of a cross-bank
// option exercise (the 4-posting MONAS+STOCK transaction described in
// protocol §2.7.2: debit OPTION pseudo for π·k cash, credit buyer for
// π·k cash, credit OPTION pseudo for k stocks, debit buyer for k
// stocks).
//
// Outbound (Direction='outbound'): we own the option, our bank is the
// initiator. BuyerLocal* fields point at our own client + account that
// will be debited the cash on COMMIT_TX. OptionContractID points at
// the local InterbankOptionContract row being consumed.
//
// Inbound (Direction='inbound'): partner-bank's user holds the option
// and exercises against us. On COMMIT_TX we reduce the seller's
// holding by StockAmount and credit the seller's account by
// CashAmount; the seller identity comes from the linked
// InterbankOtcNegotiation row.
type InterbankPendingExercise struct {
	ID uint `gorm:"primaryKey"`

	TxRoutingNumber int    `gorm:"column:tx_routing_number;not null;uniqueIndex:idx_interbank_pending_exercise_key,priority:1"`
	TxID            string `gorm:"column:tx_id;type:varchar(64);not null;uniqueIndex:idx_interbank_pending_exercise_key,priority:2"`

	Direction string `gorm:"not null;index"`

	PartnerRoutingNumber int `gorm:"column:partner_routing_number;not null;index"`

	// NegotiationRoutingNumber/ID identify the option being exercised.
	// On inbound, this is also the join key to InterbankOtcNegotiation
	// so we can find the seller. On outbound, it's the key to the
	// InterbankOptionContract.
	NegotiationRoutingNumber int    `gorm:"column:negotiation_routing_number;not null;index"`
	NegotiationID            string `gorm:"column:negotiation_id;type:varchar(64);not null"`

	StockTicker string  `gorm:"column:stock_ticker;not null"`
	StockAmount float64 `gorm:"column:stock_amount;not null"`

	PricePerUnitCurrency string  `gorm:"column:price_per_unit_currency;not null"`
	PricePerUnitAmount   float64 `gorm:"column:price_per_unit_amount;not null"`

	// CashAmount is StockAmount × PricePerUnitAmount, denormalised for
	// idempotent ledger ops.
	CashAmount float64 `gorm:"column:cash_amount;not null"`

	BuyerRoutingNumber  int    `gorm:"column:buyer_routing_number;not null"`
	BuyerID             string `gorm:"column:buyer_id;type:varchar(64);not null"`
	SellerRoutingNumber int    `gorm:"column:seller_routing_number;not null"`
	SellerID            string `gorm:"column:seller_id;type:varchar(64);not null"`

	BuyerLocalAccountID *uint `gorm:"column:buyer_local_account_id;index"`
	BuyerLocalClientID  *uint `gorm:"column:buyer_local_client_id;index"`
	OptionContractID    *uint `gorm:"column:option_contract_id;index"`

	Status    string `gorm:"not null;default:'pending';index"`
	LastError string `gorm:"column:last_error;type:text"`

	PartnerFinalisedAt *time.Time `gorm:"column:partner_finalised_at"`
	CreatedAt          time.Time  `gorm:"not null"`
	UpdatedAt          time.Time  `gorm:"not null"`
	ResolvedAt         *time.Time `gorm:"column:resolved_at"`
}

func (InterbankPendingExercise) TableName() string { return "interbank_pending_exercises" }
