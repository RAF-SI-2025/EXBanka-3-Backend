package models

import "time"

// Watchlist represents a named list of market securities tracked by a user.
// Each user (client or employee) can have multiple watchlists.
type Watchlist struct {
	ID        uint            `gorm:"primaryKey;autoIncrement"                                   json:"id"`
	UserID    uint            `gorm:"not null;index:idx_watchlist_user"                           json:"user_id"`
	UserType  string          `gorm:"not null;index:idx_watchlist_user"                           json:"user_type"` // "client" | "employee"
	Name      string          `gorm:"not null"                                                    json:"name"`
	CreatedAt time.Time       `                                                                   json:"created_at"`
	Items     []WatchlistItem `gorm:"foreignKey:WatchlistID;constraint:OnDelete:CASCADE;"         json:"-"`
}

func (Watchlist) TableName() string { return "watchlists" }

// WatchlistItem is a single ticker entry in a watchlist.
// The composite unique index prevents duplicate tickers on the same list.
type WatchlistItem struct {
	ID          uint      `gorm:"primaryKey;autoIncrement"                                          json:"id"`
	WatchlistID uint      `gorm:"not null;index;uniqueIndex:idx_watchlist_ticker"                   json:"-"`
	Ticker      string    `gorm:"not null;uniqueIndex:idx_watchlist_ticker"                         json:"ticker"`
	AddedAt     time.Time `gorm:"not null"                                                          json:"added_at"`
}

func (WatchlistItem) TableName() string { return "watchlist_items" }
