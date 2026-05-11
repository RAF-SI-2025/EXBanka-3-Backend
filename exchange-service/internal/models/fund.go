package models

import "time"

// InvestmentFundRecord represents a bank-managed investment fund. The fund is
// always denominated in RSD and owns a dedicated bank account (AccountID) that
// holds the fund's liquid cash. Securities purchased on behalf of the fund are
// tracked in the portfolio_holdings table with user_type = "fund" and
// user_id = fund.id.
type InvestmentFundRecord struct {
	ID             uint      `gorm:"primaryKey"`
	Naziv          string    `gorm:"not null;uniqueIndex"`
	Opis           string    `gorm:"type:text;not null"`
	MinimalniUlog  float64   `gorm:"column:minimalni_ulog;not null"`
	ManagerID      uint      `gorm:"column:manager_id;not null;index"`
	AccountID      uint      `gorm:"column:account_id;not null;uniqueIndex"`
	DatumKreiranja time.Time `gorm:"column:datum_kreiranja;not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (InvestmentFundRecord) TableName() string { return "investment_funds" }

const (
	FundTransactionStatusPending   = "pending"
	FundTransactionStatusCompleted = "completed"
	FundTransactionStatusFailed    = "failed"
)

// ClientFundTransactionRecord logs each cash flow between a client (or the
// bank, when supervisors top the fund up on its behalf) and a fund. IsInflow
// is true for investments and false for withdrawals.
type ClientFundTransactionRecord struct {
	ID         uint      `gorm:"primaryKey"`
	ClientID   uint      `gorm:"column:client_id;not null;index:idx_fund_tx_client"`
	ClientType string    `gorm:"column:client_type;not null;index:idx_fund_tx_client"` // "client" or "bank"
	FundID     uint      `gorm:"column:fund_id;not null;index"`
	AccountID  uint      `gorm:"column:account_id;not null"`
	Iznos      float64   `gorm:"not null"`
	Status     string    `gorm:"not null;default:'completed'"`
	IsInflow   bool      `gorm:"column:is_inflow;not null"`
	Timestamp  time.Time `gorm:"not null"`
	CreatedAt  time.Time
}

func (ClientFundTransactionRecord) TableName() string { return "client_fund_transactions" }

// ClientFundPositionRecord tracks the cumulative principal a participant has
// invested in a fund. Withdrawals subtract from UkupanUlozeniIznos pro-rata so
// the position reflects net contributed capital.
type ClientFundPositionRecord struct {
	ID                    uint      `gorm:"primaryKey"`
	ClientID              uint      `gorm:"column:client_id;not null;index:idx_fund_pos_client_fund"`
	ClientType            string    `gorm:"column:client_type;not null;index:idx_fund_pos_client_fund"`
	FundID                uint      `gorm:"column:fund_id;not null;index:idx_fund_pos_client_fund"`
	UkupanUlozeniIznos    float64   `gorm:"column:ukupan_ulozeni_iznos;not null;default:0"`
	DatumPoslednjePromene time.Time `gorm:"column:datum_poslednje_promene;not null"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (ClientFundPositionRecord) TableName() string { return "client_fund_positions" }

// FundPerformanceHistoryRecord stores a daily snapshot of the fund's total
// value (cash + market value of securities, in RSD).
type FundPerformanceHistoryRecord struct {
	ID        uint      `gorm:"primaryKey"`
	FundID    uint      `gorm:"column:fund_id;not null;index:idx_fund_perf_fund_date"`
	Date      time.Time `gorm:"type:date;not null;index:idx_fund_perf_fund_date"`
	FundValue float64   `gorm:"column:fund_value;not null"`
	CreatedAt time.Time
}

func (FundPerformanceHistoryRecord) TableName() string { return "fund_performance_history" }

// User-type strings used inside portfolio holdings and order rows for the
// fund owner. The other two ("client" and "bank") are defined elsewhere; this
// constant is the additional owner type introduced with investment funds.
const PortfolioOwnerFund = "fund"
