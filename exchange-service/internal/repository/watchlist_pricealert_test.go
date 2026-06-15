package repository

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

func TestWatchlistRepository_CRUD(t *testing.T) {
	db := openRepoTestDB(t, "wl_crud")
	exch := models.MarketExchangeRecord{Acronym: "WX", Name: "X", MICCode: "WX1", Polity: "X", Currency: "USD", Timezone: "UTC", WorkingHours: "09-17"}
	if err := db.Create(&exch).Error; err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if err := db.Create(&models.MarketListingRecord{
		Ticker: "WLT", Name: "WLT", Type: "stock", ExchangeID: exch.ID,
		Price: 10, Ask: 10, Bid: 10, Volume: 1, LastRefresh: time.Now().UTC(),
	}).Error; err != nil {
		t.Fatalf("listing: %v", err)
	}

	r := NewWatchlistRepository(db)
	w := &models.Watchlist{UserID: 1, UserType: "client", Name: "tech"}
	if err := r.Create(w); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if list, err := r.ListByUser(1, "client"); err != nil || len(list) != 1 {
		t.Fatalf("ListByUser: %d err=%v", len(list), err)
	}
	if got, _ := r.GetByID(w.ID); got == nil {
		t.Fatal("GetByID hit returned nil")
	}
	if miss, _ := r.GetByID(99999); miss != nil {
		t.Error("GetByID miss should be nil")
	}

	if item, err := r.AddItem(w.ID, "WLT"); err != nil || item == nil {
		t.Fatalf("AddItem valid: %v", err)
	}
	if _, err := r.AddItem(w.ID, "WLT"); err != ErrDuplicateItem {
		t.Errorf("AddItem duplicate: want ErrDuplicateItem, got %v", err)
	}
	if _, err := r.AddItem(w.ID, "NOPE"); err != ErrTickerNotFound {
		t.Errorf("AddItem unknown: want ErrTickerNotFound, got %v", err)
	}

	if items, err := r.GetItems(w.ID); err != nil || len(items) != 1 {
		t.Fatalf("GetItems: %d err=%v", len(items), err)
	}
	if err := r.RemoveItem(w.ID, "WLT"); err != nil {
		t.Fatalf("RemoveItem: %v", err)
	}
	if items, _ := r.GetItems(w.ID); len(items) != 0 {
		t.Errorf("GetItems after remove: %d", len(items))
	}
	if err := r.Delete(w.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := r.GetByID(w.ID); got != nil {
		t.Error("GetByID after delete should be nil")
	}
}

func TestPriceAlertRepository_CRUD(t *testing.T) {
	db := openRepoTestDB(t, "pa_crud")
	r := NewPriceAlertRepository(db)

	a := &models.PriceAlert{
		UserID: 1, UserType: "client", Ticker: "AAA", Condition: "ABOVE",
		Threshold: 100, NotificationEmail: "e@x.com", IsActive: true,
	}
	if err := r.Create(a); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if list, err := r.ListByUser(1, "client"); err != nil || len(list) != 1 {
		t.Fatalf("ListByUser: %d err=%v", len(list), err)
	}
	if got, _ := r.GetByID(a.ID); got == nil {
		t.Fatal("GetByID hit returned nil")
	}
	_, _ = r.GetByID(99999) // miss path

	if active, err := r.GetActiveByTicker("AAA"); err != nil || len(active) != 1 {
		t.Fatalf("GetActiveByTicker: %d err=%v", len(active), err)
	}
	if err := r.Deactivate(a.ID); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if active, _ := r.GetActiveByTicker("AAA"); len(active) != 0 {
		t.Errorf("GetActiveByTicker after deactivate: %d", len(active))
	}
}
